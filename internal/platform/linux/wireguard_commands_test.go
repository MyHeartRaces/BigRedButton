package linux

import (
	"reflect"
	"testing"
)

func TestWireGuardInterfaceCommands(t *testing.T) {
	tests := []struct {
		name string
		fn   func() (Command, error)
		want []string
	}{
		{
			name: "create",
			fn:   func() (Command, error) { return WireGuardCreateInterfaceCommand("brb0") },
			want: []string{"ip", "link", "add", "dev", "brb0", "type", "wireguard"},
		},
		{
			name: "delete",
			fn:   func() (Command, error) { return WireGuardDeleteInterfaceCommand("brb0") },
			want: []string{"ip", "link", "delete", "dev", "brb0"},
		},
		{
			name: "set config",
			fn:   func() (Command, error) { return WireGuardSetConfigCommand("brb0", "/run/big-red-button/wg.conf") },
			want: []string{"wg", "setconf", "brb0", "/run/big-red-button/wg.conf"},
		},
		{
			name: "add address",
			fn:   func() (Command, error) { return WireGuardAddAddressCommand("brb0", "10.70.0.2/32") },
			want: []string{"ip", "address", "add", "10.70.0.2/32", "dev", "brb0"},
		},
		{
			name: "set MTU",
			fn:   func() (Command, error) { return WireGuardSetMTUCommand("brb0", 1280) },
			want: []string{"ip", "link", "set", "mtu", "1280", "dev", "brb0"},
		},
		{
			name: "set up",
			fn:   func() (Command, error) { return WireGuardSetUpCommand("brb0") },
			want: []string{"ip", "link", "set", "up", "dev", "brb0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := tt.fn()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(command.Argv(), tt.want) {
				t.Fatalf("argv = %#v want %#v", command.Argv(), tt.want)
			}
		})
	}
}

func TestWireGuardRouteCommands(t *testing.T) {
	command, err := WireGuardRouteReplaceCommand("brb0", "0.0.0.0/0")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ip", "-4", "route", "replace", "0.0.0.0/0", "dev", "brb0"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}

	command, err = WireGuardRouteDeleteCommand("brb0", "::/0")
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"ip", "-6", "route", "delete", "::/0", "dev", "brb0"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}
}

func TestWireGuardCommandRejectsInvalidInterfaceName(t *testing.T) {
	_, err := WireGuardCreateInterfaceCommand("bad/name")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWireGuardCommandRejectsInvalidMTU(t *testing.T) {
	_, err := WireGuardSetMTUCommand("brb0", 9000)
	if err == nil {
		t.Fatal("expected error")
	}
}
