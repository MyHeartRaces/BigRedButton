package profile

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const (
	DisplayProfileType    = "WGWS"
	legacyWGWSProfileName = "V7-WireGuard-WSTunnel-Direct"
)

type Config struct {
	Protocol            string   `json:"protocol"`
	Transport           string   `json:"transport"`
	Name                string   `json:"profile"`
	Server              string   `json:"server"`
	Port                int      `json:"port"`
	SNI                 string   `json:"sni,omitempty"`
	WSTunnelHost        string   `json:"wstunnel_host"`
	WSTunnelURL         string   `json:"wstunnel_url"`
	WSTunnelPath        string   `json:"wstunnel_path"`
	WSTunnelTLSName     string   `json:"wstunnel_tls_server_name,omitempty"`
	LocalUDPListen      string   `json:"local_udp_listen"`
	WireGuardPrivateKey string   `json:"wireguard_private_key"`
	WireGuardPublicKey  string   `json:"wireguard_public_key,omitempty"`
	ServerPublicKey     string   `json:"wireguard_server_public_key"`
	PresharedKey        string   `json:"wireguard_preshared_key,omitempty"`
	Addresses           []string `json:"addresses"`
	AllowedIPs          []string `json:"allowed_ips"`
	DNS                 string   `json:"dns,omitempty"`
	MTU                 int      `json:"mtu"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
}

type Summary struct {
	Profile             string   `json:"profile"`
	Server              string   `json:"server"`
	Port                int      `json:"port"`
	WSTunnelHost        string   `json:"wstunnel_host"`
	WSTunnelURL         string   `json:"wstunnel_url"`
	WSTunnelPath        string   `json:"wstunnel_path"`
	LocalUDPListen      string   `json:"local_udp_listen"`
	Addresses           []string `json:"addresses"`
	AllowedIPs          []string `json:"allowed_ips"`
	DNSConfigured       bool     `json:"dns_configured"`
	MTU                 int      `json:"mtu"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
	HasPresharedKey     bool     `json:"has_preshared_key"`
	Fingerprint         string   `json:"fingerprint"`
}

type ValidationError struct {
	Problems []string `json:"problems"`
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 0 {
		return "profile validation failed"
	}
	return "profile validation failed: " + strings.Join(e.Problems, "; ")
}

func AsValidationError(err error) (*ValidationError, bool) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return validationErr, true
	}
	return nil, false
}

func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseWGWS(data)
}

func ParseWGWS(data []byte) (Config, error) {
	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse profile json: %w", err)
	}
	return normalize(raw)
}

func (c Config) Summary() Summary {
	return Summary{
		Profile:             DisplayProfileType,
		Server:              c.Server,
		Port:                c.Port,
		WSTunnelHost:        c.WSTunnelHost,
		WSTunnelURL:         c.WSTunnelURL,
		WSTunnelPath:        c.WSTunnelPath,
		LocalUDPListen:      c.LocalUDPListen,
		Addresses:           append([]string(nil), c.Addresses...),
		AllowedIPs:          append([]string(nil), c.AllowedIPs...),
		DNSConfigured:       strings.TrimSpace(c.DNS) != "",
		MTU:                 c.MTU,
		PersistentKeepalive: c.PersistentKeepalive,
		HasPresharedKey:     strings.TrimSpace(c.PresharedKey) != "",
		Fingerprint:         c.Fingerprint(),
	}
}

func (c Config) Fingerprint() string {
	payload := struct {
		Name                string   `json:"profile"`
		WSTunnelURL         string   `json:"wstunnel_url"`
		LocalUDPListen      string   `json:"local_udp_listen"`
		WireGuardPrivateKey string   `json:"wireguard_private_key"`
		ServerPublicKey     string   `json:"wireguard_server_public_key"`
		PresharedKey        string   `json:"wireguard_preshared_key"`
		Addresses           []string `json:"addresses"`
		AllowedIPs          []string `json:"allowed_ips"`
		DNS                 string   `json:"dns,omitempty"`
		MTU                 int      `json:"mtu"`
		PersistentKeepalive int      `json:"persistent_keepalive"`
	}{
		Name:                c.Name,
		WSTunnelURL:         c.WSTunnelURL,
		LocalUDPListen:      c.LocalUDPListen,
		WireGuardPrivateKey: c.WireGuardPrivateKey,
		ServerPublicKey:     c.ServerPublicKey,
		PresharedKey:        c.PresharedKey,
		Addresses:           c.Addresses,
		AllowedIPs:          c.AllowedIPs,
		DNS:                 strings.TrimSpace(c.DNS),
		MTU:                 c.MTU,
		PersistentKeepalive: c.PersistentKeepalive,
	}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])[:16]
}

