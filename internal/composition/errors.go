package composition

import (
	"context"
	"errors"
	"fmt"
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

// ErrRuleMatched is returned when an error rule matched a status code.
// This is not a true error - the step succeeded but with a replaced body.
var ErrRuleMatched = fmt.Errorf("error rule matched")

// NewErrorRuleMatchedError creates an error indicating which status was matched.
func NewErrorRuleMatchedError(statusCode int) error {
	return fmt.Errorf("error rule matched status %d: %w", statusCode, ErrRuleMatched)
}

// IsErrorRuleMatched checks if an error is from error rule matching.
func IsErrorRuleMatched(err error) bool {
	return errors.Is(err, ErrRuleMatched)
}

// SanitizeErrorMessage converts internal errors to user-friendly messages.
// Per CONTEXT.md: timeout appears as "timeout", other errors as "upstream error".
// Per RESEARCH.md Pitfall 5: Don't expose internal stack traces.
func SanitizeErrorMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, ErrRuleMatched) {
		return "error rule matched"
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
