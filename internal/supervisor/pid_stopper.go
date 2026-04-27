package supervisor

import (
	"context"
	"fmt"
	"os"
)

type ProcessStopper interface {
	StopPID(ctx context.Context, pid int) error
}

type OSProcessStopper struct{}

func (OSProcessStopper) StopPID(ctx context.Context, pid int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pid < 1 {
		return fmt.Errorf("process PID is required")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("interrupt process %d: %w", pid, err)
	}
	return nil
}
