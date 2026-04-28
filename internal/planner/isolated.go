package planner

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"unicode"

	"github.com/MyHeartRaces/BigRedButton/internal/profile"
)

const (
	IsolatedAppTunnelKind  = "isolated-app-tunnel"
	IsolatedAppStopKind    = "isolated-app-stop"
	IsolatedAppCleanupKind = "isolated-app-cleanup"

	DefaultIsolatedRuntimeSubdir = "isolated"
)

type IsolatedAppOptions struct {
	SessionID          string   `json:"session_id,omitempty"`
	AppID              string   `json:"app_id,omitempty"`
	AppCommand         []string `json:"app_command,omitempty"`
	DNS                []string `json:"dns,omitempty"`
	WSTunnelBinary     string   `json:"wstunnel_binary,omitempty"`
	WireGuardInterface string   `json:"wireguard_interface,omitempty"`
	RuntimeRoot        string   `json:"runtime_root,omitempty"`
	Namespace          string   `json:"namespace,omitempty"`
	HostVeth           string   `json:"host_veth,omitempty"`
	NamespaceVeth      string   `json:"namespace_veth,omitempty"`
	HostAddress        string   `json:"host_address,omitempty"`
	NamespaceAddress   string   `json:"namespace_address,omitempty"`
	HostGateway        string   `json:"host_gateway,omitempty"`
	LaunchUID          string   `json:"launch_uid,omitempty"`
	LaunchGID          string   `json:"launch_gid,omitempty"`
	LaunchEnv          []string `json:"launch_env,omitempty"`
}

type IsolatedAppStopOptions struct {
	SessionID   string `json:"session_id,omitempty"`
	RuntimeRoot string `json:"runtime_root,omitempty"`
}

type isolatedAppNormalizedOptions struct {
	SessionID          string
	AppID              string
	AppCommand         []string
	DNS                []string
	WSTunnelBinary     string
	WireGuardInterface string
	RuntimeRoot        string
	SessionRuntimeRoot string
	Namespace          string
	HostVeth           string
	NamespaceVeth      string
	HostAddress        string
	NamespaceAddress   string
	HostGateway        string
	WSTunnelListen     string
	WireGuardEndpoint  string
	LaunchUID          string
	LaunchGID          string
	LaunchEnv          []string
}

