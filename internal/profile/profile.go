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
	defaultLocalUDPListen = "127.0.0.1:51820"
	defaultWGWSMTU        = 1280
	defaultWGKeepalive    = 25
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
	if config, ok, err := parseSingBoxWGWS(data); ok {
		return config, err
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
	Protocol         string        `json:"protocol"`
	Transport        string        `json:"transport"`
	Name             string        `json:"profile"`
	Server           string        `json:"server"`
	Port             *int          `json:"port"`
	SNI              string        `json:"sni"`
	WSTunnelURL      string        `json:"wstunnel_url"`
	WSTunnelPath     string        `json:"wstunnel_path"`
	WSTunnelTLSName  string        `json:"wstunnel_tls_server_name"`
	LocalUDPListen   string        `json:"local_udp_listen"`
	WireGuardPrivKey string        `json:"wireguard_private_key"`
	WireGuardPubKey  string        `json:"wireguard_public_key"`
	ServerPubKey     string        `json:"wireguard_server_public_key"`
	PresharedKey     string        `json:"wireguard_preshared_key"`
	Addresses        any           `json:"addresses"`
	AllowedIPs       any           `json:"allowed_ips"`
	DNS              string        `json:"dns"`
	MTU              *int          `json:"mtu"`
	Keepalive        *int          `json:"persistent_keepalive"`
	WSTunnel         rawWSTunnel   `json:"wstunnel"`
	WireGuard        rawWireGuard  `json:"wireguard"`
	LocalSock        any           `json:"local_socks"`
	Extra            []interface{} `json:"-"`
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
	if protocol == "" && looksLikeWGWSProfile(raw) {
		protocol = "wireguard"
	}
	if protocol != "wireguard" {
		addProblem("protocol must be wireguard")
	}

	transport := strings.ToLower(strings.TrimSpace(raw.Transport))
	if transport == "" && looksLikeWGWSProfile(raw) {
		transport = "wstunnel"
	}
	if transport != "wstunnel" {
		addProblem("transport must be wstunnel")
	}

	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = DisplayProfileType
	}

	port := 443
	if raw.Port != nil && *raw.Port != 443 {
		addProblem("port must be 443")
	} else if raw.Port != nil {
		port = *raw.Port
	}

	wstunnelRaw := mergeWSTunnel(raw.WSTunnel, raw)
	wstunnelURL, wstunnelPath, wstunnelHost := normalizeWSTunnelURL(wstunnelRaw.URL, &problems)
	rawPath := strings.TrimSpace(wstunnelRaw.Path)
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

	localUDP := strings.TrimSpace(wstunnelRaw.LocalUDPListen)
	if localUDP == "" && looksLikeWGWSProfile(raw) {
		localUDP = defaultLocalUDPListen
	}
	localUDP = normalizeLoopbackEndpoint(localUDP, "wstunnel.local_udp_listen", &problems)

	wireguardRaw := mergeWireGuard(raw.WireGuard, raw)
	privateKey := strings.TrimSpace(wireguardRaw.PrivateKey)
	validateRequiredWGKey(privateKey, "wireguard.private_key", &problems)

	publicKey := strings.TrimSpace(wireguardRaw.PublicKey)
	if publicKey != "" {
		validateWGKey(publicKey, "wireguard.public_key", &problems)
	}

	serverPublicKey := strings.TrimSpace(wireguardRaw.ServerPublicKey)
	validateRequiredWGKey(serverPublicKey, "wireguard.server_public_key", &problems)

	presharedKey := strings.TrimSpace(wireguardRaw.PresharedKey)
	if presharedKey != "" {
		validateWGKey(presharedKey, "wireguard.preshared_key", &problems)
	}

	addresses := normalizeStringList(wireguardRaw.Address)
	if len(addresses) == 0 {
		addProblem("wireguard.address is required")
	}
	for _, address := range addresses {
		if !validAddressOrPrefix(address) {
			addProblem("wireguard.address entry is invalid: %s", address)
		}
	}

	allowedIPs := normalizeStringList(wireguardRaw.AllowedIPs)
	if len(allowedIPs) == 0 {
		addProblem("wireguard.allowed_ips is required")
	}
	for _, allowedIP := range allowedIPs {
		if _, err := netip.ParsePrefix(allowedIP); err != nil {
			addProblem("wireguard.allowed_ips entry must be CIDR: %s", allowedIP)
		}
	}

	mtu := 0
	if wireguardRaw.MTU == nil {
		if looksLikeWGWSProfile(raw) {
			mtu = defaultWGWSMTU
		} else {
			addProblem("wireguard.mtu is required")
		}
	} else {
		mtu = *wireguardRaw.MTU
		if mtu < 1200 || mtu > 1420 {
			addProblem("wireguard.mtu must be in 1200..1420")
		}
	}

	keepalive := 0
	if wireguardRaw.PersistentKeepalive == nil {
		if looksLikeWGWSProfile(raw) {
			keepalive = defaultWGKeepalive
		} else {
			addProblem("wireguard.persistent_keepalive is required")
		}
	} else {
		keepalive = *wireguardRaw.PersistentKeepalive
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
		WSTunnelTLSName:     strings.TrimSpace(wstunnelRaw.TLSServerName),
		LocalUDPListen:      localUDP,
		WireGuardPrivateKey: privateKey,
		WireGuardPublicKey:  publicKey,
		ServerPublicKey:     serverPublicKey,
		PresharedKey:        presharedKey,
		Addresses:           addresses,
		AllowedIPs:          allowedIPs,
		DNS:                 strings.TrimSpace(wireguardRaw.DNS),
		MTU:                 mtu,
		PersistentKeepalive: keepalive,
	}, nil
}

