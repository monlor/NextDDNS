package source

import (
	"context"
	"net/netip"
)

type ResolvedIPs struct {
	IPv4 netip.Addr
	IPv6 netip.Addr
}

func (r ResolvedIPs) HasIPv4() bool {
	return r.IPv4.IsValid()
}

func (r ResolvedIPs) HasIPv6() bool {
	return r.IPv6.IsValid()
}

type Source interface {
	Resolve(context.Context) (ResolvedIPs, error)
}
