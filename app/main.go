// Command app is a minimal HTTP API used to exercise the Kubership CI/CD
// pipeline. It exposes a single /health endpoint and shuts down gracefully on
// SIGTERM so Kubernetes rolling updates do not drop in-flight requests.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=<git-sha>"
var version = "dev"

const (
	defaultPort     = "8080"
	shutdownTimeout = 10 * time.Second
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// newHandler returns the application's HTTP routes. Method-qualified patterns
// make the mux answer 405 for non-GET requests and 404 for unknown paths.
func newHandler(version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(version))
	return mux
}

func healthHandler(version string) http.HandlerFunc {
	resp := healthResponse{Status: "ok", Version: version}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("write health response", "error", err)
		}
	}
}

// run serves h on ln until ctx is cancelled, then drains in-flight requests for
// up to timeout. It returns nil on a clean shutdown.
func run(ctx context.Context, ln net.Listener, h http.Handler, timeout time.Duration) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		// Serve only returns on its own when something went wrong.
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() { os.Exit(realMain()) }

// realMain exists so deferred cleanup runs before the process exits.
func realMain() int {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := ":" + getenv("PORT", defaultPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", "addr", addr, "error", err)
		return 1
	}

	slog.Info("starting", "addr", ln.Addr().String(), "version", version)
	if err := run(ctx, ln, newHandler(version), shutdownTimeout); err != nil {
		slog.Error("server stopped", "error", err)
		return 1
	}
	slog.Info("stopped cleanly")
	return 0
}
