package services

import (
	"context"
	"net"
	"testing"
)

// TestBindListener_FreePort verifies that bindListener returns a valid listener
// when no other process holds the port.
func TestBindListener_FreePort(t *testing.T) {
	t.Parallel()

	ln, err := bindListener(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected bind to succeed on a free port: %v", err)
	}
	ln.Close()
}

// TestBindListener_OccupiedPort verifies that bindListener returns a non-nil
// error when another listener already holds the port. This is the path that
// Start() uses to surface port conflicts immediately rather than swallowing
// them inside a goroutine.
func TestBindListener_OccupiedPort(t *testing.T) {
	t.Parallel()

	// Hold a port so the second bind must fail.
	anchor, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to acquire anchor listener: %v", err)
	}
	defer anchor.Close()

	addr := anchor.Addr().String()

	_, bindErr := bindListener(context.Background(), addr)
	if bindErr == nil {
		t.Fatal("expected bindListener to return an error while anchor holds the port, but it succeeded")
	}
}

// TestDashboardURL verifies bind addresses turn into clickable dashboard
// URLs, substituting localhost for any address a browser can't open and
// bracketing IPv6 literals.
func TestDashboardURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bindAddr string
		want     string
	}{
		{bindAddr: "0.0.0.0:40114", want: "http://localhost:40114/internal/ui/"},
		{bindAddr: "[::]:40114", want: "http://localhost:40114/internal/ui/"},
		{bindAddr: "127.0.0.1:8080", want: "http://127.0.0.1:8080/internal/ui/"},
		{bindAddr: "192.168.1.5:40114", want: "http://192.168.1.5:40114/internal/ui/"},
		{bindAddr: "[fd00::1]:40114", want: "http://[fd00::1]:40114/internal/ui/"},
		{bindAddr: ":40114", want: "http://localhost:40114/internal/ui/"},
	}

	for _, tt := range tests {
		t.Run(tt.bindAddr, func(t *testing.T) {
			t.Parallel()

			got, ok := dashboardURL(tt.bindAddr)
			if !ok {
				t.Fatalf("dashboardURL(%q) reported unparseable, want %q", tt.bindAddr, tt.want)
			}
			if got != tt.want {
				t.Fatalf("dashboardURL(%q) = %q, want %q", tt.bindAddr, got, tt.want)
			}
		})
	}
}

// TestDashboardURL_Unparseable confirms a malformed bind address is skipped
// rather than producing a bogus URL or panicking.
func TestDashboardURL_Unparseable(t *testing.T) {
	t.Parallel()

	if _, ok := dashboardURL("not-a-valid-address"); ok {
		t.Fatal("expected dashboardURL to report unparseable for a malformed address")
	}
}
