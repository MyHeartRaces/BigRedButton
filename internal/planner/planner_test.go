package planner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tracegate/tracegate-launcher/internal/profile"
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
	if !hasStep(plan, "add-route-exclusion-203-0-113-10") {
		t.Fatalf("missing route exclusion step: %#v", plan.Steps)
	}
	if !hasStep(plan, "start-wstunnel") {
		t.Fatalf("missing wstunnel step: %#v", plan.Steps)
	}
	if !hasStep(plan, "apply-wireguard-peer") {
		t.Fatalf("missing wireguard peer step: %#v", plan.Steps)
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
	config, err := profile.LoadFile("../../testdata/profiles/valid-v7.json")
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
