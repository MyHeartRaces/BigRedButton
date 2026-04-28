package status

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strconv"
	"strings"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
)

type State string

const (
	StateIdle      State = "Idle"
	StateConnected State = "Connected"
	StateDirty     State = "Dirty"
)

type Snapshot struct {
	State       State           `json:"state"`
	RuntimeRoot string          `json:"runtime_root"`
	Active      *truntime.State `json:"active,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type IsolatedSessionSnapshot struct {
	SessionID string   `json:"session_id"`
	Snapshot  Snapshot `json:"snapshot"`
}

func FromStore(ctx context.Context, store truntime.Store) Snapshot {
	state, err := store.Load(ctx)
	if err == nil {
		return snapshotFromState(store.Root, &state)
	}
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{
			State:       StateIdle,
			RuntimeRoot: store.Root,
		}
	}
	return Snapshot{
		State:       StateDirty,
		RuntimeRoot: store.Root,
		Error:       err.Error(),
	}
}

func IsolatedSessions(ctx context.Context, runtimeRoot string) ([]IsolatedSessionSnapshot, error) {
	sessions, err := truntime.ListIsolatedSessions(ctx, runtimeRoot)
	if err != nil {
		return nil, err
	}
	snapshots := make([]IsolatedSessionSnapshot, 0, len(sessions))
	for _, session := range sessions {
		snapshot := Snapshot{RuntimeRoot: session.Root}
		if session.State != nil {
			snapshot = snapshotFromState(session.Root, session.State)
		} else if session.Error != "" {
			snapshot.State = StateDirty
			snapshot.Error = session.Error
		} else {
			snapshot.State = StateIdle
		}
		snapshots = append(snapshots, IsolatedSessionSnapshot{
			SessionID: session.SessionID,
			Snapshot:  snapshot,
		})
	}
	return snapshots, nil
}

func snapshotFromState(root string, state *truntime.State) Snapshot {
	snapshot := Snapshot{
		State:       StateConnected,
		RuntimeRoot: root,
		Active:      state,
	}
	markMissingIsolatedProcesses(&snapshot)
	return snapshot
}

func markMissingIsolatedProcesses(snapshot *Snapshot) {
	if snapshot == nil || snapshot.Active == nil || snapshot.Active.Mode != planner.IsolatedAppTunnelKind || stdruntime.GOOS != "linux" {
		return
	}
	var missing []string
	if process := snapshot.Active.AppProcess; process != nil && !linuxPIDExists(process.PID) {
		missing = append(missing, "app pid "+strconv.Itoa(process.PID))
	}
	if process := snapshot.Active.WSTunnelProcess; process != nil && !linuxPIDExists(process.PID) {
		missing = append(missing, "wstunnel pid "+strconv.Itoa(process.PID))
	}
	if len(missing) == 0 {
		return
	}
	snapshot.State = StateDirty
	snapshot.Error = "isolated session process is not running: " + strings.Join(missing, ", ")
}

func linuxPIDExists(pid int) bool {
	if pid < 1 {
		return false
	}
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil || !os.IsNotExist(err)
}
