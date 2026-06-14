package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/rakrisi/flowtest/internal/config"
	"github.com/rakrisi/flowtest/internal/engine"
)

// KafkaDriver searches Kafka topics for matching messages.
// Uses direct partition reads — no consumer groups, no side effects.
type KafkaDriver struct{}

func (d *KafkaDriver) Name() string { return "kafka" }

func (d *KafkaDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *engine.Context, env *config.EnvConfig) (map[string]interface{}, error) {
	cfg, ok := stepConfig.(*config.KafkaConfig)
	if !ok {
		return nil, fmt.Errorf("kafka driver: invalid step config type %T", stepConfig)
	}

	if env.KafkaBrokers == "" {
		return nil, fmt.Errorf("kafka driver: no kafka brokers configured (set env.kafka_brokers)")
	}

	if cfg.Action == "produce" {
		return d.produce(ctx, env.KafkaBrokers, cfg)
	}

	return d.search(ctx, env.KafkaBrokers, cfg)
}

// produce writes a message to a Kafka topic.
func (d *KafkaDriver) produce(ctx context.Context, brokers string, cfg *config.KafkaConfig) (map[string]interface{}, error) {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(parseBrokers(brokers)...),
		Topic:                  cfg.Topic,
		Balancer:               &kafka.LeastBytes{},
		WriteTimeout:           10 * time.Second,
		AllowAutoTopicCreation: true,
	}
	defer writer.Close()

	var value []byte
	switch v := cfg.Message.(type) {
	case string:
		value = []byte(v)
	default:
		var err error
		value, err = json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("kafka driver: marshaling message: %w", err)
		}
	}

	msg := kafka.Message{
		Key:   []byte(cfg.Key),
		Value: value,
	}
	for k, v := range cfg.Headers {
		msg.Headers = append(msg.Headers, kafka.Header{Key: k, Value: []byte(v)})
	}

	if err := writer.WriteMessages(ctx, msg); err != nil {
		return nil, fmt.Errorf("kafka driver: producing to %s: %w", cfg.Topic, err)
	}

	return map[string]interface{}{
		"produced": map[string]interface{}{
			"topic": cfg.Topic,
			"key":   cfg.Key,
		},
	}, nil
}

// search reads all partitions of a topic from the end, scanning backwards/forward
// for a message matching the filter. No consumer group, no side effects.
func (d *KafkaDriver) search(ctx context.Context, brokers string, cfg *config.KafkaConfig) (map[string]interface{}, error) {
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}

	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	brokerList := parseBrokers(brokers)

	// Look up partitions for the topic
	conn, err := kafka.DialContext(searchCtx, "tcp", brokerList[0])
	if err != nil {
		return nil, fmt.Errorf("kafka driver: connecting to broker: %w", err)
	}

	partitions, err := conn.ReadPartitions(cfg.Topic)
	conn.Close()
	if err != nil {
		return nil, fmt.Errorf("kafka driver: reading partitions for %q: %w", cfg.Topic, err)
	}

	// Cap partition count to avoid unbounded goroutines.
	const maxPartitions = 64
	if len(partitions) > maxPartitions {
		partitions = partitions[:maxPartitions]
	}

	// Search each partition from the earliest offset, polling until timeout.
	// For a test tool, topics are small — reading from start is fast and reliable.
	type result struct {
		msg kafka.Message
		ok  bool
	}
	ch := make(chan result, 1)

	var wg sync.WaitGroup
	for _, p := range partitions {
		wg.Add(1)
		go func(partition int) {
			defer wg.Done()
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:   brokerList,
				Topic:     cfg.Topic,
				Partition: partition,
				MinBytes:  1,
				MaxBytes:  1e6,
				MaxWait:   500 * time.Millisecond,
			})
			// Start from beginning — topics in tests are small
			reader.SetOffset(kafka.FirstOffset)
			defer reader.Close()

			for {
				msg, err := reader.ReadMessage(searchCtx)
				if err != nil {
					return // context cancelled or EOF
				}
				if matchMessage(msg, cfg.Match) {
					select {
					case ch <- result{msg: msg, ok: true}:
					default:
					}
					return
				}
			}
		}(p.ID)
	}

	// Wait for a match or timeout
	select {
	case r := <-ch:
		cancel() // stop remaining goroutines
		wg.Wait()
		if r.ok {
			return buildKafkaResult(r.msg), nil
		}
	case <-searchCtx.Done():
	}

	wg.Wait() // ensure all goroutines finish before returning
	return nil, fmt.Errorf("kafka driver: no matching message found on topic %q within %s", cfg.Topic, timeout)
}

// matchMessage checks if a Kafka message's JSON payload matches all filter criteria.
func matchMessage(msg kafka.Message, match map[string]interface{}) bool {
	if len(match) == 0 {
		return true
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Value, &payload); err != nil {
		return false
	}

	for key, expected := range match {
		actual, ok := payload[key]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", expected) != fmt.Sprintf("%v", actual) {
			return false
		}
	}
	return true
}

func buildKafkaResult(msg kafka.Message) map[string]interface{} {
	result := map[string]interface{}{
		"message": map[string]interface{}{
			"topic":     msg.Topic,
			"partition": msg.Partition,
			"offset":    msg.Offset,
			"key":       string(msg.Key),
			"timestamp": msg.Time.Unix(),
		},
	}

	var payload interface{}
	if err := json.Unmarshal(msg.Value, &payload); err == nil {
		result["message"].(map[string]interface{})["payload"] = payload
	} else {
		result["message"].(map[string]interface{})["payload"] = string(msg.Value)
	}

	if len(msg.Headers) > 0 {
		headers := make(map[string]interface{}, len(msg.Headers))
		for _, h := range msg.Headers {
			headers[h.Key] = string(h.Value)
		}
		result["message"].(map[string]interface{})["headers"] = headers
	}

	return result
}

func parseBrokers(brokers string) []string {
	parts := strings.Split(brokers, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// Close is a no-op — the search driver holds no persistent state.
func (d *KafkaDriver) Close() {}
