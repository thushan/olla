package util

import (
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

func GetClientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			return strings.TrimSpace(strings.Split(ip, ",")[0])
		}
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
	}
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}