func IsolatedAppTunnel(config profile.Config, options IsolatedAppOptions) (Plan, error) {
	normalized, warnings, err := normalizeIsolatedAppOptions(config, options)
	if err != nil {
		return Plan{}, err
	}

	steps := []Step{
		{
			ID:                 "validate-profile",
			Action:             "Validate VPN profile for isolated app tunnel",
			Details:            []string{"profile validation already completed by parser", "profile=" + config.Name},
			SecretMaterialFree: true,
		},
		{
			ID:     "validate-app-command",
			Action: "Validate isolated app launch command",
			Details: []string{
				"app_id=" + normalized.AppID,
				"executable=" + normalized.AppCommand[0],
				fmt.Sprintf("argument_count=%d", len(normalized.AppCommand)-1),
			},
			SecretMaterialFree: true,
		},
		{
			ID:     "validate-linux-prerequisites",
			Action: "Validate Linux isolated tunnel prerequisites",
			Details: append([]string{
				"binary=ip",
				"binary=wg",
				"binary=nft",
				"binary=" + normalized.WSTunnelBinary,
			}, append(setprivPrerequisite(normalized), envPrerequisite(normalized)...)...),
			SecretMaterialFree: true,
		},
		{
			ID:     "create-isolated-runtime-root",
			Action: "Create isolated session runtime root",
			Details: []string{
				"runtime_root=" + normalized.RuntimeRoot,
				"session_runtime_root=" + normalized.SessionRuntimeRoot,
				"session_id=" + normalized.SessionID,
			},
			RequiresPrivilege:  false,
			Rollback:           []string{"remove isolated session runtime files"},
			SecretMaterialFree: true,
		},
		{
			ID:     "create-netns",
			Action: "Create Linux network namespace for isolated app",
			Details: []string{
				"namespace=" + normalized.Namespace,
				"session_id=" + normalized.SessionID,
			},
			RequiresPrivilege:  true,
			Rollback:           []string{"delete network namespace " + normalized.Namespace},
			SecretMaterialFree: true,
		},
		{
			ID:     "create-veth-pair",
			Action: "Create veth pair between host and isolated namespace",
			Details: []string{
				"host_veth=" + normalized.HostVeth,
				"namespace_veth=" + normalized.NamespaceVeth,
				"namespace=" + normalized.Namespace,
			},
			RequiresPrivilege:  true,
			Rollback:           []string{"delete host veth " + normalized.HostVeth},
			SecretMaterialFree: true,
		},
		{
			ID:     "configure-host-veth",
			Action: "Configure host-side veth address for WSTunnel control path",
			Details: []string{
				"host_veth=" + normalized.HostVeth,
				"host_address=" + normalized.HostAddress,
				"host_gateway=" + normalized.HostGateway,
			},
			RequiresPrivilege:  true,
			Rollback:           []string{"bring down host veth " + normalized.HostVeth},
			SecretMaterialFree: true,
		},
		{
			ID:     "configure-namespace-veth",
			Action: "Configure namespace-side veth address",
			Details: []string{
				"namespace=" + normalized.Namespace,
				"namespace_veth=" + normalized.NamespaceVeth,
				"namespace_address=" + normalized.NamespaceAddress,
				"host_gateway=" + normalized.HostGateway,
			},
			RequiresPrivilege:  true,
			Rollback:           []string{"namespace is deleted during rollback"},
			SecretMaterialFree: true,
		},
		{
			ID:     "configure-namespace-dns",
			Action: "Write namespace-scoped DNS configuration",
			Details: append([]string{
				"namespace=" + normalized.Namespace,
			}, prefixed("dns=", normalized.DNS)...),
			RequiresPrivilege:  true,
			Rollback:           []string{"remove namespace DNS configuration"},
			SecretMaterialFree: true,
		},
		{
			ID:     "start-wstunnel-control",
			Action: "Start WSTunnel control process in host namespace",
			Details: []string{
				"binary=" + normalized.WSTunnelBinary,
				"target=" + config.WSTunnelURL,
				"path_prefix=" + config.WSTunnelPath,
				"tls_server_name=" + firstNonEmpty(config.WSTunnelTLSName, config.SNI),
				"local_udp=" + normalized.WSTunnelListen,
				"wireguard_endpoint=" + normalized.WireGuardEndpoint,
			},
			RequiresPrivilege:  false,
			Rollback:           []string{"stop WSTunnel control process"},
			SecretMaterialFree: true,
		},
		{
			ID:     "create-wireguard-interface-in-netns",
			Action: "Create WireGuard interface inside isolated namespace",
			Details: []string{
				"namespace=" + normalized.Namespace,
				"interface=" + normalized.WireGuardInterface,
			},
			RequiresPrivilege:  true,
			Rollback:           []string{"delete namespace WireGuard interface " + normalized.WireGuardInterface},
			SecretMaterialFree: true,
		},
		{
			ID:     "apply-wireguard-addresses-in-netns",
			Action: "Apply WireGuard addresses and MTU inside isolated namespace",
			Details: append([]string{
				"namespace=" + normalized.Namespace,
				"interface=" + normalized.WireGuardInterface,
				fmt.Sprintf("mtu=%d", config.MTU),
			}, prefixed("address=", config.Addresses)...),
			RequiresPrivilege:  true,
			Rollback:           []string{"delete namespace WireGuard interface " + normalized.WireGuardInterface},
			SecretMaterialFree: true,
		},
		{
			ID:     "apply-wireguard-peer-in-netns",
			Action: "Apply WireGuard peer with host-side WSTunnel endpoint",
			Details: []string{
				"namespace=" + normalized.Namespace,
				"interface=" + normalized.WireGuardInterface,
				"endpoint=" + normalized.WireGuardEndpoint,
				"session_runtime_root=" + normalized.SessionRuntimeRoot,
				fmt.Sprintf("persistent_keepalive=%d", config.PersistentKeepalive),
				fmt.Sprintf("preshared_key=%t", config.PresharedKey != ""),
			},
			RequiresPrivilege:  true,
			Rollback:           []string{"delete namespace WireGuard interface " + normalized.WireGuardInterface},
			SecretMaterialFree: true,
		},
		{
			ID:                 "apply-namespace-client-routes",
			Action:             "Apply client routes inside isolated namespace",
			Details:            append([]string{"namespace=" + normalized.Namespace, "interface=" + normalized.WireGuardInterface}, prefixed("allowed_ip=", config.AllowedIPs)...),
			RequiresPrivilege:  true,
			Rollback:           []string{"remove namespace client routes"},
			SecretMaterialFree: true,
		},
		{
			ID:     "apply-namespace-kill-switch",
			Action: "Apply namespace-only fail-closed egress policy",
			Details: []string{
				"namespace=" + normalized.Namespace,
				"interface=" + normalized.WireGuardInterface,
				"namespace_veth=" + normalized.NamespaceVeth,
				"allowed_control_endpoint=" + normalized.WireGuardEndpoint,
				"session_runtime_root=" + normalized.SessionRuntimeRoot,
			},
			RequiresPrivilege:  true,
			Rollback:           []string{"remove namespace-only egress policy"},
			SecretMaterialFree: true,
		},
		{
			ID:     "launch-app-in-netns",
			Action: "Launch selected app inside isolated namespace",
			Details: append(append([]string{
				"namespace=" + normalized.Namespace,
				"app_id=" + normalized.AppID,
				"executable=" + normalized.AppCommand[0],
				fmt.Sprintf("argument_count=%d", len(normalized.AppCommand)-1),
			}, launchIdentityDetails(normalized)...), append(prefixed("app_env=", normalized.LaunchEnv), prefixed("app_arg=", normalized.AppCommand[1:])...)...),
			RequiresPrivilege:  true,
			Rollback:           []string{"stop isolated app process tree"},
			SecretMaterialFree: true,
		},
		{
			ID:     "monitor-process-tree",
			Action: "Track isolated app process tree for session cleanup",
			Details: []string{
				"session_id=" + normalized.SessionID,
				"app_id=" + normalized.AppID,
				"namespace=" + normalized.Namespace,
			},
			RequiresPrivilege:  false,
			SecretMaterialFree: true,
		},
		{
			ID:     "store-isolated-runtime-state",
			Action: "Store isolated session runtime state",
			Details: []string{
				"runtime_root=" + normalized.RuntimeRoot,
				"session_runtime_root=" + normalized.SessionRuntimeRoot,
				"fingerprint=" + config.Fingerprint(),
			},
			Rollback:           []string{"clear isolated session runtime state"},
			SecretMaterialFree: true,
		},
	}

	warnings = append(warnings,
		"isolated app tunnel keeps host default routes and host DNS unchanged",
		"first Linux implementation uses network namespaces and requires a privileged helper for apply mode",
	)
	if normalized.LaunchUID == "" || normalized.LaunchGID == "" {
		warnings = append(warnings, "app launch UID/GID were not provided; apply mode will run the app as the helper user")
	}

	return Plan{
		Kind:               IsolatedAppTunnelKind,
		Profile:            config.Name,
		ProfileFingerprint: config.Fingerprint(),
		WSTunnelHost:       config.WSTunnelHost,
		WSTunnelURL:        config.WSTunnelURL,
		WireGuardInterface: normalized.WireGuardInterface,
		RuntimeRoot:        normalized.RuntimeRoot,
		SessionID:          normalized.SessionID,
		AppID:              normalized.AppID,
		Namespace:          normalized.Namespace,
		HostVeth:           normalized.HostVeth,
		NamespaceVeth:      normalized.NamespaceVeth,
		HostAddress:        normalized.HostAddress,
		NamespaceAddress:   normalized.NamespaceAddress,
		HostGateway:        normalized.HostGateway,
		Warnings:           warnings,
		Steps:              steps,
	}, nil
}

