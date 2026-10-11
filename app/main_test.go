package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"health GET", http.MethodGet, "/health", http.StatusOK},
		{"health HEAD", http.MethodHead, "/health", http.StatusOK},
		{"health POST not allowed", http.MethodPost, "/health", http.StatusMethodNotAllowed},
		{"health DELETE not allowed", http.MethodDelete, "/health", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound},
		{"root", http.MethodGet, "/", http.StatusNotFound},
	}

	h := newHandler("test-version")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestHealthBody(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandler("abc123").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var got healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	want := healthResponse{Status: "ok", Version: "abc123"}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}

func TestGetenv(t *testing.T) {
	t.Setenv("KUBERSHIP_TEST_SET", "9090")
	t.Setenv("KUBERSHIP_TEST_EMPTY", "")

	tests := []struct {
		name, key, fallback, want string
	}{
		{"set", "KUBERSHIP_TEST_SET", "8080", "9090"},
		{"empty uses fallback", "KUBERSHIP_TEST_EMPTY", "8080", "8080"},
		{"unset uses fallback", "KUBERSHIP_TEST_UNSET", "8080", "8080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := getenv(tc.key, tc.fallback); got != tc.want {
				t.Errorf("getenv(%q, %q) = %q, want %q", tc.key, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestRunServesAndShutsDownGracefully(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- run(ctx, ln, newHandler("test"), 2*time.Second) }()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + ln.Addr().String() + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	cancel() // simulates SIGTERM
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v, want nil on clean shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after context cancellation")
	}
}

func TestRunReturnsErrorWhenListenerBroken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln.Close() // Serve fails immediately on a closed listener.

	err = run(context.Background(), ln, newHandler("test"), time.Second)
	if err == nil {
		t.Fatal("run returned nil, want an error for a closed listener")
	}
}
