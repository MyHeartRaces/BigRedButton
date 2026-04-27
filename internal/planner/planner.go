package planner

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/MyHeartRaces/BigRedButton/internal/profile"
	"github.com/MyHeartRaces/BigRedButton/internal/routes"
)

const (
	DefaultWSTunnelBinary     = "wstunnel"
	DefaultWireGuardInterface = "tg-v7"
	DefaultRuntimeRoot        = "/run/big-red-button"
)

type Options struct {
	EndpointIPs        []string `json:"endpoint_ips,omitempty"`
	DefaultGateway     string   `json:"default_gateway,omitempty"`
	DefaultInterface   string   `json:"default_interface,omitempty"`
	WSTunnelBinary     string   `json:"wstunnel_binary,omitempty"`
	WireGuardInterface string   `json:"wireguard_interface,omitempty"`
	RuntimeRoot        string   `json:"runtime_root,omitempty"`
}

type Plan struct {
	Kind               string                     `json:"kind"`
	Profile            string                     `json:"profile,omitempty"`
	ProfileFingerprint string                     `json:"profile_fingerprint,omitempty"`
	WSTunnelHost       string                     `json:"wstunnel_host,omitempty"`
	WSTunnelURL        string                     `json:"wstunnel_url,omitempty"`
	EndpointIPs        []string                   `json:"endpoint_ips,omitempty"`
	WireGuardInterface string                     `json:"wireguard_interface,omitempty"`
	RuntimeRoot        string                     `json:"runtime_root,omitempty"`
	Warnings           []string                   `json:"warnings,omitempty"`
	Steps              []Step                     `json:"steps"`
	RouteExclusions    []routes.EndpointExclusion `json:"route_exclusions,omitempty"`
}

type Step struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"`
	Details            []string `json:"details,omitempty"`
	RequiresPrivilege  bool     `json:"requires_privilege"`
	Rollback           []string `json:"rollback,omitempty"`
	SkippedUntilApply  bool     `json:"skipped_until_apply,omitempty"`
	DependsOnRuntime   bool     `json:"depends_on_runtime,omitempty"`
	SecretMaterialFree bool     `json:"secret_material_free"`
}

