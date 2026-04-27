package linux

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tracegate/big-red-button/internal/planner"
	"github.com/tracegate/big-red-button/internal/routes"
	truntime "github.com/tracegate/big-red-button/internal/runtime"
)

var ErrUnsupportedRouteStep = errors.New("unsupported linux route executor step")

type RouteExecutor struct {
	plan                      planner.Plan
	routeExclusionsByEndpoint map[string]routes.EndpointExclusion
	discoveredByEndpoint      map[string]routes.EndpointExclusion
	runtimeState              truntime.State
	runner                    CommandRunner
	store                     truntime.Store
	operations                []Operation
}

type RouteExecutorOptions struct {
	Runner      CommandRunner
	RuntimeRoot string
}

func NewRouteExecutor(plan planner.Plan, options RouteExecutorOptions) (*RouteExecutor, error) {
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = plan.RuntimeRoot
	}

	routeExclusionsByEndpoint := routeExclusionMap(plan.RouteExclusions)
	for _, exclusion := range routeExclusionsByEndpoint {
		if _, err := AddEndpointExclusionCommand(exclusion); err != nil {
			return nil, err
		}
	}

	return &RouteExecutor{
		plan:                      plan,
		routeExclusionsByEndpoint: routeExclusionsByEndpoint,
		discoveredByEndpoint:      make(map[string]routes.EndpointExclusion),
		runner:                    runner,
		store:                     truntime.Store{Root: runtimeRoot},
	}, nil
}

func RouteSteps(plan planner.Plan) []planner.Step {
	steps := make([]planner.Step, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		if isRouteExecutorStep(plan.Kind, step) {
			steps = append(steps, step)
		}
	}
	return steps
}

func (e *RouteExecutor) Apply(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux route executor is nil")
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
		output, err := e.run(ctx, OperationApply, step.ID, command)
		if err != nil {
			return err
		}
		exclusion, err := EndpointExclusionFromRouteGet(string(output))
		if err != nil {
			return err
		}
		e.discoveredByEndpoint[exclusion.EndpointIP] = exclusion
	case strings.HasPrefix(step.ID, "add-route-exclusion-"):
		exclusion, err := e.routeExclusionForStep(step)
		if err != nil {
			return err
		}
		command, err := AddEndpointExclusionCommand(exclusion)
		if err != nil {
			return err
		}
		if _, err := e.run(ctx, OperationApply, step.ID, command); err != nil {
			return err
		}
	case step.ID == "store-runtime-state":
		state, err := e.stateFromPlan()
		if err != nil {
			return err
		}
		if err := e.store.Save(ctx, state); err != nil {
			return err
		}
		e.runtimeState = state
		e.recordRuntime(OperationApply, step.ID, "save "+e.storePath())
	case step.ID == "read-runtime-state":
		state, err := e.store.Load(ctx)
		if err != nil {
			return err
		}
		e.runtimeState = state
		e.routeExclusionsByEndpoint = routeExclusionMap(state.RouteExclusions)
		e.recordRuntime(OperationApply, step.ID, "load "+e.storePath())
	case step.ID == "remove-endpoint-route-exclusions":
		state, err := e.stateForDisconnect(ctx)
		if err != nil {
			return err
		}
		for _, exclusion := range state.RouteExclusions {
			command, err := DeleteEndpointExclusionCommand(exclusion)
			if err != nil {
				return err
			}
			if _, err := e.run(ctx, OperationApply, step.ID, command); err != nil {
				return err
			}
		}
	case step.ID == "clear-runtime-state":
		if err := e.store.Clear(ctx); err != nil {
			return err
		}
		e.recordRuntime(OperationApply, step.ID, "clear "+e.storePath())
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedRouteStep, step.ID)
	}
	return nil
}

func (e *RouteExecutor) Rollback(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("linux route executor is nil")
	}
	if !strings.HasPrefix(step.ID, "add-route-exclusion-") {
		return fmt.Errorf("%w: %s", ErrUnsupportedRouteStep, step.ID)
	}

	exclusion, err := e.routeExclusionForStep(step)
	if err != nil {
		return err
	}
	command, err := DeleteEndpointExclusionCommand(exclusion)
	if err != nil {
		return err
	}
	_, err = e.run(ctx, OperationRollback, step.ID, command)
	return err
}

func (e *RouteExecutor) Operations() []Operation {
	if e == nil {
		return nil
	}
	operations := make([]Operation, len(e.operations))
	copy(operations, e.operations)
	return operations
}

func (e *RouteExecutor) RuntimeState() (truntime.State, bool) {
	if e == nil || e.runtimeState.Version == 0 {
		return truntime.State{}, false
	}
	return e.runtimeState, true
}

func (e *RouteExecutor) run(ctx context.Context, phase OperationPhase, stepID string, command Command) ([]byte, error) {
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

func (e *RouteExecutor) record(phase OperationPhase, stepID string, command Command) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Command: &command,
	})
}

func (e *RouteExecutor) recordRuntime(phase OperationPhase, stepID string, runtime string) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Runtime: runtime,
	})
}

func (e *RouteExecutor) storePath() string {
	path, err := e.store.Path()
	if err != nil {
		return e.store.Root
	}
	return path
}

func (e *RouteExecutor) routeExclusionForStep(step planner.Step) (routes.EndpointExclusion, error) {
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

func (e *RouteExecutor) stateFromPlan() (truntime.State, error) {
	state, err := truntime.NewStateFromConnectPlan(e.plan)
	if err != nil {
		return truntime.State{}, err
	}
	state.RouteExclusions = mergeRouteExclusions(state.RouteExclusions, e.discoveredByEndpoint)
	if err := state.Validate(); err != nil {
		return truntime.State{}, err
	}
	return state, nil
}

func (e *RouteExecutor) stateForDisconnect(ctx context.Context) (truntime.State, error) {
	if e.runtimeState.Version != 0 {
		return e.runtimeState, nil
	}
	state, err := e.store.Load(ctx)
	if err != nil {
		return truntime.State{}, err
	}
	e.runtimeState = state
	e.routeExclusionsByEndpoint = routeExclusionMap(state.RouteExclusions)
	return state, nil
}

func isRouteExecutorStep(planKind string, step planner.Step) bool {
	switch planKind {
	case "connect":
		return strings.HasPrefix(step.ID, "snapshot-route-") ||
			strings.HasPrefix(step.ID, "add-route-exclusion-") ||
			step.ID == "store-runtime-state"
	case "disconnect":
		return step.ID == "read-runtime-state" ||
			step.ID == "remove-endpoint-route-exclusions" ||
			step.ID == "clear-runtime-state"
	default:
		return false
	}
}

func mergeRouteExclusions(base []routes.EndpointExclusion, discovered map[string]routes.EndpointExclusion) []routes.EndpointExclusion {
	if len(discovered) == 0 {
		return cloneRouteExclusions(base)
	}
	merged := make([]routes.EndpointExclusion, 0, len(base)+len(discovered))
	seen := map[string]struct{}{}
	for _, exclusion := range base {
		merged = append(merged, exclusion)
		seen[exclusion.EndpointIP] = struct{}{}
	}
	for _, exclusion := range discovered {
		if _, ok := seen[exclusion.EndpointIP]; ok {
			continue
		}
		merged = append(merged, exclusion)
	}
	return merged
}

func cloneRouteExclusions(routeExclusions []routes.EndpointExclusion) []routes.EndpointExclusion {
	if len(routeExclusions) == 0 {
		return nil
	}
	cloned := make([]routes.EndpointExclusion, len(routeExclusions))
	copy(cloned, routeExclusions)
	return cloned
}
