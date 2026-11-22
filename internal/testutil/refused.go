// Package testutil holds helpers shared by tests in several packages. It is
// imported only from _test.go files.
package testutil

import "testing"

// RefusedBackendURL returns a backend URL whose every connection attempt fails
// immediately, so engines see a transport error before any byte is written.
//
// Port 0 is the choice because no process can ever listen on it (binding port 0
// asks the kernel for an ephemeral port instead), so unlike the URL of a closed
// httptest server it cannot be reissued to a parallel test's server and answer.
// Dialling it fails at once with a net.OpError on Linux (ECONNREFUSED), macOS and
// Windows (address not available), which core.IsConnectionError treats alike.
func RefusedBackendURL(t testing.TB) string {
	t.Helper()
	return "http://127.0.0.1:0"
}
