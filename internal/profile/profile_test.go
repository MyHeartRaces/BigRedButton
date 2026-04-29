package profile

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseWGWSValidFixture(t *testing.T) {
	data, err := os.ReadFile("../../testdata/profiles/valid-wgws.json")
	if err != nil {
		t.Fatal(err)
	}

	config, err := ParseWGWS(data)
	if err != nil {
		t.Fatalf("ParseWGWS() error = %v", err)
	}

	if config.Name != legacyWGWSProfileName {
		t.Fatalf("unexpected profile name: %s", config.Name)
	}
	if config.WSTunnelURL != "wss://edge.example.com:443/cdn/ws" {
		t.Fatalf("unexpected wstunnel URL: %s", config.WSTunnelURL)
	}
	if config.WSTunnelHost != "edge.example.com" {
		t.Fatalf("unexpected wstunnel host: %s", config.WSTunnelHost)
	}
	if config.LocalUDPListen != "127.0.0.1:51820" {
		t.Fatalf("unexpected local UDP endpoint: %s", config.LocalUDPListen)
	}
	if got := config.AllowedIPs; len(got) != 2 || got[0] != "0.0.0.0/0" || got[1] != "::/0" {
		t.Fatalf("unexpected allowed IPs: %#v", got)
	}
	if config.MTU != 1280 {
		t.Fatalf("unexpected MTU: %d", config.MTU)
	}
	if config.PersistentKeepalive != 25 {
		t.Fatalf("unexpected keepalive: %d", config.PersistentKeepalive)
	}
	if config.Fingerprint() == "" {
		t.Fatal("expected fingerprint")
	}
}

