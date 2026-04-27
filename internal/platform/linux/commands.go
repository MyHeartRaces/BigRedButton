package linux

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/tracegate/big-red-button/internal/routes"
)

const DefaultIPBinary = "ip"

type Command struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

func (c Command) Argv() []string {
	argv := make([]string, 0, 1+len(c.Args))
	argv = append(argv, c.Name)
	argv = append(argv, c.Args...)
	return argv
}

func (c Command) String() string {
	return strings.Join(c.Argv(), " ")
}

func RouteGetCommand(endpointIP string) (Command, error) {
	endpoint, err := netip.ParseAddr(strings.TrimSpace(endpointIP))
	if err != nil {
		return Command{}, fmt.Errorf("endpoint IP %q is invalid: %w", endpointIP, err)
	}
	return Command{
		Name: DefaultIPBinary,
		Args: []string{familyFlag(endpoint), "route", "get", endpoint.String()},
	}, nil
}

func AddEndpointExclusionCommand(exclusion routes.EndpointExclusion) (Command, error) {
	return endpointExclusionCommand("replace", exclusion)
}

func DeleteEndpointExclusionCommand(exclusion routes.EndpointExclusion) (Command, error) {
	return endpointExclusionCommand("delete", exclusion)
}

func endpointExclusionCommand(action string, exclusion routes.EndpointExclusion) (Command, error) {
	destination, err := netip.ParsePrefix(strings.TrimSpace(exclusion.Destination))
	if err != nil {
		return Command{}, fmt.Errorf("route exclusion destination %q is invalid: %w", exclusion.Destination, err)
	}
	destination = destination.Masked()
	if destination != routes.HostPrefix(destination.Addr()) {
		return Command{}, fmt.Errorf("route exclusion destination must be a host prefix")
	}

	if exclusion.EndpointIP != "" {
		endpoint, err := netip.ParseAddr(strings.TrimSpace(exclusion.EndpointIP))
		if err != nil {
			return Command{}, fmt.Errorf("route exclusion endpoint %q is invalid: %w", exclusion.EndpointIP, err)
		}
		if endpoint != destination.Addr() {
			return Command{}, fmt.Errorf("route exclusion endpoint does not match destination")
		}
	}

	family := routes.FamilyOf(destination.Addr())
	if exclusion.Family != "" && exclusion.Family != family {
		return Command{}, fmt.Errorf("route exclusion family does not match destination")
	}

	gateway := strings.TrimSpace(exclusion.Gateway)
	if gateway != "" {
		gatewayAddr, err := netip.ParseAddr(gateway)
		if err != nil {
			return Command{}, fmt.Errorf("route exclusion gateway %q is invalid: %w", exclusion.Gateway, err)
		}
		if routes.FamilyOf(gatewayAddr) != family {
			return Command{}, fmt.Errorf("route exclusion gateway family does not match destination")
		}
		gateway = gatewayAddr.String()
	}
	iface := strings.TrimSpace(exclusion.Interface)
	if gateway == "" && iface == "" {
		return Command{}, fmt.Errorf("route exclusion requires gateway or interface")
	}

	args := []string{familyFlag(destination.Addr()), "route", action, destination.String()}
	if gateway != "" {
		args = append(args, "via", gateway)
	}
	if iface != "" {
		args = append(args, "dev", iface)
	}

	return Command{Name: DefaultIPBinary, Args: args}, nil
}

func familyFlag(addr netip.Addr) string {
	if addr.Is4() {
		return "-4"
	}
	return "-6"
}
