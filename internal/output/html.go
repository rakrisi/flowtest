package output

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"time"

	"github.com/radhe-singh/flowtest/internal/engine"
)

// HTMLReportConfig configures HTML report generation.
type HTMLReportConfig struct {
	OutputPath string
	Title      string
	Subtitle   string
}

// GenerateHTMLReport creates a comprehensive HTML report with Tailwind CSS styling.
func GenerateHTMLReport(result *engine.FlowResult, config HTMLReportConfig) error {
	if config.Title == "" {
		config.Title = "FlowTest Report"
	}
	if config.Subtitle == "" {
		config.Subtitle = result.Name
	}

	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"formatDuration": func(d time.Duration) string {
			return FormatDuration(d.Milliseconds())
		},
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05 MST")
		},
		"statusColor": func(status engine.StepStatus) string {
			switch status {
			case engine.StatusPassed:
				return "bg-green-100 text-green-800"
			case engine.StatusFailed:
				return "bg-red-100 text-red-800"
			case engine.StatusSkipped:
				return "bg-yellow-100 text-yellow-800"
			case engine.StatusErrored:
				return "bg-red-200 text-red-900"
			default:
				return "bg-gray-100 text-gray-800"
			}
		},
		"statusIcon": func(status engine.StepStatus) string {
			switch status {
			case engine.StatusPassed:
				return "✓"
			case engine.StatusFailed:
				return "✗"
			case engine.StatusSkipped:
				return "−"
			case engine.StatusErrored:
				return "!"
			default:
				return "?"
			}
		},
		"toJSON": func(v interface{}) string {
			b, _ := json.MarshalIndent(v, "", "  ")
			return string(b)
		},
		"add": func(a, b int) int {
			return a + b
		},
	}).Parse(htmlTemplate))

	f, err := os.Create(config.OutputPath)
	if err != nil {
		return fmt.Errorf("creating HTML report file: %w", err)
	}
	defer f.Close()

	data := map[string]interface{}{
		"Title":       config.Title,
		"Subtitle":    config.Subtitle,
		"Result":      result,
		"GeneratedAt": time.Now(),
		"TotalSteps":  len(result.Steps),
		"PassRate":    float64(result.Passed) / float64(len(result.Steps)) * 100,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("executing HTML template: %w", err)
	}

	return nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Title }} - {{ .Subtitle }}</title>
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-50">
    <div class="min-h-screen py-8 px-4">
        <div class="max-w-7xl mx-auto">
            <!-- Header -->
            <div class="bg-white rounded-lg shadow-sm p-6 mb-6">
                <div class="flex items-center justify-between">
                    <div>
                        <h1 class="text-3xl font-bold text-gray-900">{{ .Title }}</h1>
                        <p class="text-xl text-gray-600 mt-1">{{ .Subtitle }}</p>
                    </div>
                    <div class="text-right">
                        <div class="text-sm text-gray-500">Generated</div>
                        <div class="text-lg font-medium text-gray-900">{{ formatTime .GeneratedAt }}</div>
                    </div>
                </div>
            </div>

            <!-- Summary Cards -->
            <div class="grid grid-cols-1 md:grid-cols-5 gap-4 mb-6">
                <div class="bg-white rounded-lg shadow-sm p-6">
                    <div class="text-sm font-medium text-gray-500 uppercase">Total Steps</div>
                    <div class="mt-2 text-3xl font-bold text-gray-900">{{ .TotalSteps }}</div>
                </div>
                <div class="bg-white rounded-lg shadow-sm p-6">
                    <div class="text-sm font-medium text-green-600 uppercase">Passed</div>
                    <div class="mt-2 text-3xl font-bold text-green-600">{{ .Result.Passed }}</div>
                </div>
                <div class="bg-white rounded-lg shadow-sm p-6">
                    <div class="text-sm font-medium text-red-600 uppercase">Failed</div>
                    <div class="mt-2 text-3xl font-bold text-red-600">{{ .Result.Failed }}</div>
                </div>
                <div class="bg-white rounded-lg shadow-sm p-6">
                    <div class="text-sm font-medium text-yellow-600 uppercase">Skipped</div>
                    <div class="mt-2 text-3xl font-bold text-yellow-600">{{ .Result.Skipped }}</div>
                </div>
                <div class="bg-white rounded-lg shadow-sm p-6">
                    <div class="text-sm font-medium text-gray-500 uppercase">Duration</div>
                    <div class="mt-2 text-2xl font-bold text-gray-900">{{ formatDuration .Result.Duration }}</div>
                </div>
            </div>

            <!-- Pass Rate -->
            <div class="bg-white rounded-lg shadow-sm p-6 mb-6">
                <div class="flex items-center justify-between mb-3">
                    <span class="text-sm font-medium text-gray-700">Pass Rate</span>
                    <span class="text-sm font-bold {{ if ge .PassRate 80.0 }}text-green-600{{ else if ge .PassRate 50.0 }}text-yellow-600{{ else }}text-red-600{{ end }}">
                        {{ printf "%.1f" .PassRate }}%
                    </span>
                </div>
                <div class="w-full bg-gray-200 rounded-full h-4">
                    <div class="{{ if ge .PassRate 80.0 }}bg-green-500{{ else if ge .PassRate 50.0 }}bg-yellow-500{{ else }}bg-red-500{{ end }} h-4 rounded-full transition-all duration-300" 
                         style="width: {{ printf "%.1f" .PassRate }}%"></div>
                </div>
            </div>

            <!-- Steps -->
            <div class="bg-white rounded-lg shadow-sm p-6">
                <h2 class="text-2xl font-bold text-gray-900 mb-6">Test Steps</h2>
                
                <div class="space-y-4">
                    {{ range $index, $step := .Result.Steps }}
                    <details class="border border-gray-200 rounded-lg overflow-hidden">
                        <summary class="cursor-pointer bg-gray-50 hover:bg-gray-100 p-4 list-none">
                            <div class="flex items-center justify-between">
                                <div class="flex items-center space-x-4 flex-1">
                                    <span class="flex items-center justify-center w-10 h-10 rounded-full {{ statusColor $step.Status }}">
                                        <span class="text-lg font-bold">{{ statusIcon $step.Status }}</span>
                                    </span>
                                    <div class="flex-1">
                                        <div class="flex items-center space-x-3">
                                            <span class="text-sm text-gray-500">[{{ add $index 1 }}/{{ $.TotalSteps }}]</span>
                                            <span class="font-semibold text-gray-900">{{ $step.Name }}</span>
                                            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
                                                {{ $step.Driver }}
                                            </span>
                                        </div>
                                        {{ if $step.Error }}
                                        <div class="mt-1 text-sm text-red-600">{{ $step.Error }}</div>
                                        {{ end }}
                                        {{ if $step.SkipReason }}
                                        <div class="mt-1 text-sm text-yellow-600">{{ $step.SkipReason }}</div>
                                        {{ end }}
                                    </div>
                                </div>
                                <div class="flex items-center space-x-4">
                                    <span class="text-sm text-gray-500">{{ formatDuration $step.Duration }}</span>
                                    {{ if gt $step.Retries 0 }}
                                    <span class="text-xs text-gray-400">({{ $step.Retries }} retries)</span>
                                    {{ end }}
                                    <svg class="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
                                    </svg>
                                </div>
                            </div>
                        </summary>

                        <div class="border-t border-gray-200 bg-white p-4 space-y-4">
                            <!-- Assertions -->
                            {{ if $step.Assertions }}
                            <div>
                                <h4 class="font-semibold text-gray-700 mb-2">Assertions</h4>
                                <div class="space-y-2">
                                    {{ range $step.Assertions }}
                                    <div class="flex items-start space-x-2 text-sm">
                                        <span class="{{ if .Passed }}text-green-600{{ else }}text-red-600{{ end }}">
                                            {{ if .Passed }}✓{{ else }}✗{{ end }}
                                        </span>
                                        <div class="flex-1">
                                            <code class="text-xs bg-gray-100 px-2 py-1 rounded">{{ .Expression }}</code>
                                            {{ if .Message }}
                                            <span class="text-gray-600">— {{ .Message }}</span>
                                            {{ end }}
                                            {{ if and (not .Passed) .Actual }}
                                            <div class="mt-1 text-red-600">{{ .Actual }}</div>
                                            {{ end }}
                                            {{ if .Error }}
                                            <div class="mt-1 text-red-600">{{ .Error }}</div>
                                            {{ end }}
                                        </div>
                                    </div>
                                    {{ end }}
                                </div>
                            </div>
                            {{ end }}

                            <!-- Driver Details -->
                            {{ if $step.Detail }}
                            <div>
                                <h4 class="font-semibold text-gray-700 mb-2">Details</h4>
                                <div class="bg-gray-50 rounded p-3 space-y-2 text-sm">
                                    {{ if $step.Detail.Method }}
                                    <div><span class="text-gray-600">Request:</span> <code class="text-xs bg-white px-2 py-1 rounded">{{ $step.Detail.Method }} {{ $step.Detail.URL }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.RequestBody }}
                                    <div><span class="text-gray-600">Body:</span> <pre class="text-xs bg-white px-2 py-1 rounded block mt-1 overflow-x-auto">{{ $step.Detail.RequestBody }}</pre></div>
                                    {{ end }}
                                    {{ if $step.Detail.StatusCode }}
                                    <div><span class="text-gray-600">Status:</span> <span class="font-mono">{{ $step.Detail.StatusCode }}</span></div>
                                    {{ end }}
                                    {{ if $step.Detail.ResponseBody }}
                                    <div><span class="text-gray-600">Response:</span> <pre class="text-xs bg-white px-2 py-1 rounded block mt-1 overflow-x-auto max-h-64">{{ $step.Detail.ResponseBody }}</pre></div>
                                    {{ end }}
                                    {{ if $step.Detail.Query }}
                                    <div><span class="text-gray-600">Query:</span> <code class="text-xs bg-white px-2 py-1 rounded block mt-1 overflow-x-auto">{{ $step.Detail.Query }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.Params }}
                                    <div><span class="text-gray-600">Params:</span> <code class="text-xs bg-white px-2 py-1 rounded">{{ $step.Detail.Params }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.Topic }}
                                    <div><span class="text-gray-600">Kafka Topic:</span> <code class="text-xs">{{ $step.Detail.Topic }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.Action }}
                                    <div><span class="text-gray-600">Action:</span> <code class="text-xs">{{ $step.Detail.Action }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.RedisAction }}
                                    <div><span class="text-gray-600">Redis Action:</span> <code class="text-xs">{{ $step.Detail.RedisAction }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.Key }}
                                    <div><span class="text-gray-600">Key:</span> <code class="text-xs">{{ $step.Detail.Key }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.Command }}
                                    <div><span class="text-gray-600">Command:</span> <code class="text-xs bg-white px-2 py-1 rounded">{{ $step.Detail.Command }}</code></div>
                                    {{ end }}
                                    {{ if $step.Detail.Stdout }}
                                    <div><span class="text-gray-600">Stdout:</span> <pre class="text-xs bg-white px-2 py-1 rounded block mt-1 overflow-x-auto">{{ $step.Detail.Stdout }}</pre></div>
                                    {{ end }}
                                </div>
                            </div>
                            {{ end }}

                            <!-- Saved Variables -->
                            {{ if and $step.Detail $step.Detail.Saved }}
                            <div>
                                <h4 class="font-semibold text-gray-700 mb-2">Saved Variables</h4>
                                <div class="bg-gray-50 rounded p-3 space-y-1 text-sm font-mono">
                                    {{ range $key, $value := $step.Detail.Saved }}
                                    <div><span class="text-gray-600">{{ $key }}:</span> <span class="text-gray-900">{{ $value }}</span></div>
                                    {{ end }}
                                </div>
                            </div>
                            {{ end }}
                        </div>
                    </details>
                    {{ end }}
                </div>
            </div>

            <!-- Footer -->
            <div class="mt-8 text-center text-sm text-gray-500">
                <p>Generated by <a href="https://github.com/radhe-singh/flowtest" class="text-blue-600 hover:text-blue-800">FlowTest</a></p>
            </div>
        </div>
    </div>
</body>
</html>
`
