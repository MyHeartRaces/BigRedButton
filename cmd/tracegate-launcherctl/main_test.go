package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateProfileCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"validate-profile", "../../testdata/profiles/valid-v7.json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "profile valid") {
		t.Fatalf("expected success output, got: %s", out)
	}
	if strings.Contains(out, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") {
		t.Fatalf("validate output leaked private key: %s", out)
	}
}

func TestValidateProfileCommandJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"validate-profile", "-json", "../../testdata/profiles/valid-v7.json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"valid": true`) {
		t.Fatalf("expected JSON success output, got: %s", out)
	}
	if !strings.Contains(out, `"fingerprint"`) {
		t.Fatalf("expected JSON summary fingerprint, got: %s", out)
	}
}

func TestValidateProfileCommandFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"validate-profile", "../../testdata/profiles/invalid-placeholder-v7.json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "profile invalid") {
		t.Fatalf("expected validation error output, got: %s", stderr.String())
	}
}
