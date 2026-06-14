package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// FlowConfig represents a complete flow file.
type FlowConfig struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Tags        []string      `yaml:"tags,omitempty"`
	Timeout     time.Duration `yaml:"timeout,omitempty"`
	FailFast    bool          `yaml:"fail_fast,omitempty"`
	Env         EnvConfig     `yaml:"env,omitempty"`
	Setup       []SetupStep   `yaml:"setup,omitempty"`
	Cleanup     []CleanupStep `yaml:"cleanup,omitempty"`
	Steps       []Step        `yaml:"steps"`
}

// EnvConfig holds connection strings and base URLs.
type EnvConfig struct {
	APIBase      string            `yaml:"api_base,omitempty"`
	Databases    map[string]string `yaml:"databases,omitempty"`
	KafkaBrokers string            `yaml:"kafka_brokers,omitempty"`
	Redis        string            `yaml:"redis,omitempty"`
}

// GlobalConfig represents the root flowtest.yaml with profiles.
type GlobalConfig struct {
	Env      EnvConfig            `yaml:"env,omitempty"`
	Profiles map[string]EnvConfig `yaml:"profiles,omitempty"`
}

// SetupStep represents a pre-flow setup action.
type SetupStep struct {
	Seed   *SeedConfig   `yaml:"seed,omitempty"`
	DBStep *DBStepConfig `yaml:"-"` // populated by custom unmarshaling
}

// knownSetupKeys are YAML keys that SetupStep handles via struct tags.
var knownSetupKeys = map[string]bool{"seed": true}

// UnmarshalYAML implements custom unmarshaling to detect dynamic database keys.
func (s *SetupStep) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("setup step must be a mapping")
	}

	// Decode known fields
	type setupAlias SetupStep
	var alias setupAlias
	if err := value.Decode(&alias); err != nil {
		return fmt.Errorf("decoding setup step: %w", err)
	}
	*s = SetupStep(alias)

	// Scan for unknown keys — these are database step references
	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i].Value
		if !knownSetupKeys[key] {
			var dbCfg DBStepConfig
			if err := value.Content[i+1].Decode(&dbCfg); err != nil {
				return fmt.Errorf("decoding database setup step %q: %w", key, err)
			}
			dbCfg.Database = key
			s.DBStep = &dbCfg
			break
		}
	}

	return nil
}

// CleanupStep represents a post-flow cleanup action.
type CleanupStep struct {
	DBStep *DBStepConfig `yaml:"-"` // populated by custom unmarshaling
}

// UnmarshalYAML implements custom unmarshaling to detect dynamic database keys.
func (s *CleanupStep) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("cleanup step must be a mapping")
	}

	// All keys in cleanup are database references
	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i].Value
		var dbCfg DBStepConfig
		if err := value.Content[i+1].Decode(&dbCfg); err != nil {
			return fmt.Errorf("decoding database cleanup step %q: %w", key, err)
		}
		dbCfg.Database = key
		s.DBStep = &dbCfg
		break
	}

	return nil
}

// Step represents a single test step in the flow.
type Step struct {
	Name   string            `yaml:"name"`
	When   string            `yaml:"when,omitempty"`
	Delay  time.Duration     `yaml:"delay,omitempty"`
	Retry  *RetryConfig      `yaml:"retry,omitempty"`
	API    *APIConfig        `yaml:"api,omitempty"`
	DBStep *DBStepConfig     `yaml:"-"` // populated by custom unmarshaling
	Kafka  *KafkaConfig      `yaml:"kafka,omitempty"`
	Redis  *RedisConfig      `yaml:"redis,omitempty"`
	Script *ScriptConfig     `yaml:"script,omitempty"`
	Assert []AssertConfig    `yaml:"assert,omitempty"`
	Save   map[string]string `yaml:"save,omitempty"`
}

// knownStepKeys are YAML keys that Step handles via struct tags.
var knownStepKeys = map[string]bool{
	"name": true, "when": true, "delay": true, "retry": true,
	"api": true, "kafka": true, "redis": true, "script": true,
	"assert": true, "save": true,
}

// UnmarshalYAML implements custom unmarshaling to detect dynamic database keys.
func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("step must be a mapping")
	}

	// Decode known fields via alias to avoid infinite recursion
	type stepAlias Step
	var alias stepAlias
	if err := value.Decode(&alias); err != nil {
		return fmt.Errorf("decoding step: %w", err)
	}
	*s = Step(alias)

	// Scan for unknown keys — these are database step references
	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i].Value
		if !knownStepKeys[key] {
			var dbCfg DBStepConfig
			if err := value.Content[i+1].Decode(&dbCfg); err != nil {
				return fmt.Errorf("decoding database step %q: %w", key, err)
			}
			dbCfg.Database = key
			s.DBStep = &dbCfg
			break
		}
	}

	return nil
}

