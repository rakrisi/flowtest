package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/rakrisi/flowtest/internal/config"
)

// Driver is the interface all step drivers must implement.
type Driver interface {
	Name() string
	Execute(ctx context.Context, stepConfig interface{}, flowCtx *Context, env *config.EnvConfig) (map[string]interface{}, error)
}

// Printer is the interface for step-by-step output.
type Printer interface {
	FlowHeader(name string)
	SectionHeader(name string)
	StepStart(name string, driverType string, stepNum int, totalSteps int)
	StepResult(result *StepResult, stepNum int, totalSteps int, verbose bool)
	SetupStart(description string)
	SetupResult(description string, err error)
	CleanupStart(description string)
	CleanupResult(description string, err error)
}

// Engine orchestrates flow execution.
type Engine struct {
	drivers  map[string]Driver
	printer  Printer
	verbose  bool
	failFast bool
	dryRun   bool
}

// NewEngine creates a new engine with the given options.
func NewEngine(printer Printer, verbose bool, failFast bool) *Engine {
	return &Engine{
		drivers:  make(map[string]Driver),
		printer:  printer,
		verbose:  verbose,
		failFast: failFast,
	}
}

// SetDryRun enables dry-run mode.
func (e *Engine) SetDryRun(v bool) { e.dryRun = v }

// RegisterDriver adds a driver to the engine.
func (e *Engine) RegisterDriver(name string, d Driver) {
	e.drivers[name] = d
}

// Run executes a complete flow and returns the result.
// initVars optionally seeds the variable store before execution.
func (e *Engine) Run(ctx context.Context, flowCfg *config.FlowConfig, env *config.EnvConfig, initVars ...map[string]interface{}) *FlowResult {
	flowStart := time.Now()
	flowCtx := NewContext()

	// Seed initial variables
	if len(initVars) > 0 && initVars[0] != nil {
		for k, v := range initVars[0] {
			flowCtx.Set(k, v)
		}
	}

	result := &FlowResult{
		Name: flowCfg.Name,
	}

	// Determine timeout
	if flowCfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, flowCfg.Timeout)
		defer cancel()
	}

	failFast := e.failFast || flowCfg.FailFast

	e.printer.FlowHeader(flowCfg.Name)

	// Run setup
	if len(flowCfg.Setup) > 0 {
		e.printer.SectionHeader("Setup")
		e.runSetup(ctx, flowCfg.Setup, flowCtx, env)
	}

	// Run steps
	e.printer.SectionHeader("Steps")
	aborted := false
	for i, step := range flowCfg.Steps {
		if aborted {
			result.Steps = append(result.Steps, StepResult{
				Name:       step.Name,
				Status:     StatusSkipped,
				Driver:     step.DriverType(),
				SkipReason: "aborted due to previous failure (fail_fast)",
			})
			continue
		}

		// Check context deadline
		if ctx.Err() != nil {
			result.Steps = append(result.Steps, StepResult{
				Name:   step.Name,
				Status: StatusErrored,
				Driver: step.DriverType(),
				Error:  "flow timeout exceeded",
			})
			aborted = true
			continue
		}

		stepResult := e.runStepWithRetry(ctx, &step, i, len(flowCfg.Steps), flowCtx, env)
		result.Steps = append(result.Steps, *stepResult)

		if failFast && (stepResult.Status == StatusFailed || stepResult.Status == StatusErrored) {
			aborted = true
		}
	}

	// Run cleanup (always)
	if len(flowCfg.Cleanup) > 0 {
		e.printer.SectionHeader("Cleanup")
		e.runCleanup(ctx, flowCfg.Cleanup, flowCtx, env)
	}

	result.Duration = time.Since(flowStart)
	result.Tally()
	return result
}

func (e *Engine) runStepWithRetry(ctx context.Context, step *config.Step, idx int, total int, flowCtx *Context, env *config.EnvConfig) *StepResult {
	maxAttempts := 1
	interval := 1 * time.Second
	backoff := "linear"

	if step.Retry != nil && step.Retry.Times > 1 {
		maxAttempts = step.Retry.Times
		if step.Retry.Interval > 0 {
			interval = step.Retry.Interval
		}
		if step.Retry.Backoff != "" {
			backoff = step.Retry.Backoff
		}
	}

	var sr *StepResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		sr = e.runStep(ctx, step, idx, total, flowCtx, env, attempt > 1)

		if sr.Status == StatusPassed || sr.Status == StatusSkipped {
			if attempt > 1 {
				sr.Retries = attempt - 1
			}
			return sr
		}

		// Don't retry on last attempt
		if attempt < maxAttempts {
			wait := interval
			if backoff == "exponential" {
				wait = interval * time.Duration(1<<(attempt-1))
			} else {
				wait = interval * time.Duration(attempt)
			}
			time.Sleep(wait)
		}
	}

	if maxAttempts > 1 {
		sr.Retries = maxAttempts - 1
	}
	return sr
}

