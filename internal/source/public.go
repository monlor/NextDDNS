package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
)

type PublicConfig struct {
	IPv4URLs []string
	IPv6URLs []string
	Client   *http.Client
}

type PublicSource struct {
	cfg PublicConfig
}

func NewPublic(cfg PublicConfig) *PublicSource {
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	return &PublicSource{cfg: cfg}
}

func (s *PublicSource) Resolve(ctx context.Context) (ResolvedIPs, error) {
	var result ResolvedIPs
	var err4 error
	var err6 error
	if len(s.cfg.IPv4URLs) > 0 {
		result.IPv4, err4 = s.resolveFirst(ctx, s.cfg.IPv4URLs, true)
	}
	if len(s.cfg.IPv6URLs) > 0 {
		result.IPv6, err6 = s.resolveFirst(ctx, s.cfg.IPv6URLs, false)
	}
	if !result.HasIPv4() && !result.HasIPv6() {
		return ResolvedIPs{}, fmt.Errorf("public source failed: ipv4=%v ipv6=%v", err4, err6)
	}
	return result, nil
}

func (s *PublicSource) resolveFirst(ctx context.Context, urls []string, wantV4 bool) (netip.Addr, error) {
	var lastErr error
	for _, rawURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := s.cfg.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			continue
		}
		ip, err := netip.ParseAddr(strings.TrimSpace(string(body)))
		if err != nil {
			lastErr = err
			continue
		}
		if wantV4 && !ip.Is4() {
			lastErr = fmt.Errorf("expected ipv4, got %s", ip)
			continue
		}
		if !wantV4 && !ip.Is6() {
			lastErr = fmt.Errorf("expected ipv6, got %s", ip)
			continue
		}
		return ip, nil
	}
	return netip.Addr{}, lastErr
}
