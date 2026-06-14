package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFlowConfig_Valid(t *testing.T) {
	yaml := `
name: Test Flow
description: A test
timeout: 10s
steps:
  - name: Step 1
    api:
      method: GET
      url: /health
    assert:
      - expr: "response.status == 200"
  - name: Step 2
    script:
      run: echo hello
      assert_exit: 0
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	if cfg.Name != "Test Flow" {
		t.Errorf("Name = %q, want %q", cfg.Name, "Test Flow")
	}
	if len(cfg.Steps) != 2 {
		t.Errorf("Steps count = %d, want 2", len(cfg.Steps))
	}
	if cfg.Steps[0].DriverType() != "http" {
		t.Errorf("Step 1 driver = %q, want %q", cfg.Steps[0].DriverType(), "http")
	}
	if cfg.Steps[1].DriverType() != "shell" {
		t.Errorf("Step 2 driver = %q, want %q", cfg.Steps[1].DriverType(), "shell")
	}
}

func TestLoadFlowConfig_DBStep(t *testing.T) {
	yaml := `
name: DB Flow
steps:
  - name: Query Users
    mydb:
      query: "SELECT * FROM users WHERE id = $1"
      params:
        - 42
    assert:
      - expr: "row_count == 1"
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	step := cfg.Steps[0]
	if step.DBStep == nil {
		t.Fatal("expected DBStep to be set")
	}
	if step.DBStep.Database != "mydb" {
		t.Errorf("Database = %q, want %q", step.DBStep.Database, "mydb")
	}
	if step.DBStep.Query != "SELECT * FROM users WHERE id = $1" {
		t.Errorf("Query = %q, want SELECT query", step.DBStep.Query)
	}
	if step.DriverType() != "mydb" {
		t.Errorf("DriverType() = %q, want %q", step.DriverType(), "mydb")
	}
}

func TestLoadFlowConfig_MongoStep(t *testing.T) {
	yaml := `
name: Mongo Flow
steps:
  - name: Find Events
    events:
      collection: events
      operation: find
      filter:
        user_id: "abc123"
    assert:
      - expr: "row_count > 0"
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	step := cfg.Steps[0]
	if step.DBStep == nil {
		t.Fatal("expected DBStep to be set")
	}
	if step.DBStep.Database != "events" {
		t.Errorf("Database = %q, want %q", step.DBStep.Database, "events")
	}
	if step.DBStep.Collection != "events" {
		t.Errorf("Collection = %q, want %q", step.DBStep.Collection, "events")
	}
	if step.DBStep.Operation != "find" {
		t.Errorf("Operation = %q, want %q", step.DBStep.Operation, "find")
	}
	if step.DBStep.Filter["user_id"] != "abc123" {
		t.Errorf("Filter[user_id] = %v, want %q", step.DBStep.Filter["user_id"], "abc123")
	}
}

func TestLoadFlowConfig_SetupCleanupDB(t *testing.T) {
	yaml := `
name: Setup Cleanup Flow
setup:
  - mydb:
      query: "DELETE FROM users WHERE test = true"
  - seed:
      target: mydb
      table: users
      data:
        email: test@example.com
cleanup:
  - mydb:
      query: "DELETE FROM users WHERE email = 'test@example.com'"
steps:
  - name: Check
    script:
      run: echo ok
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	// Setup DB step
	if cfg.Setup[0].DBStep == nil {
		t.Fatal("expected Setup[0].DBStep to be set")
	}
	if cfg.Setup[0].DBStep.Database != "mydb" {
		t.Errorf("Setup DB database = %q, want %q", cfg.Setup[0].DBStep.Database, "mydb")
	}

	// Setup seed
	if cfg.Setup[1].Seed == nil {
		t.Fatal("expected Setup[1].Seed to be set")
	}
	if cfg.Setup[1].Seed.Target != "mydb" {
		t.Errorf("Seed target = %q, want %q", cfg.Setup[1].Seed.Target, "mydb")
	}

	// Cleanup DB step
	if cfg.Cleanup[0].DBStep == nil {
		t.Fatal("expected Cleanup[0].DBStep to be set")
	}
	if cfg.Cleanup[0].DBStep.Database != "mydb" {
		t.Errorf("Cleanup DB database = %q, want %q", cfg.Cleanup[0].DBStep.Database, "mydb")
	}
}

