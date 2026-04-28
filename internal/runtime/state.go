package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/routes"
)

const (
	StateVersion         = 1
	DefaultStateFileName = "state.json"
)

type State struct {
	Version             int                        `json:"version"`
	Mode                string                     `json:"mode,omitempty"`
	Profile             string                     `json:"profile,omitempty"`
	ProfileFingerprint  string                     `json:"profile_fingerprint"`
	WireGuardInterface  string                     `json:"wireguard_interface"`
	RouteExclusions     []routes.EndpointExclusion `json:"route_exclusions,omitempty"`
	WSTunnelProcess     *ProcessState              `json:"wstunnel_process,omitempty"`
	AppProcess          *ProcessState              `json:"app_process,omitempty"`
	WireGuardAllowedIPs []string                   `json:"wireguard_allowed_ips,omitempty"`
	SessionID           string                     `json:"session_id,omitempty"`
	AppID               string                     `json:"app_id,omitempty"`
	Namespace           string                     `json:"namespace,omitempty"`
	HostVeth            string                     `json:"host_veth,omitempty"`
	NamespaceVeth       string                     `json:"namespace_veth,omitempty"`
	HostAddress         string                     `json:"host_address,omitempty"`
	NamespaceAddress    string                     `json:"namespace_address,omitempty"`
	HostGateway         string                     `json:"host_gateway,omitempty"`
}

type ProcessState struct {
	PID  int      `json:"pid"`
	Argv []string `json:"argv,omitempty"`
}

type Store struct {
	Root string
}

type IsolatedSession struct {
	SessionID string `json:"session_id"`
	Root      string `json:"root"`
	State     *State `json:"state,omitempty"`
	Error     string `json:"error,omitempty"`
}

func NewStateFromConnectPlan(plan planner.Plan) (State, error) {
	if plan.Kind != "connect" {
		return State{}, fmt.Errorf("runtime state can only be built from connect plan")
	}
	state := State{
		Version:             StateVersion,
		Profile:             plan.Profile,
		ProfileFingerprint:  plan.ProfileFingerprint,
		WireGuardInterface:  plan.WireGuardInterface,
		RouteExclusions:     cloneRouteExclusions(plan.RouteExclusions),
		WireGuardAllowedIPs: allowedIPsFromPlan(plan),
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func NewStateFromIsolatedAppPlan(plan planner.Plan) (State, error) {
	if plan.Kind != planner.IsolatedAppTunnelKind {
		return State{}, fmt.Errorf("runtime state can only be built from isolated app tunnel plan")
	}
	state := State{
		Version:             StateVersion,
		Mode:                planner.IsolatedAppTunnelKind,
		Profile:             plan.Profile,
		ProfileFingerprint:  plan.ProfileFingerprint,
		WireGuardInterface:  plan.WireGuardInterface,
		WireGuardAllowedIPs: allowedIPsFromIsolatedPlan(plan),
		SessionID:           plan.SessionID,
		AppID:               plan.AppID,
		Namespace:           plan.Namespace,
		HostVeth:            plan.HostVeth,
		NamespaceVeth:       plan.NamespaceVeth,
		HostAddress:         plan.HostAddress,
		NamespaceAddress:    plan.NamespaceAddress,
		HostGateway:         plan.HostGateway,
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
	if s.Mode == planner.IsolatedAppTunnelKind {
		if strings.TrimSpace(s.SessionID) == "" {
			return fmt.Errorf("isolated runtime state session ID is required")
		}
		if strings.TrimSpace(s.Namespace) == "" {
			return fmt.Errorf("isolated runtime state namespace is required")
		}
		if strings.TrimSpace(s.HostVeth) == "" {
			return fmt.Errorf("isolated runtime state host veth is required")
		}
		if strings.TrimSpace(s.NamespaceVeth) == "" {
			return fmt.Errorf("isolated runtime state namespace veth is required")
		}
	}
	for index, exclusion := range s.RouteExclusions {
		if strings.TrimSpace(exclusion.EndpointIP) == "" {
			return fmt.Errorf("runtime state route exclusion %d endpoint IP is required", index)
		}
		if strings.TrimSpace(exclusion.Destination) == "" {
			return fmt.Errorf("runtime state route exclusion %d destination is required", index)
		}
	}
	if s.WSTunnelProcess != nil && s.WSTunnelProcess.PID < 1 {
		return fmt.Errorf("runtime state wstunnel process PID is required")
	}
	if s.AppProcess != nil && s.AppProcess.PID < 1 {
		return fmt.Errorf("runtime state app process PID is required")
	}
	return nil
}

func (s State) WithWSTunnelProcess(pid int, argv []string) State {
	s.WSTunnelProcess = &ProcessState{
		PID:  pid,
		Argv: append([]string(nil), argv...),
	}
	return s
}

func (s State) WithAppProcess(pid int, argv []string) State {
	s.AppProcess = &ProcessState{
		PID:  pid,
		Argv: append([]string(nil), argv...),
	}
	return s
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

func ListIsolatedSessions(ctx context.Context, runtimeRoot string) ([]IsolatedSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := strings.TrimSpace(runtimeRoot)
	if root == "" {
		root = planner.DefaultRuntimeRoot
	}
	isolatedRoot := filepath.Join(root, planner.DefaultIsolatedRuntimeSubdir)
	entries, err := os.ReadDir(isolatedRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read isolated runtime sessions: %w", err)
	}

	sessions := make([]IsolatedSession, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		sessionRoot := filepath.Join(isolatedRoot, sessionID)
		state, err := (Store{Root: sessionRoot}).Load(ctx)
		session := IsolatedSession{
			SessionID: sessionID,
			Root:      sessionRoot,
		}
		if err != nil {
			session.Error = err.Error()
		} else if state.Mode != planner.IsolatedAppTunnelKind {
			session.Error = "runtime state is not an isolated app tunnel session"
		} else if state.SessionID != sessionID {
			session.Error = fmt.Sprintf("runtime state session ID %q does not match directory %q", state.SessionID, sessionID)
		} else {
			session.State = &state
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func cloneRouteExclusions(routeExclusions []routes.EndpointExclusion) []routes.EndpointExclusion {
	if len(routeExclusions) == 0 {
		return nil
	}
	cloned := make([]routes.EndpointExclusion, len(routeExclusions))
	copy(cloned, routeExclusions)
	return cloned
}

func allowedIPsFromPlan(plan planner.Plan) []string {
	for _, step := range plan.Steps {
		if step.ID != "apply-client-routes" {
			continue
		}
		var allowedIPs []string
		for _, detail := range step.Details {
			if allowedIP, ok := strings.CutPrefix(detail, "allowed_ip="); ok {
				allowedIPs = append(allowedIPs, allowedIP)
			}
		}
		return allowedIPs
	}
	return nil
}

func allowedIPsFromIsolatedPlan(plan planner.Plan) []string {
	for _, step := range plan.Steps {
		if step.ID != "apply-namespace-client-routes" {
			continue
		}
		var allowedIPs []string
		for _, detail := range step.Details {
			if allowedIP, ok := strings.CutPrefix(detail, "allowed_ip="); ok {
				allowedIPs = append(allowedIPs, allowedIP)
			}
		}
		return allowedIPs
	}
	return nil
}
