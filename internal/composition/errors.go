package composition

import (
	"context"
	"errors"
)

// StepErrorDetail represents a single step failure for the _errors array.
// Per CONTEXT.md: contains step name + error message (not status codes or timing).
type StepErrorDetail struct {
	Step    string `json:"step"`
	Message string `json:"message"`
}

// stepError is an internal type for collecting errors during execution.
type stepError struct {
	stepName string
	err      error
	optional bool // Whether the step was optional
}

// SanitizeErrorMessage converts internal errors to user-friendly messages.
// Per CONTEXT.md: timeout appears as "timeout", other errors as "upstream error".
// Per RESEARCH.md Pitfall 5: Don't expose internal stack traces.
func SanitizeErrorMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	// Don't expose internal details
	return "upstream error"
}

// BuildErrorsArray converts internal step errors to StepErrorDetail array.
func BuildErrorsArray(stepErrors []stepError) []StepErrorDetail {
	if len(stepErrors) == 0 {
		return nil
	}

	details := make([]StepErrorDetail, len(stepErrors))
	for i, se := range stepErrors {
		details[i] = StepErrorDetail{
			Step:    se.stepName,
			Message: SanitizeErrorMessage(se.err),
		}
	}
	return details
}

// HasRequiredFailure checks if any non-optional step failed.
func HasRequiredFailure(stepErrors []stepError) bool {
	for _, se := range stepErrors {
		if !se.optional {
			return true
		}
	}
	return false
}
