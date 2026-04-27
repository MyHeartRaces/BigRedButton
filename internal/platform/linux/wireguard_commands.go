package linux

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unicode"
)

const DefaultWGBinary = "wg"

func WireGuardCreateInterfaceCommand(iface string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "add", "dev", iface, "type", "wireguard"}}, nil
}

func WireGuardDeleteInterfaceCommand(iface string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "delete", "dev", iface}}, nil
}

func WireGuardSetConfigCommand(iface string, configPath string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return Command{}, fmt.Errorf("wireguard config path is required")
	}
	return Command{Name: DefaultWGBinary, Args: []string{"setconf", iface, configPath}}, nil
}

func WireGuardAddAddressCommand(iface string, address string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(address))
	if err != nil {
		return Command{}, fmt.Errorf("wireguard address %q is invalid: %w", address, err)
	}
	return Command{Name: DefaultIPBinary, Args: []string{"address", "add", prefix.String(), "dev", iface}}, nil
}

func WireGuardSetMTUCommand(iface string, mtu int) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	if mtu < 1200 || mtu > 1420 {
		return Command{}, fmt.Errorf("wireguard MTU must be in 1200..1420")
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "set", "mtu", strconv.Itoa(mtu), "dev", iface}}, nil
}

func WireGuardSetUpCommand(iface string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "set", "up", "dev", iface}}, nil
}

func WireGuardRouteReplaceCommand(iface string, prefix string) (Command, error) {
	return wireGuardRouteCommand("replace", iface, prefix)
}

func WireGuardRouteDeleteCommand(iface string, prefix string) (Command, error) {
	return wireGuardRouteCommand("delete", iface, prefix)
}

func wireGuardRouteCommand(action string, iface string, rawPrefix string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(rawPrefix))
	if err != nil {
		return Command{}, fmt.Errorf("wireguard route prefix %q is invalid: %w", rawPrefix, err)
	}
	return Command{Name: DefaultIPBinary, Args: []string{familyFlag(prefix.Addr()), "route", action, prefix.String(), "dev", iface}}, nil
}

func validateInterfaceName(iface string) (string, error) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return "", fmt.Errorf("wireguard interface name is required")
	}
	if len(iface) > 15 {
		return "", fmt.Errorf("wireguard interface name must be 15 bytes or fewer")
	}
	for _, r := range iface {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", fmt.Errorf("wireguard interface name contains unsupported character %q", r)
	}
	return iface, nil
}
