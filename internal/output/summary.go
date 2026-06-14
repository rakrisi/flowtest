package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/radhe-singh/flowtest/internal/engine"
)

// PrintSummary prints the final pass/fail/skip count line.
func PrintSummary(result *engine.FlowResult) {
	fmt.Println()

	parts := []string{}

	if result.Passed > 0 {
		parts = append(parts, green(fmt.Sprintf("%d passed", result.Passed)))
	}
	if result.Failed > 0 {
		parts = append(parts, red(fmt.Sprintf("%d failed", result.Failed)))
	}
	if result.Errored > 0 {
		parts = append(parts, red(fmt.Sprintf("%d errored", result.Errored)))
	}
	if result.Skipped > 0 {
		parts = append(parts, yellow(fmt.Sprintf("%d skipped", result.Skipped)))
	}

	duration := FormatDuration(result.Duration.Milliseconds())

	fmt.Printf("  %s", parts[0])
	for _, p := range parts[1:] {
		fmt.Printf("  %s", p)
	}
	fmt.Printf("  %s\n\n", dim(fmt.Sprintf("(%s)", duration)))
}

// PrintJSONResult outputs the flow result as JSON.
func PrintJSONResult(result *engine.FlowResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
