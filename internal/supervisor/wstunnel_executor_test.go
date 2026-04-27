package supervisor

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
)

func TestWSTunnelExecutorStartsAndRollsBackProcess(t *testing.T) {
	command := Command{Name: "wstunnel", Args: []string{"client", "-L", "udp://127.0.0.1:51820:localhost:51820?timeout_sec=0", "wss://edge.example.com:443"}}
	runner := &fakeProcessRunner{process: &fakeProcess{info: ProcessInfo{PID: 4242, Command: command}}}
	executor, err := NewWSTunnelExecutor(WSTunnelExecutorOptions{
		Command: command,
		Runner:  runner,
	})
	if err != nil {
		t.Fatal(err)
	}

	step := planner.Step{ID: "start-wstunnel"}
	if err := executor.Apply(context.Background(), step); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runner.started, []Command{command}) {
		t.Fatalf("started = %#v", runner.started)
	}
	info, ok := executor.ProcessInfo()
	if !ok {
		t.Fatal("expected process info")
	}
	if info.PID != 4242 {
		t.Fatalf("pid = %d", info.PID)
	}

	if err := executor.Rollback(context.Background(), step); err != nil {
		t.Fatal(err)
	}
	if !runner.process.stopped {
		t.Fatal("expected process stop")
	}
	if _, ok := executor.ProcessInfo(); ok {
		t.Fatal("expected no active process after rollback")
	}

	operations := executor.Operations()
	if len(operations) != 2 {
		t.Fatalf("operations = %#v", operations)
	}
	if operations[0].Phase != WSTunnelOperationApply || operations[1].Phase != WSTunnelOperationRollback {
		t.Fatalf("operations = %#v", operations)
	}
}

func TestWSTunnelExecutorReturnsStartFailure(t *testing.T) {
	command := Command{Name: "wstunnel"}
	executor, err := NewWSTunnelExecutor(WSTunnelExecutorOptions{
		Command: command,
		Runner:  &fakeProcessRunner{err: errors.New("start failed")},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = executor.Apply(context.Background(), planner.Step{ID: "start-wstunnel"})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(executor.Operations()) != 0 {
		t.Fatalf("unexpected operations: %#v", executor.Operations())
	}
}

func TestWSTunnelExecutorRejectsUnsupportedStep(t *testing.T) {
	executor, err := NewWSTunnelExecutor(WSTunnelExecutorOptions{
		Command: Command{Name: "wstunnel"},
		Runner:  &fakeProcessRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = executor.Apply(context.Background(), planner.Step{ID: "apply-wireguard-peer"})
	if !errors.Is(err, ErrUnsupportedWSTunnelStep) {
		t.Fatalf("expected unsupported step error, got %v", err)
	}
}

type fakeProcessRunner struct {
	process *fakeProcess
	started []Command
	err     error
}

func (f *fakeProcessRunner) Start(_ context.Context, command Command) (Process, error) {
	f.started = append(f.started, command)
	if f.err != nil {
		return nil, f.err
	}
	if f.process == nil {
		f.process = &fakeProcess{info: ProcessInfo{PID: 1, Command: command}}
	}
	return f.process, nil
}

type fakeProcess struct {
	info    ProcessInfo
	stopped bool
	err     error
}

func (f *fakeProcess) Info() ProcessInfo {
	return f.info
}

func (f *fakeProcess) Stop(context.Context) error {
	f.stopped = true
	return f.err
}
