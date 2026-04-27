package routes

import (
	"fmt"
	"net/netip"
	"strings"
)

type Family string

const (
	FamilyIPv4 Family = "ipv4"
	FamilyIPv6 Family = "ipv6"
)

type EndpointExclusion struct {
	EndpointIP  string `json:"endpoint_ip"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Family      Family `json:"family"`
}

func NewEndpointExclusion(endpointIP string, gateway string, iface string) (EndpointExclusion, error) {
	endpoint, err := netip.ParseAddr(strings.TrimSpace(endpointIP))
	if err != nil {
		return EndpointExclusion{}, fmt.Errorf("endpoint IP %q is invalid: %w", endpointIP, err)
	}
	if !endpoint.Is4() && !endpoint.Is6() {
		return EndpointExclusion{}, fmt.Errorf("endpoint IP %q must be IPv4 or IPv6", endpointIP)
	}

	gateway = strings.TrimSpace(gateway)
	iface = strings.TrimSpace(iface)
	if gateway == "" && iface == "" {
		return EndpointExclusion{}, fmt.Errorf("endpoint route exclusion requires gateway or interface")
	}

	if gateway != "" {
		gatewayAddr, err := netip.ParseAddr(gateway)
		if err != nil {
			return EndpointExclusion{}, fmt.Errorf("gateway %q is invalid: %w", gateway, err)
		}
		if endpoint.Is4() != gatewayAddr.Is4() {
			return EndpointExclusion{}, fmt.Errorf("gateway family must match endpoint family")
		}
		gateway = gatewayAddr.String()
	}

	return EndpointExclusion{
		EndpointIP:  endpoint.String(),
		Destination: HostPrefix(endpoint).String(),
		Gateway:     gateway,
		Interface:   iface,
		Family:      FamilyOf(endpoint),
	}, nil
}

func HostPrefix(addr netip.Addr) netip.Prefix {
	if addr.Is4() {
		return netip.PrefixFrom(addr, 32)
	}
	return netip.PrefixFrom(addr, 128)
}

func FamilyOf(addr netip.Addr) Family {
	if addr.Is4() {
		return FamilyIPv4
	}
	return FamilyIPv6
}

func CommandKey(exclusion EndpointExclusion) string {
	parts := []string{string(exclusion.Family), exclusion.Destination}
	if exclusion.Gateway != "" {
		parts = append(parts, "via", exclusion.Gateway)
	}
	if exclusion.Interface != "" {
		parts = append(parts, "dev", exclusion.Interface)
	}
	return strings.Join(parts, " ")
}
