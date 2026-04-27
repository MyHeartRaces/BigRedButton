package wireguard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tracegate/big-red-button/internal/profile"
)

func TestRenderSetConf(t *testing.T) {
	profileConfig := loadProfile(t)
	config := ConfigFromProfile(profileConfig, "tg-v7")

	rendered, err := RenderSetConf(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[Interface]",
		"PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"[Peer]",
		"PublicKey = CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
		"PresharedKey = DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD=",
		"AllowedIPs = 0.0.0.0/0, ::/0",
		"Endpoint = 127.0.0.1:51820",
		"PersistentKeepalive = 25",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered config:\n%s", want, rendered)
		}
	}
}

func TestSummaryIsSecretFree(t *testing.T) {
	profileConfig := loadProfile(t)
	config := ConfigFromProfile(profileConfig, "tg-v7")
	summary := config.Summary()

	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{profileConfig.WireGuardPrivateKey, profileConfig.ServerPublicKey, profileConfig.PresharedKey} {
		if secret != "" && strings.Contains(string(payload), secret) {
			t.Fatalf("summary leaked secret material: %s", payload)
		}
	}
	if !summary.HasPresharedKey {
		t.Fatal("expected preshared key flag")
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	config := ConfigFromProfile(loadProfile(t), "tg-v7")
	config.AllowedIPs = []string{"not-a-prefix"}

	if err := config.Validate(); err == nil {
		t.Fatal("expected error")
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