func Connect(config profile.Config, options Options) (Plan, error) {
	normalized, warnings, err := normalizeOptions(options)
	if err != nil {
		return Plan{}, err
	}
	var routeExclusions []routes.EndpointExclusion

	steps := []Step{
		{
			ID:                 "validate-profile",
			Action:             "Validate normalized V7 profile",
			Details:            []string{"profile validation already completed by parser", "profile=" + config.Name},
			SecretMaterialFree: true,
		},
		{
			ID:                 "resolve-wstunnel-host",
			Action:             "Resolve WSTunnel host before applying tunnel routes",
			Details:            []string{"host=" + config.WSTunnelHost},
			SecretMaterialFree: true,
		},
	}

	if len(normalized.EndpointIPs) == 0 {
		steps = append(steps, Step{
			ID:                 "snapshot-endpoint-routes",
			Action:             "Snapshot current route to each resolved WSTunnel endpoint",
			Details:            []string{"pending DNS resolution by platform adapter"},
			RequiresPrivilege:  true,
			SkippedUntilApply:  true,
			SecretMaterialFree: true,
		})
		steps = append(steps, Step{
			ID:                 "add-endpoint-route-exclusions",
			Action:             "Add host routes that keep WSTunnel endpoint traffic outside WireGuard",
			Details:            []string{"pending DNS resolution by platform adapter"},
			RequiresPrivilege:  true,
			SkippedUntilApply:  true,
			Rollback:           []string{"remove launcher-owned endpoint route exclusions"},
			SecretMaterialFree: true,
		})
		warnings = append(warnings, "endpoint IPs were not provided; concrete route exclusion will be built at apply time")
	} else {
		for _, endpointIP := range normalized.EndpointIPs {
			details := []string{"endpoint_ip=" + endpointIP}
			if normalized.DefaultGateway != "" {
				details = append(details, "gateway="+normalized.DefaultGateway)
			}
			if normalized.DefaultInterface != "" {
				details = append(details, "interface="+normalized.DefaultInterface)
			}
			if normalized.DefaultGateway != "" || normalized.DefaultInterface != "" {
				exclusion, err := routes.NewEndpointExclusion(endpointIP, normalized.DefaultGateway, normalized.DefaultInterface)
				if err != nil {
					return Plan{}, err
				}
				routeExclusions = append(routeExclusions, exclusion)
				details = append(details, "destination="+exclusion.Destination)
			} else {
				warnings = append(warnings, "default route gateway/interface were not provided; route exclusion remains abstract until apply time")
			}
			steps = append(steps, Step{
				ID:                 "snapshot-route-" + safeID(endpointIP),
				Action:             "Snapshot current route to WSTunnel endpoint",
				Details:            details,
				RequiresPrivilege:  true,
				SecretMaterialFree: true,
			})
			steps = append(steps, Step{
				ID:                 "add-route-exclusion-" + safeID(endpointIP),
				Action:             "Add launcher-owned host route for WSTunnel endpoint",
				Details:            details,
				RequiresPrivilege:  true,
				Rollback:           []string{"remove launcher-owned route for " + endpointIP},
				SecretMaterialFree: true,
			})
		}
	}

	steps = append(steps,
		Step{
			ID:                 "start-wstunnel",
			Action:             "Start WSTunnel client process",
			Details:            []string{"binary=" + normalized.WSTunnelBinary, "target=" + config.WSTunnelURL, "local_udp=" + config.LocalUDPListen},
			Rollback:           []string{"stop WSTunnel client process"},
			RequiresPrivilege:  false,
			SecretMaterialFree: true,
		},
		Step{
			ID:                 "create-wireguard-interface",
			Action:             "Create or claim launcher WireGuard interface",
			Details:            []string{"interface=" + normalized.WireGuardInterface},
			RequiresPrivilege:  true,
			Rollback:           []string{"remove launcher WireGuard interface " + normalized.WireGuardInterface},
			SecretMaterialFree: true,
		},
		Step{
			ID:                 "apply-wireguard-addresses",
			Action:             "Apply WireGuard tunnel addresses and MTU",
			Details:            append([]string{"interface=" + normalized.WireGuardInterface, fmt.Sprintf("mtu=%d", config.MTU)}, prefixed("address=", config.Addresses)...),
			RequiresPrivilege:  true,
			Rollback:           []string{"remove launcher WireGuard addresses"},
			SecretMaterialFree: true,
		},
		Step{
			ID:                 "apply-wireguard-peer",
			Action:             "Apply WireGuard peer, endpoint and keepalive",
			Details:            []string{"endpoint=" + config.LocalUDPListen, fmt.Sprintf("persistent_keepalive=%d", config.PersistentKeepalive), fmt.Sprintf("preshared_key=%t", config.PresharedKey != "")},
			RequiresPrivilege:  true,
			Rollback:           []string{"remove launcher WireGuard peer"},
			SecretMaterialFree: true,
		},
		Step{
			ID:                 "apply-client-routes",
			Action:             "Apply launcher-owned routes for client AllowedIPs",
			Details:            prefixed("allowed_ip=", config.AllowedIPs),
			RequiresPrivilege:  true,
			Rollback:           []string{"remove launcher-owned client routes"},
			SecretMaterialFree: true,
		},
	)

	if strings.TrimSpace(config.DNS) == "" {
		steps = append(steps, Step{
			ID:                 "dns-plan",
			Action:             "Skip DNS changes",
			Details:            []string{"profile has no DNS value"},
			SecretMaterialFree: true,
		})
	} else {
		steps = append(steps, Step{
			ID:                 "dns-plan",
			Action:             "Plan platform DNS update",
			Details:            []string{"dns_configured=true", "adapter selected at apply time"},
			RequiresPrivilege:  true,
			Rollback:           []string{"restore previous DNS state if changed by launcher"},
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		})
		warnings = append(warnings, "DNS adapter is not implemented in the first Linux MVP; dry-run records intent only")
	}

	steps = append(steps,
		Step{
			ID:                 "verify-connected",
			Action:             "Verify WSTunnel process and WireGuard interface health",
			Details:            []string{"interface=" + normalized.WireGuardInterface},
			SecretMaterialFree: true,
		},
		Step{
			ID:                 "store-runtime-state",
			Action:             "Store sanitized runtime state for rollback and status",
			Details:            []string{"runtime_root=" + normalized.RuntimeRoot, "fingerprint=" + config.Fingerprint()},
			Rollback:           []string{"clear launcher runtime state"},
			SecretMaterialFree: true,
		},
	)

	return Plan{
		Kind:               "connect",
		Profile:            config.Name,
		ProfileFingerprint: config.Fingerprint(),
		WSTunnelHost:       config.WSTunnelHost,
		WSTunnelURL:        config.WSTunnelURL,
		EndpointIPs:        normalized.EndpointIPs,
		WireGuardInterface: normalized.WireGuardInterface,
		RuntimeRoot:        normalized.RuntimeRoot,
		Warnings:           warnings,
		Steps:              steps,
		RouteExclusions:    routeExclusions,
	}, nil
}

