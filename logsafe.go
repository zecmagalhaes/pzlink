package main

import (
	"io"
	"net/netip"
	"regexp"
	"sync"
)

// The log file is meant to be attached to public GitHub issues, so it must not
// contain auth keys or the users' real (public) IP addresses.

var (
	reKey  = regexp.MustCompile(`tskey-[A-Za-z0-9_-]+`)
	reInv  = regexp.MustCompile(invitePrefix + `[A-Za-z0-9_-]+`)
	reIPv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reIPv6 = regexp.MustCompile(`\[?[0-9a-fA-F]{0,4}(?::[0-9a-fA-F]{0,4}){2,7}\]?`)
)

// redact hides secrets and public IP addresses. Loopback, private, link-local
// and Tailscale (100.64.0.0/10, fd7a:115c:a1e0::/48) addresses are kept because
// they are useful for debugging and say nothing about where the user lives.
func redact(s string) string {
	s = reKey.ReplaceAllString(s, "tskey-[oculto]")
	s = reInv.ReplaceAllString(s, invitePrefix+"[oculto]")
	s = reIPv4.ReplaceAllStringFunc(s, func(m string) string {
		a, err := netip.ParseAddr(m)
		if err != nil || keepAddr(a) {
			return m
		}
		return "[ip-oculto]"
	})
	s = reIPv6.ReplaceAllStringFunc(s, func(m string) string {
		trim := m
		if len(trim) > 1 && trim[0] == '[' {
			trim = trim[1:]
		}
		if n := len(trim); n > 0 && trim[n-1] == ']' {
			trim = trim[:n-1]
		}
		a, err := netip.ParseAddr(trim)
		if err != nil || keepAddr(a) {
			return m
		}
		return "[ip-oculto]"
	})
	return s
}

var tailscaleV4 = netip.MustParsePrefix("100.64.0.0/10")
var tailscaleV6 = netip.MustParsePrefix("fd7a:115c:a1e0::/48")

func keepAddr(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified() ||
		tailscaleV4.Contains(a) || tailscaleV6.Contains(a)
}

// redactWriter applies redact to everything written to the log file.
type redactWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (r *redactWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.w.Write([]byte(redact(string(p)))); err != nil {
		return 0, err
	}
	return len(p), nil
}
