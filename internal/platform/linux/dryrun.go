package linux

import (
	"context"
	"fmt"
	"strings"

	"github.com/tracegate/tracegate-launcher/internal/planner"
	"github.com/tracegate/tracegate-launcher/internal/routes"
)

type OperationPhase string

const (
	OperationApply    OperationPhase = "apply"
	OperationRollback OperationPhase = "rollback"
)

type Operation struct {
	Phase   OperationPhase `json:"phase"`
	StepID  string         `json:"step_id"`
	Command Command        `json:"command"`
}

type DryRunExecutor struct {
	routeExclusionsByEndpoint map[string]routes.EndpointExclusion
	discoveredByEndpoint      map[string]routes.EndpointExclusion
	runner                    CommandRunner
	readOnlyDiscovery         bool
	operations                []Operation
}

type DryRunOptions struct {
	ReadOnlyDiscovery bool
	Runner            CommandRunner
}

func NewDryRunExecutor(plan planner.Plan) (*DryRunExecutor, error) {
	return NewDryRunExecutorWithOptions(plan, DryRunOptions{})
}

func NewDryRunExecutorWithOptions(plan planner.Plan, options DryRunOptions) (*DryRunExecutor, error) {
	routeExclusionsByEndpoint := make(map[string]routes.EndpointExclusion, len(plan.RouteExclusions))
	for _, exclusion := range plan.RouteExclusions {
		if _, err := AddEndpointExclusionCommand(exclusion); err != nil {
			return nil, err
		}
		routeExclusionsByEndpoint[exclusion.EndpointIP] = exclusion
	}
	runner := options.Runner
	if options.ReadOnlyDiscovery && runner == nil {
		runner = ExecRunner{}
	}
	return &DryRunExecutor{
		routeExclusionsByEndpoint: routeExclusionsByEndpoint,
		discoveredByEndpoint:      make(map[string]routes.EndpointExclusion),
		runner:                    runner,
		readOnlyDiscovery:         options.ReadOnlyDiscovery,
	}, nil
}

func (e *DryRunExecutor) Apply(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux dry-run executor is nil")
	}

	switch {
	case strings.HasPrefix(step.ID, "snapshot-route-"):
		endpointIP, err := endpointIPFromStep(step)
		if err != nil {
			return err
		}
		command, err := RouteGetCommand(endpointIP)
		if err != nil {
			return err
		}
		e.record(OperationApply, step.ID, command)
		if e.readOnlyDiscovery {
			output, err := e.runner.Run(ctx, command)
			if err != nil {
				detail := strings.TrimSpace(string(output))
				if detail == "" {
					return fmt.Errorf("run %s: %w", command.String(), err)
				}
				return fmt.Errorf("run %s: %w: %s", command.String(), err, detail)
			}
			exclusion, err := EndpointExclusionFromRouteGet(string(output))
			if err != nil {
				return err
			}
			e.discoveredByEndpoint[exclusion.EndpointIP] = exclusion
		}
	case strings.HasPrefix(step.ID, "add-route-exclusion-"):
		exclusion, err := e.routeExclusionForStep(step)
		if err != nil {
			return err
		}
		command, err := AddEndpointExclusionCommand(exclusion)
		if err != nil {
			return err
		}
		e.record(OperationApply, step.ID, command)
	}
	return nil
}

func (e *DryRunExecutor) Rollback(_ context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux dry-run executor is nil")
	}
	if !strings.HasPrefix(step.ID, "add-route-exclusion-") {
		return nil
	}

	exclusion, err := e.routeExclusionForStep(step)
	if err != nil {
		return err
	}
	command, err := DeleteEndpointExclusionCommand(exclusion)
	if err != nil {
		return err
	}
	e.record(OperationRollback, step.ID, command)
	return nil
}

func (e *DryRunExecutor) Operations() []Operation {
	if e == nil {
		return nil
	}
	operations := make([]Operation, len(e.operations))
	copy(operations, e.operations)
	return operations
}

func (e *DryRunExecutor) record(phase OperationPhase, stepID string, command Command) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Command: command,
	})
}

func (e *DryRunExecutor) routeExclusionForStep(step planner.Step) (routes.EndpointExclusion, error) {
	endpointIP, err := endpointIPFromStep(step)
	if err != nil {
		return routes.EndpointExclusion{}, err
	}
	if exclusion, ok := e.routeExclusionsByEndpoint[endpointIP]; ok {
		return exclusion, nil
	}
	if exclusion, ok := e.discoveredByEndpoint[endpointIP]; ok {
		return exclusion, nil
	}
	return routes.EndpointExclusion{}, fmt.Errorf("route exclusion for endpoint %s is not resolved", endpointIP)
}

func endpointIPFromStep(step planner.Step) (string, error) {
	for _, detail := range step.Details {
		if endpointIP, ok := strings.CutPrefix(detail, "endpoint_ip="); ok {
			endpointIP = strings.TrimSpace(endpointIP)
			if endpointIP == "" {
				break
			}
			return endpointIP, nil
		}
	}
	return "", fmt.Errorf("step %s does not contain endpoint_ip detail", step.ID)
}
