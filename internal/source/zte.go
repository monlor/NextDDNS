package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
)

type ZTEConfig struct {
	BaseURL        string
	Username       string
	Password       string
	LoginPath      string
	LoginMethod    string
	LoginBody      string
	LoginHeaders   map[string]string
	TokenPath      string
	AuthHeader     string
	AuthScheme     string
	DevicesPath    string
	DevicesMethod  string
	DevicesBody    string
	DevicesHeaders map[string]string
	DeviceListPath string
	DeviceMAC      string
	DeviceMACField string
	IPv4Field      string
	IPv6Field      string
	Client         *http.Client
}

type ZTERouterSource struct {
	cfg ZTEConfig
}

func NewZTERouter(cfg ZTEConfig) *ZTERouterSource {
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	if cfg.LoginMethod == "" {
		cfg.LoginMethod = http.MethodPost
	}
	if cfg.DevicesMethod == "" {
		cfg.DevicesMethod = http.MethodGet
	}
	if cfg.AuthHeader == "" {
		cfg.AuthHeader = "Authorization"
	}
	if cfg.DeviceMACField == "" {
		cfg.DeviceMACField = "mac"
	}
	if cfg.IPv4Field == "" {
		cfg.IPv4Field = "ipv4"
	}
	if cfg.IPv6Field == "" {
		cfg.IPv6Field = "ipv6"
	}
	return &ZTERouterSource{cfg: cfg}
}

func (s *ZTERouterSource) Resolve(ctx context.Context) (ResolvedIPs, error) {
	token, err := s.login(ctx)
	if err != nil {
		return ResolvedIPs{}, err
	}

	reqBody := strings.NewReader(renderTemplate(s.cfg.DevicesBody, s.cfg.Username, s.cfg.Password))
	req, err := http.NewRequestWithContext(ctx, s.cfg.DevicesMethod, joinURL(s.cfg.BaseURL, s.cfg.DevicesPath), reqBody)
	if err != nil {
		return ResolvedIPs{}, err
	}
	for key, value := range s.cfg.DevicesHeaders {
		req.Header.Set(key, value)
	}
	if token != "" {
		authValue := token
		if s.cfg.AuthScheme != "" {
			authValue = s.cfg.AuthScheme + " " + token
		}
		req.Header.Set(s.cfg.AuthHeader, authValue)
	}

	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("fetch device list: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("read device list: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ResolvedIPs{}, fmt.Errorf("device list status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ResolvedIPs{}, fmt.Errorf("decode device list: %w", err)
	}
	listValue, ok := lookupJSONPath(payload, s.cfg.DeviceListPath)
	if !ok {
		return ResolvedIPs{}, fmt.Errorf("device list path %q not found", s.cfg.DeviceListPath)
	}
	devices, ok := listValue.([]any)
	if !ok {
		return ResolvedIPs{}, fmt.Errorf("device list path %q is not an array", s.cfg.DeviceListPath)
	}

	targetMAC := normalizeMAC(s.cfg.DeviceMAC)
	for _, item := range devices {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		macValue, ok := lookupJSONPath(obj, s.cfg.DeviceMACField)
		if !ok {
			continue
		}
		if normalizeMAC(fmt.Sprint(macValue)) != targetMAC {
			continue
		}

		var result ResolvedIPs
		if rawIPv4, ok := lookupJSONPath(obj, s.cfg.IPv4Field); ok {
			if ip, err := parseMaybeIP(fmt.Sprint(rawIPv4)); err == nil && ip.Is4() {
				result.IPv4 = ip
			}
		}
		if rawIPv6, ok := lookupJSONPath(obj, s.cfg.IPv6Field); ok {
			if ip, err := parseMaybeIP(fmt.Sprint(rawIPv6)); err == nil && ip.Is6() {
				result.IPv6 = ip
			}
		}
		if !result.HasIPv4() && !result.HasIPv6() {
			return ResolvedIPs{}, fmt.Errorf("device %s found but no valid IP extracted", s.cfg.DeviceMAC)
		}
		return result, nil
	}

	return ResolvedIPs{}, fmt.Errorf("device %s not found in router response", s.cfg.DeviceMAC)
}

func (s *ZTERouterSource) login(ctx context.Context) (string, error) {
	if s.cfg.LoginPath == "" {
		return "", nil
	}
	body := renderTemplate(s.cfg.LoginBody, s.cfg.Username, s.cfg.Password)
	req, err := http.NewRequestWithContext(ctx, s.cfg.LoginMethod, joinURL(s.cfg.BaseURL, s.cfg.LoginPath), bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("build login request: %w", err)
	}
	for key, value := range s.cfg.LoginHeaders {
		req.Header.Set(key, value)
	}
	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("router login: %w", err)
	}
	payloadBytes, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("login status %d: %s", resp.StatusCode, strings.TrimSpace(string(payloadBytes)))
	}
	if s.cfg.TokenPath == "" {
		return "", nil
	}
	var payload any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	value, ok := lookupJSONPath(payload, s.cfg.TokenPath)
	if !ok {
		return "", fmt.Errorf("login token path %q not found", s.cfg.TokenPath)
	}
	return fmt.Sprint(value), nil
}

func renderTemplate(body string, username string, password string) string {
	replacer := strings.NewReplacer("{{username}}", username, "{{password}}", password)
	return replacer.Replace(body)
}

func joinURL(base string, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

func normalizeMAC(raw string) string {
	replacer := strings.NewReplacer(":", "", "-", "", ".", "")
	return strings.ToLower(replacer.Replace(raw))
}

func parseMaybeIP(value string) (netip.Addr, error) {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}
	return netip.ParseAddr(value)
}

func lookupJSONPath(value any, path string) (any, bool) {
	if path == "" {
		return value, true
	}
	current := value
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			return nil, false
		default:
			return nil, false
		}
	}
	return current, true
}
