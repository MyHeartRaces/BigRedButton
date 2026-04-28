package status

import (
	"context"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"testing"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
)

func TestFromStoreIdleWhenStateMissing(t *testing.T) {
	store := truntime.Store{Root: t.TempDir()}

	snapshot := FromStore(context.Background(), store)

	if snapshot.State != StateIdle {
		t.Fatalf("state = %s", snapshot.State)
	}
	if snapshot.Active != nil {
		t.Fatalf("active = %#v", snapshot.Active)
	}
}

func TestFromStoreConnectedWhenStateExists(t *testing.T) {
	store := truntime.Store{Root: t.TempDir()}
	state := truntime.State{
		Version:            truntime.StateVersion,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "tg-test",
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	snapshot := FromStore(context.Background(), store)

	if snapshot.State != StateConnected {
		t.Fatalf("state = %s error = %s", snapshot.State, snapshot.Error)
	}
	if snapshot.Active == nil || snapshot.Active.ProfileFingerprint != "abc123" {
		t.Fatalf("active = %#v", snapshot.Active)
	}
}

func TestFromStoreDirtyWhenStateIsInvalid(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	snapshot := FromStore(context.Background(), truntime.Store{Root: root})

	if snapshot.State != StateDirty {
		t.Fatalf("state = %s", snapshot.State)
	}
	if !strings.Contains(snapshot.Error, "profile fingerprint is required") {
		t.Fatalf("error = %s", snapshot.Error)
	}
}

func TestFromStoreDirtyWhenLinuxIsolatedProcessIsMissing(t *testing.T) {
	if stdruntime.GOOS != "linux" {
		t.Skip("Linux /proc process health check")
	}
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	store := truntime.Store{Root: t.TempDir()}
	err := store.Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		Mode:               planner.IsolatedAppTunnelKind,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "brbwg123e4567",
		SessionID:          sessionID,
		Namespace:          "brb-123e4567",
		HostVeth:           "brbh123e4567",
		NamespaceVeth:      "brbn123e4567",
	}.WithAppProcess(999999999, []string{"missing"}))
	if err != nil {
		t.Fatal(err)
	}

	snapshot := FromStore(context.Background(), store)

	if snapshot.State != StateDirty {
		t.Fatalf("state = %s", snapshot.State)
	}
	if !strings.Contains(snapshot.Error, "app pid 999999999") {
		t.Fatalf("error = %s", snapshot.Error)
	}
}

func TestIsolatedSessions(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionID := "123e4567-e89b-12d3-a456-426614174000"
	store := truntime.Store{Root: filepath.Join(runtimeRoot, planner.DefaultIsolatedRuntimeSubdir, sessionID)}
	err := store.Save(context.Background(), truntime.State{
		Version:            truntime.StateVersion,
		Mode:               planner.IsolatedAppTunnelKind,
		ProfileFingerprint: "abc123",
		WireGuardInterface: "brbwg123e4567",
		SessionID:          sessionID,
		Namespace:          "brb-123e4567",
		HostVeth:           "brbh123e4567",
		NamespaceVeth:      "brbn123e4567",
	})
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := IsolatedSessions(context.Background(), runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v", sessions)
	}
	if sessions[0].SessionID != sessionID || sessions[0].Snapshot.State != StateConnected {
		t.Fatalf("session = %#v", sessions[0])
	}
}
