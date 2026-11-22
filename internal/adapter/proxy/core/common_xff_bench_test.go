package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/core/constants"
)

// xffChain builds the X-Forwarded-For value a request carries after n hops,
// each hop appending the address it saw. Both address families appear so the
// parser meets the bracketed IPv6 form real deployments send.
func xffChain(hops int) string {
	parts := make([]string, hops)
	for i := range hops {
		if i%3 == 2 {
			parts[i] = fmt.Sprintf("2001:db8:%x::%d", i, 10+i)
		} else {
			parts[i] = fmt.Sprintf("10.%d.%d.%d", i/8, i%8, 1+i)
		}
	}
	return strings.Join(parts, ", ")
}

func xffRequest(hops int) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", nil)
	r.RemoteAddr = "192.168.1.42:51423"
	if hops > 0 {
		r.Header.Set(constants.HeaderXForwardedFor, xffChain(hops))
	}
	return r
}

var benchXFFSink string

// BenchmarkExtractClientIP covers the stringscut finding on the X-Forwarded-For
// split in extractClientIP. Multi-address rows matter because the split is what
// a rewrite changes; the zero-hop row is the RemoteAddr fallback.
func BenchmarkExtractClientIP(b *testing.B) {
	cases := []struct {
		name string
		hops int
	}{
		{"xff-absent-remoteaddr", 0},
		{"xff-single-address", 1},
		{"xff-three-addresses", 3},
		{"xff-eight-addresses", 8},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			r := xffRequest(tc.hops)

			b.ReportAllocs()
			for range b.N {
				benchXFFSink = extractClientIP(r)
			}
		})
	}
}

// BenchmarkUpdateForwardedHeaders_MultiAddressXFF times the whole forwarded
// header rewrite, which reads the chain once in updateForwardedHeaders and
// again in extractClientIP, then sets the joined value.
func BenchmarkUpdateForwardedHeaders_MultiAddressXFF(b *testing.B) {
	original := xffRequest(3)
	original.Host = "olla.local"

	b.ReportAllocs()
	for range b.N {
		proxyReq := httptest.NewRequest(http.MethodPost, "http://backend/v1/chat/completions", nil)
		updateForwardedHeaders(proxyReq, original)
		benchXFFSink = proxyReq.Header.Get(constants.HeaderXForwardedFor)
	}
}
