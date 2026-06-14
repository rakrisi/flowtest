package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/radhe-singh/flowtest/internal/config"
)

// mockDriver is a test driver that returns configured results.
type mockDriver struct {
	name   string
	result map[string]interface{}
	err    error
	called int
}

func (d *mockDriver) Name() string { return d.name }

func (d *mockDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *Context, env *config.EnvConfig) (map[string]interface{}, error) {
	d.called++
	return d.result, d.err
}

// nullPrinter discards all output.
type nullPrinter struct{}

func (p *nullPrinter) FlowHeader(string)                      {}
func (p *nullPrinter) SectionHeader(string)                    {}
func (p *nullPrinter) StepStart(string, string, int, int)      {}
func (p *nullPrinter) StepResult(*StepResult, int, int, bool)  {}
func (p *nullPrinter) SetupStart(string)                       {}
func (p *nullPrinter) SetupResult(string, error)               {}
func (p *nullPrinter) CleanupStart(string)                     {}
func (p *nullPrinter) CleanupResult(string, error)             {}

func TestEngine_RunPassingFlow(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, false)

	driver := &mockDriver{
		name: "shell",
		result: map[string]interface{}{
			"stdout":    "hello",
			"exit_code": 0,
		},
	}
	eng.RegisterDriver("shell", driver)

	flow := &config.FlowConfig{
		Name: "Test Flow",
		Steps: []config.Step{
			{
				Name:   "Step 1",
				Script: &config.ScriptConfig{Run: "echo hello"},
				Assert: []config.AssertConfig{
					{Expr: "stdout == 'hello'"},
				},
			},
		},
	}

	env := &config.EnvConfig{}
	result := eng.Run(context.Background(), flow, env)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if !result.Success() {
		t.Error("expected Success() = true")
	}
}

func TestEngine_RunFailingAssertion(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, false)

	driver := &mockDriver{
		name: "shell",
		result: map[string]interface{}{
			"stdout":    "wrong",
			"exit_code": 0,
		},
	}
	eng.RegisterDriver("shell", driver)

	flow := &config.FlowConfig{
		Name: "Test Flow",
		Steps: []config.Step{
			{
				Name:   "Failing Step",
				Script: &config.ScriptConfig{Run: "echo wrong"},
				Assert: []config.AssertConfig{
					{Expr: "stdout == 'expected'", Msg: "should match"},
				},
			},
		},
	}

	env := &config.EnvConfig{}
	result := eng.Run(context.Background(), flow, env)

	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if result.Success() {
		t.Error("expected Success() = false")
	}
}

func TestEngine_FailFast(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, true)

	driver := &mockDriver{
		name: "shell",
		result: map[string]interface{}{
			"stdout":    "wrong",
			"exit_code": 0,
		},
	}
	eng.RegisterDriver("shell", driver)

	flow := &config.FlowConfig{
		Name: "Test Flow",
		Steps: []config.Step{
			{
				Name:   "Failing",
				Script: &config.ScriptConfig{Run: "echo wrong"},
				Assert: []config.AssertConfig{
					{Expr: "stdout == 'expected'"},
				},
			},
			{
				Name:   "Should Skip",
				Script: &config.ScriptConfig{Run: "echo skip"},
			},
		},
	}

	env := &config.EnvConfig{}
	result := eng.Run(context.Background(), flow, env)

	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
}

func TestEngine_WhenCondition(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, false)

	driver := &mockDriver{
		name:   "shell",
		result: map[string]interface{}{"stdout": "ok", "exit_code": 0},
	}
	eng.RegisterDriver("shell", driver)

	flow := &config.FlowConfig{
		Name: "Test Flow",
		Steps: []config.Step{
			{
				Name:   "Should Skip",
				When:   "1 == 2",
				Script: &config.ScriptConfig{Run: "echo nope"},
			},
			{
				Name:   "Should Run",
				When:   "1 == 1",
				Script: &config.ScriptConfig{Run: "echo yes"},
			},
		},
	}

	env := &config.EnvConfig{}
	result := eng.Run(context.Background(), flow, env)

	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}