func looksLikeWGWSProfile(raw rawConfig) bool {
	return strings.TrimSpace(raw.WSTunnel.URL) != "" ||
		strings.TrimSpace(raw.WSTunnelURL) != "" ||
		strings.TrimSpace(raw.WireGuard.PrivateKey) != "" ||
		strings.TrimSpace(raw.WireGuardPrivKey) != ""
}

func mergeWSTunnel(nested rawWSTunnel, raw rawConfig) rawWSTunnel {
	if strings.TrimSpace(nested.URL) == "" {
		nested.URL = raw.WSTunnelURL
	}
	if strings.TrimSpace(nested.Path) == "" {
		nested.Path = raw.WSTunnelPath
	}
	if strings.TrimSpace(nested.TLSServerName) == "" {
		nested.TLSServerName = raw.WSTunnelTLSName
	}
	if strings.TrimSpace(nested.LocalUDPListen) == "" {
		nested.LocalUDPListen = raw.LocalUDPListen
	}
	return nested
}

func mergeWireGuard(nested rawWireGuard, raw rawConfig) rawWireGuard {
	if strings.TrimSpace(nested.PrivateKey) == "" {
		nested.PrivateKey = raw.WireGuardPrivKey
	}
	if strings.TrimSpace(nested.PublicKey) == "" {
		nested.PublicKey = raw.WireGuardPubKey
	}
	if strings.TrimSpace(nested.ServerPublicKey) == "" {
		nested.ServerPublicKey = raw.ServerPubKey
	}
	if strings.TrimSpace(nested.PresharedKey) == "" {
		nested.PresharedKey = raw.PresharedKey
	}
	if nested.Address == nil {
		nested.Address = raw.Addresses
	}
	if nested.AllowedIPs == nil {
		nested.AllowedIPs = raw.AllowedIPs
	}
	if strings.TrimSpace(nested.DNS) == "" {
		nested.DNS = raw.DNS
	}
	if nested.MTU == nil {
		nested.MTU = raw.MTU
	}
	if nested.PersistentKeepalive == nil {
		nested.PersistentKeepalive = raw.Keepalive
	}
	return nested
}

type singBoxConfig struct {
	Outbounds []singBoxWireGuard `json:"outbounds"`
	Endpoints []singBoxWireGuard `json:"endpoints"`
	WSTunnel  rawWSTunnel        `json:"wstunnel"`
}

type singBoxWireGuard struct {
	Type                  string                 `json:"type"`
	Tag                   string                 `json:"tag"`
	Server                string                 `json:"server"`
	ServerPort            *int                   `json:"server_port"`
	System                bool                   `json:"system"`
	SystemInterface       bool                   `json:"system_interface"`
	InterfaceName         string                 `json:"interface_name"`
	Name                  string                 `json:"name"`
	LocalAddress          any                    `json:"local_address"`
	Address               any                    `json:"address"`
	PrivateKey            string                 `json:"private_key"`
	PeerPublicKey         string                 `json:"peer_public_key"`
	ServerPublicKey       string                 `json:"server_public_key"`
	PreSharedKey          string                 `json:"pre_shared_key"`
	PresharedKey          string                 `json:"preshared_key"`
	AllowedIPs            any                    `json:"allowed_ips"`
	MTU                   *int                   `json:"mtu"`
	PersistentKeepalive   *int                   `json:"persistent_keepalive"`
	KeepaliveInterval     *int                   `json:"persistent_keepalive_interval"`
	Peers                 []singBoxWireGuardPeer `json:"peers"`
	WSTunnel              rawWSTunnel            `json:"wstunnel"`
	Transport             singBoxTransport       `json:"transport"`
	TLS                   singBoxTLS             `json:"tls"`
	DNS                   string                 `json:"dns"`
	DomainResolver        string                 `json:"domain_resolver"`
	LegacyDomainResolver  string                 `json:"domain_strategy"`
	UnsupportedRawPayload map[string]any         `json:"-"`
}

