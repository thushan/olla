package util

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
)

func GenerateRequestID() string {
	actions := []string{
		"grazing", "trekking", "humming", "spitting", "prancing",
		"carrying", "leading", "following", "resting", "alerting",
		"browsing", "foraging", "wandering", "galloping", "ambling",
	}
	llamas := []string{
		"huacaya", "suri", "vicuna", "alpaca", "guanaco",
		"woolly", "silky", "fluffy", "curly", "shaggy",
		"noble", "gentle", "swift", "steady", "proud",
	}

	group := llamas[rand.Intn(len(llamas))]
	action := actions[rand.Intn(len(actions))]
	suffix := fmt.Sprintf("%04x", rand.Intn(65536))

	return fmt.Sprintf("%s_%s_%s", group, action, suffix)
}

func GetClientIP(r *http.Request, trustProxyHeaders bool, trustedCIDRs []*net.IPNet) string {
	if !trustProxyHeaders {
		if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			return ip
		}
		return r.RemoteAddr
	}

	sourceIP := getSourceIP(r)
	if sourceIP == nil || !isIPInTrustedCIDRs(sourceIP, trustedCIDRs) {
		if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			return ip
		}
		return r.RemoteAddr
	}

	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}

	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}

func getSourceIP(r *http.Request) net.IP {
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return net.ParseIP(ip)
	}
	return net.ParseIP(r.RemoteAddr)
}

func StripRoutePrefix(ctx context.Context, path, prefix string) string {
	if routePrefix, ok := ctx.Value(prefix).(string); ok {
		if strings.HasPrefix(path, routePrefix) {
			stripped := path[len(routePrefix):]
			if stripped == "" || stripped[0] != '/' {
				stripped = "/" + stripped
			}
			return stripped
		}
	}
	return path
}
