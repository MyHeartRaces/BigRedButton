package wireguard

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/tracegate/tracegate-launcher/internal/profile"
)

type Config struct {
	InterfaceName       string
	PrivateKey          string
	PeerPublicKey       string
	PresharedKey        string
	Endpoint            string
	Addresses           []string
	AllowedIPs          []string
	MTU                 int
	PersistentKeepalive int
}

type Summary struct {
	InterfaceName       string   `json:"interface_name"`
	Endpoint            string   `json:"endpoint"`
	Addresses           []string `json:"addresses"`
	AllowedIPs          []string `json:"allowed_ips"`
	MTU                 int      `json:"mtu"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
	HasPresharedKey     bool     `json:"has_preshared_key"`
}

func ConfigFromProfile(config profile.Config, interfaceName string) Config {
	return Config{
		InterfaceName:       strings.TrimSpace(interfaceName),
		PrivateKey:          config.WireGuardPrivateKey,
		PeerPublicKey:       config.ServerPublicKey,
		PresharedKey:        config.PresharedKey,
		Endpoint:            config.LocalUDPListen,
		Addresses:           append([]string(nil), config.Addresses...),
		AllowedIPs:          append([]string(nil), config.AllowedIPs...),
		MTU:                 config.MTU,
		PersistentKeepalive: config.PersistentKeepalive,
	}
}

func RenderSetConf(config Config) (string, error) {
	if err := config.Validate(); err != nil {
		return "", err
	}

	var builder strings.Builder
	builder.WriteString("[Interface]\n")
	builder.WriteString("PrivateKey = ")
	builder.WriteString(strings.TrimSpace(config.PrivateKey))
	builder.WriteString("\n\n[Peer]\n")
	builder.WriteString("PublicKey = ")
	builder.WriteString(strings.TrimSpace(config.PeerPublicKey))
	builder.WriteByte('\n')
	if strings.TrimSpace(config.PresharedKey) != "" {
		builder.WriteString("PresharedKey = ")
		builder.WriteString(strings.TrimSpace(config.PresharedKey))
		builder.WriteByte('\n')
	}
	builder.WriteString("AllowedIPs = ")
	builder.WriteString(strings.Join(normalizedPrefixes(config.AllowedIPs), ", "))
	builder.WriteByte('\n')
	builder.WriteString("Endpoint = ")
	builder.WriteString(strings.TrimSpace(config.Endpoint))
	builder.WriteByte('\n')
	builder.WriteString("PersistentKeepalive = ")
	builder.WriteString(fmt.Sprintf("%d", config.PersistentKeepalive))
	builder.WriteByte('\n')
	return builder.String(), nil
}

func (c Config) Summary() Summary {
	return Summary{
		InterfaceName:       c.InterfaceName,
		Endpoint:            c.Endpoint,
		Addresses:           append([]string(nil), c.Addresses...),
		AllowedIPs:          append([]string(nil), c.AllowedIPs...),
		MTU:                 c.MTU,
		PersistentKeepalive: c.PersistentKeepalive,
		HasPresharedKey:     strings.TrimSpace(c.PresharedKey) != "",
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.InterfaceName) == "" {
		return fmt.Errorf("wireguard interface name is required")
	}
	if strings.TrimSpace(c.PrivateKey) == "" {
		return fmt.Errorf("wireguard private key is required")
	}
	if strings.TrimSpace(c.PeerPublicKey) == "" {
		return fmt.Errorf("wireguard peer public key is required")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("wireguard endpoint is required")
	}
	if len(c.Addresses) == 0 {
		return fmt.Errorf("wireguard address is required")
	}
	for _, address := range c.Addresses {
		if _, err := netip.ParsePrefix(strings.TrimSpace(address)); err != nil {
			return fmt.Errorf("wireguard address %q is invalid: %w", address, err)
		}
	}
	if len(c.AllowedIPs) == 0 {
		return fmt.Errorf("wireguard allowed IPs are required")
	}
	for _, allowedIP := range c.AllowedIPs {
		if _, err := netip.ParsePrefix(strings.TrimSpace(allowedIP)); err != nil {
			return fmt.Errorf("wireguard allowed IP %q is invalid: %w", allowedIP, err)
		}
	}
	if c.MTU < 1200 || c.MTU > 1420 {
		return fmt.Errorf("wireguard MTU must be in 1200..1420")
	}
	if c.PersistentKeepalive < 0 || c.PersistentKeepalive > 60 {
		return fmt.Errorf("wireguard persistent keepalive must be in 0..60")
	}
	return nil
}

func normalizedPrefixes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			out = append(out, strings.TrimSpace(value))
			continue
		}
		out = append(out, prefix.String())
	}
	return out
}