type rawConfig struct {
	Protocol  string        `json:"protocol"`
	Transport string        `json:"transport"`
	Name      string        `json:"profile"`
	Server    string        `json:"server"`
	Port      *int          `json:"port"`
	SNI       string        `json:"sni"`
	WSTunnel  rawWSTunnel   `json:"wstunnel"`
	WireGuard rawWireGuard  `json:"wireguard"`
	LocalSock any           `json:"local_socks"`
	Extra     []interface{} `json:"-"`
}

type rawWSTunnel struct {
	Mode           string `json:"mode"`
	URL            string `json:"url"`
	Path           string `json:"path"`
	TLSServerName  string `json:"tls_server_name"`
	LocalUDPListen string `json:"local_udp_listen"`
}

type rawWireGuard struct {
	PrivateKey          string `json:"private_key"`
	PublicKey           string `json:"public_key"`
	PresharedKey        string `json:"preshared_key"`
	ServerPublicKey     string `json:"server_public_key"`
	Address             any    `json:"address"`
	AllowedIPs          any    `json:"allowed_ips"`
	DNS                 string `json:"dns"`
	MTU                 *int   `json:"mtu"`
	PersistentKeepalive *int   `json:"persistent_keepalive"`
}

func normalize(raw rawConfig) (Config, error) {
	var problems []string
	addProblem := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	protocol := strings.ToLower(strings.TrimSpace(raw.Protocol))
	if protocol != "wireguard" {
		addProblem("protocol must be wireguard")
	}

	transport := strings.ToLower(strings.TrimSpace(raw.Transport))
	if transport != "wstunnel" {
		addProblem("transport must be wstunnel")
	}

	name := strings.TrimSpace(raw.Name)
	if name != legacyWGWSProfileName {
		addProblem("profile type is not supported")
	}

	port := 443
	if raw.Port == nil {
		addProblem("port is required and must be 443")
	} else if *raw.Port != 443 {
		addProblem("port must be 443")
	} else {
		port = *raw.Port
	}

	wstunnelURL, wstunnelPath, wstunnelHost := normalizeWSTunnelURL(raw.WSTunnel.URL, &problems)
	rawPath := strings.TrimSpace(raw.WSTunnel.Path)
	if rawPath != "" {
		if !validHTTPPath(rawPath) {
			addProblem("wstunnel.path must be a clean absolute HTTP path")
		} else if wstunnelPath != "" && rawPath != wstunnelPath {
			addProblem("wstunnel.path must match the WSTunnel URL path")
		}
	} else {
		rawPath = wstunnelPath
	}

	mode := strings.TrimSpace(raw.WSTunnel.Mode)
	if mode != "" && mode != "wireguard-over-websocket" {
		addProblem("wstunnel.mode must be wireguard-over-websocket")
	}

	localUDP := normalizeLoopbackEndpoint(raw.WSTunnel.LocalUDPListen, "wstunnel.local_udp_listen", &problems)

	privateKey := strings.TrimSpace(raw.WireGuard.PrivateKey)
	validateRequiredWGKey(privateKey, "wireguard.private_key", &problems)

	publicKey := strings.TrimSpace(raw.WireGuard.PublicKey)
	if publicKey != "" {
		validateWGKey(publicKey, "wireguard.public_key", &problems)
	}

	serverPublicKey := strings.TrimSpace(raw.WireGuard.ServerPublicKey)
	validateRequiredWGKey(serverPublicKey, "wireguard.server_public_key", &problems)

	presharedKey := strings.TrimSpace(raw.WireGuard.PresharedKey)
	if presharedKey != "" {
		validateWGKey(presharedKey, "wireguard.preshared_key", &problems)
	}

	addresses := normalizeStringList(raw.WireGuard.Address)
	if len(addresses) == 0 {
		addProblem("wireguard.address is required")
	}
	for _, address := range addresses {
		if !validAddressOrPrefix(address) {
			addProblem("wireguard.address entry is invalid: %s", address)
		}
	}

	allowedIPs := normalizeStringList(raw.WireGuard.AllowedIPs)
	if len(allowedIPs) == 0 {
		addProblem("wireguard.allowed_ips is required")
	}
	for _, allowedIP := range allowedIPs {
		if _, err := netip.ParsePrefix(allowedIP); err != nil {
			addProblem("wireguard.allowed_ips entry must be CIDR: %s", allowedIP)
		}
	}

	mtu := 0
	if raw.WireGuard.MTU == nil {
		addProblem("wireguard.mtu is required")
	} else {
		mtu = *raw.WireGuard.MTU
		if mtu < 1200 || mtu > 1420 {
			addProblem("wireguard.mtu must be in 1200..1420")
		}
	}

	keepalive := 0
	if raw.WireGuard.PersistentKeepalive == nil {
		addProblem("wireguard.persistent_keepalive is required")
	} else {
		keepalive = *raw.WireGuard.PersistentKeepalive
		if keepalive < 0 || keepalive > 60 {
			addProblem("wireguard.persistent_keepalive must be in 0..60")
		}
	}

	server := strings.TrimSpace(raw.Server)
	if server == "" {
		server = wstunnelHost
	}

	if len(problems) > 0 {
		return Config{}, &ValidationError{Problems: problems}
	}

	return Config{
		Protocol:            protocol,
		Transport:           transport,
		Name:                name,
		Server:              server,
		Port:                port,
		SNI:                 strings.TrimSpace(raw.SNI),
		WSTunnelHost:        wstunnelHost,
		WSTunnelURL:         wstunnelURL,
		WSTunnelPath:        rawPath,
		WSTunnelTLSName:     strings.TrimSpace(raw.WSTunnel.TLSServerName),
		LocalUDPListen:      localUDP,
		WireGuardPrivateKey: privateKey,
		WireGuardPublicKey:  publicKey,
		ServerPublicKey:     serverPublicKey,
		PresharedKey:        presharedKey,
		Addresses:           addresses,
		AllowedIPs:          allowedIPs,
		DNS:                 strings.TrimSpace(raw.WireGuard.DNS),
		MTU:                 mtu,
		PersistentKeepalive: keepalive,
	}, nil
}

