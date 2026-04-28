package planner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MyHeartRaces/BigRedButton/internal/profile"
)

func TestConnectPlanWithEndpointIPs(t *testing.T) {
	config := loadValidProfile(t)

	plan, err := Connect(config, Options{
		EndpointIPs:        []string{"203.0.113.10,203.0.113.11"},
		DefaultGateway:     "192.0.2.1",
		DefaultInterface:   "eth0",
		WireGuardInterface: "tg-test",
		RuntimeRoot:        "/run/test-tracegate",
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if plan.Kind != "connect" {
		t.Fatalf("unexpected plan kind: %s", plan.Kind)
	}
	if plan.WSTunnelHost != "edge.example.com" {
		t.Fatalf("unexpected wstunnel host: %s", plan.WSTunnelHost)
	}
	if len(plan.EndpointIPs) != 2 {
		t.Fatalf("unexpected endpoint IPs: %#v", plan.EndpointIPs)
	}
	if len(plan.RouteExclusions) != 2 {
		t.Fatalf("unexpected route exclusions: %#v", plan.RouteExclusions)
	}
	if plan.RouteExclusions[0].Destination != "203.0.113.10/32" {
		t.Fatalf("unexpected route exclusion destination: %#v", plan.RouteExclusions[0])
	}
	if !hasStep(plan, "add-route-exclusion-203-0-113-10") {
		t.Fatalf("missing route exclusion step: %#v", plan.Steps)
	}
	if !hasStep(plan, "start-wstunnel") {
		t.Fatalf("missing wstunnel step: %#v", plan.Steps)
	}
	if !hasStep(plan, "validate-linux-prerequisites") {
		t.Fatalf("missing Linux prerequisite step: %#v", plan.Steps)
	}
	if !hasStep(plan, "apply-wireguard-peer") {
		t.Fatalf("missing wireguard peer step: %#v", plan.Steps)
	}
	if !hasStep(plan, "apply-dns") {
		t.Fatalf("missing DNS apply step: %#v", plan.Steps)
	}
	if len(plan.DNSServers) != 1 || plan.DNSServers[0] != "1.1.1.1" {
		t.Fatalf("unexpected DNS servers: %#v", plan.DNSServers)
	}
	dnsStep := findStep(plan, "apply-dns")
	dnsDetails := strings.Join(dnsStep.Details, "\n")
	for _, want := range []string{"interface=tg-test", "dns=1.1.1.1", "routing_domain=~.", "default_route=yes"} {
		if !strings.Contains(dnsDetails, want) {
			t.Fatalf("missing DNS detail %q: %#v", want, dnsStep.Details)
		}
	}
	if strings.Contains(strings.Join(plan.Warnings, "\n"), "DNS adapter is not implemented") {
		t.Fatalf("unexpected obsolete DNS warning: %#v", plan.Warnings)
	}
	prereqDetails := strings.Join(findStep(plan, "validate-linux-prerequisites").Details, "\n")
	for _, want := range []string{"binary=ip", "binary=wg", "binary=wstunnel", "binary=resolvectl"} {
		if !strings.Contains(prereqDetails, want) {
			t.Fatalf("missing prerequisite detail %q: %#v", want, findStep(plan, "validate-linux-prerequisites").Details)
		}
	}
	assertPlanHasNoSecret(t, plan, config.WireGuardPrivateKey)
	assertPlanHasNoSecret(t, plan, config.ServerPublicKey)
	assertPlanHasNoSecret(t, plan, config.PresharedKey)
}

func TestConnectPlanWithoutEndpointIPsDefersRouteExclusion(t *testing.T) {
	config := loadValidProfile(t)

	plan, err := Connect(config, Options{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if !hasStep(plan, "add-endpoint-route-exclusions") {
		t.Fatalf("missing deferred route exclusion step: %#v", plan.Steps)
	}
	step := findStep(plan, "add-endpoint-route-exclusions")
	if !step.SkippedUntilApply {
		t.Fatalf("expected route exclusion step to be apply-time only: %#v", step)
	}
	if len(plan.Warnings) == 0 || !strings.Contains(strings.Join(plan.Warnings, "\n"), "endpoint IPs were not provided") {
		t.Fatalf("missing endpoint warning: %#v", plan.Warnings)
	}
}

func TestConnectPlanRejectsInvalidEndpointIP(t *testing.T) {
	config := loadValidProfile(t)

	_, err := Connect(config, Options{EndpointIPs: []string{"not-an-ip"}})
	if err == nil {
		t.Fatal("expected invalid endpoint IP error")
	}
	if !strings.Contains(err.Error(), "invalid endpoint IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnectPlanRejectsInvalidDNS(t *testing.T) {
	config := loadValidProfile(t)
	config.DNS = "1.1.1.1,not-an-ip"

	_, err := Connect(config, Options{})
	if err == nil {
		t.Fatal("expected invalid DNS error")
	}
	if !strings.Contains(err.Error(), "invalid DNS server") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDisconnectPlan(t *testing.T) {
	plan, err := Disconnect(Options{WireGuardInterface: "tg-test", RuntimeRoot: "/run/test-tracegate"})
	if err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if plan.Kind != "disconnect" {
		t.Fatalf("unexpected plan kind: %s", plan.Kind)
	}
	for _, id := range []string{
		"read-runtime-state",
		"restore-dns",
		"remove-client-routes",
		"remove-wireguard-interface",
		"stop-wstunnel",
		"remove-endpoint-route-exclusions",
		"clear-runtime-state",
	} {
		if !hasStep(plan, id) {
			t.Fatalf("missing step %s: %#v", id, plan.Steps)
		}
	}
}

func TestIsolatedAppTunnelPlan(t *testing.T) {
	config := loadValidProfile(t)

	plan, err := IsolatedAppTunnel(config, IsolatedAppOptions{
		SessionID:   "123e4567-e89b-12d3-a456-426614174000",
		AppCommand:  []string{"/usr/bin/curl", "https://example.com"},
		RuntimeRoot: "/run/test-brb",
	})
	if err != nil {
		t.Fatalf("IsolatedAppTunnel() error = %v", err)
	}

	if plan.Kind != IsolatedAppTunnelKind {
		t.Fatalf("unexpected plan kind: %s", plan.Kind)
	}
	if plan.Namespace != "brb-123e4567" {
		t.Fatalf("unexpected namespace: %s", plan.Namespace)
	}
	if plan.HostVeth != "brbh123e4567" || plan.NamespaceVeth != "brbn123e4567" {
		t.Fatalf("unexpected veth names: host=%s namespace=%s", plan.HostVeth, plan.NamespaceVeth)
	}
	for _, id := range []string{
		"create-netns",
		"validate-linux-prerequisites",
		"create-veth-pair",
		"configure-namespace-dns",
		"start-wstunnel-control",
		"create-wireguard-interface-in-netns",
		"apply-namespace-kill-switch",
		"launch-app-in-netns",
		"store-isolated-runtime-state",
	} {
		if !hasStep(plan, id) {
			t.Fatalf("missing step %s: %#v", id, plan.Steps)
		}
	}
	if !strings.Contains(strings.Join(plan.Warnings, "\n"), "host default routes and host DNS unchanged") {
		t.Fatalf("missing host network invariant warning: %#v", plan.Warnings)
	}
	assertPlanHasNoSecret(t, plan, config.WireGuardPrivateKey)
	assertPlanHasNoSecret(t, plan, config.ServerPublicKey)
	assertPlanHasNoSecret(t, plan, config.PresharedKey)
}

func TestIsolatedAppTunnelRequiresSessionUUID(t *testing.T) {
	config := loadValidProfile(t)

	_, err := IsolatedAppTunnel(config, IsolatedAppOptions{
		SessionID:  "manual",
		AppCommand: []string{"/usr/bin/curl"},
	})
	if err == nil {
		t.Fatal("expected invalid session UUID error")
	}
	if !strings.Contains(err.Error(), "session ID must be an RFC 4122 UUID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsolatedAppTunnelLaunchEnv(t *testing.T) {
	config := loadValidProfile(t)

	_, err := IsolatedAppTunnel(config, IsolatedAppOptions{
		SessionID:  "123e4567-e89b-12d3-a456-426614174000",
		AppCommand: []string{"/usr/bin/firefox"},
		LaunchEnv:  []string{"UNSAFE=value"},
	})
	if err == nil {
		t.Fatal("expected unsafe launch env error")
	}

	plan, err := IsolatedAppTunnel(config, IsolatedAppOptions{
		SessionID:  "123e4567-e89b-12d3-a456-426614174000",
		AppCommand: []string{"/usr/bin/firefox"},
		LaunchEnv:  []string{"DISPLAY=:1", "XDG_RUNTIME_DIR=/run/user/1000"},
	})
	if err != nil {
		t.Fatalf("IsolatedAppTunnel() error = %v", err)
	}
	step := findStep(plan, "launch-app-in-netns")
	details := strings.Join(step.Details, "\n")
	if !strings.Contains(details, "app_env=DISPLAY=:1") || !strings.Contains(details, "app_env=XDG_RUNTIME_DIR=/run/user/1000") {
		t.Fatalf("launch details = %#v", step.Details)
	}
}

func TestIsolatedAppStopPlan(t *testing.T) {
	plan, err := IsolatedAppStop(IsolatedAppStopOptions{
		SessionID:   "123e4567-e89b-12d3-a456-426614174000",
		RuntimeRoot: "/run/test-brb",
	})
	if err != nil {
		t.Fatalf("IsolatedAppStop() error = %v", err)
	}
	if plan.Kind != IsolatedAppStopKind {
		t.Fatalf("unexpected plan kind: %s", plan.Kind)
	}
	if plan.SessionID != "123e4567-e89b-12d3-a456-426614174000" {
		t.Fatalf("unexpected session ID: %s", plan.SessionID)
	}
	for _, id := range []string{
		"read-isolated-runtime-state",
		"stop-isolated-app",
		"remove-namespace-kill-switch",
		"remove-namespace-client-routes",
		"remove-wireguard-interface-in-netns",
		"stop-wstunnel-control",
		"remove-namespace-dns",
		"delete-netns",
		"clear-isolated-runtime-state",
	} {
		if !hasStep(plan, id) {
			t.Fatalf("missing step %s: %#v", id, plan.Steps)
		}
	}
}

func TestIsolatedAppCleanupPlan(t *testing.T) {
	plan, err := IsolatedAppCleanup(IsolatedAppStopOptions{
		SessionID:   "123e4567-e89b-12d3-a456-426614174000",
		RuntimeRoot: "/run/test-brb",
	})
	if err != nil {
		t.Fatalf("IsolatedAppCleanup() error = %v", err)
	}
	if plan.Kind != IsolatedAppCleanupKind {
		t.Fatalf("unexpected plan kind: %s", plan.Kind)
	}
	if plan.Namespace != "brb-123e4567" || plan.HostVeth != "brbh123e4567" {
		t.Fatalf("unexpected cleanup names: namespace=%s host_veth=%s", plan.Namespace, plan.HostVeth)
	}
	if plan.WireGuardInterface != "brbwg123e4567" {
		t.Fatalf("wireguard interface = %s", plan.WireGuardInterface)
	}
	for _, id := range []string{
		"cleanup-isolated-processes",
		"cleanup-namespace-kill-switch",
		"cleanup-wireguard-interface-in-netns",
		"cleanup-netns",
		"cleanup-namespace-dns",
		"cleanup-isolated-runtime-root",
	} {
		if !hasStep(plan, id) {
			t.Fatalf("missing step %s: %#v", id, plan.Steps)
		}
	}
}

func assertPlanHasNoSecret(t *testing.T, plan Plan, secret string) {
	t.Helper()
	if secret == "" {
		return
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("plan leaked secret material: %s", encoded)
	}
}

func loadValidProfile(t *testing.T) profile.Config {
	t.Helper()
	config, err := profile.LoadFile("../../testdata/profiles/valid-wgws.json")
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func hasStep(plan Plan, id string) bool {
	return findStep(plan, id).ID != ""
}

func findStep(plan Plan, id string) Step {
	for _, step := range plan.Steps {
		if step.ID == id {
			return step
		}
	}
	return Step{}
}
