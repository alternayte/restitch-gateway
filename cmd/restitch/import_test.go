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

package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
)

func TestImportOpenAPI_Golden(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := importCmd([]string{"openapi", "testdata/petstore.yaml", "--upstream", "pets"})

	w.Close()
	os.Stdout = oldStdout

	if code != 0 {
		t.Fatalf("importCmd returned %d, want 0", code)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	got := string(buf[:n])

	expected, err := os.ReadFile("testdata/petstore_expected.yaml")
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}

	gotTrimmed := strings.TrimSpace(got)
	expectedTrimmed := strings.TrimSpace(string(expected))

	if gotTrimmed != expectedTrimmed {
		t.Errorf("output mismatch.\n--- got ---\n%s\n--- expected ---\n%s", gotTrimmed, expectedTrimmed)
	}

	// The scaffold must load as it is, once its key variable is set.
	t.Setenv("GATEWAY_API_KEY", "k")
	expanded, err := gwconfig.ExpandEnvStrict(got)
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.Validate(context.Background(), []byte(expanded)); err != nil {
		t.Errorf("the generated scaffold does not validate: %v", err)
	}
}
