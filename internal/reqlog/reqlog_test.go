// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reqlog

import (
	"encoding/json"
	"testing"
)

func TestStepRecord_JSON(t *testing.T) {
	sr := StepRecord{
		Name:          "user",
		Status:        "success",
		Wave:          1,
		DurationMS:    42.5,
		HTTPStatus:    200,
		Upstream:      "api",
		URL:           "http://localhost:8081/users/1",
		StartOffsetMS: 0.3,
		BodySize:      256,
		Error:         "",
		Cached:        false,
		Retries:       0,
	}
	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded StepRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Upstream != "api" {
		t.Errorf("Upstream = %q, want %q", decoded.Upstream, "api")
	}
	if decoded.URL != "http://localhost:8081/users/1" {
		t.Errorf("URL = %q, want %q", decoded.URL, "http://localhost:8081/users/1")
	}
	if decoded.StartOffsetMS != 0.3 {
		t.Errorf("StartOffsetMS = %f, want 0.3", decoded.StartOffsetMS)
	}
	if decoded.BodySize != 256 {
		t.Errorf("BodySize = %d, want 256", decoded.BodySize)
	}
}

func TestStepRecord_ErrorOmitted(t *testing.T) {
	sr := StepRecord{Name: "x", Status: "success"}
	data, _ := json.Marshal(sr)
	if string(data) != `{"name":"x","status":"success","wave":0,"duration_ms":0,"http_status":0,"upstream":"","url":"","start_offset_ms":0,"body_size":0,"cached":false,"retries":0}` {
		// Just check error is not present (omitempty)
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if _, hasErr := m["error"]; hasErr {
			t.Error("empty error should be omitted from JSON")
		}
	}
}
