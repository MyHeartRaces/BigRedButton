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
			fn:   func() (Command, error) { return WireGuardCreateInterfaceCommand("tg-v7") },
			want: []string{"ip", "link", "add", "dev", "tg-v7", "type", "wireguard"},
		},
		{
			name: "delete",
			fn:   func() (Command, error) { return WireGuardDeleteInterfaceCommand("tg-v7") },
			want: []string{"ip", "link", "delete", "dev", "tg-v7"},
		},
		{
			name: "set config",
			fn:   func() (Command, error) { return WireGuardSetConfigCommand("tg-v7", "/run/big-red-button/wg.conf") },
			want: []string{"wg", "setconf", "tg-v7", "/run/big-red-button/wg.conf"},
		},
		{
			name: "add address",
			fn:   func() (Command, error) { return WireGuardAddAddressCommand("tg-v7", "10.70.0.2/32") },
			want: []string{"ip", "address", "add", "10.70.0.2/32", "dev", "tg-v7"},
		},
		{
			name: "set MTU",
			fn:   func() (Command, error) { return WireGuardSetMTUCommand("tg-v7", 1280) },
			want: []string{"ip", "link", "set", "mtu", "1280", "dev", "tg-v7"},
		},
		{
			name: "set up",
			fn:   func() (Command, error) { return WireGuardSetUpCommand("tg-v7") },
			want: []string{"ip", "link", "set", "up", "dev", "tg-v7"},
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
	command, err := WireGuardRouteReplaceCommand("tg-v7", "0.0.0.0/0")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ip", "-4", "route", "replace", "0.0.0.0/0", "dev", "tg-v7"}
	if !reflect.DeepEqual(command.Argv(), want) {
		t.Fatalf("argv = %#v want %#v", command.Argv(), want)
	}

	command, err = WireGuardRouteDeleteCommand("tg-v7", "::/0")
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"ip", "-6", "route", "delete", "::/0", "dev", "tg-v7"}
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
	_, err := WireGuardSetMTUCommand("tg-v7", 9000)
	if err == nil {
		t.Fatal("expected error")
	}
}
