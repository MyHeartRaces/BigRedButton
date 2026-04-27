package linux

import (
	"reflect"
	"testing"

	"github.com/tracegate/tracegate-launcher/internal/routes"
)

func TestRouteGetCommandIPv4(t *testing.T) {
	command, err := RouteGetCommand("203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ip", "-4", "route", "get", "203.0.113.10"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}
	if command.String() != "ip -4 route get 203.0.113.10" {
		t.Fatalf("string = %q", command.String())
	}
}

func TestRouteGetCommandIPv6(t *testing.T) {
	command, err := RouteGetCommand("2001:db8::10")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ip", "-6", "route", "get", "2001:db8::10"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}
}

func TestAddEndpointExclusionCommand(t *testing.T) {
	exclusion, err := routes.NewEndpointExclusion("203.0.113.10", "192.0.2.1", "eth0")
	if err != nil {
		t.Fatal(err)
	}

	command, err := AddEndpointExclusionCommand(exclusion)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"ip", "-4", "route", "replace", "203.0.113.10/32", "via", "192.0.2.1", "dev", "eth0"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}
}

func TestAddEndpointExclusionCommandDirect(t *testing.T) {
	exclusion, err := routes.NewEndpointExclusion("203.0.113.10", "", "eth0")
	if err != nil {
		t.Fatal(err)
	}

	command, err := AddEndpointExclusionCommand(exclusion)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"ip", "-4", "route", "replace", "203.0.113.10/32", "dev", "eth0"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}
}

func TestDeleteEndpointExclusionCommandIPv6(t *testing.T) {
	exclusion, err := routes.NewEndpointExclusion("2001:db8::10", "2001:db8::1", "eth0")
	if err != nil {
		t.Fatal(err)
	}

	command, err := DeleteEndpointExclusionCommand(exclusion)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"ip", "-6", "route", "delete", "2001:db8::10/128", "via", "2001:db8::1", "dev", "eth0"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}
}

func TestEndpointExclusionCommandRejectsFamilyMismatch(t *testing.T) {
	_, err := AddEndpointExclusionCommand(routes.EndpointExclusion{
		EndpointIP:  "203.0.113.10",
		Destination: "203.0.113.10/32",
		Gateway:     "2001:db8::1",
		Interface:   "eth0",
		Family:      routes.FamilyIPv4,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEndpointExclusionCommandRejectsNonHostPrefix(t *testing.T) {
	_, err := AddEndpointExclusionCommand(routes.EndpointExclusion{
		EndpointIP:  "203.0.113.10",
		Destination: "203.0.113.0/24",
		Gateway:     "192.0.2.1",
		Interface:   "eth0",
		Family:      routes.FamilyIPv4,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
