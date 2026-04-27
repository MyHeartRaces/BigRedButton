package supervisor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tracegate/big-red-button/internal/profile"
)

func TestWSTunnelClientCommandFromProfile(t *testing.T) {
	config := loadProfile(t)
	command, err := WSTunnelClientCommand(WSTunnelClientConfigFromProfile(config, "/opt/tracegate/wstunnel"))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"/opt/tracegate/wstunnel",
		"client",
		"--log-lvl", "INFO",
		"--http-upgrade-path-prefix", "cdn/ws",
		"--tls-sni-override", "edge.example.com",
		"-L", "udp://127.0.0.1:51820:localhost:51820?timeout_sec=0",
		"wss://edge.example.com:443",
	}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}

	encoded := strings.Join(command.Argv(), " ")
	for _, secret := range []string{config.WireGuardPrivateKey, config.ServerPublicKey, config.PresharedKey} {
		if secret != "" && strings.Contains(encoded, secret) {
			t.Fatalf("wstunnel command leaked secret material: %s", encoded)
		}
	}
}

func TestWSTunnelClientCommandSupportsCustomRemoteUDP(t *testing.T) {
	command, err := WSTunnelClientCommand(WSTunnelClientConfig{
		Binary:         "wstunnel",
		ServerURL:      "wss://edge.example.com:443/cdn/ws",
		PathPrefix:     "/cdn/ws",
		LocalUDPListen: "127.0.0.1:51820",
		RemoteUDPHost:  "127.0.0.1",
		RemoteUDPPort:  51821,
		LogLevel:       "WARN",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"wstunnel",
		"client",
		"--log-lvl", "WARN",
		"--http-upgrade-path-prefix", "cdn/ws",
		"-L", "udp://127.0.0.1:51820:127.0.0.1:51821?timeout_sec=0",
		"wss://edge.example.com:443",
	}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}
}

func TestWSTunnelClientCommandSupportsIPv6Bind(t *testing.T) {
	command, err := WSTunnelClientCommand(WSTunnelClientConfig{
		Binary:         "wstunnel",
		ServerURL:      "wss://edge.example.com:443",
		LocalUDPListen: "[::1]:51820",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := command.Argv()[5]; got != "udp://[::1]:51820:localhost:51820?timeout_sec=0" {
		t.Fatalf("local-to-remote = %s", got)
	}
}

func TestWSTunnelClientCommandRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		config WSTunnelClientConfig
	}{
		{
			name: "empty binary",
			config: WSTunnelClientConfig{
				ServerURL:      "wss://edge.example.com:443",
				LocalUDPListen: "127.0.0.1:51820",
			},
		},
		{
			name: "bad scheme",
			config: WSTunnelClientConfig{
				Binary:         "wstunnel",
				ServerURL:      "ftp://edge.example.com:443",
				LocalUDPListen: "127.0.0.1:51820",
			},
		},
		{
			name: "bad local endpoint",
			config: WSTunnelClientConfig{
				Binary:         "wstunnel",
				ServerURL:      "wss://edge.example.com:443",
				LocalUDPListen: "not-an-endpoint",
			},
		},
		{
			name: "bad remote port",
			config: WSTunnelClientConfig{
				Binary:         "wstunnel",
				ServerURL:      "wss://edge.example.com:443",
				LocalUDPListen: "127.0.0.1:51820",
				RemoteUDPPort:  70000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := WSTunnelClientCommand(tt.config); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func loadProfile(t *testing.T) profile.Config {
	t.Helper()
	config, err := profile.LoadFile("../../testdata/profiles/valid-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	return config
}