func Disconnect(options Options) (Plan, error) {
	normalized, warnings, err := normalizeOptions(options)
	if err != nil {
		return Plan{}, err
	}

	steps := []Step{
		{
			ID:                 "read-runtime-state",
			Action:             "Read launcher runtime state",
			Details:            []string{"runtime_root=" + normalized.RuntimeRoot},
			SecretMaterialFree: true,
		},
		{
			ID:                 "restore-dns",
			Action:             "Restore launcher-owned DNS changes if present",
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "remove-client-routes",
			Action:             "Remove launcher-owned WireGuard client routes",
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "remove-wireguard-interface",
			Action:             "Remove or bring down launcher WireGuard interface",
			Details:            []string{"interface=" + normalized.WireGuardInterface},
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "stop-wstunnel",
			Action:             "Stop launcher-owned WSTunnel process",
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "remove-endpoint-route-exclusions",
			Action:             "Remove launcher-owned WSTunnel endpoint route exclusions",
			RequiresPrivilege:  true,
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
		{
			ID:                 "clear-runtime-state",
			Action:             "Clear launcher runtime state",
			Details:            []string{"runtime_root=" + normalized.RuntimeRoot},
			DependsOnRuntime:   true,
			SecretMaterialFree: true,
		},
	}

	return Plan{
		Kind:               "disconnect",
		WireGuardInterface: normalized.WireGuardInterface,
		RuntimeRoot:        normalized.RuntimeRoot,
		Warnings:           warnings,
		Steps:              steps,
	}, nil
}

func normalizeOptions(options Options) (Options, []string, error) {
	var warnings []string
	normalized := options
	normalized.EndpointIPs = nil
	seen := map[string]struct{}{}
	for _, rawIP := range options.EndpointIPs {
		for _, token := range strings.Split(rawIP, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			addr, err := netip.ParseAddr(token)
			if err != nil {
				return Options{}, nil, fmt.Errorf("invalid endpoint IP %q: %w", token, err)
			}
			normalizedIP := addr.String()
			if !addr.Is4() {
				warnings = append(warnings, "non-IPv4 endpoint route exclusion is planned but platform support must be explicit")
			}
			if _, ok := seen[normalizedIP]; ok {
				continue
			}
			seen[normalizedIP] = struct{}{}
			normalized.EndpointIPs = append(normalized.EndpointIPs, normalizedIP)
		}
	}
	if normalized.WSTunnelBinary == "" {
		normalized.WSTunnelBinary = DefaultWSTunnelBinary
	}
	if normalized.WireGuardInterface == "" {
		normalized.WireGuardInterface = DefaultWireGuardInterface
	}
	if normalized.RuntimeRoot == "" {
		normalized.RuntimeRoot = DefaultRuntimeRoot
	}
	return normalized, warnings, nil
}

func prefixed(prefix string, values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, prefix+value)
	}
	return out
}

func safeID(value string) string {
	replacer := strings.NewReplacer(".", "-", ":", "-", "/", "-", "[", "", "]", "")
	return replacer.Replace(value)
}
