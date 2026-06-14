package driver

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/radhe-singh/flowtest/internal/config"
	"github.com/radhe-singh/flowtest/internal/engine"
)

// cappedBuffer is a writer that stops accepting data beyond its limit,
// preventing unbounded memory usage from verbose commands.
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len() >= b.limit {
		return len(p), nil // discard silently so command doesn't fail
	}
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) String() string {
	return b.buf.String()
}

// ShellDriver executes shell commands.
type ShellDriver struct{}

func (d *ShellDriver) Name() string { return "shell" }

func (d *ShellDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *engine.Context, env *config.EnvConfig) (map[string]interface{}, error) {
	script, ok := stepConfig.(*config.ScriptConfig)
	if !ok {
		return nil, fmt.Errorf("shell driver: invalid step config type %T", stepConfig)
	}

	// Determine shell
	shell := "/bin/sh"
	if script.Lang == "bash" {
		shell = "/bin/bash"
	}

	// Set timeout
	timeout := 30 * time.Second
	if script.Timeout > 0 {
		timeout = script.Timeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, shell, "-c", script.Run)

	// Inject context variables as environment variables
	vars := flowCtx.All()
	for k, v := range vars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("FLOWTEST_%s=%v", strings.ToUpper(k), v))
	}

	stdout := &cappedBuffer{limit: 1 << 20} // 1MB cap
	stderr := &cappedBuffer{limit: 1 << 20} // 1MB cap
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("shell driver: execution failed: %w", err)
		}
	}

	result := map[string]interface{}{
		"stdout":    strings.TrimSpace(stdout.String()),
		"stderr":    strings.TrimSpace(stderr.String()),
		"exit_code": exitCode,
	}

	// Check assert_exit if specified
	if script.AssertExit != nil && exitCode != *script.AssertExit {
		return result, fmt.Errorf("shell driver: expected exit code %d, got %d\nstderr: %s", *script.AssertExit, exitCode, stderr.String())
	}

	return result, nil
}