func TestEngine_VariableChaining(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, false)

	// Override Execute to return different results per call
	eng.RegisterDriver("shell", &variableChainingDriver{})

	flow := &config.FlowConfig{
		Name: "Test Flow",
		Steps: []config.Step{
			{
				Name:   "Step 1",
				Script: &config.ScriptConfig{Run: "echo first"},
				Save: map[string]string{
					"myvar": "stdout",
				},
			},
			{
				Name:   "Step 2",
				Script: &config.ScriptConfig{Run: "echo ${myvar}"},
				Assert: []config.AssertConfig{
					{Expr: "myvar == 'first_output'"},
				},
			},
		},
	}

	env := &config.EnvConfig{}
	result := eng.Run(context.Background(), flow, env)

	if result.Passed != 2 {
		t.Errorf("Passed = %d, want 2", result.Passed)
		for _, s := range result.Steps {
			t.Logf("  %s: %s (err: %s)", s.Name, s.Status, s.Error)
			for _, a := range s.Assertions {
				t.Logf("    assert %q passed=%v err=%s", a.Expression, a.Passed, a.Error)
			}
		}
	}
}

type variableChainingDriver struct {
	calls int
}

func (d *variableChainingDriver) Name() string { return "shell" }

func (d *variableChainingDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *Context, env *config.EnvConfig) (map[string]interface{}, error) {
	d.calls++
	if d.calls == 1 {
		return map[string]interface{}{
			"stdout":    "first_output",
			"exit_code": 0,
		}, nil
	}
	return map[string]interface{}{
		"stdout":    "second_output",
		"exit_code": 0,
	}, nil
}

func TestEngine_DriverError(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, false)

	driver := &mockDriver{
		name: "shell",
		err:  fmt.Errorf("connection refused"),
	}
	eng.RegisterDriver("shell", driver)

	flow := &config.FlowConfig{
		Name: "Test Flow",
		Steps: []config.Step{
			{
				Name:   "Error Step",
				Script: &config.ScriptConfig{Run: "echo fail"},
			},
		},
	}

	env := &config.EnvConfig{}
	result := eng.Run(context.Background(), flow, env)

	if result.Errored != 1 {
		t.Errorf("Errored = %d, want 1", result.Errored)
	}
	if result.Steps[0].Error != "connection refused" {
		t.Errorf("Error = %q, want 'connection refused'", result.Steps[0].Error)
	}
}

func TestEngine_InitialVariables(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, false)

	driver := &mockDriver{
		name:   "shell",
		result: map[string]interface{}{"stdout": "ok", "exit_code": 0},
	}
	eng.RegisterDriver("shell", driver)

	flow := &config.FlowConfig{
		Name: "Test Flow",
		Steps: []config.Step{
			{
				Name:   "Check Var",
				Script: &config.ScriptConfig{Run: "echo"},
				Assert: []config.AssertConfig{
					{Expr: "myvar == 'injected'"},
				},
			},
		},
	}

	env := &config.EnvConfig{}
	initVars := map[string]interface{}{"myvar": "injected"}
	result := eng.Run(context.Background(), flow, env, initVars)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
}

func TestEngine_DBStep(t *testing.T) {
	eng := NewEngine(&nullPrinter{}, false, false)

	driver := &mockDriver{
		name: "mydb",
		result: map[string]interface{}{
			"rows":      []interface{}{map[string]interface{}{"id": 1, "name": "test"}},
			"row_count": 1,
		},
	}
	eng.RegisterDriver("mydb", driver)

	flow := &config.FlowConfig{
		Name: "DB Test",
		Steps: []config.Step{
			{
				Name:   "Query DB",
				DBStep: &config.DBStepConfig{Database: "mydb", Query: "SELECT * FROM users"},
				Assert: []config.AssertConfig{
					{Expr: "row_count == 1"},
				},
			},
		},
	}

	env := &config.EnvConfig{}
	result := eng.Run(context.Background(), flow, env)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
		for _, s := range result.Steps {
			t.Logf("  %s: %s (err: %s)", s.Name, s.Status, s.Error)
		}
	}
	if driver.called != 1 {
		t.Errorf("driver.called = %d, want 1", driver.called)
	}
}