func normalizeWSTunnelURL(raw string, problems *[]string) (string, string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		*problems = append(*problems, "wstunnel.url is required")
		return "", "", ""
	}
	if hasWhitespace(value) {
		*problems = append(*problems, "wstunnel.url must not contain whitespace")
		return "", "", ""
	}

	parsed, err := url.Parse(value)
	if err != nil {
		*problems = append(*problems, "wstunnel.url is invalid")
		return "", "", ""
	}
	if parsed.Scheme != "wss" {
		*problems = append(*problems, "wstunnel.url must use wss scheme")
	}
	if parsed.User != nil {
		*problems = append(*problems, "wstunnel.url must not contain userinfo")
	}
	host := parsed.Hostname()
	if host == "" {
		*problems = append(*problems, "wstunnel.url host is required")
	}
	if parsed.Port() != "443" {
		*problems = append(*problems, "wstunnel.url must explicitly use port 443")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		*problems = append(*problems, "wstunnel.url must not contain query or fragment")
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = parsed.Path
	}
	if !validHTTPPath(path) {
		*problems = append(*problems, "wstunnel.url path must be a clean absolute HTTP path")
	}
	return value, path, host
}

func validHTTPPath(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.Contains(path, "://") || hasWhitespace(path) {
		return false
	}
	return true
}

func normalizeLoopbackEndpoint(raw string, label string, problems *[]string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		*problems = append(*problems, label+" is required")
		return ""
	}
	host, portRaw, err := net.SplitHostPort(value)
	if err != nil {
		*problems = append(*problems, label+" must use host:port")
		return ""
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil || port < 1 || port > 65535 {
		*problems = append(*problems, label+" must use a valid port")
		return ""
	}
	normalizedHost := strings.TrimSpace(host)
	if normalizedHost == "localhost" {
		normalizedHost = "127.0.0.1"
	}
	addr, err := netip.ParseAddr(normalizedHost)
	if err != nil || !addr.IsLoopback() {
		*problems = append(*problems, label+" must be bound to loopback")
		return ""
	}
	return net.JoinHostPort(addr.String(), strconv.Itoa(port))
}

func validateRequiredWGKey(value string, label string, problems *[]string) {
	if value == "" {
		*problems = append(*problems, label+" is required")
		return
	}
	validateWGKey(value, label, problems)
}

func validateWGKey(value string, label string, problems *[]string) {
	if isPlaceholder(value) {
		*problems = append(*problems, label+" must not be a placeholder")
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		*problems = append(*problems, label+" must be a base64 WireGuard key")
	}
}

func normalizeStringList(value any) []string {
	var out []string
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		for _, item := range strings.Split(strings.ReplaceAll(typed, ";", ","), ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	case []any:
		for _, item := range typed {
			trimmed := strings.TrimSpace(fmt.Sprint(item))
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	case []string:
		for _, item := range typed {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	default:
		trimmed := strings.TrimSpace(fmt.Sprint(value))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func validAddressOrPrefix(value string) bool {
	if _, err := netip.ParsePrefix(value); err == nil {
		return true
	}
	if _, err := netip.ParseAddr(value); err == nil {
		return true
	}
	return false
}

func isPlaceholder(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(upper, "REPLACE_") || strings.Contains(upper, "PLACEHOLDER")
}

func hasWhitespace(value string) bool {
	return strings.ContainsFunc(value, unicode.IsSpace)
}
