package engine

import "time"

// StepStatus represents the outcome of a step execution.
type StepStatus string

const (
	StatusPassed  StepStatus = "passed"
	StatusFailed  StepStatus = "failed"
	StatusSkipped StepStatus = "skipped"
	StatusErrored StepStatus = "errored"
)

// StepResult captures the outcome of a single step.
type StepResult struct {
	Name       string            `json:"name"`
	Status     StepStatus        `json:"status"`
	Duration   time.Duration     `json:"duration"`
	Driver     string            `json:"driver"`
	Assertions []AssertionResult `json:"assertions,omitempty"`
	Error      string            `json:"error,omitempty"`
	SkipReason string            `json:"skip_reason,omitempty"`
	Retries    int               `json:"retries,omitempty"`
	Detail     *StepDetail       `json:"detail,omitempty"` // populated in verbose mode
}

// StepDetail holds driver-specific info for verbose output.
type StepDetail struct {
	// HTTP
	Method       string `json:"method,omitempty"`
	URL          string `json:"url,omitempty"`
	RequestBody  string `json:"request_body,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`
	// DB
	Query  string `json:"query,omitempty"`
	Params string `json:"params,omitempty"`
	// Kafka
	Topic  string `json:"topic,omitempty"`
	Action string `json:"action,omitempty"`
	Match  string `json:"match,omitempty"`
	// Redis
	RedisAction string `json:"redis_action,omitempty"`
	Key         string `json:"key,omitempty"`
	// Shell
	Command string `json:"command,omitempty"`
	Stdout  string `json:"stdout,omitempty"`
	// Saved variables
	Saved map[string]string `json:"saved,omitempty"`
}

// AssertionResult captures the outcome of a single assertion.
type AssertionResult struct {
	Expression string `json:"expression"`
	Passed     bool   `json:"passed"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
	Actual     string `json:"actual,omitempty"` // what the expression evaluated to
}

// FlowResult captures the outcome of an entire flow execution.
type FlowResult struct {
	Name     string        `json:"name"`
	Duration time.Duration `json:"duration"`
	Steps    []StepResult  `json:"steps"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Skipped  int           `json:"skipped"`
	Errored  int           `json:"errored"`
}

// Success returns true if all steps passed or were skipped.
func (r *FlowResult) Success() bool {
	return r.Failed == 0 && r.Errored == 0
}

// Tally counts results by status.
func (r *FlowResult) Tally() {
	r.Passed = 0
	r.Failed = 0
	r.Skipped = 0
	r.Errored = 0
	for _, s := range r.Steps {
		switch s.Status {
		case StatusPassed:
			r.Passed++
		case StatusFailed:
			r.Failed++
		case StatusSkipped:
			r.Skipped++
		case StatusErrored:
			r.Errored++
		}
	}
}
