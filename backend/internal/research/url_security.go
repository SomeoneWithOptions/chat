package research

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var (
	errInvalidURLScheme = errors.New("unsupported url scheme")
	errBlockedURLHost   = errors.New("blocked url host")
	errBlockedURLPort   = errors.New("blocked url port")
)

func validateResearchURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed == nil || parsed.Host == "" {
		return nil, errors.New("url host is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errInvalidURLScheme
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if hostname == "" {
		return nil, errors.New("url hostname is required")
	}
	if isBlockedHostname(hostname) {
		return nil, errBlockedURLHost
	}
	if !isAllowedPort(parsed.Port()) {
		return nil, errBlockedURLPort
	}
	return parsed, nil
}

func isAllowedPort(rawPort string) bool {
	trimmed := strings.TrimSpace(rawPort)
	if trimmed == "" {
		return true
	}
	port, err := strconv.Atoi(trimmed)
	if err != nil {
		return false
	}
	return port == 80 || port == 443
}

func isBlockedHostname(hostname string) bool {
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return true
	}
	if strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") {
		return true
	}
	if ip, err := netip.ParseAddr(hostname); err == nil {
		return isPrivateIP(ip)
	}
	return false
}

func validateDialAddress(ctx context.Context, host string) error {
	if isBlockedHostname(host) {
		return errBlockedURLHost
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("no ip addresses for host %q", host)
	}

	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		if isPrivateIP(addr) {
			return errBlockedURLHost
		}
	}
	return nil
}

func isPrivateIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	if ip.Is6() {
		if ip.IsInterfaceLocalMulticast() {
			return true
		}
		if strings.HasPrefix(ip.String(), "fc") || strings.HasPrefix(ip.String(), "fd") {
			return true
		}
	}
	return false
}

func secureDialContext(base *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if base == nil {
		base = &net.Dialer{}
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return nil, errors.New("empty host")
		}
		if err := validateDialAddress(ctx, host); err != nil {
			return nil, err
		}
		return base.DialContext(ctx, network, address)
	}
}
