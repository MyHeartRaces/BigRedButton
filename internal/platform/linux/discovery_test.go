package linux

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverEndpointExclusion(t *testing.T) {
	runner := &fakeCommandRunner{
		output: "203.0.113.10 via 192.0.2.1 dev eth0 src 192.0.2.55 uid 1000 cache",
	}

	exclusion, err := DiscoverEndpointExclusion(context.Background(), runner, "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}

	wantCommand := []string{"ip", "-4", "route", "get", "203.0.113.10"}
	if !reflect.DeepEqual(runner.command.Argv(), wantCommand) {
		t.Fatalf("argv = %#v want %#v", runner.command.Argv(), wantCommand)
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

func TestDiscoverEndpointExclusionRejectsNilRunner(t *testing.T) {
	_, err := DiscoverEndpointExclusion(context.Background(), nil, "203.0.113.10")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDiscoverEndpointExclusionReturnsCommandOutputOnFailure(t *testing.T) {
	runner := &fakeCommandRunner{
		output: "RTNETLINK answers: Network is unreachable",
		err:    errors.New("exit status 2"),
	}

	_, err := DiscoverEndpointExclusion(context.Background(), runner, "203.0.113.10")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Network is unreachable") {
		t.Fatalf("expected command output in error, got: %v", err)
	}
}

type fakeCommandRunner struct {
	command Command
	output  string
	err     error
}

func (f *fakeCommandRunner) Run(_ context.Context, command Command) ([]byte, error) {
	f.command = command
	return []byte(f.output), f.err
}