func (e *Engine) runStep(ctx context.Context, step *config.Step, idx int, total int, flowCtx *Context, env *config.EnvConfig, isRetry bool) *StepResult {
	stepStart := time.Now()

	if !isRetry {
		e.printer.StepStart(step.Name, step.DriverType(), idx+1, total)
	}

	// Handle delay
	if step.Delay > 0 {
		time.Sleep(step.Delay)
	}

	// Evaluate 'when' condition
	if step.When != "" {
		resolved := flowCtx.ResolveString(step.When)
		ok, err := flowCtx.EvalBool(resolved)
		if err != nil {
			sr := &StepResult{
				Name:     step.Name,
				Status:   StatusErrored,
				Duration: time.Since(stepStart),
				Driver:   step.DriverType(),
				Error:    fmt.Sprintf("evaluating 'when' condition: %v", err),
			}
			e.printer.StepResult(sr, idx+1, total, e.verbose)
			return sr
		}
		if !ok {
			sr := &StepResult{
				Name:       step.Name,
				Status:     StatusSkipped,
				Duration:   time.Since(stepStart),
				Driver:     step.DriverType(),
				SkipReason: fmt.Sprintf("when condition false: %s", step.When),
			}
			e.printer.StepResult(sr, idx+1, total, e.verbose)
			return sr
		}
	}

	// Resolve variables in step config
	driverType := step.DriverType()
	driver, ok := e.drivers[driverType]
	if !ok {
		sr := &StepResult{
			Name:     step.Name,
			Status:   StatusErrored,
			Duration: time.Since(stepStart),
			Driver:   driverType,
			Error:    fmt.Sprintf("no driver registered for type %q", driverType),
		}
		e.printer.StepResult(sr, idx+1, total, e.verbose)
		return sr
	}

	// Get the step's driver-specific config
	stepConfig := e.resolveStepConfig(step, flowCtx)

	// Capture verbose detail before execution
	var detail *StepDetail
	if e.verbose {
		detail = buildStepDetail(stepConfig, env)
	}

	// Dry-run: show what would execute, don't actually run
	if e.dryRun {
		sr := &StepResult{
			Name:       step.Name,
			Status:     StatusSkipped,
			Duration:   time.Since(stepStart),
			Driver:     driverType,
			SkipReason: "dry-run",
			Detail:     detail,
		}
		e.printer.StepResult(sr, idx+1, total, true) // always verbose in dry-run
		return sr
	}

	// Execute driver
	driverResult, err := driver.Execute(ctx, stepConfig, flowCtx, env)
	if err != nil {
		sr := &StepResult{
			Name:     step.Name,
			Status:   StatusErrored,
			Duration: time.Since(stepStart),
			Driver:   driverType,
			Error:    err.Error(),
			Detail:   detail,
		}
		e.printer.StepResult(sr, idx+1, total, e.verbose)
		return sr
	}

	// Enrich verbose detail with response data
	if detail != nil {
		enrichDetail(detail, driverResult)
	}

	// Merge driver result into context for assertions
	for k, v := range driverResult {
		flowCtx.Set(k, v)
	}

	// Run assertions
	assertions := e.runAssertions(step.Assert, flowCtx)
	allPassed := true
	for _, a := range assertions {
		if !a.Passed {
			allPassed = false
		}
	}

	// Save variables
	if step.Save != nil && allPassed {
		if err := flowCtx.SaveFromResult(step.Save, driverResult); err != nil {
			sr := &StepResult{
				Name:       step.Name,
				Status:     StatusErrored,
				Duration:   time.Since(stepStart),
				Driver:     driverType,
				Assertions: assertions,
				Error:      fmt.Sprintf("saving variables: %v", err),
				Detail:     detail,
			}
			e.printer.StepResult(sr, idx+1, total, e.verbose)
			return sr
		}

		// Capture saved variable values for verbose
		if detail != nil {
			detail.Saved = make(map[string]string, len(step.Save))
			for varName, expr := range step.Save {
				if val, ok := flowCtx.Get(varName); ok {
					detail.Saved[varName] = fmt.Sprintf("%v", val)
				} else {
					detail.Saved[varName] = fmt.Sprintf("(%s)", expr)
				}
			}
		}
	}

	status := StatusPassed
	if !allPassed {
		status = StatusFailed
	}

	sr := &StepResult{
		Name:       step.Name,
		Status:     status,
		Duration:   time.Since(stepStart),
		Driver:     driverType,
		Assertions: assertions,
		Detail:     detail,
	}
	e.printer.StepResult(sr, idx+1, total, e.verbose)
	return sr
}

