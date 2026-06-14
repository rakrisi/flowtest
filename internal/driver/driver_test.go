package driver

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/rakrisi/flowtest/internal/config"
	"github.com/rakrisi/flowtest/internal/engine"
)

// --- HTTP Driver Tests ---

func TestHTTPDriver_InvalidConfigType(t *testing.T) {
	d := &HTTPDriver{}
	_, err := d.Execute(context.Background(), "not-an-api-config", nil, &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}

func TestHTTPDriver_RelativeURLWithoutBase(t *testing.T) {
	d := &HTTPDriver{}
	cfg := &config.APIConfig{Method: "GET", URL: "/health"}
	// Empty APIBase means relative URL resolves to just "/health" which fails
	_, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for request to invalid URL")
	}
}

func TestHTTPDriver_Name(t *testing.T) {
	d := &HTTPDriver{}
	if d.Name() != "http" {
		t.Errorf("Name() = %q, want %q", d.Name(), "http")
	}
}

// --- HTTP Auth Tests ---

func TestApplyAuth_Bearer(t *testing.T) {
	req, _ := newTestRequest()
	auth := &config.AuthConfig{Bearer: "my-token-12345"}

	if err := applyAuth(req, auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := req.Header.Get("Authorization")
	want := "Bearer my-token-12345"
	if got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

func TestApplyAuth_Basic(t *testing.T) {
	req, _ := newTestRequest()
	auth := &config.AuthConfig{
		Basic: &config.BasicAuthConfig{
			Username: "admin",
			Password: "secret",
		},
	}

	if err := applyAuth(req, auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := req.Header.Get("Authorization")
	// base64("admin:secret") = "YWRtaW46c2VjcmV0"
	want := "Basic YWRtaW46c2VjcmV0"
	if got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

func TestApplyAuth_APIKeyHeader(t *testing.T) {
	req, _ := newTestRequest()
	auth := &config.AuthConfig{
		APIKey: &config.APIKeyConfig{
			Header: "X-API-Key",
			Value:  "api-key-abc",
		},
	}

	if err := applyAuth(req, auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := req.Header.Get("X-API-Key")
	want := "api-key-abc"
	if got != want {
		t.Errorf("X-API-Key header = %q, want %q", got, want)
	}
}

func TestApplyAuth_APIKeyQuery(t *testing.T) {
	// API key in query is handled at URL construction time, not in applyAuth
	// Just verify applyAuth doesn't error
	req, _ := newTestRequest()
	auth := &config.AuthConfig{
		APIKey: &config.APIKeyConfig{
			Query: "api_key",
			Value: "query-key",
		},
	}

	if err := applyAuth(req, auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not set a header (query is handled elsewhere)
	if got := req.Header.Get("api_key"); got != "" {
		t.Errorf("expected no header for query-based API key, got %q", got)
	}
}

func TestApplyAuth_Nil(t *testing.T) {
	req, _ := newTestRequest()

	if err := applyAuth(req, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("expected no Authorization header, got %q", got)
	}
}

func newTestRequest() (*http.Request, error) {
	return http.NewRequest("GET", "http://example.com/test", nil)
}

// --- Shell Driver Tests ---

func TestShellDriver_InvalidConfigType(t *testing.T) {
	d := &ShellDriver{}
	_, err := d.Execute(context.Background(), "not-a-script-config", nil, &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}

func TestShellDriver_Name(t *testing.T) {
	d := &ShellDriver{}
	if d.Name() != "shell" {
		t.Errorf("Name() = %q, want %q", d.Name(), "shell")
	}
}

func TestShellDriver_SimpleEcho(t *testing.T) {
	d := &ShellDriver{}
	cfg := &config.ScriptConfig{Run: "echo hello"}
	result, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["stdout"] != "hello" {
		t.Errorf("stdout = %q, want %q", result["stdout"], "hello")
	}
	if result["exit_code"] != 0 {
		t.Errorf("exit_code = %v, want 0", result["exit_code"])
	}
}

func TestShellDriver_ExitCodeMismatch(t *testing.T) {
	d := &ShellDriver{}
	exitCode := 0
	cfg := &config.ScriptConfig{Run: "exit 1", AssertExit: &exitCode}
	_, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for exit code mismatch")
	}
}

func TestShellDriver_BashLang(t *testing.T) {
	d := &ShellDriver{}
	cfg := &config.ScriptConfig{Lang: "bash", Run: "echo $BASH_VERSION"}
	result, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["stdout"] == "" {
		t.Error("expected BASH_VERSION to be non-empty")
	}
}

func TestShellDriver_EnvInjection(t *testing.T) {
	d := &ShellDriver{}
	cfg := &config.ScriptConfig{Run: "echo $FLOWTEST_MYVAR"}
	ctx := engine.NewContext()
	ctx.Set("myvar", "injected_value")
	result, err := d.Execute(context.Background(), cfg, ctx, &config.EnvConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["stdout"] != "injected_value" {
		t.Errorf("stdout = %q, want %q", result["stdout"], "injected_value")
	}
}

// --- Kafka Driver Tests ---

func TestKafkaDriver_InvalidConfigType(t *testing.T) {
	d := &KafkaDriver{}
	_, err := d.Execute(context.Background(), "not-kafka-config", nil, &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}

func TestKafkaDriver_NoBrokers(t *testing.T) {
	d := &KafkaDriver{}
	cfg := &config.KafkaConfig{Topic: "test"}
	_, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error when no brokers configured")
	}
}

func TestKafkaDriver_Name(t *testing.T) {
	d := &KafkaDriver{}
	if d.Name() != "kafka" {
		t.Errorf("Name() = %q, want %q", d.Name(), "kafka")
	}
}

// --- Redis Driver Tests ---

func TestRedisDriver_InvalidConfigType(t *testing.T) {
	d := &RedisDriver{}
	_, err := d.Execute(context.Background(), "not-redis-config", nil, &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}

func TestRedisDriver_Name(t *testing.T) {
	d := &RedisDriver{}
	if d.Name() != "redis" {
		t.Errorf("Name() = %q, want %q", d.Name(), "redis")
	}
}

// --- Postgres Driver Tests ---

func TestPostgresDriver_InvalidConfigType(t *testing.T) {
	d := NewPostgresDriver("testdb", "postgres://user:pass@localhost:5432/db")
	_, err := d.Execute(context.Background(), "not-db-config", nil, &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}

func TestPostgresDriver_NoDSN(t *testing.T) {
	d := NewPostgresDriver("testdb", "")
	_, err := d.Execute(context.Background(), &config.DBStepConfig{Query: "SELECT 1"}, engine.NewContext(), &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error when no DSN configured")
	}
}

func TestPostgresDriver_SeedMissingTable(t *testing.T) {
	// Can't actually connect, but test validation path
	d := &PostgresDriver{name: "testdb"}
	d.pool = nil
	// This will fail on getPool since no DSN, but let's test with a mock approach
	// For now just verify Name()
	if d.Name() != "testdb" {
		t.Errorf("Name() = %q, want %q", d.Name(), "testdb")
	}
}

// --- GenericSQL Driver Tests ---

func TestGenericSQLDriver_InvalidConfigType(t *testing.T) {
	d := NewSQLiteDriver("testdb", "sqlite://./nonexistent.db")
	_, err := d.Execute(context.Background(), "not-db-config", nil, &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}

func TestGenericSQLDriver_NoDSN(t *testing.T) {
	d := &GenericSQLDriver{name: "testdb", dialect: "sqlite", dsn: ""}
	_, err := d.Execute(context.Background(), &config.DBStepConfig{Query: "SELECT 1"}, engine.NewContext(), &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error when no DSN configured")
	}
}

func TestGenericSQLDriver_Name(t *testing.T) {
	d := NewMySQLDriver("mydb", "mysql://user:pass@localhost:3306/test")
	if d.Name() != "mydb" {
		t.Errorf("Name() = %q, want %q", d.Name(), "mydb")
	}
}

// --- MongoDB Driver Tests ---

func TestMongoDriver_InvalidConfigType(t *testing.T) {
	d, err := NewMongoDriver("testdb", "mongodb://localhost:27017/testdb")
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}
	_, err = d.Execute(context.Background(), "not-db-config", nil, &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}

func TestMongoDriver_MissingDBName(t *testing.T) {
	_, err := NewMongoDriver("testdb", "mongodb://localhost:27017/")
	if err == nil {
		t.Fatal("expected error when database name missing from DSN")
	}
}

func TestMongoDriver_Name(t *testing.T) {
	d, err := NewMongoDriver("mydb", "mongodb://localhost:27017/testdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Name() != "mydb" {
		t.Errorf("Name() = %q, want %q", d.Name(), "mydb")
	}
}

// --- Registry Tests ---

func TestNewRegistry_EmptyDatabases(t *testing.T) {
	r, err := NewRegistry(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	// Should have http and shell at minimum
	if _, ok := r.drivers["http"]; !ok {
		t.Error("expected http driver to be registered")
	}
	if _, ok := r.drivers["shell"]; !ok {
		t.Error("expected shell driver to be registered")
	}
}

func TestNewRegistry_InvalidDSN(t *testing.T) {
	_, err := NewRegistry(map[string]string{"bad": "not-a-valid-dsn"})
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

func TestNewRegistry_SQLiteDriver(t *testing.T) {
	r, err := NewRegistry(map[string]string{"local": "sqlite://./test_registry.db"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	if _, ok := r.drivers["local"]; !ok {
		t.Error("expected 'local' sqlite driver to be registered")
	}
}

// --- Kafka matchMessage Tests ---

func TestMatchMessage_EmptyFilter(t *testing.T) {
	msg := kafka.Message{Value: []byte(`{"a":1}`)}
	if !matchMessage(msg, nil) {
		t.Error("empty filter should match any message")
	}
}

func TestMatchMessage_MatchingFields(t *testing.T) {
	msg := kafka.Message{Value: []byte(`{"user_id":"123","action":"created"}`)}
	filter := map[string]interface{}{"user_id": "123"}
	if !matchMessage(msg, filter) {
		t.Error("message should match filter")
	}
}

func TestMatchMessage_NonMatchingFields(t *testing.T) {
	msg := kafka.Message{Value: []byte(`{"user_id":"123","action":"created"}`)}
	filter := map[string]interface{}{"user_id": "456"}
	if matchMessage(msg, filter) {
		t.Error("message should not match filter")
	}
}

func TestMatchMessage_InvalidJSON(t *testing.T) {
	msg := kafka.Message{Value: []byte(`not valid json`)}
	filter := map[string]interface{}{"user_id": "123"}
	if matchMessage(msg, filter) {
		t.Error("invalid JSON should not match")
	}
}

func TestMatchMessage_NestedField(t *testing.T) {
	msg := kafka.Message{Value: []byte(`{"user":{"id":"123"},"action":"created"}`)}
	filter := map[string]interface{}{"action": "created"}
	if !matchMessage(msg, filter) {
		t.Error("message should match filter on flat field")
	}
}

// --- HTTP headerMap Tests ---

func TestHeaderMap_SingleValue(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Type", "application/json")

	m := headerMap(h)
	if m["Content-Type"] != "application/json" {
		t.Errorf("expected single string value, got %v", m["Content-Type"])
	}
}

func TestHeaderMap_MultipleValues(t *testing.T) {
	h := http.Header{}
	h.Add("X-Custom", "value1")
	h.Add("X-Custom", "value2")

	m := headerMap(h)
	val, ok := m["X-Custom"].(string)
	if !ok {
		t.Fatalf("expected string for multiple values, got %T", m["X-Custom"])
	}
	if val != "value1, value2" {
		t.Errorf("expected 'value1, value2', got %q", val)
	}
}

func TestRawHeaderMap(t *testing.T) {
	h := http.Header{}
	h.Add("X-Custom", "value1")
	h.Add("X-Custom", "value2")

	m := rawHeaderMap(h)
	vals, ok := m["X-Custom"].([]interface{})
	if !ok {
		t.Fatalf("expected array for multiple values, got %T", m["X-Custom"])
	}
	if len(vals) != 2 || vals[0] != "value1" || vals[1] != "value2" {
		t.Errorf("expected ['value1', 'value2'], got %v", vals)
	}
}

// --- Shell Driver Advanced Tests ---

func TestShellDriver_StderrCapture(t *testing.T) {
	d := &ShellDriver{}
	cfg := &config.ScriptConfig{Run: "echo error >&2"}
	result, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["stderr"] != "error" {
		t.Errorf("stderr = %q, want %q", result["stderr"], "error")
	}
}

func TestShellDriver_NonZeroExit(t *testing.T) {
	d := &ShellDriver{}
	cfg := &config.ScriptConfig{Run: "exit 42"}
	result, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["exit_code"] != 42 {
		t.Errorf("exit_code = %v, want 42", result["exit_code"])
	}
}

func TestShellDriver_MultipleEnvVars(t *testing.T) {
	d := &ShellDriver{}
	cfg := &config.ScriptConfig{Run: "echo $FLOWTEST_VAR1 $FLOWTEST_VAR2"}
	ctx := engine.NewContext()
	ctx.Set("var1", "hello")
	ctx.Set("var2", "world")
	result, err := d.Execute(context.Background(), cfg, ctx, &config.EnvConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["stdout"] != "hello world" {
		t.Errorf("stdout = %q, want %q", result["stdout"], "hello world")
	}
}

// --- Redis Driver Config Tests ---

func TestRedisDriver_NoRedisURL(t *testing.T) {
	d := &RedisDriver{}
	cfg := &config.RedisConfig{Action: "get", Key: "test"}
	_, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	if err == nil {
		t.Fatal("expected error when Redis URL not configured")
	}
}

// --- Postgres Driver Name Tests ---

func TestPostgresDriver_NameCustom(t *testing.T) {
	d := NewPostgresDriver("custom_name", "postgres://localhost/db")
	if d.Name() != "custom_name" {
		t.Errorf("Name() = %q, want %q", d.Name(), "custom_name")
	}
}

// --- SQLite Driver Tests ---

func TestSQLiteDriver_NameCustom(t *testing.T) {
	d := NewSQLiteDriver("mylocal", "sqlite://./test.db")
	if d.Name() != "mylocal" {
		t.Errorf("Name() = %q, want %q", d.Name(), "mylocal")
	}
}

// --- MySQL Driver Tests ---

func TestMySQLDriver_NameCustom(t *testing.T) {
	d := NewMySQLDriver("production", "mysql://user:pass@localhost:3306/prod")
	if d.Name() != "production" {
		t.Errorf("Name() = %q, want %q", d.Name(), "production")
	}
}

// --- Registry Close Tests ---

func TestRegistry_CloseDoesNotPanic(t *testing.T) {
	r, err := NewRegistry(map[string]string{
		"db": "postgres://localhost/test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not panic
	r.Close()
	r.Close() // Double close should also not panic
}

// --- HTTP Auth with Empty Credentials Tests ---

func TestApplyAuth_BasicEmptyPassword(t *testing.T) {
	req, _ := newTestRequest()
	auth := &config.AuthConfig{
		Basic: &config.BasicAuthConfig{
			Username: "user",
			Password: "",
		},
	}

	if err := applyAuth(req, auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still set header even with empty password
	got := req.Header.Get("Authorization")
	if !strings.HasPrefix(got, "Basic ") {
		t.Errorf("expected Basic auth header, got %q", got)
	}
}

func TestApplyAuth_BearerEmpty(t *testing.T) {
	req, _ := newTestRequest()
	auth := &config.AuthConfig{Bearer: ""}

	if err := applyAuth(req, auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Empty bearer should not set header
	got := req.Header.Get("Authorization")
	if got != "" {
		t.Errorf("expected no Authorization header for empty bearer, got %q", got)
	}
}

// --- Shell Driver Timeout Tests ---

func TestShellDriver_WithTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	d := &ShellDriver{}
	cfg := &config.ScriptConfig{
		Run:     "sleep 0.1",
		Timeout: 200 * time.Millisecond,
	}
	_, err := d.Execute(context.Background(), cfg, engine.NewContext(), &config.EnvConfig{})
	// Should complete within timeout
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Kafka Driver Produce Tests ---

func TestKafkaProduceMessage(t *testing.T) {
	msg := kafka.Message{
		Topic: "test-topic",
		Key:   []byte("key1"),
		Value: []byte(`{"test":"data"}`),
	}

	if msg.Topic != "test-topic" {
		t.Errorf("topic = %q, want %q", msg.Topic, "test-topic")
	}
}

func TestKafkaDriver_ProduceAction(t *testing.T) {
	d := &KafkaDriver{}
	cfg := &config.KafkaConfig{
		Action:  "produce",
		Topic:   "test",
		Key:     "key1",
		Message: map[string]interface{}{"test": "data"},
	}

	env := &config.EnvConfig{KafkaBrokers: "localhost:9092"}

	// This will fail to connect, but should validate config properly
	_, err := d.Execute(context.Background(), cfg, engine.NewContext(), env)
	if err != nil {
		// Expected to fail connecting
		if !strings.Contains(err.Error(), "kafka") {
			t.Errorf("expected kafka error, got: %v", err)
		}
	}
}

// --- Registry Multiple Databases Tests ---

func TestNewRegistry_MultipleDatabases(t *testing.T) {
	dbs := map[string]string{
		"postgres": "postgres://localhost/test1",
		"mysql":    "mysql://localhost/test2",
		"mongodb":  "mongodb://localhost/test3",
		"sqlite":   "sqlite://./test.db",
	}

	r, err := NewRegistry(dbs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	for name := range dbs {
		if _, ok := r.drivers[name]; !ok {
			t.Errorf("expected %q driver to be registered", name)
		}
	}
}

// --- CappedBuffer Tests ---

func TestCappedBuffer_UnderLimit(t *testing.T) {
	buf := &cappedBuffer{limit: 100}
	n, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("Write() = %d, want 5", n)
	}
	if buf.String() != "hello" {
		t.Errorf("String() = %q, want %q", buf.String(), "hello")
	}
}

func TestCappedBuffer_OverLimit(t *testing.T) {
	buf := &cappedBuffer{limit: 5}
	n, err := buf.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Reports full len even though truncated (so command doesn't fail)
	if n != 11 {
		t.Errorf("Write() = %d, want 11", n)
	}
	if buf.String() != "hello" {
		t.Errorf("String() = %q, want %q", buf.String(), "hello")
	}
}

func TestCappedBuffer_MultipleWrites(t *testing.T) {
	buf := &cappedBuffer{limit: 10}
	buf.Write([]byte("12345"))
	buf.Write([]byte("67890"))
	buf.Write([]byte("extra")) // should be discarded
	if buf.String() != "1234567890" {
		t.Errorf("String() = %q, want %q", buf.String(), "1234567890")
	}
}
