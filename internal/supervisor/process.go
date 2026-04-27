package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const DefaultStopTimeout = 5 * time.Second

type ProcessInfo struct {
	PID     int     `json:"pid"`
	Command Command `json:"command"`
}

type Process interface {
	Info() ProcessInfo
	Stop(ctx context.Context) error
}

type ProcessRunner interface {
	Start(ctx context.Context, command Command) (Process, error)
}

type ExecProcessRunner struct {
	StopTimeout time.Duration
}

func (r ExecProcessRunner) Start(ctx context.Context, command Command) (Process, error) {
	if command.Name == "" {
		return nil, fmt.Errorf("process command name is required")
	}
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command.String(), err)
	}
	timeout := r.StopTimeout
	if timeout == 0 {
		timeout = DefaultStopTimeout
	}
	return &execProcess{
		cmd:         cmd,
		info:        ProcessInfo{PID: cmd.Process.Pid, Command: command},
		stopTimeout: timeout,
		done:        waitAsync(cmd),
	}, nil
}

type execProcess struct {
	cmd         *exec.Cmd
	info        ProcessInfo
	stopTimeout time.Duration
	done        <-chan error
}

func (p *execProcess) Info() ProcessInfo {
	return p.info
}

func (p *execProcess) Stop(ctx context.Context) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-p.done:
		return err
	default:
	}

	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		if killErr := p.cmd.Process.Kill(); killErr != nil {
			return fmt.Errorf("interrupt process: %w; kill process: %w", err, killErr)
		}
		return <-p.done
	}

	timer := time.NewTimer(p.stopTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
		<-p.done
		return ctx.Err()
	case err := <-p.done:
		return err
	case <-timer.C:
		if err := p.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("kill process after stop timeout: %w", err)
		}
		return <-p.done
	}
}

func waitAsync(cmd *exec.Cmd) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	return done
}
