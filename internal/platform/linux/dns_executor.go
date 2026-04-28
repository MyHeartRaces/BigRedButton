package linux

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
)

var ErrUnsupportedDNSStep = errors.New("unsupported linux DNS executor step")

type DNSExecutor struct {
	plan       planner.Plan
	runner     CommandRunner
	operations []Operation
}

func NewDNSExecutor(plan planner.Plan, runner CommandRunner) *DNSExecutor {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &DNSExecutor{
		plan:   plan,
		runner: runner,
	}
}

func (e *DNSExecutor) Apply(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux DNS executor is nil")
	}
	if step.ID != "apply-dns" {
		return fmt.Errorf("%w: %s", ErrUnsupportedDNSStep, step.ID)
	}
	commands, err := e.applyCommands()
	if err != nil {
		return err
	}
	for index, command := range commands {
		if _, err := e.run(ctx, OperationApply, step.ID, command); err != nil {
			if index > 0 {
				if rollbackErr := e.rollbackPlan(ctx, step.ID); rollbackErr != nil {
					return fmt.Errorf("%w; DNS rollback failed: %v", err, rollbackErr)
				}
			}
			return err
		}
	}
	return nil
}

func (e *DNSExecutor) Restore(ctx context.Context, step planner.Step, state truntime.State) error {
	if e == nil {
		return fmt.Errorf("linux DNS executor is nil")
	}
	if step.ID != "restore-dns" {
		return fmt.Errorf("%w: %s", ErrUnsupportedDNSStep, step.ID)
	}
	if !state.DNSApplied {
		e.recordRuntime(OperationApply, step.ID, "no launcher-owned DNS state")
		return nil
	}
	iface := strings.TrimSpace(state.DNSInterface)
	if iface == "" {
		iface = state.WireGuardInterface
	}
	command, err := ResolveCtlRevertCommand(iface)
	if err != nil {
		return err
	}
	_, err = e.run(ctx, OperationApply, step.ID, command)
	return err
}

func (e *DNSExecutor) Rollback(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux DNS executor is nil")
	}
	if step.ID != "apply-dns" {
		return nil
	}
	return e.rollbackPlan(ctx, step.ID)
}

func (e *DNSExecutor) Operations() []Operation {
	if e == nil {
		return nil
	}
	operations := make([]Operation, len(e.operations))
	copy(operations, e.operations)
	return operations
}

func (e *DNSExecutor) applyCommands() ([]Command, error) {
	if len(e.plan.DNSServers) == 0 {
		return nil, nil
	}
	dnsCommand, err := ResolveCtlDNSCommand(e.plan.WireGuardInterface, e.plan.DNSServers)
	if err != nil {
		return nil, err
	}
	domainCommand, err := ResolveCtlDomainCommand(e.plan.WireGuardInterface, []string{"~."})
	if err != nil {
		return nil, err
	}
	defaultRouteCommand, err := ResolveCtlDefaultRouteCommand(e.plan.WireGuardInterface, true)
	if err != nil {
		return nil, err
	}
	return []Command{dnsCommand, domainCommand, defaultRouteCommand}, nil
}

func (e *DNSExecutor) rollbackPlan(ctx context.Context, stepID string) error {
	if len(e.plan.DNSServers) == 0 {
		return nil
	}
	command, err := ResolveCtlRevertCommand(e.plan.WireGuardInterface)
	if err != nil {
		return err
	}
	_, err = e.run(ctx, OperationRollback, stepID, command)
	return err
}

func (e *DNSExecutor) run(ctx context.Context, phase OperationPhase, stepID string, command Command) ([]byte, error) {
	e.record(phase, stepID, command)
	output, err := e.runner.Run(ctx, command)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return output, fmt.Errorf("run %s: %w", command.String(), err)
		}
		return output, fmt.Errorf("run %s: %w: %s", command.String(), err, detail)
	}
	return output, nil
}

func (e *DNSExecutor) record(phase OperationPhase, stepID string, command Command) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Command: &command,
	})
}

func (e *DNSExecutor) recordRuntime(phase OperationPhase, stepID string, runtime string) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Runtime: runtime,
	})
}
