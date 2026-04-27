package linux

import (
	"context"
	"fmt"
	"strings"

	"github.com/tracegate/tracegate-launcher/internal/planner"
	"github.com/tracegate/tracegate-launcher/internal/routes"
	truntime "github.com/tracegate/tracegate-launcher/internal/runtime"
)

type OperationPhase string

const (
	OperationApply    OperationPhase = "apply"
	OperationRollback OperationPhase = "rollback"
)

type Operation struct {
	Phase   OperationPhase `json:"phase"`
	StepID  string         `json:"step_id"`
	Command *Command       `json:"command,omitempty"`
	Runtime string         `json:"runtime,omitempty"`
}

type DryRunExecutor struct {
	plan                      planner.Plan
	routeExclusionsByEndpoint map[string]routes.EndpointExclusion
	discoveredByEndpoint      map[string]routes.EndpointExclusion
	runtimeState              truntime.State
	runner                    CommandRunner
	store                     truntime.Store
	readOnlyDiscovery         bool
	persistRuntimeState       bool
	operations                []Operation
}

type DryRunOptions struct {
	ReadOnlyDiscovery bool
	PersistRuntime    bool
	Runner            CommandRunner
	RuntimeRoot       string
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
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = plan.RuntimeRoot
	}
	return &DryRunExecutor{
		plan:                      plan,
		routeExclusionsByEndpoint: routeExclusionsByEndpoint,
		discoveredByEndpoint:      make(map[string]routes.EndpointExclusion),
		runner:                    runner,
		store:                     truntime.Store{Root: runtimeRoot},
		readOnlyDiscovery:         options.ReadOnlyDiscovery,
		persistRuntimeState:       options.PersistRuntime,
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
	case step.ID == "store-runtime-state":
		state, err := e.stateFromPlan()
		if err != nil {
			return err
		}
		e.runtimeState = state
		if e.persistRuntimeState {
			if err := e.store.Save(ctx, state); err != nil {
				return err
			}
			e.recordRuntime(OperationApply, step.ID, "save "+e.storePath())
		} else {
			e.recordRuntime(OperationApply, step.ID, "would save "+e.storePath())
		}
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
			e.record(OperationApply, step.ID, command)
		}
	case step.ID == "clear-runtime-state":
		if e.persistRuntimeState {
			if err := e.store.Clear(ctx); err != nil {
				return err
			}
			e.recordRuntime(OperationApply, step.ID, "clear "+e.storePath())
		} else {
			e.recordRuntime(OperationApply, step.ID, "would clear "+e.storePath())
		}
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

func (e *DryRunExecutor) RuntimeState() (truntime.State, bool) {
	if e == nil || e.runtimeState.Version == 0 {
		return truntime.State{}, false
	}
	return e.runtimeState, true
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
		Command: &command,
	})
}

func (e *DryRunExecutor) recordRuntime(phase OperationPhase, stepID string, runtime string) {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Runtime: runtime,
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

func (e *DryRunExecutor) stateFromPlan() (truntime.State, error) {
	state, err := truntime.NewStateFromConnectPlan(e.plan)
	if err != nil {
		return truntime.State{}, err
	}
	if len(e.discoveredByEndpoint) == 0 {
		return state, nil
	}
	merged := make([]routes.EndpointExclusion, 0, len(e.routeExclusionsByEndpoint)+len(e.discoveredByEndpoint))
	seen := map[string]struct{}{}
	for _, exclusion := range state.RouteExclusions {
		merged = append(merged, exclusion)
		seen[exclusion.EndpointIP] = struct{}{}
	}
	for _, exclusion := range e.discoveredByEndpoint {
		if _, ok := seen[exclusion.EndpointIP]; ok {
			continue
		}
		merged = append(merged, exclusion)
	}
	state.RouteExclusions = merged
	if err := state.Validate(); err != nil {
		return truntime.State{}, err
	}
	return state, nil
}

func (e *DryRunExecutor) stateForDisconnect(ctx context.Context) (truntime.State, error) {
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

func (e *DryRunExecutor) storePath() string {
	path, err := e.store.Path()
	if err != nil {
		return e.store.Root
	}
	return path
}

func routeExclusionMap(exclusions []routes.EndpointExclusion) map[string]routes.EndpointExclusion {
	out := make(map[string]routes.EndpointExclusion, len(exclusions))
	for _, exclusion := range exclusions {
		out[exclusion.EndpointIP] = exclusion
	}
	return out
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
