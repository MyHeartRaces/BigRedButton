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
		`Cleanup`,
		`known isolated sessions`,
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
