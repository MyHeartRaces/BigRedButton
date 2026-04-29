package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/daemon"
	"github.com/MyHeartRaces/BigRedButton/internal/engine"
	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	platformlinux "github.com/MyHeartRaces/BigRedButton/internal/platform/linux"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
	"github.com/MyHeartRaces/BigRedButton/internal/status"
)

func TestVersionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Big Red Button 0.2.1") {
		t.Fatalf("unexpected version output: %s", stdout.String())
	}
}

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

func TestPlanIsolatedAppGeneratesSessionID(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"plan-isolated-app",
		"../../testdata/profiles/valid-wgws.json",
		"--",
		"/usr/bin/curl",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	sessionID := valueAfterPrefix(stdout.String(), "session: ")
	if len(sessionID) != 36 || sessionID[14] != '4' {
		t.Fatalf("unexpected generated session ID %q in output: %s", sessionID, stdout.String())
	}
	shortID := strings.ReplaceAll(sessionID, "-", "")[:8]
	if !strings.Contains(stdout.String(), "namespace: brb-"+shortID) {
		t.Fatalf("expected namespace from generated session ID, got: %s", stdout.String())
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

func TestDaemonStatusCommand(t *testing.T) {
	if stdruntime.GOOS == "windows" {
		t.Skip("Unix socket daemon transport is not available on Windows")
	}
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
	socketPath := filepath.Join(t.TempDir(), "launcher.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- daemon.ServeUnix(ctx, socketPath, daemon.NewHandler(daemon.Options{RuntimeRoot: runtimeRoot}))
	}()
	waitForDaemonSocket(t, socketPath)
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("daemon did not stop")
		}
	}()

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon-status", "-socket", socketPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"daemon runtime root: " + runtimeRoot,
		"state: Connected",
		"profile fingerprint: abc123",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
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
	}.WithAppProcess(pid, nil).WithWSTunnelProcess(pid, nil))
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
	}.WithAppProcess(pid, nil))
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
		"cleanup: big-red-button linux-cleanup-isolated-app -yes -session-id " + dirtySessionID + isolatedRuntimeRootArg(runtimeRoot),
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

func TestEffectiveWSTunnelBinaryPrefersExplicitPath(t *testing.T) {
	if got := effectiveWSTunnelBinary(" /custom/wstunnel "); got != "/custom/wstunnel" {
		t.Fatalf("wstunnel binary = %q", got)
	}
}

