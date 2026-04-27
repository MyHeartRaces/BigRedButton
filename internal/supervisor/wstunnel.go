package supervisor

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/tracegate/tracegate-launcher/internal/profile"
)

const (
	DefaultWSTunnelRemoteHost = "localhost"
	DefaultWSTunnelLogLevel   = "INFO"
)

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

type WSTunnelClientConfig struct {
	Binary         string
	ServerURL      string
	PathPrefix     string
	TLSServerName  string
	LocalUDPListen string
	RemoteUDPHost  string
	RemoteUDPPort  int
	LogLevel       string
}

func WSTunnelClientConfigFromProfile(config profile.Config, binary string) WSTunnelClientConfig {
	return WSTunnelClientConfig{
		Binary:         binary,
		ServerURL:      config.WSTunnelURL,
		PathPrefix:     config.WSTunnelPath,
		TLSServerName:  firstNonEmpty(config.WSTunnelTLSName, config.SNI),
		LocalUDPListen: config.LocalUDPListen,
	}
}

func WSTunnelClientCommand(config WSTunnelClientConfig) (Command, error) {
	binary := strings.TrimSpace(config.Binary)
	if binary == "" {
		return Command{}, fmt.Errorf("wstunnel binary is required")
	}
	serverURL, err := wstunnelServerBaseURL(config.ServerURL)
	if err != nil {
		return Command{}, err
	}
	localToRemote, err := wstunnelUDPForward(config)
	if err != nil {
		return Command{}, err
	}

	logLevel := strings.TrimSpace(config.LogLevel)
	if logLevel == "" {
		logLevel = DefaultWSTunnelLogLevel
	}

	args := []string{"client", "--log-lvl", logLevel}
	if pathPrefix := normalizePathPrefix(config.PathPrefix); pathPrefix != "" {
		args = append(args, "--http-upgrade-path-prefix", pathPrefix)
	}
	if tlsName := strings.TrimSpace(config.TLSServerName); tlsName != "" {
		args = append(args, "--tls-sni-override", tlsName)
	}
	args = append(args, "-L", localToRemote, serverURL)

	return Command{Name: binary, Args: args}, nil
}

func wstunnelServerBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse wstunnel URL: %w", err)
	}
	if parsed.Scheme != "wss" && parsed.Scheme != "ws" && parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("wstunnel URL scheme must be ws, wss, http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("wstunnel URL host is required")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func wstunnelUDPForward(config WSTunnelClientConfig) (string, error) {
	localHost, localPort, err := net.SplitHostPort(strings.TrimSpace(config.LocalUDPListen))
	if err != nil {
		return "", fmt.Errorf("parse local UDP listen endpoint: %w", err)
	}
	if strings.TrimSpace(localHost) == "" {
		return "", fmt.Errorf("local UDP listen host is required")
	}
	if _, err := strconv.Atoi(localPort); err != nil {
		return "", fmt.Errorf("local UDP listen port is invalid: %w", err)
	}

	remoteHost := strings.TrimSpace(config.RemoteUDPHost)
	if remoteHost == "" {
		remoteHost = DefaultWSTunnelRemoteHost
	}
	remotePort := config.RemoteUDPPort
	if remotePort == 0 {
		remotePort, err = strconv.Atoi(localPort)
		if err != nil {
			return "", fmt.Errorf("derive remote UDP port: %w", err)
		}
	}
	if remotePort < 1 || remotePort > 65535 {
		return "", fmt.Errorf("remote UDP port must be in 1..65535")
	}

	local := formatHostPort(localHost, localPort)
	remote := formatHostPort(remoteHost, strconv.Itoa(remotePort))
	return "udp://" + local + ":" + remote + "?timeout_sec=0", nil
}

func normalizePathPrefix(pathPrefix string) string {
	pathPrefix = strings.TrimSpace(pathPrefix)
	pathPrefix = strings.TrimPrefix(pathPrefix, "/")
	return pathPrefix
}

func formatHostPort(host string, port string) string {
	if addr, err := netip.ParseAddr(host); err == nil && addr.Is6() {
		return net.JoinHostPort(host, port)
	}
	return host + ":" + port
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
