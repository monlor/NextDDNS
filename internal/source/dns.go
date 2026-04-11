package source

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

type DNSConfig struct {
	Hostname string
	Resolver []string
}

type DNS struct {
	cfg DNSConfig
}

func NewDNS(cfg DNSConfig) *DNS {
	return &DNS{cfg: cfg}
}

func (d *DNS) Resolve(ctx context.Context) (ResolvedIPs, error) {
	resolver := net.DefaultResolver
	if len(d.cfg.Resolver) > 0 {
		target := d.cfg.Resolver[0]
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				dialer := net.Dialer{Timeout: 5 * time.Second}
				if !strings.Contains(target, ":") {
					target += ":53"
				}
				return dialer.DialContext(ctx, "udp", target)
			},
		}
	}

	var result ResolvedIPs
	ips, err := resolver.LookupNetIP(ctx, "ip", d.cfg.Hostname)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("lookup %s: %w", d.cfg.Hostname, err)
	}
	for _, ip := range ips {
		switch {
		case ip.Is4() && !result.IPv4.IsValid():
			result.IPv4 = netip.MustParseAddr(ip.String())
		case ip.Is6() && !result.IPv6.IsValid():
			result.IPv6 = netip.MustParseAddr(ip.String())
		}
	}
	if !result.HasIPv4() && !result.HasIPv6() {
		return ResolvedIPs{}, fmt.Errorf("dns source returned no addresses for %s", d.cfg.Hostname)
	}
	return result, nil
}
