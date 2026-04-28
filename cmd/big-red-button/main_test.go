package main

import (
	"bytes"
	"context"
	"fmt"
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

func TestPlanIsolatedAppCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"plan-isolated-app",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
		"-app-env", "DISPLAY=:1",
		"../../testdata/profiles/valid-wgws.json",
		"--",
		"/usr/bin/curl",
		"https://example.com",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"isolated-app-tunnel plan",
		"session: 123e4567-e89b-12d3-a456-426614174000",
		"namespace: brb-123e4567",
		"Start WSTunnel control process in host namespace",
		"app_env=DISPLAY=:1",
		"Launch selected app inside isolated namespace",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if strings.Contains(out, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") {
		t.Fatalf("plan output leaked private key: %s", out)
	}
}

func TestPlanIsolatedStopCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"plan-isolated-stop",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"isolated-app-stop plan",
		"session: 123e4567-e89b-12d3-a456-426614174000",
		"Stop isolated app process tree",
		"Delete isolated network namespace",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestPlanIsolatedCleanupCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"plan-isolated-cleanup",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"isolated-app-cleanup plan",
		"session: 123e4567-e89b-12d3-a456-426614174000",
		"Best-effort stop remaining isolated namespace processes",
		"Best-effort delete isolated namespace and host veth",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
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

func TestIsolatedStatusCommandConnected(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	pid := os.Getpid()
	store := truntime.Store{Root: filepath.Join(runtimeRoot, "isolated", sessionID)}
	err := store.Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		Mode:               "isolated-app-tunnel",
		ProfileFingerprint: "abc123",
		WireGuardInterface: "brbwg123e4567",
		SessionID:          sessionID,
		Namespace:          "brb-123e4567",
		HostVeth:           "brbh123e4567",
		NamespaceVeth:      "brbn123e4567",
	}.WithAppProcess(pid, []string{"ip", "netns", "exec"}).WithWSTunnelProcess(pid, []string{"wstunnel", "client"}))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"isolated-status", "-runtime-root", runtimeRoot, "-session-id", sessionID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"state: Connected",
		"mode: isolated-app-tunnel",
		"session: " + sessionID,
		"namespace: brb-123e4567",
		fmt.Sprintf("app pid: %d", pid),
		fmt.Sprintf("wstunnel pid: %d", pid),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestIsolatedSessionsCommand(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	pid := os.Getpid()
	store := truntime.Store{Root: filepath.Join(runtimeRoot, "isolated", sessionID)}
	err := store.Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		Mode:               "isolated-app-tunnel",
		ProfileFingerprint: "abc123",
		WireGuardInterface: "brbwg123e4567",
		SessionID:          sessionID,
		Namespace:          "brb-123e4567",
		HostVeth:           "brbh123e4567",
		NamespaceVeth:      "brbn123e4567",
	}.WithAppProcess(pid, []string{"ip", "netns", "exec"}))
	if err != nil {
		t.Fatal(err)
	}
	dirtySessionID := "223e4567-e89b-12d3-a456-426614174000"
	dirtyRoot := filepath.Join(runtimeRoot, "isolated", dirtySessionID)
	if err := os.MkdirAll(dirtyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirtyRoot, "state.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"isolated-sessions", "-runtime-root", runtimeRoot}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"isolated sessions:",
		"session: " + sessionID,
		"state: Connected",
		"namespace: brb-123e4567",
		fmt.Sprintf("app pid: %d", pid),
		"session: " + dirtySessionID,
		"state: Dirty",
		"cleanup: big-red-button linux-cleanup-isolated-app -yes -session-id " + dirtySessionID + " -runtime-root " + runtimeRoot,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote(`/tmp/Big Red Button/runtime`); got != `'/tmp/Big Red Button/runtime'` {
		t.Fatalf("quote = %s", got)
	}
	if got := shellQuote(`/tmp/brb-runtime`); got != `/tmp/brb-runtime` {
		t.Fatalf("quote = %s", got)
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

func TestLinuxDryRunIsolatedAppCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-dry-run-isolated-app",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
		"../../testdata/profiles/valid-wgws.json",
		"--",
		"/usr/bin/curl",
		"https://example.com",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"engine state: Connected",
		"ip netns add brb-123e4567",
		"ip link add brbh123e4567 type veth peer name brbn123e4567",
		"wstunnel client --log-lvl INFO",
		"ip netns exec brb-123e4567 ip link add dev brbwg123e4567 type wireguard",
		"ip netns exec brb-123e4567 /usr/bin/curl https://example.com",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
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

func TestLinuxIsolatedAppRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-isolated-app",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
		"../../testdata/profiles/valid-wgws.json",
		"--",
		"/usr/bin/curl",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires -yes") {
		t.Fatalf("expected confirmation error, got: %s", stderr.String())
	}
}

func TestLinuxStopIsolatedAppRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-stop-isolated-app",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires -yes") {
		t.Fatalf("expected confirmation error, got: %s", stderr.String())
	}
}

func TestLinuxCleanupIsolatedAppRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-cleanup-isolated-app",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
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