func TestLoadFlowConfig_MissingName(t *testing.T) {
	yaml := `
steps:
  - name: Step 1
    script:
      run: echo hello
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	_, err := LoadFlowConfig(path)
	if err == nil {
		t.Fatal("expected validation error for missing name")
	}
}

func TestLoadFlowConfig_NoSteps(t *testing.T) {
	yaml := `
name: Empty Flow
steps: []
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	_, err := LoadFlowConfig(path)
	if err == nil {
		t.Fatal("expected validation error for no steps")
	}
}

func TestLoadFlowConfig_StepMissingDriver(t *testing.T) {
	yaml := `
name: Bad Flow
steps:
  - name: No driver
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	_, err := LoadFlowConfig(path)
	if err == nil {
		t.Fatal("expected validation error for step without driver")
	}
}

func TestResolveSystemEnvVars(t *testing.T) {
	t.Setenv("FLOWTEST_TEST_VAR", "resolved_value")

	input := "url: ${FLOWTEST_TEST_VAR}/path"
	got := resolveSystemEnvVars(input)
	want := "url: resolved_value/path"
	if got != want {
		t.Errorf("resolveSystemEnvVars = %q, want %q", got, want)
	}
}

func TestResolveSystemEnvVars_Unset(t *testing.T) {
	input := "url: ${FLOWTEST_NONEXISTENT}/path"
	got := resolveSystemEnvVars(input)
	if got != input {
		t.Errorf("expected unresolved var to stay, got %q", got)
	}
}

func TestMergeEnv(t *testing.T) {
	global := &GlobalConfig{
		Env: EnvConfig{
			APIBase:   "http://global:8000",
			Databases: map[string]string{"db": "postgres://global"},
		},
		Profiles: map[string]EnvConfig{
			"staging": {
				APIBase: "http://staging:8000",
			},
		},
	}

	flow := &FlowConfig{
		Name:  "test",
		Steps: []Step{{Name: "s", Script: &ScriptConfig{Run: "echo"}}},
		Env: EnvConfig{
			Redis: "redis://flow",
		},
	}

	t.Run("no profile", func(t *testing.T) {
		env, err := MergeEnv(global, flow, "")
		if err != nil {
			t.Fatal(err)
		}
		if env.APIBase != "http://global:8000" {
			t.Errorf("APIBase = %q, want global", env.APIBase)
		}
		if env.Databases["db"] != "postgres://global" {
			t.Errorf("Databases[db] = %q, want global", env.Databases["db"])
		}
		if env.Redis != "redis://flow" {
			t.Errorf("Redis = %q, want flow", env.Redis)
		}
	})

	t.Run("with profile", func(t *testing.T) {
		env, err := MergeEnv(global, flow, "staging")
		if err != nil {
			t.Fatal(err)
		}
		if env.APIBase != "http://staging:8000" {
			t.Errorf("APIBase = %q, want staging", env.APIBase)
		}
		if env.Databases["db"] != "postgres://global" {
			t.Errorf("Databases[db] = %q, want global (not overridden by profile)", env.Databases["db"])
		}
	})

	t.Run("merge databases", func(t *testing.T) {
		flowWithDB := &FlowConfig{
			Name:  "test",
			Steps: []Step{{Name: "s", Script: &ScriptConfig{Run: "echo"}}},
			Env: EnvConfig{
				Databases: map[string]string{"analytics": "mysql://flow-analytics"},
			},
		}
		env, err := MergeEnv(global, flowWithDB, "")
		if err != nil {
			t.Fatal(err)
		}
		if env.Databases["db"] != "postgres://global" {
			t.Errorf("Databases[db] = %q, want global", env.Databases["db"])
		}
		if env.Databases["analytics"] != "mysql://flow-analytics" {
			t.Errorf("Databases[analytics] = %q, want flow", env.Databases["analytics"])
		}
	})

	t.Run("unknown profile", func(t *testing.T) {
		_, err := MergeEnv(global, flow, "unknown")
		if err == nil {
			t.Fatal("expected error for unknown profile")
		}
	})
}

func TestLoadFlowFromJSON(t *testing.T) {
	jsonData := `{
		"name": "JSON Flow",
		"steps": [
			{
				"name": "Echo",
				"script": {
					"run": "echo hello"
				}
			}
		]
	}`
	path := writeTempFile(t, "flow-*.json", jsonData)
	cfg, err := LoadFlowFromJSON(path)
	if err != nil {
		t.Fatalf("LoadFlowFromJSON error: %v", err)
	}
	if cfg.Name != "JSON Flow" {
		t.Errorf("Name = %q, want %q", cfg.Name, "JSON Flow")
	}
}

func TestStep_DriverType(t *testing.T) {
	tests := []struct {
		name string
		step Step
		want string
	}{
		{"api", Step{Name: "s", API: &APIConfig{}}, "http"},
		{"db", Step{Name: "s", DBStep: &DBStepConfig{Database: "mydb"}}, "mydb"},
		{"kafka", Step{Name: "s", Kafka: &KafkaConfig{}}, "kafka"},
		{"redis", Step{Name: "s", Redis: &RedisConfig{}}, "redis"},
		{"shell", Step{Name: "s", Script: &ScriptConfig{}}, "shell"},
		{"empty", Step{Name: "s"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.step.DriverType(); got != tt.want {
				t.Errorf("DriverType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadFlowConfig_AuthBearer(t *testing.T) {
	yaml := `
name: Bearer Auth Flow
steps:
  - name: Protected Endpoint
    api:
      method: GET
      url: /protected
      auth:
        bearer: my-token-123
    assert:
      - expr: "response.status == 200"
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	step := cfg.Steps[0]
	if step.API == nil {
		t.Fatal("expected API to be set")
	}
	if step.API.Auth == nil {
		t.Fatal("expected Auth to be set")
	}
	if step.API.Auth.Bearer != "my-token-123" {
		t.Errorf("Bearer = %q, want %q", step.API.Auth.Bearer, "my-token-123")
	}
}

