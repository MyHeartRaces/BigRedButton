package status

import (
	"context"
	"errors"
	"os"

	truntime "github.com/tracegate/big-red-button/internal/runtime"
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

func FromStore(ctx context.Context, store truntime.Store) Snapshot {
	state, err := store.Load(ctx)
	if err == nil {
		return Snapshot{
			State:       StateConnected,
			RuntimeRoot: store.Root,
			Active:      &state,
		}
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