func IsolatedAppStop(options IsolatedAppStopOptions) (Plan, error) {
	sessionID, err := normalizeUUID(options.SessionID, "session ID")
	if err != nil {
		return Plan{}, err
	}
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = DefaultRuntimeRoot
	}
	sessionRuntimeRoot := runtimeRoot + "/" + DefaultIsolatedRuntimeSubdir + "/" + sessionID

	steps := []Step{
		{
			ID:                 "read-isolated-runtime-state",
			Action:             "Read isolated session runtime state",
			Details:            []string{"runtime_root=" + runtimeRoot, "session_runtime_root=" + sessionRuntimeRoot, "session_id=" + sessionID},
			SecretMaterialFree: true,
		},
		{
			ID:                 "stop-isolated-app",
			Action:             "Stop isolated app process tree",
			Details:            []string{"session_id=" + sessionID},
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "remove-namespace-kill-switch",
			Action:             "Remove namespace-only egress policy",
			Details:            []string{"session_id=" + sessionID},
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "remove-namespace-client-routes",
			Action:             "Remove isolated namespace client routes",
			Details:            []string{"session_id=" + sessionID},
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "remove-wireguard-interface-in-netns",
			Action:             "Remove WireGuard interface from isolated namespace",
			Details:            []string{"session_id=" + sessionID},
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "stop-wstunnel-control",
			Action:             "Stop WSTunnel control process",
			Details:            []string{"session_id=" + sessionID},
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "remove-namespace-dns",
			Action:             "Remove namespace-scoped DNS configuration",
			Details:            []string{"session_id=" + sessionID},
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "delete-netns",
			Action:             "Delete isolated network namespace",
			Details:            []string{"session_id=" + sessionID},
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "clear-isolated-runtime-state",
			Action:             "Clear isolated session runtime state",
			Details:            []string{"runtime_root=" + runtimeRoot, "session_runtime_root=" + sessionRuntimeRoot, "session_id=" + sessionID},
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
	}

	return Plan{
		Kind:        IsolatedAppStopKind,
		RuntimeRoot: runtimeRoot,
		SessionID:   sessionID,
		Warnings:    []string{"isolated app stop only removes launcher-owned session state for the requested UUID"},
		Steps:       steps,
	}, nil
}

