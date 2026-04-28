package linux

import (
	"reflect"
	"testing"
)

func TestIsolatedNamespaceCommands(t *testing.T) {
	tests := []struct {
		name string
		got  Command
		want []string
	}{
		{
			name: "netns add",
			got:  mustCommand(NetNSAddCommand("brb-123e4567")),
			want: []string{"ip", "netns", "add", "brb-123e4567"},
		},
		{
			name: "veth create",
			got:  mustCommand(VethCreateCommand("brbh123e4567", "brbn123e4567")),
			want: []string{"ip", "link", "add", "brbh123e4567", "type", "veth", "peer", "name", "brbn123e4567"},
		},
		{
			name: "netns pids",
			got:  mustCommand(NetNSPidsCommand("brb-123e4567")),
			want: []string{"ip", "netns", "pids", "brb-123e4567"},
		},
		{
			name: "move peer",
			got:  mustCommand(LinkSetNetNSCommand("brbn123e4567", "brb-123e4567")),
			want: []string{"ip", "link", "set", "brbn123e4567", "netns", "brb-123e4567"},
		},
		{
			name: "namespace address",
			got:  mustCommand(NetNSAddressReplaceCommand("brb-123e4567", "brbn123e4567", "169.254.77.2/30")),
			want: []string{"ip", "-n", "brb-123e4567", "address", "replace", "169.254.77.2/30", "dev", "brbn123e4567"},
		},
		{
			name: "namespace exec",
			got:  mustCommand(NetNSExecCommand("brb-123e4567", Command{Name: "/usr/bin/curl", Args: []string{"https://example.com"}})),
			want: []string{"ip", "netns", "exec", "brb-123e4567", "/usr/bin/curl", "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got.Argv(), tt.want) {
				t.Fatalf("argv = %#v want %#v", tt.got.Argv(), tt.want)
			}
		})
	}
}

func TestIsolatedCommandRejectsBadNamespace(t *testing.T) {
	_, err := NetNSAddCommand("bad/name")
	if err == nil {
		t.Fatal("expected invalid namespace error")
	}
}

func mustCommand(command Command, err error) Command {
	if err != nil {
		panic(err)
	}
	return command
}
