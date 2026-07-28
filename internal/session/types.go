// Package session provides cookie-identified, login-free browser sessions
// for Studio and the per-browser UI preferences attached to them.
package session

import (
	"fmt"
	"strings"
)

const (
	// maxPinnedCompositions caps how many compositions one browser may pin.
	maxPinnedCompositions = 50
	// maxPinnedNameLen caps the length of a single pinned composition name.
	maxPinnedNameLen = 128
)

// validTimeRanges mirrors the TimeRange union in
// studio/src/components/charts/TimeRangeSelector.tsx. If that union gains a
// member, this map must be updated in the same change or the new range 400s.
var validTimeRanges = map[string]bool{"1h": true, "6h": true, "24h": true}

// Preferences is the per-browser UI state Studio persists.
type Preferences struct {
	PinnedCompositions []string `json:"pinned_compositions"`
	SidebarCollapsed   bool     `json:"sidebar_collapsed"`
	DefaultTimeRange   string   `json:"default_time_range"`
}

// FieldError describes one validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError aggregates per-field validation failures.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Errors))
	for _, fe := range e.Errors {
		parts = append(parts, fe.Field+": "+fe.Message)
	}
	return "invalid preferences: " + strings.Join(parts, "; ")
}

// DefaultPreferences returns the preferences a browser starts with. The
// pinned slice is non-nil so it marshals to [] rather than null.
func DefaultPreferences() Preferences {
	return Preferences{
		PinnedCompositions: []string{},
		SidebarCollapsed:   false,
		DefaultTimeRange:   "1h",
	}
}

// Validate checks p field by field and normalises a nil pinned slice to an
// empty one. It returns a *ValidationError listing every problem found.
func (p *Preferences) Validate() error {
	var errs []FieldError

	if p.PinnedCompositions == nil {
		p.PinnedCompositions = []string{}
	}

	if len(p.PinnedCompositions) > maxPinnedCompositions {
		errs = append(errs, FieldError{
			Field:   "pinned_compositions",
			Message: fmt.Sprintf("at most %d entries allowed, got %d", maxPinnedCompositions, len(p.PinnedCompositions)),
		})
	}

	seen := make(map[string]bool, len(p.PinnedCompositions))
	for i, name := range p.PinnedCompositions {
		switch {
		case name == "":
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("pinned_compositions[%d]", i),
				Message: "must not be empty",
			})
		case len(name) > maxPinnedNameLen:
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("pinned_compositions[%d]", i),
				Message: fmt.Sprintf("must be at most %d characters", maxPinnedNameLen),
			})
		case seen[name]:
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("pinned_compositions[%d]", i),
				Message: "duplicate entry " + name,
			})
		}
		seen[name] = true
	}

	if !validTimeRanges[p.DefaultTimeRange] {
		errs = append(errs, FieldError{
			Field:   "default_time_range",
			Message: "must be one of 1h, 6h, 24h",
		})
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}