func IsolatedAppCleanup(options IsolatedAppStopOptions) (Plan, error) {
	sessionID, err := normalizeUUID(options.SessionID, "session ID")
	if err != nil {
		return Plan{}, err
	}
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = DefaultRuntimeRoot
	}
	shortID := shortSessionID(sessionID)
	namespace := "brb-" + shortID
	hostVeth := "brbh" + shortID
	wireGuardInterface := "brbwg" + shortID
	sessionRuntimeRoot := runtimeRoot + "/" + DefaultIsolatedRuntimeSubdir + "/" + sessionID

	steps := []Step{
		{
			ID:                 "cleanup-isolated-processes",
			Action:             "Best-effort stop remaining isolated namespace processes",
			Details:            []string{"session_id=" + sessionID, "namespace=" + namespace},
			RequiresPrivilege:  true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "cleanup-namespace-kill-switch",
			Action:             "Best-effort flush namespace firewall rules",
			Details:            []string{"session_id=" + sessionID, "namespace=" + namespace},
			RequiresPrivilege:  true,
			SecretMaterialFree: true,
		},
		{
			ID:     "cleanup-wireguard-interface-in-netns",
			Action: "Best-effort remove isolated WireGuard interface",
			Details: []string{
				"session_id=" + sessionID,
				"namespace=" + namespace,
				"interface=" + wireGuardInterface,
			},
			RequiresPrivilege:  true,
			SecretMaterialFree: true,
		},
		{
			ID:     "cleanup-netns",
			Action: "Best-effort delete isolated namespace and host veth",
			Details: []string{
				"session_id=" + sessionID,
				"namespace=" + namespace,
				"host_veth=" + hostVeth,
			},
			RequiresPrivilege:  true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "cleanup-namespace-dns",
			Action:             "Best-effort remove namespace DNS configuration",
			Details:            []string{"session_id=" + sessionID, "namespace=" + namespace},
			RequiresPrivilege:  true,
			SecretMaterialFree: true,
		},
		{
			ID:     "cleanup-isolated-runtime-root",
			Action: "Best-effort remove isolated runtime root",
			Details: []string{
				"runtime_root=" + runtimeRoot,
				"session_runtime_root=" + sessionRuntimeRoot,
				"session_id=" + sessionID,
			},
			SecretMaterialFree: true,
		},
	}

	return Plan{
		Kind:               IsolatedAppCleanupKind,
		RuntimeRoot:        runtimeRoot,
		SessionID:          sessionID,
		Namespace:          namespace,
		HostVeth:           hostVeth,
		WireGuardInterface: wireGuardInterface,
		Warnings: []string{
			"isolated cleanup is best-effort and uses deterministic launcher-owned names for the requested UUID",
			"use normal isolated stop first when runtime state is available",
		},
		Steps: steps,
	}, nil
}