func buildStepDetail(stepConfig interface{}, env *config.EnvConfig) *StepDetail {
	d := &StepDetail{}
	switch cfg := stepConfig.(type) {
	case *config.APIConfig:
		d.Method = cfg.Method
		url := cfg.URL
		if !strings.HasPrefix(url, "http") {
			url = strings.TrimRight(env.APIBase, "/") + "/" + strings.TrimLeft(url, "/")
		}
		d.URL = url
		if cfg.Body != nil {
			if b, err := json.Marshal(cfg.Body); err == nil {
				d.RequestBody = truncate(string(b), 200)
			}
		}
	case *config.DBStepConfig:
		if cfg.Query != "" {
			d.Query = cfg.Query
			if len(cfg.Params) > 0 {
				if b, err := json.Marshal(cfg.Params); err == nil {
					d.Params = string(b)
				}
			}
		} else if cfg.Collection != "" {
			d.Query = fmt.Sprintf("%s.%s", cfg.Collection, cfg.Operation)
			if len(cfg.Filter) > 0 {
				if b, err := json.Marshal(cfg.Filter); err == nil {
					d.Params = string(b)
				}
			}
		}
	case *config.KafkaConfig:
		d.Topic = cfg.Topic
		d.Action = cfg.Action
		if d.Action == "" {
			d.Action = "search"
		}
		if len(cfg.Match) > 0 {
			if b, err := json.Marshal(cfg.Match); err == nil {
				d.Match = string(b)
			}
		}
	case *config.RedisConfig:
		d.RedisAction = cfg.Action
		d.Key = cfg.Key
	case *config.ScriptConfig:
		d.Command = truncate(cfg.Run, 100)
	}
	return d
}

func enrichDetail(d *StepDetail, result map[string]interface{}) {
	if resp, ok := result["response"].(map[string]interface{}); ok {
		if status, ok := resp["status"].(int); ok {
			d.StatusCode = status
		}
		if body := resp["body"]; body != nil {
			if b, err := json.Marshal(body); err == nil {
				d.ResponseBody = truncate(string(b), 300)
			}
		}
	}
	if stdout, ok := result["stdout"].(string); ok {
		d.Stdout = truncate(stdout, 200)
	}
}

func (e *Engine) resolveStepConfig(step *config.Step, flowCtx *Context) interface{} {
	switch {
	case step.API != nil:
		api := *step.API
		// Parse query parameters separately to URL-encode resolved values correctly
		parts := strings.SplitN(api.URL, "?", 2)
		resolvedBase := flowCtx.ResolveString(parts[0])
		if len(parts) > 1 {
			queryParams, err := url.ParseQuery(parts[1])
			if err == nil {
				for paramName, paramValues := range queryParams {
					resolvedValues := make([]string, len(paramValues))
					for i, val := range paramValues {
						resolvedValues[i] = flowCtx.ResolveString(val)
					}
					queryParams[paramName] = resolvedValues
				}
				api.URL = resolvedBase + "?" + queryParams.Encode()
			} else {
				api.URL = resolvedBase + "?" + flowCtx.ResolveString(parts[1])
			}
		} else {
			api.URL = resolvedBase
		}
		if api.Headers != nil {
			resolved := make(map[string]string, len(api.Headers))
			for k, v := range api.Headers {
				resolved[k] = flowCtx.ResolveString(v)
			}
			api.Headers = resolved
		}
		if api.Body != nil {
			api.Body = flowCtx.ResolveInterface(api.Body)
		}
		// Resolve auth fields
		if api.Auth != nil {
			auth := *api.Auth
			if auth.Bearer != "" {
				auth.Bearer = flowCtx.ResolveString(auth.Bearer)
			}
			if auth.Basic != nil {
				basic := *auth.Basic
				basic.Username = flowCtx.ResolveString(basic.Username)
				basic.Password = flowCtx.ResolveString(basic.Password)
				auth.Basic = &basic
			}
			if auth.APIKey != nil {
				apiKey := *auth.APIKey
				apiKey.Value = flowCtx.ResolveString(apiKey.Value)
				auth.APIKey = &apiKey
			}
			api.Auth = &auth
		}
		return &api
	case step.DBStep != nil:
		db := *step.DBStep
		db.Query = flowCtx.ResolveString(db.Query)
		if len(db.Params) > 0 {
			resolved := make([]interface{}, len(db.Params))
			for i, p := range db.Params {
				resolved[i] = flowCtx.ResolveInterface(p)
			}
			db.Params = resolved
		}
		if db.Filter != nil {
			resolved := make(map[string]interface{}, len(db.Filter))
			for k, v := range db.Filter {
				resolved[k] = flowCtx.ResolveInterface(v)
			}
			db.Filter = resolved
		}
		if db.Document != nil {
			db.Document = flowCtx.ResolveInterface(db.Document)
		}
		if db.Update != nil {
			resolved := make(map[string]interface{}, len(db.Update))
			for k, v := range db.Update {
				resolved[k] = flowCtx.ResolveInterface(v)
			}
			db.Update = resolved
		}
		return &db
	case step.Kafka != nil:
		kafka := *step.Kafka
		kafka.Topic = flowCtx.ResolveString(kafka.Topic)
		kafka.Key = flowCtx.ResolveString(kafka.Key)
		if kafka.Match != nil {
			resolved := make(map[string]interface{}, len(kafka.Match))
			for k, v := range kafka.Match {
				resolved[k] = flowCtx.ResolveInterface(v)
			}
			kafka.Match = resolved
		}
		if kafka.Message != nil {
			kafka.Message = flowCtx.ResolveInterface(kafka.Message)
		}
		if kafka.Headers != nil {
			resolved := make(map[string]string, len(kafka.Headers))
			for k, v := range kafka.Headers {
				resolved[k] = flowCtx.ResolveString(v)
			}
			kafka.Headers = resolved
		}
		return &kafka
	case step.Redis != nil:
		redis := *step.Redis
		redis.Key = flowCtx.ResolveString(redis.Key)
		if redis.Value != nil {
			redis.Value = flowCtx.ResolveInterface(redis.Value)
		}
		return &redis
	case step.Script != nil:
		script := *step.Script
		script.Run = flowCtx.ResolveString(script.Run)
		return &script
	default:
		return nil
	}
}