func TestEffectiveWSTunnelBinaryUsesBundledLinuxHelper(t *testing.T) {
	oldGOOS := currentGOOS
	oldBundledPath := bundledLinuxWSTunnelPath
	defer func() {
		currentGOOS = oldGOOS
		bundledLinuxWSTunnelPath = oldBundledPath
	}()
	currentGOOS = "linux"
	bundledLinuxWSTunnelPath = filepath.Join(t.TempDir(), "wstunnel")
	if err := os.WriteFile(bundledLinuxWSTunnelPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := effectiveWSTunnelBinary(""); got != bundledLinuxWSTunnelPath {
		t.Fatalf("wstunnel binary = %q", got)
	}
}

func TestDiagnosticsCommand(t *testing.T) {
	runtimeRoot := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"diagnostics",
		"-runtime-root", runtimeRoot,
		"-profile", "../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"generated at:",
		"version: 0.2.1",
		"host:",
		"effective uid:",
		"system runtime:",
		"state: Idle",
		"profile fingerprint:",
		"profile gateway:",
		"isolated sessions: []",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if strings.Contains(out, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") {
		t.Fatalf("diagnostics leaked private key: %s", out)
	}
}

func TestDiagnosticsCommandIncludesLinuxHostChecks(t *testing.T) {
	defer forceGOOS("linux")()
	defer stubCurrentEUID(1000)()
	restoreLookPath := stubExecutableLookPath(func(binary string) (string, error) {
		if binary == "pkexec" {
			return "", fmt.Errorf("not found")
		}
		return "/usr/bin/" + filepath.Base(binary), nil
	})
	defer restoreLookPath()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"diagnostics",
		"-runtime-root", t.TempDir(),
		"-wstunnel-binary", "wstunnel-custom",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"ok binary ip: found",
		"ok binary wstunnel-custom: found",
		"failed binary pkexec:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestDiagnosticsBundleCommand(t *testing.T) {
	runtimeRoot := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "diagnostics.tar.gz")
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"diagnostics-bundle",
		"-runtime-root", runtimeRoot,
		"-profile", "../../testdata/profiles/valid-wgws.json",
		"-output", outputPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "diagnostics bundle: "+outputPath) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}

	entries := readTarGzEntries(t, outputPath)
	for _, name := range []string{"README.txt", "diagnostics.txt", "diagnostics.json", "manifest.json"} {
		if entries[name] == "" {
			t.Fatalf("missing diagnostics bundle entry %s; entries=%#v", name, entries)
		}
	}
	if !strings.Contains(entries["diagnostics.txt"], "profile fingerprint:") {
		t.Fatalf("diagnostics text missing profile summary: %s", entries["diagnostics.txt"])
	}
	if !strings.Contains(entries["diagnostics.json"], `"version"`) {
		t.Fatalf("diagnostics json missing version: %s", entries["diagnostics.json"])
	}
	if strings.Contains(strings.Join(mapValues(entries), "\n"), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") {
		t.Fatalf("diagnostics bundle leaked private key: %#v", entries)
	}
	if strings.Contains(entries["diagnostics.json"], "V7-WireGuard") {
		t.Fatalf("diagnostics bundle leaked legacy profile label: %s", entries["diagnostics.json"])
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
		"resolvectl dns brb0 1.1.1.1",
		"resolvectl domain brb0 ~.",
		"resolvectl default-route brb0 yes",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestLinuxDryRunConnectCommandResolvesEndpointWhenMissing(t *testing.T) {
	restore := stubLookupIPAddr(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "edge.example.com" {
			t.Fatalf("host = %s", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	})
	defer restore()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"linux-dry-run-connect",
		"-default-gateway", "192.0.2.1",
		"-default-interface", "eth0",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"endpoint IPs: [203.0.113.10]",
		"ip -4 route get 203.0.113.10",
		"ip -4 route replace 203.0.113.10/32 via 192.0.2.1 dev eth0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if strings.Contains(out, "endpoint IPs were not provided") {
		t.Fatalf("dry-run should resolve endpoint IPs before planning: %s", out)
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

func TestLinuxPreflightCommand(t *testing.T) {
	defer forceGOOS("linux")()
	restoreLookup := stubLookupIPAddr(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	})
	defer restoreLookup()
	restoreLookPath := stubExecutableLookPath(func(binary string) (string, error) {
		return "/usr/bin/" + binary, nil
	})
	defer restoreLookPath()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"linux-preflight",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"linux preflight",
		"endpoint IPs: [203.0.113.10]",
		"ok profile: profile is valid",
		"ok binary ip: found",
		"ok binary wstunnel: found",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestLinuxPreflightCommandRequiresPKExec(t *testing.T) {
	defer forceGOOS("linux")()
	defer stubCurrentEUID(1000)()
	restoreLookPath := stubExecutableLookPath(func(binary string) (string, error) {
		if binary == "pkexec" {
			return "/usr/bin/pkexec", nil
		}
		return "/usr/bin/" + binary, nil
	})
	defer restoreLookPath()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"linux-preflight",
		"-require-pkexec",
		"-endpoint-ip", "203.0.113.10",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "ok privilege helper pkexec: found") {
		t.Fatalf("expected pkexec check, got: %s", stdout.String())
	}
}

func TestLinuxPreflightCommandFailsMissingPKExec(t *testing.T) {
	defer forceGOOS("linux")()
	defer stubCurrentEUID(1000)()
	restoreLookPath := stubExecutableLookPath(func(binary string) (string, error) {
		if binary == "pkexec" {
			return "", fmt.Errorf("not found")
		}
		return "/usr/bin/" + binary, nil
	})
	defer restoreLookPath()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"linux-preflight",
		"-require-pkexec",
		"-endpoint-ip", "203.0.113.10",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "failed privilege helper pkexec") {
		t.Fatalf("expected missing pkexec failure, got: %s", stdout.String())
	}
}

func TestLinuxPreflightCommandFailsMissingBinary(t *testing.T) {
	defer forceGOOS("linux")()
	restoreLookPath := stubExecutableLookPath(func(binary string) (string, error) {
		if binary == "wstunnel" {
			return "", fmt.Errorf("not found")
		}
		return "/usr/bin/" + binary, nil
	})
	defer restoreLookPath()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"linux-preflight",
		"-endpoint-ip", "203.0.113.10",
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "failed binary wstunnel") {
		t.Fatalf("expected missing binary failure, got: %s", out)
	}
}

