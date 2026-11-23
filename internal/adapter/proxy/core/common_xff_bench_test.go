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
// each hop appending the address it saw, as plain (unbracketed) IPv4 and IPv6
// text joined with ", ". Only the first address is ever parsed, so the chain
// length changes the header size, not the parsing work.
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
// split in extractClientIP. extractClientIP reads the header and cuts at the
// first comma, so the single-, three- and eight-address rows all parse one
// address and do the same work; they differ only in the header string the
// request carries. The zero-hop row is the RemoteAddr fallback (a
// net.SplitHostPort and one allocation).
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
// header rewrite on a three-address chain: one extractClientIP call (the
// updateForwardedHeaders branch with an existing XFF header does not call it
// twice), three header sets, and the httptest.NewRequest that builds the
// outbound request each iteration. That request construction is the larger
// part of the row, so the header work itself is a fraction of the time
// reported; BenchmarkExtractClientIP is the row that isolates the parsing.
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
