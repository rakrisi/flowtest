package engine

import (
	"testing"
	"time"
)

func TestFlowResult_Tally(t *testing.T) {
	r := &FlowResult{
		Name: "test",
		Steps: []StepResult{
			{Name: "s1", Status: StatusPassed},
			{Name: "s2", Status: StatusPassed},
			{Name: "s3", Status: StatusFailed},
			{Name: "s4", Status: StatusSkipped},
			{Name: "s5", Status: StatusErrored},
		},
		Duration: 5 * time.Second,
	}

	r.Tally()

	if r.Passed != 2 {
		t.Errorf("Passed = %d, want 2", r.Passed)
	}
	if r.Failed != 1 {
		t.Errorf("Failed = %d, want 1", r.Failed)
	}
	if r.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", r.Skipped)
	}
	if r.Errored != 1 {
		t.Errorf("Errored = %d, want 1", r.Errored)
	}
}

func TestFlowResult_Success(t *testing.T) {
	tests := []struct {
		name   string
		steps  []StepResult
		want   bool
	}{
		{
			name: "all passed",
			steps: []StepResult{
				{Status: StatusPassed},
				{Status: StatusPassed},
			},
			want: true,
		},
		{
			name: "passed and skipped",
			steps: []StepResult{
				{Status: StatusPassed},
				{Status: StatusSkipped},
			},
			want: true,
		},
		{
			name: "one failed",
			steps: []StepResult{
				{Status: StatusPassed},
				{Status: StatusFailed},
			},
			want: false,
		},
		{
			name: "one errored",
			steps: []StepResult{
				{Status: StatusPassed},
				{Status: StatusErrored},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &FlowResult{Steps: tt.steps}
			r.Tally()
			if got := r.Success(); got != tt.want {
				t.Errorf("Success() = %v, want %v", got, tt.want)
			}
		})
	}
}
