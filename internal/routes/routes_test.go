package routes

import "testing"

func TestNewEndpointExclusionIPv4(t *testing.T) {
	exclusion, err := NewEndpointExclusion("203.0.113.10", "192.0.2.1", "eth0")
	if err != nil {
		t.Fatal(err)
	}
	if exclusion.EndpointIP != "203.0.113.10" {
		t.Fatalf("endpoint = %s", exclusion.EndpointIP)
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
	if exclusion.Family != FamilyIPv4 {
		t.Fatalf("family = %s", exclusion.Family)
	}
}

func TestNewEndpointExclusionIPv6(t *testing.T) {
	exclusion, err := NewEndpointExclusion("2001:db8::10", "2001:db8::1", "eth0")
	if err != nil {
		t.Fatal(err)
	}
	if exclusion.Destination != "2001:db8::10/128" {
		t.Fatalf("destination = %s", exclusion.Destination)
	}
	if exclusion.Family != FamilyIPv6 {
		t.Fatalf("family = %s", exclusion.Family)
	}
}

func TestNewEndpointExclusionRequiresGatewayOrInterface(t *testing.T) {
	_, err := NewEndpointExclusion("203.0.113.10", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewEndpointExclusionRejectsGatewayFamilyMismatch(t *testing.T) {
	_, err := NewEndpointExclusion("203.0.113.10", "2001:db8::1", "eth0")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCommandKey(t *testing.T) {
	exclusion, err := NewEndpointExclusion("203.0.113.10", "192.0.2.1", "eth0")
	if err != nil {
		t.Fatal(err)
	}
	got := CommandKey(exclusion)
	want := "ipv4 203.0.113.10/32 via 192.0.2.1 dev eth0"
	if got != want {
		t.Fatalf("CommandKey() = %q want %q", got, want)
	}
}
