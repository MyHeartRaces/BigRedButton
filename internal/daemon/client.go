package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
)

type Client struct {
	SocketPath string
	HTTPClient *http.Client
}

func NewClient(socketPath string) *Client {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	return &Client{
		SocketPath: socketPath,
	}
}

func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var response HealthResponse
	if err := c.getJSON(ctx, "/v1/health", &response); err != nil {
		return HealthResponse{}, err
	}
	return response, nil
}

func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var response StatusResponse
	if err := c.getJSON(ctx, "/v1/status", &response); err != nil {
		return StatusResponse{}, err
	}
	return response, nil
}

func (c *Client) ValidateProfile(ctx context.Context, payload []byte) (ValidateProfileResponse, error) {
	var response ValidateProfileResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/profile/validate", payload, &response); err != nil {
		return ValidateProfileResponse{}, err
	}
	return response, nil
}

func (c *Client) PlanConnect(ctx context.Context, payload []byte, options planner.Options) (PlanConnectResponse, error) {
	request, err := json.Marshal(PlanConnectRequest{
		Profile: append([]byte(nil), payload...),
		Options: options,
	})
	if err != nil {
		return PlanConnectResponse{}, err
	}
	var response PlanConnectResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/plan/connect", request, &response); err != nil {
		return PlanConnectResponse{}, err
	}
	return response, nil
}

func (c *Client) Diagnostics(ctx context.Context) (DiagnosticsResponse, error) {
	var response DiagnosticsResponse
	if err := c.getJSON(ctx, "/v1/diagnostics", &response); err != nil {
		return DiagnosticsResponse{}, err
	}
	return response, nil
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, target)
}

func (c *Client) doJSON(ctx context.Context, method string, path string, payload []byte, target any) error {
	if c == nil {
		return fmt.Errorf("daemon client is nil")
	}
	client := c.HTTPClient
	if client == nil {
		client = unixHTTPClient(c.SocketPath)
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://big-red-button"+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("call daemon %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("daemon %s returned %s: %s", path, response.Status, strings.TrimSpace(string(payload)))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode daemon %s response: %w", path, err)
	}
	return nil
}

func unixHTTPClient(socketPath string) *http.Client {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}