func TestLoadFlowConfig_AuthBasic(t *testing.T) {
	yaml := `
name: Basic Auth Flow
steps:
  - name: Protected Endpoint
    api:
      method: GET
      url: /protected
      auth:
        basic:
          username: admin
          password: secret123
    assert:
      - expr: "response.status == 200"
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	step := cfg.Steps[0]
	if step.API == nil {
		t.Fatal("expected API to be set")
	}
	if step.API.Auth == nil {
		t.Fatal("expected Auth to be set")
	}
	if step.API.Auth.Basic == nil {
		t.Fatal("expected Basic auth to be set")
	}
	if step.API.Auth.Basic.Username != "admin" {
		t.Errorf("Username = %q, want %q", step.API.Auth.Basic.Username, "admin")
	}
	if step.API.Auth.Basic.Password != "secret123" {
		t.Errorf("Password = %q, want %q", step.API.Auth.Basic.Password, "secret123")
	}
}

func TestLoadFlowConfig_AuthAPIKeyHeader(t *testing.T) {
	yaml := `
name: API Key Header Flow
steps:
  - name: Protected Endpoint
    api:
      method: GET
      url: /protected
      auth:
        api_key:
          header: X-API-Key
          value: key-abc-123
    assert:
      - expr: "response.status == 200"
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	step := cfg.Steps[0]
	if step.API == nil {
		t.Fatal("expected API to be set")
	}
	if step.API.Auth == nil {
		t.Fatal("expected Auth to be set")
	}
	if step.API.Auth.APIKey == nil {
		t.Fatal("expected APIKey auth to be set")
	}
	if step.API.Auth.APIKey.Header != "X-API-Key" {
		t.Errorf("Header = %q, want %q", step.API.Auth.APIKey.Header, "X-API-Key")
	}
	if step.API.Auth.APIKey.Value != "key-abc-123" {
		t.Errorf("Value = %q, want %q", step.API.Auth.APIKey.Value, "key-abc-123")
	}
}

func TestLoadFlowConfig_AuthAPIKeyQuery(t *testing.T) {
	yaml := `
name: API Key Query Flow
steps:
  - name: Protected Endpoint
    api:
      method: GET
      url: /protected
      auth:
        api_key:
          query: api_key
          value: query-key-456
    assert:
      - expr: "response.status == 200"
`
	path := writeTempFile(t, "flow-*.yaml", yaml)
	cfg, err := LoadFlowConfig(path)
	if err != nil {
		t.Fatalf("LoadFlowConfig error: %v", err)
	}

	step := cfg.Steps[0]
	if step.API == nil {
		t.Fatal("expected API to be set")
	}
	if step.API.Auth == nil {
		t.Fatal("expected Auth to be set")
	}
	if step.API.Auth.APIKey == nil {
		t.Fatal("expected APIKey auth to be set")
	}
	if step.API.Auth.APIKey.Query != "api_key" {
		t.Errorf("Query = %q, want %q", step.API.Auth.APIKey.Query, "api_key")
	}
	if step.API.Auth.APIKey.Value != "query-key-456" {
		t.Errorf("Value = %q, want %q", step.API.Auth.APIKey.Value, "query-key-456")
	}
}

func writeTempFile(t *testing.T, pattern string, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, pattern)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
