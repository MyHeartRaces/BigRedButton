package profile

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseV7ValidFixture(t *testing.T) {
	data, err := os.ReadFile("../../testdata/profiles/valid-v7.json")
	if err != nil {
		t.Fatal(err)
	}

	config, err := ParseV7(data)
	if err != nil {
		t.Fatalf("ParseV7() error = %v", err)
	}

	if config.Name != ExpectedName {
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

func TestSummaryDoesNotExposeSecrets(t *testing.T) {
	data, err := os.ReadFile("../../testdata/profiles/valid-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	config, err := ParseV7(data)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(config.Summary())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
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

func TestParseV7RejectsPlaceholders(t *testing.T) {
	data, err := os.ReadFile("../../testdata/profiles/invalid-placeholder-v7.json")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseV7(data)
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

func TestParseV7RejectsUnsafeWSTunnelURL(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, "wss://edge.example.com:443/cdn/ws", "http://edge.example.com:80/cdn/ws?debug=1", 1)

	_, err := ParseV7([]byte(raw))
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

func TestParseV7RejectsNonLoopbackLocalUDP(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, `"local_udp_listen": "127.0.0.1:51820"`, `"local_udp_listen": "0.0.0.0:51820"`, 1)

	_, err := ParseV7([]byte(raw))
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

func TestParseV7RejectsUnsafeMTUAndKeepalive(t *testing.T) {
	raw := validProfileJSON()
	raw = strings.Replace(raw, `"mtu": 1280`, `"mtu": 1500`, 1)
	raw = strings.Replace(raw, `"persistent_keepalive": 25`, `"persistent_keepalive": 90`, 1)

	_, err := ParseV7([]byte(raw))
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
	data, err := os.ReadFile("../../testdata/profiles/valid-v7.json")
	if err != nil {
		panic(err)
	}
	return string(data)
}