// RetryConfig defines retry behavior for flaky steps.
type RetryConfig struct {
	Times    int           `yaml:"times"`              // max retry attempts (default 1 = no retry)
	Interval time.Duration `yaml:"interval,omitempty"` // wait between retries (default 1s)
	Backoff  string        `yaml:"backoff,omitempty"`  // "linear" or "exponential" (default: linear)
}

// DriverType returns which driver this step should use.
func (s *Step) DriverType() string {
	switch {
	case s.API != nil:
		return "http"
	case s.DBStep != nil:
		return s.DBStep.Database
	case s.Kafka != nil:
		return "kafka"
	case s.Redis != nil:
		return "redis"
	case s.Script != nil:
		return "shell"
	default:
		return ""
	}
}

// APIConfig defines an HTTP request.
type APIConfig struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Auth    *AuthConfig       `yaml:"auth,omitempty"`
	Body    interface{}       `yaml:"body,omitempty"`
	Timeout time.Duration     `yaml:"timeout,omitempty"`
}

// AuthConfig defines authentication configuration for HTTP requests.
// Only one auth method should be specified per request.
type AuthConfig struct {
	Bearer string           `yaml:"bearer,omitempty"`  // Bearer token value
	Basic  *BasicAuthConfig `yaml:"basic,omitempty"`   // Basic auth credentials
	APIKey *APIKeyConfig    `yaml:"api_key,omitempty"` // API key auth
}

// BasicAuthConfig defines HTTP Basic authentication credentials.
type BasicAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// APIKeyConfig defines API key authentication.
// Either Header or Query should be set, not both.
type APIKeyConfig struct {
	Header string `yaml:"header,omitempty"` // Header name (e.g., "X-API-Key")
	Query  string `yaml:"query,omitempty"`  // Query param name (e.g., "api_key")
	Value  string `yaml:"value"`            // The API key value
}

// DBStepConfig defines a database operation (SQL or document-based).
type DBStepConfig struct {
	Database string `yaml:"-"` // resolved database name (from YAML key)

	// SQL fields (Postgres, MySQL, SQLite)
	Query  string        `yaml:"query,omitempty"`
	Params []interface{} `yaml:"params,omitempty"`

	// MongoDB fields
	Collection string                 `yaml:"collection,omitempty"`
	Operation  string                 `yaml:"operation,omitempty"`
	Filter     map[string]interface{} `yaml:"filter,omitempty"`
	Document   interface{}            `yaml:"document,omitempty"`
	Documents  []interface{}          `yaml:"documents,omitempty"`
	Update     map[string]interface{} `yaml:"update,omitempty"`
}

// SeedConfig defines a database seed operation.
type SeedConfig struct {
	Target string                 `yaml:"target,omitempty"` // database name; defaults to first DB if omitted
	Table  string                 `yaml:"table"`
	Data   map[string]interface{} `yaml:"data"`
}

// KafkaConfig defines a Kafka produce or consume/wait operation.
type KafkaConfig struct {
	Action  string                 `yaml:"action,omitempty"` // "produce" or "consume" (default: consume)
	Topic   string                 `yaml:"topic"`
	Timeout time.Duration          `yaml:"timeout,omitempty"`
	Match   map[string]interface{} `yaml:"match,omitempty"`
	Key     string                 `yaml:"key,omitempty"`
	Message interface{}            `yaml:"message,omitempty"` // for produce
	Headers map[string]string      `yaml:"headers,omitempty"` // for produce
}

// RedisConfig defines a Redis operation.
type RedisConfig struct {
	Action string        `yaml:"action"`
	Key    string        `yaml:"key"`
	Value  interface{}   `yaml:"value,omitempty"`
	TTL    time.Duration `yaml:"ttl,omitempty"`
}

// ScriptConfig defines a shell script execution.
type ScriptConfig struct {
	Lang       string        `yaml:"lang,omitempty"`
	Run        string        `yaml:"run"`
	AssertExit *int          `yaml:"assert_exit,omitempty"`
	Timeout    time.Duration `yaml:"timeout,omitempty"`
}

// AssertConfig defines a single assertion.
type AssertConfig struct {
	Expr string `yaml:"expr"`
	Msg  string `yaml:"msg,omitempty"`
}
