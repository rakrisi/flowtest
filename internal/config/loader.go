package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envVarRegex = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

// LoadGlobalConfig reads the root flowtest.yaml if it exists.
func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return nil, fmt.Errorf("reading global config: %w", err)
	}

	data = []byte(resolveSystemEnvVars(string(data)))

	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing global config %s: %w", path, err)
	}
	return &cfg, nil
}

// LoadFlowConfig reads and parses a flow YAML file.
func LoadFlowConfig(path string) (*FlowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading flow file: %w", err)
	}

	data = []byte(resolveSystemEnvVars(string(data)))

	var cfg FlowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing flow file %s: %w", path, err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating flow %s: %w", path, err)
	}
	return &cfg, nil
}

// LoadFlowFromJSON reads a flow definition from a JSON file.
func LoadFlowFromJSON(path string) (*FlowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading JSON flow file: %w", err)
	}

	var cfg FlowConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing JSON flow file %s: %w", path, err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating flow %s: %w", path, err)
	}
	return &cfg, nil
}

// MergeEnv merges global config with flow-level env, then applies profile.
// Priority: profile > flow > global.
func MergeEnv(global *GlobalConfig, flow *FlowConfig, profile string) (EnvConfig, error) {
	env := global.Env

	// Flow-level overrides global
	mergeEnvField(&env.APIBase, flow.Env.APIBase)
	mergeEnvField(&env.KafkaBrokers, flow.Env.KafkaBrokers)
	mergeEnvField(&env.Redis, flow.Env.Redis)
	mergeDatabases(&env.Databases, flow.Env.Databases)

	// Profile overrides everything
	if profile != "" {
		p, ok := global.Profiles[profile]
		if !ok {
			return env, fmt.Errorf("profile %q not found in global config", profile)
		}
		mergeEnvField(&env.APIBase, p.APIBase)
		mergeEnvField(&env.KafkaBrokers, p.KafkaBrokers)
		mergeEnvField(&env.Redis, p.Redis)
		mergeDatabases(&env.Databases, p.Databases)
	}

	return env, nil
}

func mergeEnvField(target *string, override string) {
	if override != "" {
		*target = override
	}
}

// mergeDatabases merges override databases into target. Individual entries override by name.
func mergeDatabases(target *map[string]string, override map[string]string) {
	if len(override) == 0 {
		return
	}
	if *target == nil {
		*target = make(map[string]string)
	}
	for k, v := range override {
		(*target)[k] = v
	}
}

// resolveSystemEnvVars replaces ${ENV_VAR} patterns with system environment values.
// Only resolves uppercase env vars (system-level), not flow variables like ${token}.
func resolveSystemEnvVars(input string) string {
	return envVarRegex.ReplaceAllStringFunc(input, func(match string) string {
		key := envVarRegex.FindStringSubmatch(match)[1]
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return match // leave unresolved if env var not set
	})
}

func validate(cfg *FlowConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("flow must have a name")
	}
	if len(cfg.Steps) == 0 {
		return fmt.Errorf("flow must have at least one step")
	}

	var errs []string
	for i, step := range cfg.Steps {
		if step.Name == "" {
			errs = append(errs, fmt.Sprintf("step %d: must have a name", i+1))
		}
		if step.DriverType() == "" {
			errs = append(errs, fmt.Sprintf("step %d (%s): must specify exactly one of: <database>, api, kafka, redis, script", i+1, step.Name))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// ValidateEnv checks that the merged environment has the required infrastructure
// for all drivers referenced in the flow. Call this after MergeEnv.
func ValidateEnv(cfg *FlowConfig, env *EnvConfig) error {
	needsDB := map[string]bool{}
	needsKafka := false
	needsRedis := false

	// Collect requirements from steps
	for _, step := range cfg.Steps {
		switch {
		case step.DBStep != nil:
			needsDB[step.DBStep.Database] = true
		case step.Kafka != nil:
			needsKafka = true
		case step.Redis != nil:
			needsRedis = true
		}
	}

	// Collect requirements from setup
	for _, s := range cfg.Setup {
		if s.Seed != nil && s.Seed.Target != "" {
			needsDB[s.Seed.Target] = true
		}
		if s.DBStep != nil {
			needsDB[s.DBStep.Database] = true
		}
	}

	// Collect requirements from cleanup
	for _, s := range cfg.Cleanup {
		if s.DBStep != nil {
			needsDB[s.DBStep.Database] = true
		}
	}

	var errs []string

	for db := range needsDB {
		if _, ok := env.Databases[db]; !ok {
			errs = append(errs, fmt.Sprintf("step references database %q but it is not configured in env.databases", db))
		}
	}

	if needsKafka && env.KafkaBrokers == "" {
		errs = append(errs, "flow uses kafka steps but env.kafka_brokers is not configured")
	}

	if needsRedis && env.Redis == "" {
		errs = append(errs, "flow uses redis steps but env.redis is not configured")
	}

	if len(errs) > 0 {
		return fmt.Errorf("environment validation errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
