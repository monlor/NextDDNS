package source

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

type InterfaceConfig struct {
	Name           string
	AllowLinkLocal bool
	AllowLoopback  bool
}

type InterfaceSource struct {
	cfg InterfaceConfig
}

func NewInterface(cfg InterfaceConfig) *InterfaceSource {
	return &InterfaceSource{cfg: cfg}
}

func (s *InterfaceSource) Resolve(_ context.Context) (ResolvedIPs, error) {
	iface, err := net.InterfaceByName(s.cfg.Name)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("get interface %q: %w", s.cfg.Name, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("list interface addresses: %w", err)
	}

	var result ResolvedIPs
	for _, addr := range addrs {
		prefix, err := netip.ParsePrefix(addr.String())
		if err != nil {
			continue
		}
		ip := prefix.Addr()
		if !s.cfg.AllowLoopback && ip.IsLoopback() {
			continue
		}
		if ip.Is4() {
			if !result.IPv4.IsValid() {
				result.IPv4 = ip
			}
			continue
		}
		if !s.cfg.AllowLinkLocal && ip.IsLinkLocalUnicast() {
			continue
		}
		if !result.IPv6.IsValid() {
			result.IPv6 = ip
		}
	}
	if !result.HasIPv4() && !result.HasIPv6() {
		return ResolvedIPs{}, fmt.Errorf("interface %q returned no usable addresses", s.cfg.Name)
	}
	return result, nil
}
