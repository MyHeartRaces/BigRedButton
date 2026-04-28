package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MyHeartRaces/BigRedButton/internal/buildinfo"
	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	truntime "github.com/MyHeartRaces/BigRedButton/internal/runtime"
	"github.com/MyHeartRaces/BigRedButton/internal/status"
)

const DefaultSocketPath = "/run/big-red-button/launcher.sock"

type Options struct {
	RuntimeRoot string
}

type HealthResponse struct {
	OK          bool           `json:"ok"`
	Version     buildinfo.Info `json:"version"`
	RuntimeRoot string         `json:"runtime_root"`
}

type StatusResponse struct {
	Version          buildinfo.Info                   `json:"version"`
	RuntimeRoot      string                           `json:"runtime_root"`
	Runtime          status.Snapshot                  `json:"runtime"`
	IsolatedSessions []status.IsolatedSessionSnapshot `json:"isolated_sessions,omitempty"`
	Error            string                           `json:"error,omitempty"`
}

type DiagnosticsResponse struct {
	GeneratedAt string         `json:"generated_at"`
	Status      StatusResponse `json:"status"`
}

func NewHandler(options Options) http.Handler {
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = planner.DefaultRuntimeRoot
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, HealthResponse{
			OK:          true,
			Version:     buildinfo.Current(),
			RuntimeRoot: runtimeRoot,
		})
	})
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, collectStatus(r.Context(), runtimeRoot))
	})
	mux.HandleFunc("/v1/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, DiagnosticsResponse{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Status:      collectStatus(r.Context(), runtimeRoot),
		})
	})
	return mux
}

func ServeUnix(ctx context.Context, socketPath string, handler http.Handler) error {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	if handler == nil {
		return fmt.Errorf("daemon handler is required")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on unix socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()
	_ = os.Chmod(socketPath, 0o600)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

func collectStatus(ctx context.Context, runtimeRoot string) StatusResponse {
	response := StatusResponse{
		Version:     buildinfo.Current(),
		RuntimeRoot: runtimeRoot,
		Runtime:     status.FromStore(ctx, truntime.Store{Root: runtimeRoot}),
	}
	sessions, err := status.IsolatedSessions(ctx, runtimeRoot)
	if err != nil {
		response.Error = "list isolated sessions: " + err.Error()
		return response
	}
	response.IsolatedSessions = sessions
	return response
}

func writeJSON(w http.ResponseWriter, value any) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