func TestLinuxPreflightIsolatedAppCommand(t *testing.T) {
	defer forceGOOS("linux")()
	restoreLookPath := stubExecutableLookPath(func(binary string) (string, error) {
		return "/usr/bin/" + binary, nil
	})
	defer restoreLookPath()
	runtimeRoot := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"linux-preflight-isolated-app",
		"-runtime-root", runtimeRoot,
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
		"-app-uid", "1000",
		"-app-gid", "1000",
		"-app-env", "DISPLAY=:1",
		"../../testdata/profiles/valid-wgws.json",
		"--",
		"curl", "https://example.com",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"linux isolated app preflight",
		"ok profile: profile is valid",
		"ok binary nft: found",
		"ok binary setpriv: found",
		"ok binary env: found",
		"ok app curl: found",
		"ok isolated runtime: no active session",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestLinuxPreflightIsolatedAppCommandFailsMissingApp(t *testing.T) {
	defer forceGOOS("linux")()
	restoreLookPath := stubExecutableLookPath(func(binary string) (string, error) {
		if binary == "curl" {
			return "", fmt.Errorf("not found")
		}
		return "/usr/bin/" + binary, nil
	})
	defer restoreLookPath()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"linux-preflight-isolated-app",
		"-runtime-root", t.TempDir(),
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
		"../../testdata/profiles/valid-wgws.json",
		"--",
		"curl", "https://example.com",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "failed app curl") {
		t.Fatalf("expected missing app failure, got: %s", stdout.String())
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
		"resolvectl revert brb0",
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

func TestPrintLinuxLifecycleIncludesDNSOperations(t *testing.T) {
	var stdout bytes.Buffer
	command := platformlinux.Command{Name: "resolvectl", Args: []string{"revert", "brb0"}}
	printLinuxLifecycle(linuxLifecycleOutput{
		Result: engine.Result{State: engine.StateIdle},
		DNSOperations: []platformlinux.Operation{
			{Phase: platformlinux.OperationApply, StepID: "restore-dns", Command: &command},
		},
	}, &stdout)

	out := stdout.String()
	if !strings.Contains(out, "dns operations:") || !strings.Contains(out, "resolvectl revert brb0") {
		t.Fatalf("expected DNS operations in output, got: %s", out)
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

func TestLinuxConnectAlreadyConnectedSkipsMutation(t *testing.T) {
	defer forceGOOS("linux")()
	runtimeRoot := t.TempDir()
	config, err := profile.LoadFile("../../testdata/profiles/valid-wgws.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := (truntime.Store{Root: runtimeRoot}).Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		ProfileFingerprint: config.Fingerprint(),
		WireGuardInterface: "brb0",
	}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-connect",
		"-yes",
		"-runtime-root", runtimeRoot,
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"engine state: Connected",
		"engine note: already connected; no changes applied",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if strings.Contains(out, "route operations:") || strings.Contains(out, "wireguard operations:") {
		t.Fatalf("expected no lifecycle operations, got: %s", out)
	}
}

func TestLinuxConnectRejectsDifferentActiveProfile(t *testing.T) {
	defer forceGOOS("linux")()
	runtimeRoot := t.TempDir()
	if err := (truntime.Store{Root: runtimeRoot}).Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		ProfileFingerprint: "other",
		WireGuardInterface: "brb0",
	}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-connect",
		"-yes",
		"-runtime-root", runtimeRoot,
		"../../testdata/profiles/valid-wgws.json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "already connected with a different profile") {
		t.Fatalf("expected profile mismatch error, got: %s", stdout.String())
	}
}

func TestResolveEndpointIPsDeduplicatesAndSorts(t *testing.T) {
	restore := stubLookupIPAddr(func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "edge.example.com" {
			t.Fatalf("host = %s", host)
		}
		return []net.IPAddr{
			{IP: net.ParseIP("203.0.113.20")},
			{IP: net.ParseIP("203.0.113.10")},
			{IP: net.ParseIP("203.0.113.20")},
		}, nil
	})
	defer restore()

	got, err := resolveEndpointIPs(context.Background(), " edge.example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "203.0.113.10,203.0.113.20" {
		t.Fatalf("endpoint IPs = %#v", got)
	}
}

func TestResolveEndpointIPsRejectsEmptyResult(t *testing.T) {
	restore := stubLookupIPAddr(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{}}, nil
	})
	defer restore()

	_, err := resolveEndpointIPs(context.Background(), "edge.example.com")
	if err == nil || !strings.Contains(err.Error(), "did not resolve") {
		t.Fatalf("expected empty resolve error, got %v", err)
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

func TestLinuxStopIsolatedAppMissingStateIsIdle(t *testing.T) {
	defer forceGOOS("linux")()
	var stdout, stderr bytes.Buffer
	runtimeRoot := t.TempDir()

	code := run([]string{
		"linux-stop-isolated-app",
		"-yes",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
		"-runtime-root", runtimeRoot,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"engine state: Idle",
		"no runtime state at",
		"skip stop-isolated-app: no runtime state",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if strings.Contains(out, "engine error:") {
		t.Fatalf("expected no engine error, got: %s", out)
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

func TestLinuxRecoverIsolatedSessionsRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"linux-recover-isolated-sessions"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires -yes") {
		t.Fatalf("expected confirmation error, got: %s", stderr.String())
	}
}

func TestLinuxRecoverIsolatedSessionsStartupMode(t *testing.T) {
	defer forceGOOS("linux")()
	var stdout, stderr bytes.Buffer
	runtimeRoot := t.TempDir()

	code := run([]string{
		"linux-recover-isolated-sessions",
		"-yes",
		"-startup",
		"-runtime-root", runtimeRoot,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"mode: startup recovery",
		"targets: []",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestLinuxMonitorIsolatedAppRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-monitor-isolated-app",
		"-session-id", "123e4567-e89b-12d3-a456-426614174000",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires -yes") {
		t.Fatalf("expected confirmation error, got: %s", stderr.String())
	}
}

func TestWaitForIsolatedAppExitMissingRuntimeState(t *testing.T) {
	store := truntime.Store{Root: filepath.Join(t.TempDir(), "isolated", "123e4567-e89b-12d3-a456-426614174000")}

	got, err := waitForIsolatedAppExit(context.Background(), store, time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reason != "runtime state missing" {
		t.Fatalf("monitor state = %#v", got)
	}
}

func TestWaitForIsolatedAppExitWhenPIDIsGone(t *testing.T) {
	restore := stubMonitorPIDExists(func(int) bool { return false })
	defer restore()
	store := truntime.Store{Root: filepath.Join(t.TempDir(), "isolated", "123e4567-e89b-12d3-a456-426614174000")}
	state := monitorTestState().WithAppProcess(4242, []string{"/usr/bin/curl"})
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	got, err := waitForIsolatedAppExit(context.Background(), store, time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.AppPID != 4242 || got.Reason != "app process exited" {
		t.Fatalf("monitor state = %#v", got)
	}
}

func TestWaitForIsolatedAppExitHonorsTimeout(t *testing.T) {
	restore := stubMonitorPIDExists(func(int) bool { return true })
	defer restore()
	store := truntime.Store{Root: filepath.Join(t.TempDir(), "isolated", "123e4567-e89b-12d3-a456-426614174000")}
	state := monitorTestState().WithAppProcess(4242, []string{"/usr/bin/curl"})
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	got, err := waitForIsolatedAppExit(context.Background(), store, time.Millisecond, time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got state=%#v err=%v", got, err)
	}
	if got.AppPID != 4242 {
		t.Fatalf("monitor state = %#v", got)
	}
}

func TestRecordCurrentMonitorProcess(t *testing.T) {
	store := truntime.Store{Root: filepath.Join(t.TempDir(), "isolated", "123e4567-e89b-12d3-a456-426614174000")}
	state := monitorTestState().WithAppProcess(4242, []string{"/usr/bin/curl"})
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	if err := recordCurrentMonitorProcess(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MonitorProcess == nil || loaded.MonitorProcess.PID != os.Getpid() {
		t.Fatalf("monitor process = %#v", loaded.MonitorProcess)
	}
	if len(loaded.MonitorProcess.Argv) == 0 {
		t.Fatalf("monitor argv = %#v", loaded.MonitorProcess.Argv)
	}
}

func TestRecordCurrentMonitorProcessIgnoresMissingRuntimeState(t *testing.T) {
	store := truntime.Store{Root: filepath.Join(t.TempDir(), "isolated", "123e4567-e89b-12d3-a456-426614174000")}
	if err := recordCurrentMonitorProcess(context.Background(), store); err != nil {
		t.Fatal(err)
	}
}

func TestIsolatedRecoveryTargets(t *testing.T) {
	sessions := []status.IsolatedSessionSnapshot{
		{SessionID: "dirty", Snapshot: status.Snapshot{State: status.StateDirty}},
		{SessionID: "connected", Snapshot: status.Snapshot{State: status.StateConnected}},
	}
	targets, skipped := isolatedRecoveryTargets(sessions, false)
	if strings.Join(targets, ",") != "dirty" || strings.Join(skipped, ",") != "connected" {
		t.Fatalf("targets=%#v skipped=%#v", targets, skipped)
	}
	targets, skipped = isolatedRecoveryTargets(sessions, true)
	if strings.Join(targets, ",") != "dirty,connected" || len(skipped) != 0 {
		t.Fatalf("all targets=%#v skipped=%#v", targets, skipped)
	}
}

func TestIsolatedStartupRecoveryTargets(t *testing.T) {
	restore := stubMonitorPIDExists(func(pid int) bool { return pid != 999999999 && pid != 999999997 })
	defer restore()
	pid := os.Getpid()
	healthy := func(sessionID string, monitor *truntime.ProcessState) status.IsolatedSessionSnapshot {
		state := monitorTestState().
			WithAppProcess(pid, nil).
			WithWSTunnelProcess(pid, nil)
		state.SessionID = sessionID
		state.MonitorProcess = monitor
		return status.IsolatedSessionSnapshot{
			SessionID: sessionID,
			Snapshot:  status.Snapshot{State: status.StateConnected, Active: &state},
		}
	}
	deadApp := monitorTestState().
		WithAppProcess(999999999, nil).
		WithWSTunnelProcess(pid, nil).
		WithMonitorProcess(999999998, nil)
	deadApp.SessionID = "423e4567-e89b-12d3-a456-426614174000"

	sessions := []status.IsolatedSessionSnapshot{
		healthy("123e4567-e89b-12d3-a456-426614174000", nil),
		healthy("223e4567-e89b-12d3-a456-426614174000", &truntime.ProcessState{PID: 999999997}),
		{
			SessionID: deadApp.SessionID,
			Snapshot:  status.Snapshot{State: status.StateDirty, Active: &deadApp},
		},
		{SessionID: "523e4567-e89b-12d3-a456-426614174000", Snapshot: status.Snapshot{State: status.StateConnected}},
	}

	monitors, cleanup, skipped := isolatedStartupRecoveryTargets(sessions, false)
	if strings.Join(monitors, ",") != "123e4567-e89b-12d3-a456-426614174000,223e4567-e89b-12d3-a456-426614174000" {
		t.Fatalf("monitors=%#v cleanup=%#v skipped=%#v", monitors, cleanup, skipped)
	}
	if strings.Join(cleanup, ",") != deadApp.SessionID {
		t.Fatalf("cleanup=%#v", cleanup)
	}
	if strings.Join(skipped, ",") != "523e4567-e89b-12d3-a456-426614174000" {
		t.Fatalf("skipped=%#v", skipped)
	}

	_, cleanup, skipped = isolatedStartupRecoveryTargets(sessions, true)
	if strings.Join(cleanup, ",") != deadApp.SessionID+",523e4567-e89b-12d3-a456-426614174000" || len(skipped) != 0 {
		t.Fatalf("all cleanup=%#v skipped=%#v", cleanup, skipped)
	}
}

func TestLinuxDisconnectRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{
		"linux-disconnect",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires -yes") {
		t.Fatalf("expected confirmation error, got: %s", stderr.String())
	}
}

func TestLinuxDisconnectNoProfileIsIdleWhenRuntimeMissing(t *testing.T) {
	defer forceGOOS("linux")()
	var stdout, stderr bytes.Buffer
	runtimeRoot := t.TempDir()

	code := run([]string{
		"linux-disconnect",
		"-yes",
		"-runtime-root", runtimeRoot,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "engine state: Idle") || !strings.Contains(out, "no runtime state at") {
		t.Fatalf("expected idle disconnect without profile, got: %s", out)
	}
}

func waitForDaemonSocket(t *testing.T, socketPath string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket was not ready: %s", socketPath)
}

func forceGOOS(value string) func() {
	previous := currentGOOS
	currentGOOS = value
	return func() {
		currentGOOS = previous
	}
}

func stubLookupIPAddr(fn func(context.Context, string) ([]net.IPAddr, error)) func() {
	previous := lookupIPAddr
	lookupIPAddr = fn
	return func() {
		lookupIPAddr = previous
	}
}

func stubExecutableLookPath(fn func(string) (string, error)) func() {
	previous := executableLookPath
	executableLookPath = fn
	return func() {
		executableLookPath = previous
	}
}

func stubCurrentEUID(value int) func() {
	previous := currentEUID
	currentEUID = func() int { return value }
	return func() {
		currentEUID = previous
	}
}

func stubMonitorPIDExists(fn func(int) bool) func() {
	previous := monitorPIDExists
	monitorPIDExists = fn
	return func() {
		monitorPIDExists = previous
	}
}

func monitorTestState() truntime.State {
	return truntime.State{
		Version:            truntime.StateVersion,
		Mode:               planner.IsolatedAppTunnelKind,
		ProfileFingerprint: "test-fingerprint",
		WireGuardInterface: "brbwg123e4567",
		SessionID:          "123e4567-e89b-12d3-a456-426614174000",
		AppID:              "223e4567-e89b-12d3-a456-426614174000",
		Namespace:          "brb-123e4567",
		HostVeth:           "brbh123e4567",
		NamespaceVeth:      "brbn123e4567",
	}
}

func readTarGzEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	entries := map[string]string{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = string(payload)
	}
	return entries
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func valueAfterPrefix(value string, prefix string) string {
	for _, line := range strings.Split(value, "\n") {
		if suffix, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(suffix)
		}
	}
	return ""
}
