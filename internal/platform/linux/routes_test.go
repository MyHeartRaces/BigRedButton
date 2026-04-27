package linux

import "testing"

func TestParseRouteGetIPv4ViaGateway(t *testing.T) {
	route, err := ParseRouteGet("203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55 uid 1000\n    cache")
	if err != nil {
		t.Fatal(err)
	}
	if route.Destination != "203.0.113.10" {
		t.Fatalf("destination = %s", route.Destination)
	}
	if route.Gateway != "192.0.2.1" {
		t.Fatalf("gateway = %s", route.Gateway)
	}
	if route.Interface != "eth0" {
		t.Fatalf("interface = %s", route.Interface)
	}
	if route.Source != "192.0.2.55" {
		t.Fatalf("source = %s", route.Source)
	}
}

func TestParseRouteGetIPv4Direct(t *testing.T) {
	route, err := ParseRouteGet("203.0.113.10 dev eth0 src 192.0.2.55 uid 1000 cache")
	if err != nil {
		t.Fatal(err)
	}
	if route.Gateway != "" {
		t.Fatalf("gateway = %s", route.Gateway)
	}
	if route.Interface != "eth0" {
		t.Fatalf("interface = %s", route.Interface)
	}
}

func TestParseRouteGetIPv6(t *testing.T) {
	route, err := ParseRouteGet("2001:db8::10 from :: via 2001:db8::1 dev eth0 src 2001:db8::55 metric 100 pref medium")
	if err != nil {
		t.Fatal(err)
	}
	if route.Destination != "2001:db8::10" {
		t.Fatalf("destination = %s", route.Destination)
	}
	if route.Gateway != "2001:db8::1" {
		t.Fatalf("gateway = %s", route.Gateway)
	}
}

func TestParseRouteGetRejectsMissingInterface(t *testing.T) {
	_, err := ParseRouteGet("203.0.113.10 via 192.0.2.1 src 192.0.2.55")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEndpointExclusionFromRouteGet(t *testing.T) {
	exclusion, err := EndpointExclusionFromRouteGet("203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55")
	if err != nil {
		t.Fatal(err)
	}
	if exclusion.Destination != "203.0.113.10/32" {
		t.Fatalf("destination = %s", exclusion.Destination)
	}
	if exclusion.Gateway != "192.0.2.1" {
		t.Fatalf("gateway = %s", exclusion.Gateway)
	}
	if exclusion.Interface != "eth0" {
		t.Fatalf("interface = %s", exclusion.Interface)
	}
}