func normalizeIsolatedAppOptions(config profile.Config, options IsolatedAppOptions) (isolatedAppNormalizedOptions, []string, error) {
	var warnings []string

	sessionID, err := normalizeUUID(options.SessionID, "session ID")
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}
	appID := strings.TrimSpace(options.AppID)
	if appID == "" {
		appID = sessionID
	}
	appID, err = normalizeUUID(appID, "app ID")
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}

	appCommand, err := normalizeAppCommand(options.AppCommand)
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}
	if !strings.HasPrefix(appCommand[0], "/") {
		warnings = append(warnings, "app executable is not absolute; apply mode will resolve it through the helper environment")
	}

	dns, err := normalizeIsolatedDNS(options.DNS, config.DNS)
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}

	shortID := shortSessionID(sessionID)
	hostAddress, namespaceAddress, hostGateway, err := normalizeVethAddresses(options.HostAddress, options.NamespaceAddress, options.HostGateway, sessionID)
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}

	wstunnelBinary := strings.TrimSpace(options.WSTunnelBinary)
	if wstunnelBinary == "" {
		wstunnelBinary = DefaultWSTunnelBinary
	}
	wireGuardInterface := strings.TrimSpace(options.WireGuardInterface)
	if wireGuardInterface == "" {
		wireGuardInterface = "brbwg" + shortID
	}
	if err := validateLinuxInterfaceName(wireGuardInterface, "wireguard interface"); err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}

	namespace := firstNonEmpty(options.Namespace, "brb-"+shortID)
	if err := validateLinuxNamespaceName(namespace); err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}
	hostVeth := firstNonEmpty(options.HostVeth, "brbh"+shortID)
	if err := validateLinuxInterfaceName(hostVeth, "host veth"); err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}
	namespaceVeth := firstNonEmpty(options.NamespaceVeth, "brbn"+shortID)
	if err := validateLinuxInterfaceName(namespaceVeth, "namespace veth"); err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}

	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = DefaultRuntimeRoot
	}
	sessionRuntimeRoot := runtimeRoot + "/" + DefaultIsolatedRuntimeSubdir + "/" + sessionID

	port, err := localUDPPort(config.LocalUDPListen)
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}
	wstunnelListen := net.JoinHostPort(hostGateway, port)
	wireGuardEndpoint := net.JoinHostPort(hostGateway, port)
	launchUID, launchGID, err := normalizeLaunchIdentity(options.LaunchUID, options.LaunchGID)
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}
	launchEnv, err := normalizeLaunchEnv(options.LaunchEnv)
	if err != nil {
		return isolatedAppNormalizedOptions{}, nil, err
	}

	return isolatedAppNormalizedOptions{
		SessionID:          sessionID,
		AppID:              appID,
		AppCommand:         appCommand,
		DNS:                dns,
		WSTunnelBinary:     wstunnelBinary,
		WireGuardInterface: wireGuardInterface,
		RuntimeRoot:        runtimeRoot,
		SessionRuntimeRoot: sessionRuntimeRoot,
		Namespace:          namespace,
		HostVeth:           hostVeth,
		NamespaceVeth:      namespaceVeth,
		HostAddress:        hostAddress,
		NamespaceAddress:   namespaceAddress,
		HostGateway:        hostGateway,
		WSTunnelListen:     wstunnelListen,
		WireGuardEndpoint:  wireGuardEndpoint,
		LaunchUID:          launchUID,
		LaunchGID:          launchGID,
		LaunchEnv:          launchEnv,
	}, warnings, nil
}

func normalizeUUID(value string, label string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 36 {
		return "", fmt.Errorf("%s must be an RFC 4122 UUID", label)
	}
	for index, r := range value {
		switch index {
		case 8, 13, 18, 23:
			if r != '-' {
				return "", fmt.Errorf("%s must be an RFC 4122 UUID", label)
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				return "", fmt.Errorf("%s must be an RFC 4122 UUID", label)
			}
		}
	}
	return value, nil
}

func normalizeAppCommand(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("isolated app command is required")
	}
	return out, nil
}

func normalizeIsolatedDNS(optionDNS []string, profileDNS string) ([]string, error) {
	var raw []string
	if len(optionDNS) > 0 {
		raw = optionDNS
	} else {
		raw = []string{profileDNS}
	}

	seen := map[string]struct{}{}
	var out []string
	for _, value := range raw {
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || unicode.IsSpace(r)
		}) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			addr, err := netip.ParseAddr(token)
			if err != nil {
				return nil, fmt.Errorf("isolated DNS server %q is invalid: %w", token, err)
			}
			normalized := addr.String()
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("isolated app tunnel requires namespace DNS server")
	}
	return out, nil
}

func normalizeLaunchIdentity(rawUID string, rawGID string) (string, string, error) {
	uid := strings.TrimSpace(rawUID)
	gid := strings.TrimSpace(rawGID)
	if uid == "" && gid == "" {
		return "", "", nil
	}
	if uid == "" || gid == "" {
		return "", "", fmt.Errorf("launch UID and GID must be provided together")
	}
	if _, err := parseNonNegativeID(uid, "launch UID"); err != nil {
		return "", "", err
	}
	if _, err := parseNonNegativeID(gid, "launch GID"); err != nil {
		return "", "", err
	}
	return uid, gid, nil
}

