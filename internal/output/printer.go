package output

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/rakrisi/flowtest/internal/engine"
)

var (
	green  = color.New(color.FgGreen).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	cyan   = color.New(color.FgCyan).SprintFunc()
	dim    = color.New(color.Faint).SprintFunc()
	bold   = color.New(color.Bold).SprintFunc()
)

// TerminalPrinter outputs step results to the terminal with colors.
type TerminalPrinter struct{}

func NewTerminalPrinter() *TerminalPrinter {
	return &TerminalPrinter{}
}

func (p *TerminalPrinter) FlowHeader(name string) {
	fmt.Printf("\n%s %s\n", bold("▸"), bold(name))
}

func (p *TerminalPrinter) SectionHeader(name string) {
	fmt.Printf("\n  %s\n", cyan(name))
}

func (p *TerminalPrinter) StepStart(name string, driverType string, stepNum int, totalSteps int) {
	// No output on start — we show result inline
}

func (p *TerminalPrinter) StepResult(result *engine.StepResult, stepNum int, totalSteps int, verbose bool) {
	prefix := fmt.Sprintf("[%d/%d]", stepNum, totalSteps)

	retryTag := ""
	if result.Retries > 0 {
		retryTag = dim(fmt.Sprintf(" (retried %dx)", result.Retries))
	}

	switch result.Status {
	case engine.StatusPassed:
		fmt.Printf("  %s %s %s %s%s\n", green("✓"), dim(prefix), result.Name, dim(result.Duration.Round(1e6).String()), retryTag)
	case engine.StatusFailed:
		fmt.Printf("  %s %s %s %s%s\n", red("✗"), dim(prefix), result.Name, dim(result.Duration.Round(1e6).String()), retryTag)
		for _, a := range result.Assertions {
			if !a.Passed {
				if a.Error != "" {
					fmt.Printf("      %s %s\n", red("→"), a.Error)
				} else {
					msg := a.Expression
					if a.Message != "" {
						msg = a.Message + " — " + a.Expression
					}
					fmt.Printf("      %s assertion failed: %s\n", red("→"), msg)
					if a.Actual != "" {
						fmt.Printf("      %s %s\n", red("→"), dim(a.Actual))
					}
				}
			}
		}
	case engine.StatusSkipped:
		fmt.Printf("  %s %s %s %s\n", yellow("−"), dim(prefix), result.Name, dim(result.SkipReason))
	case engine.StatusErrored:
		fmt.Printf("  %s %s %s %s%s\n", red("!"), dim(prefix), result.Name, dim(result.Duration.Round(1e6).String()), retryTag)
		fmt.Printf("      %s %s\n", red("→"), result.Error)
	}

	// Verbose: show driver details
	if verbose {
		p.printVerboseDetail(result)
	}
}

func (p *TerminalPrinter) printVerboseDetail(result *engine.StepResult) {
	if result.Status == engine.StatusSkipped && result.Detail == nil {
		return
	}

	d := result.Detail
	if d == nil {
		fmt.Printf("      %s driver=%s\n", dim("│"), result.Driver)
		return
	}

	switch result.Driver {
	case "http":
		if d.Method != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("request:"), dim(d.Method+" "+d.URL))
		}
		if d.RequestBody != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("body:"), dim(d.RequestBody))
		}
		if d.StatusCode > 0 {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("response:"), dim(fmt.Sprintf("%d", d.StatusCode)))
		}
		if d.ResponseBody != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("body:"), dim(d.ResponseBody))
		}
	case "db":
		if d.Query != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("query:"), dim(d.Query))
		}
		if d.Params != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("params:"), dim(d.Params))
		}
	case "kafka":
		fmt.Printf("      %s %s %s %s\n", dim("│"), dim("topic:"), dim(d.Topic), dim("("+d.Action+")"))
		if d.Match != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("match:"), dim(d.Match))
		}
	case "redis":
		fmt.Printf("      %s %s %s %s\n", dim("│"), dim("action:"), dim(d.RedisAction), dim(d.Key))
	case "shell":
		if d.Command != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("cmd:"), dim(d.Command))
		}
		if d.Stdout != "" {
			fmt.Printf("      %s %s %s\n", dim("│"), dim("stdout:"), dim(d.Stdout))
		}
	}

	// Show assertions in verbose
	if result.Status != engine.StatusErrored {
		for _, a := range result.Assertions {
			if a.Passed {
				fmt.Printf("      %s %s %s\n", dim("│"), green("✓"), dim(a.Expression))
			}
		}
	}

	// Show saved variables
	if d.Saved != nil {
		for k, v := range d.Saved {
			fmt.Printf("      %s %s %s = %s\n", dim("│"), dim("saved"), dim(k), dim(v))
		}
	}
}

func (p *TerminalPrinter) SetupStart(description string) {}

func (p *TerminalPrinter) SetupResult(description string, err error) {
	if err != nil {
		fmt.Printf("  %s setup: %s — %s\n", red("!"), description, err)
	} else {
		fmt.Printf("  %s setup: %s\n", dim("·"), description)
	}
}

func (p *TerminalPrinter) CleanupStart(description string) {}

func (p *TerminalPrinter) CleanupResult(description string, err error) {
	if err != nil {
		fmt.Printf("  %s cleanup: %s — %s\n", red("!"), description, err)
	} else {
		fmt.Printf("  %s cleanup: %s\n", dim("·"), description)
	}
}

// NullPrinter discards all output (used for JSON mode).
type NullPrinter struct{}

func (p *NullPrinter) FlowHeader(string)                             {}
func (p *NullPrinter) SectionHeader(string)                          {}
func (p *NullPrinter) StepStart(string, string, int, int)            {}
func (p *NullPrinter) StepResult(*engine.StepResult, int, int, bool) {}
func (p *NullPrinter) SetupStart(string)                             {}
func (p *NullPrinter) SetupResult(string, error)                     {}
func (p *NullPrinter) CleanupStart(string)                           {}
func (p *NullPrinter) CleanupResult(string, error)                   {}

// FormatDuration formats a duration for display.
func FormatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}

// Repeat returns a string repeated n times.
func Repeat(s string, n int) string {
	return strings.Repeat(s, n)
}
