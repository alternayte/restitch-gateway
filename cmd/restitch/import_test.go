package main

import (
	"os"
	"strings"
	"testing"
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
}
