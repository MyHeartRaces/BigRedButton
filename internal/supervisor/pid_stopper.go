package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
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
		if isProcessAlreadyDone(err) {
			return nil
		}
		return fmt.Errorf("interrupt process %d: %w", pid, err)
	}
	return nil
}

func isProcessAlreadyDone(err error) bool {
	if errors.Is(err, os.ErrProcessDone) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.ESRCH
}
