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

func TestPlanConnectCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"plan-connect",
		"-endpoint-ip", "203.0.113.10",
		"-default-gateway", "192.0.2.1",
		"-default-interface", "eth0",
		"../../testdata/profiles/valid-v7.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"connect plan",
		"Add launcher-owned host route for WSTunnel endpoint",
		"Start WSTunnel client process",
		"Apply WireGuard peer",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if strings.Contains(out, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") {
		t.Fatalf("plan output leaked private key: %s", out)
	}
}

func TestPlanConnectCommandJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"plan-connect",
		"-json",
		"-endpoint-ip", "203.0.113.10",
		"../../testdata/profiles/valid-v7.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"kind": "connect"`) {
		t.Fatalf("expected JSON plan output, got: %s", out)
	}
	if !strings.Contains(out, `"id": "add-route-exclusion-203-0-113-10"`) {
		t.Fatalf("expected route step in JSON output, got: %s", out)
	}
}

func TestPlanDisconnectCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"plan-disconnect", "-wireguard-interface", "tg-test"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "disconnect plan") {
		t.Fatalf("expected disconnect plan, got: %s", out)
	}
	if !strings.Contains(out, "Remove launcher-owned WSTunnel endpoint route exclusions") {
		t.Fatalf("expected route cleanup step, got: %s", out)
	}
}
