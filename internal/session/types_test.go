package session

import "testing"

func TestDefaultPreferences(t *testing.T) {
	p := DefaultPreferences()
	if p.DefaultTimeRange != "1h" {
		t.Errorf("DefaultTimeRange = %q, want 1h", p.DefaultTimeRange)
	}
	if p.SidebarCollapsed {
		t.Error("SidebarCollapsed should default to false")
	}
	if p.PinnedCompositions == nil {
		t.Error("PinnedCompositions must be an empty slice, not nil (it marshals to [] not null)")
	}
	if len(p.PinnedCompositions) != 0 {
		t.Errorf("PinnedCompositions = %v, want empty", p.PinnedCompositions)
	}
}

func TestValidateAcceptsEveryTimeRange(t *testing.T) {
	for _, tr := range []string{"1h", "6h", "24h"} {
		p := Preferences{PinnedCompositions: []string{}, DefaultTimeRange: tr}
		if err := p.Validate(); err != nil {
			t.Errorf("Validate() with range %q returned %v, want nil", tr, err)
		}
	}
}

func TestValidateRejectsBadTimeRange(t *testing.T) {
	for _, tr := range []string{"", "7d", "1H", "60m"} {
		p := Preferences{PinnedCompositions: []string{}, DefaultTimeRange: tr}
		if err := p.Validate(); err == nil {
			t.Errorf("Validate() with range %q returned nil, want error", tr)
		}
	}
}

func TestValidateRejectsTooManyPins(t *testing.T) {
	pins := make([]string, 51)
	for i := range pins {
		pins[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	p := Preferences{PinnedCompositions: pins, DefaultTimeRange: "1h"}
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() with 51 pins returned nil, want error")
	}
}

func TestValidateRejectsEmptyAndOverlongPins(t *testing.T) {
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'x'
	}
	cases := map[string][]string{
		"empty pin":    {""},
		"overlong pin": {string(long)},
	}
	for name, pins := range cases {
		p := Preferences{PinnedCompositions: pins, DefaultTimeRange: "1h"}
		if err := p.Validate(); err == nil {
			t.Errorf("%s: Validate() returned nil, want error", name)
		}
	}
}

func TestValidateRejectsDuplicatePins(t *testing.T) {
	p := Preferences{PinnedCompositions: []string{"a", "a"}, DefaultTimeRange: "1h"}
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() with duplicate pins returned nil, want error")
	}
}

func TestValidateNormalisesNilPins(t *testing.T) {
	p := Preferences{PinnedCompositions: nil, DefaultTimeRange: "1h"}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() returned %v, want nil", err)
	}
	if p.PinnedCompositions == nil {
		t.Error("Validate() must normalise nil pins to an empty slice")
	}
}

func TestValidationErrorNamesFields(t *testing.T) {
	p := Preferences{PinnedCompositions: []string{""}, DefaultTimeRange: "nope"}
	err := p.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil, want error")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Validate() returned %T, want *ValidationError", err)
	}
	if len(ve.Errors) != 2 {
		t.Errorf("got %d field errors, want 2: %+v", len(ve.Errors), ve.Errors)
	}
}
