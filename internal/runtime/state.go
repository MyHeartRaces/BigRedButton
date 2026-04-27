package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tracegate/tracegate-launcher/internal/planner"
	"github.com/tracegate/tracegate-launcher/internal/routes"
)

const (
	StateVersion         = 1
	DefaultStateFileName = "state.json"
)

type State struct {
	Version            int                        `json:"version"`
	Profile            string                     `json:"profile,omitempty"`
	ProfileFingerprint string                     `json:"profile_fingerprint"`
	WireGuardInterface string                     `json:"wireguard_interface"`
	RouteExclusions    []routes.EndpointExclusion `json:"route_exclusions,omitempty"`
}

type Store struct {
	Root string
}

func NewStateFromConnectPlan(plan planner.Plan) (State, error) {
	if plan.Kind != "connect" {
		return State{}, fmt.Errorf("runtime state can only be built from connect plan")
	}
	state := State{
		Version:            StateVersion,
		Profile:            plan.Profile,
		ProfileFingerprint: plan.ProfileFingerprint,
		WireGuardInterface: plan.WireGuardInterface,
		RouteExclusions:    cloneRouteExclusions(plan.RouteExclusions),
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s State) Validate() error {
	if s.Version != StateVersion {
		return fmt.Errorf("unsupported runtime state version: %d", s.Version)
	}
	if strings.TrimSpace(s.ProfileFingerprint) == "" {
		return fmt.Errorf("runtime state profile fingerprint is required")
	}
	if strings.TrimSpace(s.WireGuardInterface) == "" {
		return fmt.Errorf("runtime state WireGuard interface is required")
	}
	for index, exclusion := range s.RouteExclusions {
		if strings.TrimSpace(exclusion.EndpointIP) == "" {
			return fmt.Errorf("runtime state route exclusion %d endpoint IP is required", index)
		}
		if strings.TrimSpace(exclusion.Destination) == "" {
			return fmt.Errorf("runtime state route exclusion %d destination is required", index)
		}
	}
	return nil
}

func (s Store) Save(ctx context.Context, state State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return err
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create runtime state directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime state: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write runtime state: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set runtime state permissions: %w", err)
	}
	return nil
}

func (s Store) Load(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	path, err := s.Path()
	if err != nil {
		return State{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("read runtime state: %w", err)
	}
	var state State
	if err := json.Unmarshal(payload, &state); err != nil {
		return State{}, fmt.Errorf("decode runtime state: %w", err)
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s Store) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove runtime state: %w", err)
	}
	return nil
}

func (s Store) Path() (string, error) {
	root := strings.TrimSpace(s.Root)
	if root == "" {
		return "", fmt.Errorf("runtime state root is required")
	}
	return filepath.Join(root, DefaultStateFileName), nil
}

func cloneRouteExclusions(routeExclusions []routes.EndpointExclusion) []routes.EndpointExclusion {
	if len(routeExclusions) == 0 {
		return nil
	}
	cloned := make([]routes.EndpointExclusion, len(routeExclusions))
	copy(cloned, routeExclusions)
	return cloned
}
