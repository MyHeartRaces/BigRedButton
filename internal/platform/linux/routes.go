package linux

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/tracegate/tracegate-launcher/internal/routes"
)

type RouteGet struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Interface   string `json:"interface"`
	Source      string `json:"source,omitempty"`
}

func ParseRouteGet(output string) (RouteGet, error) {
	tokens := strings.Fields(output)
	if len(tokens) == 0 {
		return RouteGet{}, fmt.Errorf("route output is empty")
	}

	var result RouteGet
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if result.Destination == "" {
			if _, err := netip.ParseAddr(token); err == nil {
				result.Destination = token
				continue
			}
		}
		switch token {
		case "via":
			if index+1 >= len(tokens) {
				return RouteGet{}, fmt.Errorf("route output has via without gateway")
			}
			result.Gateway = tokens[index+1]
			index++
		case "dev":
			if index+1 >= len(tokens) {
				return RouteGet{}, fmt.Errorf("route output has dev without interface")
			}
			result.Interface = tokens[index+1]
			index++
		case "src":
			if index+1 >= len(tokens) {
				return RouteGet{}, fmt.Errorf("route output has src without address")
			}
			result.Source = tokens[index+1]
			index++
		}
	}

	if result.Destination == "" {
		return RouteGet{}, fmt.Errorf("route output does not contain destination address")
	}
	if _, err := netip.ParseAddr(result.Destination); err != nil {
		return RouteGet{}, fmt.Errorf("route destination is invalid: %w", err)
	}
	if result.Gateway != "" {
		if _, err := netip.ParseAddr(result.Gateway); err != nil {
			return RouteGet{}, fmt.Errorf("route gateway is invalid: %w", err)
		}
	}
	if result.Interface == "" {
		return RouteGet{}, fmt.Errorf("route output does not contain interface")
	}
	if result.Source != "" {
		if _, err := netip.ParseAddr(result.Source); err != nil {
			return RouteGet{}, fmt.Errorf("route source is invalid: %w", err)
		}
	}
	return result, nil
}

func EndpointExclusionFromRouteGet(output string) (routes.EndpointExclusion, error) {
	route, err := ParseRouteGet(output)
	if err != nil {
		return routes.EndpointExclusion{}, err
	}
	return routes.NewEndpointExclusion(route.Destination, route.Gateway, route.Interface)
}
