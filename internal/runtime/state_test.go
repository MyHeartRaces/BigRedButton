package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/profile"
)

func TestNewStateFromConnectPlanIsSecretFree(t *testing.T) {
	config := loadProfile(t)
	plan, err := planner.Connect(config, planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err := NewStateFromConnectPlan(plan)
	if err != nil {
		t.Fatal(err)
	}

	if state.ProfileFingerprint != config.Fingerprint() {
		t.Fatalf("fingerprint = %s", state.ProfileFingerprint)
	}
	if state.WireGuardInterface != "tg-v7" {
		t.Fatalf("interface = %s", state.WireGuardInterface)
	}
	if len(state.RouteExclusions) != 1 {
		t.Fatalf("route exclusions = %#v", state.RouteExclusions)
	}
	if len(state.WireGuardAllowedIPs) != 2 {
		t.Fatalf("allowed IPs = %#v", state.WireGuardAllowedIPs)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{config.WireGuardPrivateKey, config.ServerPublicKey, config.PresharedKey} {
		if secret != "" && strings.Contains(string(payload), secret) {
			t.Fatalf("runtime state leaked secret material: %s", payload)
		}
	}
}

func TestStateWithWSTunnelProcess(t *testing.T) {
	state := State{
		Version:            StateVersion,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "tg-test",
	}.WithWSTunnelProcess(4242, []string{"wstunnel", "client", "wss://edge.example.com:443"})

	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	if state.WSTunnelProcess == nil {
		t.Fatal("expected process state")
	}
	if state.WSTunnelProcess.PID != 4242 {
		t.Fatalf("pid = %d", state.WSTunnelProcess.PID)
	}
	if state.WSTunnelProcess.Argv[0] != "wstunnel" {
		t.Fatalf("argv = %#v", state.WSTunnelProcess.Argv)
	}
}

func TestStateRejectsInvalidWSTunnelProcess(t *testing.T) {
	state := State{
		Version:            StateVersion,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "tg-test",
		WSTunnelProcess:    &ProcessState{},
	}

	err := state.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "wstunnel process PID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreSaveLoadAndClear(t *testing.T) {
	store := Store{Root: t.TempDir()}
	state := State{
		Version:            StateVersion,
		Profile:            "test-profile",
		ProfileFingerprint: "abc123",
		WireGuardInterface: "tg-test",
	}

	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	path, err := store.Path()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProfileFingerprint != state.ProfileFingerprint {
		t.Fatalf("loaded state = %#v", loaded)
	}

	if err := store.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected state file to be removed, err = %v", err)
	}
	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("clear should be idempotent: %v", err)
	}
}

func TestStoreSaveTightensExistingFileMode(t *testing.T) {
	store := Store{Root: t.TempDir()}
	path, err := store.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = store.Save(context.Background(), State{
		Version:            StateVersion,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "tg-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}
}

func TestStoreRejectsEmptyRoot(t *testing.T) {
	store := Store{}
	_, err := store.Path()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidState(t *testing.T) {
	store := Store{Root: t.TempDir()}
	path, err := store.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = store.Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "profile fingerprint is required") {
		t.Fatalf("unexpected error: %v", err)
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
