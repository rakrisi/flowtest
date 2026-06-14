package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/rakrisi/flowtest/internal/config"
	"github.com/rakrisi/flowtest/internal/engine"
)

// RedisDriver executes Redis operations.
type RedisDriver struct {
	mu     sync.Mutex
	client *redis.Client
	addr   string
}

func (d *RedisDriver) Name() string { return "redis" }

func (d *RedisDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *engine.Context, env *config.EnvConfig) (map[string]interface{}, error) {
	cfg, ok := stepConfig.(*config.RedisConfig)
	if !ok {
		return nil, fmt.Errorf("redis driver: invalid step config type %T", stepConfig)
	}

	client, err := d.getClient(env)
	if err != nil {
		return nil, err
	}

	switch cfg.Action {
	case "get":
		return d.executeGet(ctx, client, cfg.Key)
	case "hgetall":
		return d.executeHGetAll(ctx, client, cfg.Key)
	case "exists":
		return d.executeExists(ctx, client, cfg.Key)
	case "keys":
		return d.executeKeys(ctx, client, cfg.Key)
	case "set":
		return d.executeSet(ctx, client, cfg)
	case "del", "delete":
		return d.executeDel(ctx, client, cfg.Key)
	case "ttl":
		return d.executeTTL(ctx, client, cfg.Key)
	default:
		return nil, fmt.Errorf("redis driver: unsupported action %q (supported: get, hgetall, exists, keys, set, del, ttl)", cfg.Action)
	}
}

func (d *RedisDriver) getClient(env *config.EnvConfig) (*redis.Client, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client != nil && d.addr == env.Redis {
		return d.client, nil
	}

	if env.Redis == "" {
		return nil, fmt.Errorf("redis driver: no redis connection string configured (set env.redis)")
	}

	opts, err := redis.ParseURL(env.Redis)
	if err != nil {
		return nil, fmt.Errorf("redis driver: parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(context.Background()).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis driver: connecting to redis: %w", err)
	}

	// Close old client if address changed
	if d.client != nil {
		d.client.Close()
	}

	d.client = client
	d.addr = env.Redis
	return client, nil
}

func (d *RedisDriver) executeGet(ctx context.Context, client *redis.Client, key string) (map[string]interface{}, error) {
	val, err := client.Get(ctx, key).Result()
	if err == redis.Nil {
		return map[string]interface{}{
			"value":  nil,
			"exists": false,
			"raw":    "",
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis driver: GET %s: %w", key, err)
	}

	result := map[string]interface{}{
		"exists": true,
		"raw":    val,
	}

	// Try to parse as JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(val), &parsed); err == nil {
		result["value"] = parsed
	} else {
		result["value"] = val
	}

	return result, nil
}

func (d *RedisDriver) executeHGetAll(ctx context.Context, client *redis.Client, key string) (map[string]interface{}, error) {
	val, err := client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis driver: HGETALL %s: %w", key, err)
	}

	// Convert to interface map
	value := make(map[string]interface{}, len(val))
	for k, v := range val {
		// Try to parse each value as JSON
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			value[k] = parsed
		} else {
			value[k] = v
		}
	}

	return map[string]interface{}{
		"value":  value,
		"exists": len(val) > 0,
	}, nil
}

func (d *RedisDriver) executeExists(ctx context.Context, client *redis.Client, key string) (map[string]interface{}, error) {
	count, err := client.Exists(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis driver: EXISTS %s: %w", key, err)
	}

	return map[string]interface{}{
		"exists": count > 0,
		"value":  count > 0,
	}, nil
}

func (d *RedisDriver) executeKeys(ctx context.Context, client *redis.Client, pattern string) (map[string]interface{}, error) {
	keys, err := client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("redis driver: KEYS %s: %w", pattern, err)
	}

	keysInterface := make([]interface{}, len(keys))
	for i, k := range keys {
		keysInterface[i] = k
	}

	return map[string]interface{}{
		"value":     keysInterface,
		"key_count": len(keys),
	}, nil
}

func (d *RedisDriver) executeSet(ctx context.Context, client *redis.Client, cfg *config.RedisConfig) (map[string]interface{}, error) {
	if cfg.Value == nil {
		return nil, fmt.Errorf("redis driver: SET requires a value")
	}

	// Marshal value to string if it's complex
	var val string
	switch v := cfg.Value.(type) {
	case string:
		val = v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("redis driver: marshaling value: %w", err)
		}
		val = string(data)
	}

	err := client.Set(ctx, cfg.Key, val, cfg.TTL).Err()
	if err != nil {
		return nil, fmt.Errorf("redis driver: SET %s: %w", cfg.Key, err)
	}

	return map[string]interface{}{
		"ok": true,
	}, nil
}

func (d *RedisDriver) executeDel(ctx context.Context, client *redis.Client, key string) (map[string]interface{}, error) {
	count, err := client.Del(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis driver: DEL %s: %w", key, err)
	}

	return map[string]interface{}{
		"deleted": count,
	}, nil
}

func (d *RedisDriver) executeTTL(ctx context.Context, client *redis.Client, key string) (map[string]interface{}, error) {
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis driver: TTL %s: %w", key, err)
	}

	return map[string]interface{}{
		"ttl":     ttl.Seconds(),
		"expires": ttl > 0,
	}, nil
}

// Close cleans up the Redis client.
func (d *RedisDriver) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.client != nil {
		d.client.Close()
		d.client = nil
	}
}
