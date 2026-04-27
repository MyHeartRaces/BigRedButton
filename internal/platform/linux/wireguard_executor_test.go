package linux

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tracegate/big-red-button/internal/planner"
	"github.com/tracegate/big-red-button/internal/profile"
	"github.com/tracegate/big-red-button/internal/wireguard"
)

func TestWireGuardExecutorAppliesInterfacePeerAndRoutes(t *testing.T) {
	config := wireGuardConfig(t)
	runner := &recordingRunner{}
	writer := &memoryConfigWriter{path: "/run/big-red-button/wg-setconf.conf"}
	executor, err := NewWireGuardExecutor(WireGuardExecutorOptions{
		Config:       config,
		Runner:       runner,
		ConfigWriter: writer,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range []planner.Step{
		{ID: "create-wireguard-interface"},
		{ID: "apply-wireguard-addresses"},
		{ID: "apply-wireguard-peer"},
		{ID: "apply-client-routes"},
	} {
		if err := executor.Apply(context.Background(), step); err != nil {
			t.Fatalf("apply %s: %v", step.ID, err)
		}
	}

	want := [][]string{
		{"ip", "link", "add", "dev", "tg-v7", "type", "wireguard"},
		{"ip", "address", "add", "10.70.0.2/32", "dev", "tg-v7"},
		{"ip", "link", "set", "mtu", "1280", "dev", "tg-v7"},
		{"ip", "link", "set", "up", "dev", "tg-v7"},
		{"wg", "setconf", "tg-v7", "/run/big-red-button/wg-setconf.conf"},
		{"ip", "-4", "route", "replace", "0.0.0.0/0", "dev", "tg-v7"},
		{"ip", "-6", "route", "replace", "::/0", "dev", "tg-v7"},
	}
	if !reflect.DeepEqual(runner.argv, want) {
		t.Fatalf("commands = %#v want %#v", runner.argv, want)
	}
	if !strings.Contains(writer.rendered, "PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") {
		t.Fatalf("expected rendered private key, got:\n%s", writer.rendered)
	}
	if !writer.cleaned {
		t.Fatal("expected temporary wireguard config cleanup")
	}
	assertOperationsDoNotLeakWireGuardSecrets(t, executor.Operations(), config)
}

func TestWireGuardExecutorDisconnectRemovesRoutesAndInterface(t *testing.T) {
	config := wireGuardConfig(t)
	runner := &recordingRunner{}
	executor, err := NewWireGuardExecutor(WireGuardExecutorOptions{
		Config: config,
		Runner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range []planner.Step{
		{ID: "remove-client-routes"},
		{ID: "remove-wireguard-interface"},
	} {
		if err := executor.Apply(context.Background(), step); err != nil {
			t.Fatalf("apply %s: %v", step.ID, err)
		}
	}

	want := [][]string{
		{"ip", "-4", "route", "delete", "0.0.0.0/0", "dev", "tg-v7"},
		{"ip", "-6", "route", "delete", "::/0", "dev", "tg-v7"},
		{"ip", "link", "delete", "dev", "tg-v7"},
	}
	if !reflect.DeepEqual(runner.argv, want) {
		t.Fatalf("commands = %#v want %#v", runner.argv, want)
	}
}

func TestWireGuardExecutorRollbackDeletesInterface(t *testing.T) {
	config := wireGuardConfig(t)
	runner := &recordingRunner{}
	executor, err := NewWireGuardExecutor(WireGuardExecutorOptions{
		Config: config,
		Runner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := executor.Rollback(context.Background(), planner.Step{ID: "create-wireguard-interface"}); err != nil {
		t.Fatal(err)
	}

	want := [][]string{{"ip", "link", "delete", "dev", "tg-v7"}}
	if !reflect.DeepEqual(runner.argv, want) {
		t.Fatalf("commands = %#v want %#v", runner.argv, want)
	}
}

func TestWireGuardExecutorReturnsCommandOutputOnFailure(t *testing.T) {
	config := wireGuardConfig(t)
	runner := &recordingRunner{
		errs: map[string]error{
			"ip link add dev tg-v7 type wireguard": errors.New("exit status 2"),
		},
		outputs: map[string]string{
			"ip link add dev tg-v7 type wireguard": "RTNETLINK answers: Operation not permitted",
		},
	}
	executor, err := NewWireGuardExecutor(WireGuardExecutorOptions{
		Config: config,
		Runner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = executor.Apply(context.Background(), planner.Step{ID: "create-wireguard-interface"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Operation not permitted") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWireGuardExecutorRejectsUnsupportedStep(t *testing.T) {
	executor, err := NewWireGuardExecutor(WireGuardExecutorOptions{
		Config: wireGuardConfig(t),
		Runner: &recordingRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = executor.Apply(context.Background(), planner.Step{ID: "start-wstunnel"})
	if !errors.Is(err, ErrUnsupportedWireGuardStep) {
		t.Fatalf("expected unsupported step error, got %v", err)
	}
}

func wireGuardConfig(t *testing.T) wireguard.Config {
	t.Helper()
	config, err := profile.LoadFile("../../../testdata/profiles/valid-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	return wireguard.ConfigFromProfile(config, "tg-v7")
}

func assertOperationsDoNotLeakWireGuardSecrets(t *testing.T, operations []Operation, config wireguard.Config) {
	t.Helper()
	encoded := operationStrings(operations)
	for _, secret := range []string{config.PrivateKey, config.PeerPublicKey, config.PresharedKey} {
		if secret != "" && strings.Contains(encoded, secret) {
			t.Fatalf("operations leaked secret material: %s", encoded)
		}
	}
}

func operationStrings(operations []Operation) string {
	var builder strings.Builder
	for _, operation := range operations {
		if operation.Command != nil {
			builder.WriteString(operation.Command.String())
			builder.WriteByte('\n')
		}
		if operation.Runtime != "" {
			builder.WriteString(operation.Runtime)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

type memoryConfigWriter struct {
	path     string
	rendered string
	cleaned  bool
}

func (w *memoryConfigWriter) WriteConfig(_ context.Context, rendered string) (string, func(context.Context) error, error) {
	w.rendered = rendered
	cleanup := func(context.Context) error {
		w.cleaned = true
		return nil
	}
	return w.path, cleanup, nil
}
