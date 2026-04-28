package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
)

func TestHealthEndpoint(t *testing.T) {
	handler := NewHandler(Options{RuntimeRoot: "/tmp/brb-runtime"})
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response HealthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.RuntimeRoot != "/tmp/brb-runtime" {
		t.Fatalf("response = %#v", response)
	}
}

func TestStatusEndpoint(t *testing.T) {
	runtimeRoot := t.TempDir()
	handler := NewHandler(Options{RuntimeRoot: runtimeRoot})
	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response StatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.RuntimeRoot != runtimeRoot || response.Runtime.State != "Idle" {
		t.Fatalf("response = %#v", response)
	}
}

func TestValidateProfileEndpoint(t *testing.T) {
	handler := NewHandler(Options{RuntimeRoot: t.TempDir()})
	request := httptest.NewRequest(http.MethodPost, "/v1/profile/validate", bytes.NewReader(readProfileFixture(t, "valid-wgws.json")))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response ValidateProfileResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Valid || response.Summary == nil || response.Summary.Fingerprint == "" {
		t.Fatalf("response = %#v", response)
	}
}

func TestValidateProfileEndpointReportsValidationErrors(t *testing.T) {
	handler := NewHandler(Options{RuntimeRoot: t.TempDir()})
	request := httptest.NewRequest(http.MethodPost, "/v1/profile/validate", bytes.NewReader(readProfileFixture(t, "invalid-placeholder-wgws.json")))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response ValidateProfileResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Valid || len(response.Errors) == 0 {
		t.Fatalf("response = %#v", response)
	}
}

func TestPlanConnectEndpoint(t *testing.T) {
	handler := NewHandler(Options{RuntimeRoot: "/tmp/brb-runtime"})
	payload, err := json.Marshal(PlanConnectRequest{
		Profile: readProfileFixture(t, "valid-wgws.json"),
		Options: planner.Options{
			EndpointIPs:      []string{"203.0.113.10"},
			DefaultGateway:   "192.0.2.1",
			DefaultInterface: "eth0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/plan/connect", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response PlanConnectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Plan == nil || response.Plan.Kind != "connect" || response.Plan.RuntimeRoot != "/tmp/brb-runtime" {
		t.Fatalf("response = %#v", response)
	}
}

func TestConnectEndpointUsesCLIBackend(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("mutating daemon endpoints are Linux-only")
	}
	runner := &fakeCommandRunner{output: `{"result":{"state":"Connected"}}`}
	runtimeRoot := t.TempDir()
	handler := NewHandler(Options{
		RuntimeRoot: runtimeRoot,
		CLIPath:     "/usr/bin/big-red-button-test",
		Runner:      runner,
	})
	payload, err := json.Marshal(ConnectRequest{
		Profile: readProfileFixture(t, "valid-wgws.json"),
		Options: planner.Options{
			EndpointIPs:    []string{"203.0.113.10"},
			WSTunnelBinary: "/usr/bin/wstunnel",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/connect", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response OperationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("response = %#v", response)
	}
	if runner.name != "/usr/bin/big-red-button-test" {
		t.Fatalf("runner name = %s", runner.name)
	}
	wantArgs := []string{"linux-connect", "-yes", "-json", "-endpoint-ip", "203.0.113.10", "-wstunnel-binary", "/usr/bin/wstunnel", "-runtime-root", runtimeRoot}
	if !hasArgPrefix(runner.args, wantArgs) {
		t.Fatalf("args = %#v want prefix %#v", runner.args, wantArgs)
	}
	if !runner.profileExisted {
		t.Fatalf("expected temporary profile file to exist during command run")
	}
}

func TestDisconnectEndpointUsesCLIBackend(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("mutating daemon endpoints are Linux-only")
	}
	runner := &fakeCommandRunner{output: `{"result":{"state":"Idle"}}`}
	handler := NewHandler(Options{
		RuntimeRoot: "/tmp/brb-runtime",
		CLIPath:     "/usr/bin/big-red-button-test",
		Runner:      runner,
	})
	payload, err := json.Marshal(DisconnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/disconnect", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	wantArgs := []string{"linux-disconnect", "-yes", "-json", "-runtime-root", "/tmp/brb-runtime"}
	if !equalStrings(runner.args, wantArgs) {
		t.Fatalf("args = %#v want %#v", runner.args, wantArgs)
	}
}

func TestServeUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket daemon transport is not available on Windows")
	}
	socketPath := filepath.Join(t.TempDir(), "launcher.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ServeUnix(ctx, socketPath, NewHandler(Options{RuntimeRoot: t.TempDir()}))
	}()
	waitForUnixSocket(t, socketPath)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestClientStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket daemon transport is not available on Windows")
	}
	socketPath := filepath.Join(t.TempDir(), "launcher.sock")
	runtimeRoot := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ServeUnix(ctx, socketPath, NewHandler(Options{RuntimeRoot: runtimeRoot}))
	}()
	waitForUnixSocket(t, socketPath)
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("daemon did not stop")
		}
	}()

	response, err := NewClient(socketPath).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.RuntimeRoot != runtimeRoot || response.Runtime.State != "Idle" {
		t.Fatalf("response = %#v", response)
	}
	validation, err := NewClient(socketPath).ValidateProfile(context.Background(), readProfileFixture(t, "valid-wgws.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || validation.Summary == nil {
		t.Fatalf("validation = %#v", validation)
	}
	plan, err := NewClient(socketPath).PlanConnect(context.Background(), readProfileFixture(t, "valid-wgws.json"), planner.Options{
		EndpointIPs:      []string{"203.0.113.10"},
		DefaultGateway:   "192.0.2.1",
		DefaultInterface: "eth0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan == nil || plan.Plan.Kind != "connect" {
		t.Fatalf("plan = %#v", plan)
	}
}

func waitForUnixSocket(t *testing.T, socketPath string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket was not ready: %s", socketPath)
}

func readProfileFixture(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "testdata", "profiles", name))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type fakeCommandRunner struct {
	name           string
	args           []string
	output         string
	err            error
	profileExisted bool
}

func (r *fakeCommandRunner) Run(ctx context.Context, name string, args []string) (string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	if len(args) > 0 {
		if _, err := os.Stat(args[len(args)-1]); err == nil {
			r.profileExisted = true
		}
	}
	return r.output, r.err
}

func hasArgPrefix(args []string, prefix []string) bool {
	if len(args) < len(prefix) {
		return false
	}
	for index := range prefix {
		if args[index] != prefix[index] {
			return false
		}
	}
	return true
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
