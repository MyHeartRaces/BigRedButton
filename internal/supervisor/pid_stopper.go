package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ProcessStopper interface {
	StopPID(ctx context.Context, pid int) error
}

type OSProcessStopper struct {
	GracePeriod  time.Duration
	PollInterval time.Duration
}

func (s OSProcessStopper) StopPID(ctx context.Context, pid int) error {
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
	if s.waitUntilDone(ctx, pid) {
		return nil
	}
	if err := process.Kill(); err != nil {
		if isProcessAlreadyDone(err) {
			return nil
		}
		return fmt.Errorf("kill process %d after stop timeout: %w", pid, err)
	}
	_ = s.waitUntilDone(ctx, pid)
	return nil
}

func (s OSProcessStopper) waitUntilDone(ctx context.Context, pid int) bool {
	gracePeriod := s.GracePeriod
	if gracePeriod == 0 {
		gracePeriod = DefaultStopTimeout
	}
	pollInterval := s.PollInterval
	if pollInterval == 0 {
		pollInterval = 100 * time.Millisecond
	}
	deadline := time.NewTimer(gracePeriod)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if !pidExists(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

func isProcessAlreadyDone(err error) bool {
	if errors.Is(err, os.ErrProcessDone) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.ESRCH
}

func pidExists(pid int) bool {
	if pid < 1 {
		return false
	}
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil || !os.IsNotExist(err)
}

func PIDArgvMatches(pid int, expected []string) bool {
	if len(expected) == 0 || runtime.GOOS != "linux" {
		return true
	}
	actual, err := readLinuxPIDArgv(pid)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		return false
	}
	return argvMatches(expected, actual)
}

func readLinuxPIDArgv(pid int) ([]string, error) {
	if pid < 1 {
		return nil, fmt.Errorf("process PID is required")
	}
	payload, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return nil, err
	}
	raw := strings.Split(strings.TrimRight(string(payload), "\x00"), "\x00")
	out := make([]string, 0, len(raw))
	for _, arg := range raw {
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out, nil
}

func argvMatches(expected []string, actual []string) bool {
	if len(expected) == 0 {
		return true
	}
	if len(expected) != len(actual) || len(actual) == 0 {
		return false
	}
	if expected[0] != actual[0] && filepath.Base(expected[0]) != filepath.Base(actual[0]) {
		return false
	}
	for index := 1; index < len(expected); index++ {
		if expected[index] != actual[index] {
			return false
		}
	}
	return true
}
