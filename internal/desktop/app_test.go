package desktop

import (
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
	args := buildDiagnosticsArgs(guiState{ProfilePath: " /tmp/profile.json "})
	got := strings.Join(args, " ")
	want := "diagnostics -runtime-root /run/big-red-button -profile /tmp/profile.json"
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

func TestUIIncludesIsolatedCleanupControl(t *testing.T) {
	for _, want := range []string{
		`id="isolated-cleanup"`,
		`/api/isolated/cleanup`,
		`id="isolated-recover"`,
		`/api/isolated/recover`,
		`Cleanup`,
		`Recover Dirty`,
		`known isolated sessions`,
	} {
		if !strings.Contains(indexHTML, want) {
			t.Fatalf("missing %q in UI", want)
		}
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
