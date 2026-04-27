package linux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/MyHeartRaces/BigRedButton/internal/routes"
)

type CommandRunner interface {
	Run(ctx context.Context, command Command) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command Command) ([]byte, error) {
	return exec.CommandContext(ctx, command.Name, command.Args...).CombinedOutput()
}

func DiscoverEndpointExclusion(ctx context.Context, runner CommandRunner, endpointIP string) (routes.EndpointExclusion, error) {
	if runner == nil {
		return routes.EndpointExclusion{}, fmt.Errorf("linux command runner is nil")
	}
	command, err := RouteGetCommand(endpointIP)
	if err != nil {
		return routes.EndpointExclusion{}, err
	}
	output, err := runner.Run(ctx, command)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return routes.EndpointExclusion{}, fmt.Errorf("run %s: %w", command.String(), err)
		}
		return routes.EndpointExclusion{}, fmt.Errorf("run %s: %w: %s", command.String(), err, detail)
	}
	return EndpointExclusionFromRouteGet(string(output))
}
