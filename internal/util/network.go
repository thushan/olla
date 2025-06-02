package util

import (
	"fmt"
	"net"
	"strings"
)

func isIPInTrustedCIDRs(ip net.IP, trustedCIDRs []*net.IPNet) bool {
	for _, cidr := range trustedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func ParseTrustedCIDRs(cidrStrings []string) ([]*net.IPNet, error) {
	if len(cidrStrings) == 0 {
		return nil, nil
	}

	var cidrs []*net.IPNet
	for _, cidrStr := range cidrStrings {
		cidrStr = strings.TrimSpace(cidrStr)
		if cidrStr == "" {
			continue
		}

		_, network, err := net.ParseCIDR(cidrStr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidrStr, err)
		}
		cidrs = append(cidrs, network)
	}

	return cidrs, nil
}


// NormaliseBaseURL ensures the base URL ends without a trailing slash
func NormaliseBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	if len(baseURL) > 1 && baseURL[len(baseURL)-1] == '/' {
		return baseURL[:len(baseURL)-1]
	}
	return baseURL
}
