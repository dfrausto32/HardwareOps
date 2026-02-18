package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

var (
	trustedProxyMu   sync.RWMutex
	trustedProxyNets []*net.IPNet
)

func ConfigureTrustedProxyCIDRs(cidrs []string) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, entry := range cidrs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, n, err := net.ParseCIDR(entry); err == nil {
				nets = append(nets, n)
			}
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			continue
		}
		maskBits := 32
		if ip.To4() == nil {
			maskBits = 128
		}
		nets = append(nets, &net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(maskBits, maskBits),
		})
	}
	trustedProxyMu.Lock()
	trustedProxyNets = nets
	trustedProxyMu.Unlock()
}

func proxyHeadersAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	remoteIP := net.ParseIP(host)
	if remoteIP == nil {
		return false
	}

	trustedProxyMu.RLock()
	nets := trustedProxyNets
	trustedProxyMu.RUnlock()
	if len(nets) == 0 {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(remoteIP) {
			return true
		}
	}
	return false
}
