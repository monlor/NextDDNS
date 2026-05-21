package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type HG2201TConfig struct {
	Mode        string
	BaseURL     string
	Username    string
	Password    string
	DeviceMAC   string
	DeviceTypes []string
	Client      *http.Client
}

type HG2201TSource struct {
	cfg HG2201TConfig
}

func NewHG2201T(cfg HG2201TConfig) *HG2201TSource {
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	if cfg.Mode == "" {
		cfg.Mode = "device"
	}
	if cfg.Username == "" {
		cfg.Username = "useradmin"
	}
	if len(cfg.DeviceTypes) == 0 {
		cfg.DeviceTypes = []string{"0", "1"}
	}
	return &HG2201TSource{cfg: cfg}
}

func (s *HG2201TSource) Resolve(ctx context.Context) (ResolvedIPs, error) {
	if err := s.login(ctx); err != nil {
		return ResolvedIPs{}, err
	}

	if s.cfg.Mode == "wan" {
		return s.resolveWAN(ctx)
	}

	targetMAC := normalizeMAC(s.cfg.DeviceMAC)
	var result ResolvedIPs
	found := false

	for _, deviceType := range s.cfg.DeviceTypes {
		ips, matched, err := s.resolveFromType(ctx, deviceType, targetMAC)
		if err != nil {
			return ResolvedIPs{}, err
		}
		if !matched {
			continue
		}
		found = true
		if ips.HasIPv4() && !result.HasIPv4() {
			result.IPv4 = ips.IPv4
		}
		if ips.HasIPv6() && !result.HasIPv6() {
			result.IPv6 = ips.IPv6
		}
		if result.HasIPv4() && result.HasIPv6() {
			return result, nil
		}
	}

	if !found {
		return ResolvedIPs{}, fmt.Errorf("device %s not found in router response", s.cfg.DeviceMAC)
	}
	if !result.HasIPv4() && !result.HasIPv6() {
		return ResolvedIPs{}, fmt.Errorf("device %s found but no valid IP extracted", s.cfg.DeviceMAC)
	}
	return result, nil
}

func (s *HG2201TSource) login(ctx context.Context) error {
	form := url.Values{
		"username": {s.cfg.Username},
		"psd":      {s.cfg.Password},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(s.cfg.BaseURL, "/cgi-bin/luci"), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("router login: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("login status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *HG2201TSource) resolveFromType(ctx context.Context, deviceType string, targetMAC string) (ResolvedIPs, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(s.cfg.BaseURL, "/cgi-bin/luci/admin/device/devInfo")+"?type="+url.QueryEscape(deviceType), nil)
	if err != nil {
		return ResolvedIPs{}, false, fmt.Errorf("build device list request: %w", err)
	}
	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return ResolvedIPs{}, false, fmt.Errorf("fetch device list: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return ResolvedIPs{}, false, fmt.Errorf("read device list: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ResolvedIPs{}, false, fmt.Errorf("device list status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ResolvedIPs{}, false, fmt.Errorf("decode device list: %w", err)
	}

	for key, value := range payload {
		if !strings.HasPrefix(key, "dev") {
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if normalizeMAC(fmt.Sprint(obj["mac"])) != targetMAC {
			continue
		}

		var result ResolvedIPs
		if ip, err := parseMaybeIP(fmt.Sprint(obj["ip"])); err == nil && ip.Is4() && !ip.IsUnspecified() {
			result.IPv4 = ip
		}
		if ip, err := parseMaybeIP(fmt.Sprint(obj["ipv6"])); err == nil && ip.Is6() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() {
			result.IPv6 = ip
		}
		return result, true, nil
	}

	return ResolvedIPs{}, false, nil
}

func (s *HG2201TSource) resolveWAN(ctx context.Context) (ResolvedIPs, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(s.cfg.BaseURL, "/cgi-bin/luci/admin/settings/gwinfo")+"?get=part", nil)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("build wan info request: %w", err)
	}
	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("fetch wan info: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("read wan info: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ResolvedIPs{}, fmt.Errorf("wan info status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ResolvedIPs{}, fmt.Errorf("decode wan info: %w", err)
	}

	var result ResolvedIPs
	if ip, err := parseMaybeIP(fmt.Sprint(payload["WANIP"])); err == nil && ip.Is4() && !ip.IsUnspecified() {
		result.IPv4 = ip
	}
	if ip, err := parseMaybeIP(fmt.Sprint(payload["WANIPv6"])); err == nil && ip.Is6() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() {
		result.IPv6 = ip
	}
	if !result.HasIPv4() && !result.HasIPv6() {
		return ResolvedIPs{}, fmt.Errorf("no valid WAN IP extracted from router response")
	}
	return result, nil
}
