package supervisor

import (
	"context"
	"errors"
	"fmt"

	"github.com/tracegate/tracegate-launcher/internal/planner"
)

var ErrUnsupportedWSTunnelStep = errors.New("unsupported wstunnel executor step")

type WSTunnelOperationPhase string

const (
	WSTunnelOperationApply    WSTunnelOperationPhase = "apply"
	WSTunnelOperationRollback WSTunnelOperationPhase = "rollback"
)

type WSTunnelOperation struct {
	Phase   WSTunnelOperationPhase `json:"phase"`
	StepID  string                 `json:"step_id"`
	Command *Command               `json:"command,omitempty"`
	Process *ProcessInfo           `json:"process,omitempty"`
}

type WSTunnelExecutor struct {
	command    Command
	runner     ProcessRunner
	process    Process
	operations []WSTunnelOperation
}

type WSTunnelExecutorOptions struct {
	Command Command
	Runner  ProcessRunner
}

func NewWSTunnelExecutor(options WSTunnelExecutorOptions) (*WSTunnelExecutor, error) {
	if options.Command.Name == "" {
		return nil, fmt.Errorf("wstunnel command is required")
	}
	runner := options.Runner
	if runner == nil {
		runner = ExecProcessRunner{}
	}
	return &WSTunnelExecutor{
		command: options.Command,
		runner:  runner,
	}, nil
}

func (e *WSTunnelExecutor) Apply(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("wstunnel executor is nil")
	}
	if step.ID != "start-wstunnel" {
		return fmt.Errorf("%w: %s", ErrUnsupportedWSTunnelStep, step.ID)
	}
	process, err := e.runner.Start(ctx, e.command)
	if err != nil {
		return err
	}
	e.process = process
	info := process.Info()
	e.operations = append(e.operations, WSTunnelOperation{
		Phase:   WSTunnelOperationApply,
		StepID:  step.ID,
		Command: &e.command,
		Process: &info,
	})
	return nil
}

func (e *WSTunnelExecutor) Rollback(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("wstunnel executor is nil")
	}
	if step.ID != "start-wstunnel" {
		return fmt.Errorf("%w: %s", ErrUnsupportedWSTunnelStep, step.ID)
	}
	if e.process == nil {
		return nil
	}
	if err := e.process.Stop(ctx); err != nil {
		return err
	}
	info := e.process.Info()
	e.operations = append(e.operations, WSTunnelOperation{
		Phase:   WSTunnelOperationRollback,
		StepID:  step.ID,
		Process: &info,
	})
	e.process = nil
	return nil
}

func (e *WSTunnelExecutor) ProcessInfo() (ProcessInfo, bool) {
	if e == nil || e.process == nil {
		return ProcessInfo{}, false
	}
	return e.process.Info(), true
}

func (e *WSTunnelExecutor) Operations() []WSTunnelOperation {
	if e == nil {
		return nil
	}
	operations := make([]WSTunnelOperation, len(e.operations))
	copy(operations, e.operations)
	return operations
}