type singBoxWireGuardPeer struct {
	Server                      string `json:"server"`
	ServerPort                  *int   `json:"server_port"`
	Address                     string `json:"address"`
	Port                        *int   `json:"port"`
	PublicKey                   string `json:"public_key"`
	PeerPublicKey               string `json:"peer_public_key"`
	ServerPublicKey             string `json:"server_public_key"`
	PreSharedKey                string `json:"pre_shared_key"`
	PresharedKey                string `json:"preshared_key"`
	AllowedIPs                  any    `json:"allowed_ips"`
	PersistentKeepalive         *int   `json:"persistent_keepalive"`
	PersistentKeepaliveInterval *int   `json:"persistent_keepalive_interval"`
}

type singBoxTransport struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type singBoxTLS struct {
	Enabled    bool   `json:"enabled"`
	ServerName string `json:"server_name"`
}

func parseSingBoxWGWS(data []byte) (Config, bool, error) {
	var raw singBoxConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, false, nil
	}
	wireguard, found := firstSingBoxWireGuard(raw)
	if !found {
		return Config{}, len(raw.Outbounds) > 0 || len(raw.Endpoints) > 0, &ValidationError{Problems: []string{"sing-box config does not contain a WireGuard outbound or endpoint"}}
	}

	peer := firstSingBoxPeer(wireguard)
	wstunnel := mergeSingBoxWSTunnel(raw.WSTunnel, wireguard)
	if strings.TrimSpace(wstunnel.URL) == "" {
		wstunnel.URL = singBoxTransportWSTunnelURL(wireguard, peer)
	}
	if strings.TrimSpace(wstunnel.URL) == "" {
		return Config{}, true, &ValidationError{Problems: []string{"sing-box WireGuard config requires WSTunnel URL or websocket transport metadata"}}
	}
	if strings.TrimSpace(wstunnel.Path) == "" {
		if _, path, _ := normalizeWSTunnelURL(wstunnel.URL, &[]string{}); path != "" {
			wstunnel.Path = path
		}
	}
	if strings.TrimSpace(wstunnel.TLSServerName) == "" {
		wstunnel.TLSServerName = strings.TrimSpace(wireguard.TLS.ServerName)
	}
	if strings.TrimSpace(wstunnel.LocalUDPListen) == "" {
		wstunnel.LocalUDPListen = singBoxLocalUDPListen(wireguard, peer)
	}

	mtu := wireguard.MTU
	if mtu == nil {
		mtu = intPointer(defaultWGWSMTU)
	}
	keepalive := firstIntPointer(wireguard.PersistentKeepalive, wireguard.KeepaliveInterval, peer.PersistentKeepalive, peer.PersistentKeepaliveInterval, intPointer(defaultWGKeepalive))
	serverPublicKey := firstNonEmpty(peer.PublicKey, peer.PeerPublicKey, peer.ServerPublicKey, wireguard.PeerPublicKey, wireguard.ServerPublicKey)
	presharedKey := firstNonEmpty(peer.PreSharedKey, peer.PresharedKey, wireguard.PreSharedKey, wireguard.PresharedKey)
	allowedIPs := firstAny(peer.AllowedIPs, wireguard.AllowedIPs)
	if allowedIPs == nil {
		allowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	addresses := firstAny(wireguard.LocalAddress, wireguard.Address)

	native := rawConfig{
		Protocol:  "wireguard",
		Transport: "wstunnel",
		Name:      firstNonEmpty(wireguard.Tag, wireguard.Name, wireguard.InterfaceName, DisplayProfileType),
		Server:    singBoxServerForProfile(wireguard, peer),
		Port:      intPointer(443),
		SNI:       strings.TrimSpace(wireguard.TLS.ServerName),
		WSTunnel:  wstunnel,
		WireGuard: rawWireGuard{
			PrivateKey:          wireguard.PrivateKey,
			ServerPublicKey:     serverPublicKey,
			PresharedKey:        presharedKey,
			Address:             addresses,
			AllowedIPs:          allowedIPs,
			DNS:                 strings.TrimSpace(wireguard.DNS),
			MTU:                 mtu,
			PersistentKeepalive: keepalive,
		},
	}
	config, err := normalize(native)
	return config, true, err
}