func (e *Engine) runAssertions(asserts []config.AssertConfig, flowCtx *Context) []AssertionResult {
	var results []AssertionResult
	for _, a := range asserts {
		resolved := flowCtx.ResolveString(a.Expr)
		ok, err := flowCtx.EvalBool(resolved)

		ar := AssertionResult{
			Expression: a.Expr,
			Message:    a.Msg,
		}

		if err != nil {
			ar.Passed = false
			ar.Error = err.Error()
		} else {
			ar.Passed = ok
			// On failure, try to show what each side evaluated to
			if !ok {
				ar.Actual = evalExprSides(resolved, flowCtx)
			}
		}

		results = append(results, ar)
	}
	return results
}

// evalExprSides tries to evaluate each side of a comparison to show actual values.
// e.g., "response.status == 200" -> "got 404"
func evalExprSides(expr string, flowCtx *Context) string {
	// Try common comparison operators
	for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		parts := strings.SplitN(expr, op, 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			lVal, lErr := flowCtx.EvalExpr(left)
			if lErr == nil {
				return fmt.Sprintf("got %v", lVal)
			}
			break
		}
	}
	return ""
}

func (e *Engine) runSetup(ctx context.Context, steps []config.SetupStep, flowCtx *Context, env *config.EnvConfig) {
	for _, s := range steps {
		if s.Seed != nil {
			desc := fmt.Sprintf("seed %s", s.Seed.Table)
			e.printer.SetupStart(desc)

			// Resolve target database for seed
			target := s.Seed.Target
			if target == "" {
				var err error
				target, err = config.DefaultDatabase(env.Databases)
				if err != nil {
					e.printer.SetupResult(desc, fmt.Errorf("seed: %w", err))
					continue
				}
			}

			driver, ok := e.drivers[target]
			if !ok {
				e.printer.SetupResult(desc, fmt.Errorf("no driver registered for database %q", target))
				continue
			}

			_, err := driver.Execute(ctx, s.Seed, flowCtx, env)
			e.printer.SetupResult(desc, err)
		}
		if s.DBStep != nil {
			desc := fmt.Sprintf("%s: %s", s.DBStep.Database, truncate(s.DBStep.Query, 50))
			e.printer.SetupStart(desc)

			driver, ok := e.drivers[s.DBStep.Database]
			if !ok {
				e.printer.SetupResult(desc, fmt.Errorf("no driver registered for database %q", s.DBStep.Database))
				continue
			}

			_, err := driver.Execute(ctx, s.DBStep, flowCtx, env)
			e.printer.SetupResult(desc, err)
		}
	}
}

func (e *Engine) runCleanup(ctx context.Context, steps []config.CleanupStep, flowCtx *Context, env *config.EnvConfig) {
	for _, s := range steps {
		if s.DBStep != nil {
			desc := fmt.Sprintf("%s: %s", s.DBStep.Database, truncate(s.DBStep.Query, 50))
			e.printer.CleanupStart(desc)

			driver, ok := e.drivers[s.DBStep.Database]
			if !ok {
				e.printer.CleanupResult(desc, fmt.Errorf("no driver registered for database %q", s.DBStep.Database))
				continue
			}

			db := *s.DBStep
			db.Query = flowCtx.ResolveString(db.Query)

			_, err := driver.Execute(ctx, &db, flowCtx, env)
			e.printer.CleanupResult(desc, err)
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
