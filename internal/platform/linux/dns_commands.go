package linux

import (
	"fmt"
	"net/netip"
	"strings"
)

const DefaultResolveCtlBinary = "resolvectl"

func ResolveCtlDNSCommand(iface string, servers []string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	if len(servers) == 0 {
		return Command{}, fmt.Errorf("DNS server list is required")
	}
	args := []string{"dns", iface}
	for _, server := range servers {
		addr, err := netip.ParseAddr(strings.TrimSpace(server))
		if err != nil {
			return Command{}, fmt.Errorf("DNS server %q is invalid: %w", server, err)
		}
		args = append(args, addr.String())
	}
	return Command{Name: DefaultResolveCtlBinary, Args: args}, nil
}

func ResolveCtlDomainCommand(iface string, domains []string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	if len(domains) == 0 {
		return Command{}, fmt.Errorf("DNS routing domains are required")
	}
	args := []string{"domain", iface}
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" || strings.ContainsAny(domain, " \t\r\n") {
			return Command{}, fmt.Errorf("DNS routing domain %q is invalid", domain)
		}
		args = append(args, domain)
	}
	return Command{Name: DefaultResolveCtlBinary, Args: args}, nil
}

func ResolveCtlDefaultRouteCommand(iface string, enabled bool) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	value := "no"
	if enabled {
		value = "yes"
	}
	return Command{Name: DefaultResolveCtlBinary, Args: []string{"default-route", iface, value}}, nil
}

func ResolveCtlRevertCommand(iface string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultResolveCtlBinary, Args: []string{"revert", iface}}, nil
}
