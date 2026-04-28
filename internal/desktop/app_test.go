package desktop

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecodeActionTrimsValues(t *testing.T) {
	request, err := decodeAction(strings.NewReader(`{"endpoint_ip":" 203.0.113.10 ","wstunnel_binary":" /usr/bin/wstunnel ","session_id":" abc ","app_command":" /usr/bin/curl "}`))
	if err != nil {
		t.Fatal(err)
	}
	if request.EndpointIP != "203.0.113.10" {
		t.Fatalf("endpoint = %q", request.EndpointIP)
	}
	if request.WSTunnelBinary != "/usr/bin/wstunnel" {
		t.Fatalf("wstunnel binary = %q", request.WSTunnelBinary)
	}
	if request.SessionID != "abc" || request.AppCommand != "/usr/bin/curl" {
		t.Fatalf("isolated fields = %#v", request)
	}
}

func TestDecodeActionAllowsEmptyBody(t *testing.T) {
	request, err := decodeAction(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if request.EndpointIP != "" || request.WSTunnelBinary != "" {
		t.Fatalf("request = %#v", request)
	}
}

func TestSplitCommandLine(t *testing.T) {
	got, err := splitCommandLine(`/usr/bin/env "A=B C" /usr/bin/curl https://example.com`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/bin/env", "A=B C", "/usr/bin/curl", "https://example.com"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("args = %#v want %#v", got, want)
	}
}

func TestSplitCommandLineRejectsUnterminatedQuote(t *testing.T) {
	_, err := splitCommandLine(`/usr/bin/curl "https://example.com`)
	if err == nil {
		t.Fatal("expected unterminated quote error")
	}
}

func TestBuildLinuxConnectArgsMakesEndpointOptional(t *testing.T) {
	state := guiState{ProfilePath: "/tmp/profile.json"}
	args, err := buildLinuxConnectArgs(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "linux-connect -yes /tmp/profile.json" {
		t.Fatalf("endpoint should be optional: %#v", args)
	}

	state.EndpointIP = "203.0.113.10"
	args, err = buildLinuxConnectArgs(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "-endpoint-ip 203.0.113.10") {
		t.Fatalf("endpoint args missing: %#v", args)
	}
}

func TestBuildDiagnosticsArgsIncludesProfileWhenSaved(t *testing.T) {
	args := buildDiagnosticsArgs(guiState{
		ProfilePath:    " /tmp/profile.json ",
		WSTunnelBinary: " /usr/bin/wstunnel ",
	})
	got := strings.Join(args, " ")
	want := "diagnostics -runtime-root /run/big-red-button -wstunnel-binary /usr/bin/wstunnel -profile /tmp/profile.json"
	if got != want {
		t.Fatalf("args = %q want %q", got, want)
	}

	args = buildDiagnosticsArgs(guiState{})
	got = strings.Join(args, " ")
	want = "diagnostics -runtime-root /run/big-red-button"
	if got != want {
		t.Fatalf("args = %q want %q", got, want)
	}
}

func TestBuildDiagnosticsBundleArgsIncludesOutputAndProfile(t *testing.T) {
	args := buildDiagnosticsBundleArgs(guiState{
		ProfilePath:    " /tmp/profile.json ",
		WSTunnelBinary: " /usr/bin/wstunnel ",
	}, "/tmp/brb-diag.tar.gz")
	got := strings.Join(args, " ")
	want := "diagnostics-bundle -runtime-root /run/big-red-button -output /tmp/brb-diag.tar.gz -wstunnel-binary /usr/bin/wstunnel -profile /tmp/profile.json"
	if got != want {
		t.Fatalf("args = %q want %q", got, want)
	}
}

func TestBuildLinuxPreflightArgs(t *testing.T) {
	args, err := buildLinuxPreflightArgs(guiState{
		ProfilePath:     " /tmp/profile.json ",
		EndpointIP:      " 203.0.113.10 ",
		WSTunnelBinary:  " /usr/bin/wstunnel ",
		IsolatedSession: "ignored",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, " ")
	want := "linux-preflight -discover-routes -require-pkexec -endpoint-ip 203.0.113.10 -wstunnel-binary /usr/bin/wstunnel /tmp/profile.json"
	if got != want {
		t.Fatalf("args = %q want %q", got, want)
	}

	_, err = buildLinuxPreflightArgs(guiState{})
	if err == nil {
		t.Fatal("expected missing profile error")
	}
}

func TestBuildLinuxIsolatedPreflightArgs(t *testing.T) {
	t.Setenv("DISPLAY", ":1")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XAUTHORITY", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("PULSE_SERVER", "")
	t.Setenv("PIPEWIRE_RUNTIME_DIR", "")

	args, err := buildLinuxIsolatedPreflightArgs(guiState{
		ProfilePath:     " /tmp/profile.json ",
		WSTunnelBinary:  " /usr/bin/wstunnel ",
		IsolatedSession: "123e4567-e89b-12d3-a456-426614174000",
		IsolatedCommand: `/usr/bin/env "A=B C" /usr/bin/curl https://example.com`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, "\n")
	for _, want := range []string{
		"linux-preflight-isolated-app",
		"-require-pkexec",
		"-session-id\n123e4567-e89b-12d3-a456-426614174000",
		"-wstunnel-binary\n/usr/bin/wstunnel",
		"-app-env\nDISPLAY=:1",
		"/tmp/profile.json\n--\n/usr/bin/env\nA=B C\n/usr/bin/curl\nhttps://example.com",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in args: %#v", want, args)
		}
	}

	_, err = buildLinuxIsolatedPreflightArgs(guiState{ProfilePath: "/tmp/profile.json"})
	if err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestClearIsolatedSessionOnSuccess(t *testing.T) {
	state := guiState{IsolatedSession: "123e4567-e89b-12d3-a456-426614174000"}
	cleared := clearIsolatedSessionOnSuccess(state, actionResponse{OK: true})
	if cleared.IsolatedSession != "" {
		t.Fatalf("expected successful lifecycle to clear session, got %#v", cleared)
	}

	kept := clearIsolatedSessionOnSuccess(state, actionResponse{OK: false})
	if kept.IsolatedSession != state.IsolatedSession {
		t.Fatalf("expected failed lifecycle to keep session, got %#v", kept)
	}
}

func TestNewUUIDShape(t *testing.T) {
	value, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 36 || value[14] != '4' {
		t.Fatalf("unexpected UUID: %s", value)
	}
}

func TestWithLinuxPrivilegeHelperRequiresPKExecForUser(t *testing.T) {
	defer forceDesktopRuntime("linux", 1000, func(binary string) (string, error) {
		if binary == "pkexec" {
			return "/usr/bin/pkexec", nil
		}
		return "", fmt.Errorf("not found")
	})()

	command, err := withLinuxPrivilegeHelper([]string{"/usr/bin/big-red-button", "linux-connect"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(command, " ")
	want := "/usr/bin/pkexec /usr/bin/big-red-button linux-connect"
	if got != want {
		t.Fatalf("command = %q want %q", got, want)
	}
}

func TestWithLinuxPrivilegeHelperReportsMissingPKExec(t *testing.T) {
	defer forceDesktopRuntime("linux", 1000, func(binary string) (string, error) {
		return "", fmt.Errorf("%s missing", binary)
	})()

	_, err := withLinuxPrivilegeHelper([]string{"/usr/bin/big-red-button", "linux-connect"})
	if err == nil || !strings.Contains(err.Error(), "pkexec was not found") {
		t.Fatalf("expected missing pkexec error, got %v", err)
	}
}

func TestWithLinuxPrivilegeHelperSkipsPKExecForRoot(t *testing.T) {
	defer forceDesktopRuntime("linux", 0, func(binary string) (string, error) {
		return "", fmt.Errorf("%s should not be resolved", binary)
	})()

	command, err := withLinuxPrivilegeHelper([]string{"/usr/bin/big-red-button", "linux-connect"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(command, " ")
	want := "/usr/bin/big-red-button linux-connect"
	if got != want {
		t.Fatalf("command = %q want %q", got, want)
	}
}

func TestLinuxPrivilegeHelperStatus(t *testing.T) {
	defer forceDesktopRuntime("linux", 1000, func(binary string) (string, error) {
		if binary == "pkexec" {
			return "/usr/bin/pkexec", nil
		}
		return "", fmt.Errorf("not found")
	})()

	got := linuxPrivilegeHelperStatus()
	if got != "pkexec: /usr/bin/pkexec" {
		t.Fatalf("status = %q", got)
	}
}

func TestLinuxPrivilegeHelperStatusMissingPKExec(t *testing.T) {
	defer forceDesktopRuntime("linux", 1000, func(binary string) (string, error) {
		return "", fmt.Errorf("%s missing", binary)
	})()

	got := linuxPrivilegeHelperStatus()
	if got != "missing pkexec" {
		t.Fatalf("status = %q", got)
	}
}

func TestUIIncludesIsolatedCleanupControl(t *testing.T) {
	for _, want := range []string{
		`id="isolated-cleanup"`,
		`/api/isolated/cleanup`,
		`id="isolated-preflight"`,
		`/api/isolated/preflight`,
		`id="diagnostics-bundle"`,
		`/api/diagnostics-bundle`,
		`id="isolated-recover"`,
		`/api/isolated/recover`,
		`Preflight`,
		`Cleanup`,
		`Recover Dirty`,
		`known isolated sessions`,
		`app version`,
		`privilege helper`,
	} {
		if !strings.Contains(indexHTML, want) {
			t.Fatalf("missing %q in UI", want)
		}
	}
}

func forceDesktopRuntime(goos string, euid int, lookPath func(string) (string, error)) func() {
	previousGOOS := desktopGOOS
	previousEUID := desktopGeteuid
	previousLookPath := desktopLookPath
	desktopGOOS = goos
	desktopGeteuid = func() int { return euid }
	desktopLookPath = lookPath
	return func() {
		desktopGOOS = previousGOOS
		desktopGeteuid = previousEUID
		desktopLookPath = previousLookPath
	}
}

func TestUIPrimaryConnectButtonIsSystemToggle(t *testing.T) {
	for _, want := range []string{
		`let currentSystemState = 'Idle';`,
		`connectButton.textContent = currentSystemState === 'Connected' || currentSystemState === 'Dirty' ? 'Disconnect' : 'Connect';`,
		`function systemTogglePath()`,
		`action(systemTogglePath())`,
	} {
		if !strings.Contains(indexHTML, want) {
			t.Fatalf("missing %q in UI", want)
		}
	}
}

func TestDesktopLaunchEnvUsesAllowlist(t *testing.T) {
	t.Setenv("DISPLAY", ":1")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	t.Setenv("SECRET_TOKEN", "nope")
	got := strings.Join(desktopLaunchEnv(), "\n")
	if !strings.Contains(got, "DISPLAY=:1") || !strings.Contains(got, "XDG_RUNTIME_DIR=/run/user/1000") {
		t.Fatalf("env = %s", got)
	}
	if strings.Contains(got, "SECRET_TOKEN") {
		t.Fatalf("env leaked unsupported key: %s", got)
	}
}
