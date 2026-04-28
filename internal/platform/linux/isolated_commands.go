package linux

import (
	"fmt"
	"net/netip"
	"strings"
	"unicode"
)

const DefaultNFTBinary = "nft"

func NetNSAddCommand(namespace string) (Command, error) {
	namespace, err := validateNamespaceName(namespace)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"netns", "add", namespace}}, nil
}

func NetNSDeleteCommand(namespace string) (Command, error) {
	namespace, err := validateNamespaceName(namespace)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"netns", "delete", namespace}}, nil
}

func NetNSPidsCommand(namespace string) (Command, error) {
	namespace, err := validateNamespaceName(namespace)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"netns", "pids", namespace}}, nil
}

func VethCreateCommand(hostVeth string, namespaceVeth string) (Command, error) {
	hostVeth, err := validateInterfaceName(hostVeth)
	if err != nil {
		return Command{}, fmt.Errorf("host veth: %w", err)
	}
	namespaceVeth, err = validateInterfaceName(namespaceVeth)
	if err != nil {
		return Command{}, fmt.Errorf("namespace veth: %w", err)
	}
	if hostVeth == namespaceVeth {
		return Command{}, fmt.Errorf("veth names must be different")
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "add", hostVeth, "type", "veth", "peer", "name", namespaceVeth}}, nil
}

func LinkSetNetNSCommand(iface string, namespace string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	namespace, err = validateNamespaceName(namespace)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "set", iface, "netns", namespace}}, nil
}

func LinkSetUpCommand(iface string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "set", "dev", iface, "up"}}, nil
}

func LinkDeleteCommand(iface string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	return Command{Name: DefaultIPBinary, Args: []string{"link", "delete", "dev", iface}}, nil
}

func AddressReplaceCommand(iface string, address string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(address))
	if err != nil {
		return Command{}, fmt.Errorf("address %q is invalid: %w", address, err)
	}
	return Command{Name: DefaultIPBinary, Args: []string{"address", "replace", prefix.String(), "dev", iface}}, nil
}

func NetNSAddressReplaceCommand(namespace string, iface string, address string) (Command, error) {
	command, err := AddressReplaceCommand(iface, address)
	if err != nil {
		return Command{}, err
	}
	return netNSIPCommand(namespace, command.Args)
}

func NetNSLinkSetUpCommand(namespace string, iface string) (Command, error) {
	iface, err := validateInterfaceName(iface)
	if err != nil {
		return Command{}, err
	}
	return netNSIPCommand(namespace, []string{"link", "set", "dev", iface, "up"})
}

func NetNSLoopbackSetUpCommand(namespace string) (Command, error) {
	return netNSIPCommand(namespace, []string{"link", "set", "dev", "lo", "up"})
}

func NetNSExecCommand(namespace string, command Command) (Command, error) {
	namespace, err := validateNamespaceName(namespace)
	if err != nil {
		return Command{}, err
	}
	if strings.TrimSpace(command.Name) == "" {
		return Command{}, fmt.Errorf("command name is required")
	}
	args := []string{"netns", "exec", namespace, command.Name}
	args = append(args, command.Args...)
	return Command{Name: DefaultIPBinary, Args: args}, nil
}

func NftApplyRulesetCommand(path string) (Command, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Command{}, fmt.Errorf("nft ruleset path is required")
	}
	return Command{Name: DefaultNFTBinary, Args: []string{"-f", path}}, nil
}

func NftFlushRulesetCommand() Command {
	return Command{Name: DefaultNFTBinary, Args: []string{"flush", "ruleset"}}
}

func netNSIPCommand(namespace string, args []string) (Command, error) {
	namespace, err := validateNamespaceName(namespace)
	if err != nil {
		return Command{}, err
	}
	commandArgs := []string{"-n", namespace}
	commandArgs = append(commandArgs, args...)
	return Command{Name: DefaultIPBinary, Args: commandArgs}, nil
}

func validateNamespaceName(namespace string) (string, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return "", fmt.Errorf("namespace name is required")
	}
	if len(namespace) > 64 {
		return "", fmt.Errorf("namespace name must be 64 bytes or fewer")
	}
	for _, r := range namespace {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", fmt.Errorf("namespace name contains unsupported character %q", r)
	}
	return namespace, nil
}
