package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
)

func TestValidateProfileCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"validate-profile", "../../testdata/profiles/valid-wgws.json"}, &stdout, &stderr)
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

	code := run([]string{"validate-profile", "-json", "../../testdata/profiles/valid-wgws.json"}, &stdout, &stderr)
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

	code := run([]string{"validate-profile", "../../testdata/profiles/invalid-placeholder-wgws.json"}, &stdout, &stderr)
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
		"../../testdata/profiles/valid-wgws.json",
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
		"../../testdata/profiles/valid-wgws.json",
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

func TestStatusCommandIdle(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"status", "-runtime-root", t.TempDir()}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "state: Idle") {
		t.Fatalf("expected idle status, got: %s", stdout.String())
	}
}

func TestStatusCommandConnected(t *testing.T) {
	runtimeRoot := t.TempDir()
	store := truntime.Store{Root: runtimeRoot}
	err := store.Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "tg-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"status", "-runtime-root", runtimeRoot}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "state: Connected") || !strings.Contains(out, "profile fingerprint: abc123") {
		t.Fatalf("expected connected status, got: %s", out)
	}
}

func TestLinuxDryRunConnectCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-dry-run-connect",
		"-endpoint-ip", "203.0.113.10",
		"-default-gateway", "192.0.2.1",
		"-default-interface", "eth0",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"engine state: Connected",
		"linux dry-run commands:",
		"ip -4 route get 203.0.113.10",
		"ip -4 route replace 203.0.113.10/32 via 192.0.2.1 dev eth0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestLinuxDryRunConnectCommandFailsWithoutConcreteRouteExclusion(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-dry-run-connect",
		"-endpoint-ip", "203.0.113.10",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "engine state: FailedRecoverable") {
		t.Fatalf("expected failed recoverable state, got: %s", out)
	}
	if !strings.Contains(out, "route exclusion for endpoint 203.0.113.10 is not resolved") {
		t.Fatalf("expected unresolved route exclusion error, got: %s", out)
	}
}

func TestLinuxDryRunConnectAndDisconnectRuntimeState(t *testing.T) {
	runtimeRoot := t.TempDir()
	var connectStdout, connectStderr bytes.Buffer

	code := run([]string{
		"linux-dry-run-connect",
		"-persist-runtime-state",
		"-runtime-root", runtimeRoot,
		"-endpoint-ip", "203.0.113.10",
		"-default-gateway", "192.0.2.1",
		"-default-interface", "eth0",
		"../../testdata/profiles/valid-wgws.json",
	}, &connectStdout, &connectStderr)
	if code != 0 {
		t.Fatalf("connect code = %d stderr = %s", code, connectStderr.String())
	}
	statePath := filepath.Join(runtimeRoot, "state.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected runtime state file: %v", err)
	}
	if !strings.Contains(connectStdout.String(), "save "+statePath) {
		t.Fatalf("expected save operation, got: %s", connectStdout.String())
	}

	var disconnectStdout, disconnectStderr bytes.Buffer
	code = run([]string{
		"linux-dry-run-disconnect",
		"-persist-runtime-state",
		"-runtime-root", runtimeRoot,
	}, &disconnectStdout, &disconnectStderr)
	if code != 0 {
		t.Fatalf("disconnect code = %d stderr = %s stdout = %s", code, disconnectStderr.String(), disconnectStdout.String())
	}
	out := disconnectStdout.String()
	for _, want := range []string{
		"engine state: Idle",
		"load " + statePath,
		"ip -4 route delete 203.0.113.10/32 via 192.0.2.1 dev eth0",
		"clear " + statePath,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("expected runtime state to be cleared, err = %v", err)
	}
}

func TestLinuxConnectRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-connect",
		"-endpoint-ip", "203.0.113.10",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires -yes") {
		t.Fatalf("expected confirmation error, got: %s", stderr.String())
	}
}

func TestLinuxDisconnectRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-disconnect",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires -yes") {
		t.Fatalf("expected confirmation error, got: %s", stderr.String())
	}
}