func firstSingBoxWireGuard(raw singBoxConfig) (singBoxWireGuard, bool) {
	for _, outbound := range raw.Outbounds {
		if strings.EqualFold(strings.TrimSpace(outbound.Type), "wireguard") {
			return outbound, true
		}
	}
	for _, endpoint := range raw.Endpoints {
		if strings.EqualFold(strings.TrimSpace(endpoint.Type), "wireguard") {
			return endpoint, true
		}
	}
	return singBoxWireGuard{}, false
}

func singBoxServerForProfile(wireguard singBoxWireGuard, peer singBoxWireGuardPeer) string {
	server := firstNonEmpty(peer.Server, peer.Address, wireguard.Server)
	if isLoopbackHost(server) {
		return ""
	}
	return server
}

func firstSingBoxPeer(wireguard singBoxWireGuard) singBoxWireGuardPeer {
	if len(wireguard.Peers) == 0 {
		return singBoxWireGuardPeer{}
	}
	return wireguard.Peers[0]
}

func mergeSingBoxWSTunnel(global rawWSTunnel, wireguard singBoxWireGuard) rawWSTunnel {
	wstunnel := global
	if strings.TrimSpace(wstunnel.URL) == "" {
		wstunnel.URL = wireguard.WSTunnel.URL
	}
	if strings.TrimSpace(wstunnel.Path) == "" {
		wstunnel.Path = wireguard.WSTunnel.Path
	}
	if strings.TrimSpace(wstunnel.TLSServerName) == "" {
		wstunnel.TLSServerName = wireguard.WSTunnel.TLSServerName
	}
	if strings.TrimSpace(wstunnel.LocalUDPListen) == "" {
		wstunnel.LocalUDPListen = wireguard.WSTunnel.LocalUDPListen
	}
	if strings.TrimSpace(wstunnel.Mode) == "" {
		wstunnel.Mode = wireguard.WSTunnel.Mode
	}
	return wstunnel
}

func singBoxTransportWSTunnelURL(wireguard singBoxWireGuard, peer singBoxWireGuardPeer) string {
	transportType := strings.ToLower(strings.TrimSpace(wireguard.Transport.Type))
	if transportType != "ws" && transportType != "websocket" {
		return ""
	}
	host := firstNonEmpty(peer.Server, peer.Address, wireguard.Server)
	if host == "" {
		return ""
	}
	port := firstInt(peer.ServerPort, peer.Port, wireguard.ServerPort)
	if port == 0 {
		port = 443
	}
	if port != 443 {
		return ""
	}
	path := strings.TrimSpace(wireguard.Transport.Path)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "wss://" + net.JoinHostPort(host, strconv.Itoa(port)) + path
}

func singBoxLocalUDPListen(wireguard singBoxWireGuard, peer singBoxWireGuardPeer) string {
	port := firstInt(wireguard.ServerPort, peer.Port, peer.ServerPort)
	if port < 1 || port > 65535 || port == 443 {
		port = 51820
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func isLoopbackHost(value string) bool {
	host := strings.TrimSpace(value)
	if host == "" {
		return false
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstAny(values ...any) any {
	for _, value := range values {
		if len(normalizeStringList(value)) > 0 {
			return value
		}
	}
	return nil
}

func firstInt(values ...*int) int {
	if value := firstIntPointer(values...); value != nil {
		return *value
	}
	return 0
}

func firstIntPointer(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func intPointer(value int) *int {
	return &value
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
	} else if isPlaceholderHost(host) {
		*problems = append(*problems, "wstunnel.url host must be the real WSTunnel target hostname, not a placeholder")
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

func isPlaceholderHost(value string) bool {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(value), "."))
	return normalized == "host" ||
		normalized == "hostname" ||
		normalized == "wstunnel-host" ||
		normalized == "wstunnel.example" ||
		normalized == "example.com" ||
		normalized == "example.net" ||
		normalized == "example.org" ||
		strings.HasPrefix(normalized, "replace-") ||
		strings.Contains(normalized, "placeholder")
}

func hasWhitespace(value string) bool {
	return strings.ContainsFunc(value, unicode.IsSpace)
}