func TestParseWGWSAcceptsNeutralProfileName(t *testing.T) {
	raw := strings.Replace(validProfileJSON(), legacyWGWSProfileName, "Big Red Button", 1)

	config, err := ParseWGWS([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "Big Red Button" {
		t.Fatalf("unexpected profile name: %s", config.Name)
	}
}

func TestParseWGWSAcceptsSingBoxWireGuardOutbound(t *testing.T) {
	data, err := os.ReadFile("../../testdata/profiles/valid-singbox-wgws.json")
	if err != nil {
		t.Fatal(err)
	}

	config, err := ParseWGWS(data)
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "wgws-out" {
		t.Fatalf("unexpected profile name: %s", config.Name)
	}
	if config.WSTunnelURL != "wss://edge.example.com:443/cdn/ws" {
		t.Fatalf("unexpected wstunnel URL: %s", config.WSTunnelURL)
	}
	if config.LocalUDPListen != "127.0.0.1:51820" {
		t.Fatalf("unexpected local UDP: %s", config.LocalUDPListen)
	}
	if config.ServerPublicKey != "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=" {
		t.Fatalf("unexpected server public key: %s", config.ServerPublicKey)
	}
	if got := config.AllowedIPs; len(got) != 2 || got[0] != "0.0.0.0/0" || got[1] != "::/0" {
		t.Fatalf("unexpected allowed IPs: %#v", got)
	}
	if config.MTU != 1280 || config.PersistentKeepalive != 25 {
		t.Fatalf("unexpected mtu/keepalive: %d/%d", config.MTU, config.PersistentKeepalive)
	}
	if config.DNS != "1.1.1.1" {
		t.Fatalf("unexpected default DNS: %s", config.DNS)
	}
}

func TestParseWGWSAcceptsTracegateSingBoxAttachmentWithWSTunnelMetadata(t *testing.T) {
	raw := `{
	  "log": {"level": "warn"},
	  "inbounds": [{"type": "mixed", "tag": "local-in", "listen": "127.0.0.1", "listen_port": 18083}],
	  "outbounds": [
	    {
	      "type": "wireguard",
	      "tag": "proxy",
	      "server": "127.0.0.1",
	      "server_port": 51820,
	      "local_address": ["10.70.0.2/32"],
	      "private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	      "peer_public_key": "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
	      "pre_shared_key": "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD=",
	      "mtu": 1280
	    }
	  ],
	  "route": {"auto_detect_interface": true, "final": "proxy"},
	  "wstunnel": {"url": "wss://edge.example.com:443/cdn/ws"}
	}`

	config, err := ParseWGWS([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if config.Server != "edge.example.com" || config.WSTunnelHost != "edge.example.com" {
		t.Fatalf("unexpected server fields: server=%s wstunnel=%s", config.Server, config.WSTunnelHost)
	}
	if config.LocalUDPListen != "127.0.0.1:51820" {
		t.Fatalf("unexpected local UDP: %s", config.LocalUDPListen)
	}
	if got := config.AllowedIPs; len(got) != 1 || got[0] != "0.0.0.0/0" {
		t.Fatalf("unexpected default allowed IPs: %#v", got)
	}
	if config.PersistentKeepalive != 25 {
		t.Fatalf("unexpected keepalive: %d", config.PersistentKeepalive)
	}
	if config.DNS != "1.1.1.1" {
		t.Fatalf("unexpected default DNS: %s", config.DNS)
	}
}

func TestParseWGWSDefaultsSingBoxAllowedIPsFromAddressFamilies(t *testing.T) {
	raw := `{
	  "outbounds": [
	    {
	      "type": "wireguard",
	      "tag": "proxy",
	      "server": "127.0.0.1",
	      "server_port": 51820,
	      "local_address": ["10.70.0.2/32", "fd00:70::2/128"],
	      "private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	      "peer_public_key": "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
	      "pre_shared_key": "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD=",
	      "mtu": 1280
	    }
	  ],
	  "wstunnel": {"url": "wss://edge.example.com:443/cdn/ws"}
	}`

	config, err := ParseWGWS([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := config.AllowedIPs; len(got) != 2 || got[0] != "0.0.0.0/0" || got[1] != "::/0" {
		t.Fatalf("unexpected dual-stack default allowed IPs: %#v", got)
	}
}

func TestParseWGWSReportsSingBoxMissingWSTunnel(t *testing.T) {
	raw := `{
	  "outbounds": [
	    {
	      "type": "wireguard",
	      "tag": "plain-wg",
	      "server": "198.51.100.10",
	      "server_port": 51820,
	      "local_address": ["10.70.0.2/32"],
	      "private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	      "peer_public_key": "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
	      "pre_shared_key": "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD=",
	      "peers": [{"allowed_ips": ["0.0.0.0/0"]}]
	    }
	  ]
	}`

	_, err := ParseWGWS([]byte(raw))
	if err == nil {
		t.Fatal("expected validation error")
	}
	validationErr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	if !strings.Contains(joined, "sing-box WireGuard config requires WSTunnel URL") {
		t.Fatalf("missing WSTunnel import error: %s", joined)
	}
}

func TestSummaryDoesNotExposeSecrets(t *testing.T) {
	data, err := os.ReadFile("../../testdata/profiles/valid-wgws.json")
	if err != nil {
		t.Fatal(err)
	}
	config, err := ParseWGWS(data)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(config.Summary())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"profile":"WGWS"`) {
		t.Fatalf("summary should use neutral profile type: %s", text)
	}
	if strings.Contains(text, legacyWGWSProfileName) {
		t.Fatalf("summary leaked legacy profile name: %s", text)
	}
	for _, secret := range []string{
		config.WireGuardPrivateKey,
		config.PresharedKey,
		config.ServerPublicKey,
	} {
		if secret != "" && strings.Contains(text, secret) {
			t.Fatalf("summary leaked secret material: %s", text)
		}
	}
}

func TestFingerprintIncludesDNS(t *testing.T) {
	config, err := ParseWGWS([]byte(validProfileJSON()))
	if err != nil {
		t.Fatal(err)
	}
	changed := config
	changed.DNS = "8.8.8.8"

	if config.Fingerprint() == changed.Fingerprint() {
		t.Fatal("expected DNS change to alter fingerprint")
	}
}

func TestParseWGWSRejectsPlaceholders(t *testing.T) {
	data, err := os.ReadFile("../../testdata/profiles/invalid-placeholder-wgws.json")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseWGWS(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	validationErr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	if !strings.Contains(joined, "wireguard.private_key must not be a placeholder") {
		t.Fatalf("missing private key placeholder error: %s", joined)
	}
	if !strings.Contains(joined, "wireguard.server_public_key must not be a placeholder") {
		t.Fatalf("missing server public key placeholder error: %s", joined)
	}
}

func TestParseWGWSAcceptsProfileWithoutLegacyName(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, legacyWGWSProfileName, "unsupported-profile", 1)

	config, err := ParseWGWS([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "unsupported-profile" {
		t.Fatalf("profile name should be accepted without legacy coupling: %s", config.Name)
	}
}

func TestParseWGWSRejectsUnsafeWSTunnelURL(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, "wss://edge.example.com:443/cdn/ws", "http://edge.example.com:80/cdn/ws?debug=1", 1)

	_, err := ParseWGWS([]byte(raw))
	if err == nil {
		t.Fatal("expected validation error")
	}
	validationErr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	for _, want := range []string{
		"wstunnel.url must use wss scheme",
		"wstunnel.url must explicitly use port 443",
		"wstunnel.url must not contain query or fragment",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in problems: %s", want, joined)
		}
	}
}

func TestParseWGWSRejectsPlaceholderWSTunnelHost(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, "wss://edge.example.com:443/cdn/ws", "wss://host:443/cdn/ws", 1)

	_, err := ParseWGWS([]byte(raw))
	if err == nil {
		t.Fatal("expected validation error")
	}
	validationErr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	if !strings.Contains(joined, "wstunnel.url host must be the real WSTunnel target hostname") {
		t.Fatalf("missing placeholder host error: %s", joined)
	}
}

func TestParseWGWSRejectsNonLoopbackLocalUDP(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, `"local_udp_listen": "127.0.0.1:51820"`, `"local_udp_listen": "0.0.0.0:51820"`, 1)

	_, err := ParseWGWS([]byte(raw))
	if err == nil {
		t.Fatal("expected validation error")
	}
	validationErr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	if !strings.Contains(joined, "wstunnel.local_udp_listen must be bound to loopback") {
		t.Fatalf("missing loopback error: %s", joined)
	}
}

func TestParseWGWSRejectsUnsafeMTUAndKeepalive(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, `"mtu": 1280`, `"mtu": 1500`, 1)
	raw = strings.Replace(raw, `"persistent_keepalive": 25`, `"persistent_keepalive": 90`, 1)

	_, err := ParseWGWS([]byte(raw))
	if err == nil {
		t.Fatal("expected validation error")
	}
	validationErr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	if !strings.Contains(joined, "wireguard.mtu must be in 1200..1420") {
		t.Fatalf("missing MTU error: %s", joined)
	}
	if !strings.Contains(joined, "wireguard.persistent_keepalive must be in 0..60") {
		t.Fatalf("missing keepalive error: %s", joined)
	}
}

func validProfileJSON() string {
	data, err := os.ReadFile("../../testdata/profiles/valid-wgws.json")
	if err != nil {
		panic(err)
	}
	return string(data)
}
