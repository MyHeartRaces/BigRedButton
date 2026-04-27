package status

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