func normalizeLaunchEnv(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key, envValue, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("launch environment value must be KEY=value")
		}
		key = strings.TrimSpace(key)
		if !isAllowedDesktopEnvKey(key) {
			return nil, fmt.Errorf("launch environment key %q is not allowed", key)
		}
		if strings.ContainsRune(envValue, '\x00') {
			return nil, fmt.Errorf("launch environment value for %s contains NUL", key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key+"="+envValue)
	}
	return normalized, nil
}

func isAllowedDesktopEnvKey(key string) bool {
	switch key {
	case "DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "PULSE_SERVER", "PIPEWIRE_RUNTIME_DIR":
		return true
	default:
		return false
	}
}

func parseNonNegativeID(value string, label string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be numeric: %w", label, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s must not be negative", label)
	}
	return parsed, nil
}

func launchIdentityDetails(options isolatedAppNormalizedOptions) []string {
	if options.LaunchUID == "" || options.LaunchGID == "" {
		return nil
	}
	return []string{"launch_uid=" + options.LaunchUID, "launch_gid=" + options.LaunchGID}
}

func setprivPrerequisite(options isolatedAppNormalizedOptions) []string {
	if options.LaunchUID == "" || options.LaunchGID == "" {
		return nil
	}
	return []string{"binary=setpriv"}
}

func envPrerequisite(options isolatedAppNormalizedOptions) []string {
	if len(options.LaunchEnv) == 0 {
		return nil
	}
	return []string{"binary=env"}
}

func normalizeVethAddresses(rawHostAddress string, rawNamespaceAddress string, rawHostGateway string, sessionID string) (string, string, string, error) {
	defaultHostAddress, defaultNamespaceAddress, defaultGateway := defaultVethAddressPair(sessionID)
	hostAddress := firstNonEmpty(rawHostAddress, defaultHostAddress)
	namespaceAddress := firstNonEmpty(rawNamespaceAddress, defaultNamespaceAddress)
	hostGateway := firstNonEmpty(rawHostGateway, defaultGateway)

	hostPrefix, err := netip.ParsePrefix(hostAddress)
	if err != nil {
		return "", "", "", fmt.Errorf("host veth address %q is invalid: %w", hostAddress, err)
	}
	namespacePrefix, err := netip.ParsePrefix(namespaceAddress)
	if err != nil {
		return "", "", "", fmt.Errorf("namespace veth address %q is invalid: %w", namespaceAddress, err)
	}
	gateway, err := netip.ParseAddr(hostGateway)
	if err != nil {
		return "", "", "", fmt.Errorf("host gateway %q is invalid: %w", hostGateway, err)
	}
	if !hostPrefix.Contains(gateway) || gateway != hostPrefix.Addr() {
		return "", "", "", fmt.Errorf("host gateway must match host veth address")
	}
	if hostPrefix.Bits() != namespacePrefix.Bits() || hostPrefix.Masked() != namespacePrefix.Masked() {
		return "", "", "", fmt.Errorf("host and namespace veth addresses must share the same prefix")
	}
	return hostPrefix.String(), namespacePrefix.String(), gateway.String(), nil
}

func defaultVethAddressPair(sessionID string) (string, string, string) {
	sum := sha256.Sum256([]byte(sessionID))
	third := 64 + int(sum[0])%64
	block := (int(sum[1]) % 63) * 4
	host := fmt.Sprintf("169.254.%d.%d", third, block+1)
	namespace := fmt.Sprintf("169.254.%d.%d", third, block+2)
	return host + "/30", namespace + "/30", host
}

func localUDPPort(endpoint string) (string, error) {
	_, port, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("parse local UDP listen endpoint: %w", err)
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("local UDP listen port is invalid: %w", err)
	}
	if value < 1 || value > 65535 {
		return "", fmt.Errorf("local UDP listen port must be in 1..65535")
	}
	return port, nil
}

func shortSessionID(sessionID string) string {
	return strings.ReplaceAll(sessionID, "-", "")[:8]
}

func validateLinuxNamespaceName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("namespace name is required")
	}
	if len(name) > 64 {
		return fmt.Errorf("namespace name must be 64 bytes or fewer")
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("namespace name contains unsupported character %q", r)
	}
	return nil
}

func validateLinuxInterfaceName(name string, label string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s name is required", label)
	}
	if len(name) > 15 {
		return fmt.Errorf("%s name must be 15 bytes or fewer", label)
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("%s name contains unsupported character %q", label, r)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
