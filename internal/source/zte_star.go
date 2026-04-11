package source

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ZTEStarConfig struct {
	BaseURL   string
	Password  string
	DeviceMAC string
	Client    *http.Client
}

type ZTEStarSource struct {
	cfg ZTEStarConfig
}

func NewZTEStar(cfg ZTEStarConfig) *ZTEStarSource {
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	return &ZTEStarSource{cfg: cfg}
}

type zteLoginTokenResp struct {
	SessionToken string `json:"_sessionToken"`
	LoginToken   string `json:"logintoken"`
}

func (s *ZTEStarSource) Resolve(ctx context.Context) (ResolvedIPs, error) {
	// Step 1: Get login token
	tokenURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/?_type=loginsceneData&_tag=login_token_json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("build token request: %w", err)
	}
	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("get login token: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("read token response: %w", err)
	}
	var tokenResp zteLoginTokenResp
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return ResolvedIPs{}, fmt.Errorf("decode token response: %w", err)
	}

	// Step 2: Hash password: SHA256(password + logintoken)
	h := sha256.New()
	h.Write([]byte(s.cfg.Password + tokenResp.LoginToken))
	hashedPwd := fmt.Sprintf("%x", h.Sum(nil))

	// Step 3: Login
	loginURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/?_type=loginData&_tag=login_entry"
	form := url.Values{
		"Username":       {"admin"},
		"Password":       {hashedPwd},
		"action":         {"login"},
		"Frm_Logintoken": {""},
		"captchaCode":    {""},
		"_sessionTOKEN":  {tokenResp.SessionToken},
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = s.cfg.Client.Do(req)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("login: %w", err)
	}
	body, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("read login response: %w", err)
	}
	var loginResp struct {
		NeedRefresh bool   `json:"login_need_refresh"`
		ErrType     string `json:"loginErrType"`
	}
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return ResolvedIPs{}, fmt.Errorf("decode login response: %w", err)
	}
	if !loginResp.NeedRefresh {
		return ResolvedIPs{}, fmt.Errorf("login failed: %s", loginResp.ErrType)
	}

	// Step 4: Get device list
	devURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/?_type=vueData&_tag=localnet_lan_info_lua"
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, devURL, nil)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("build device list request: %w", err)
	}
	resp, err = s.cfg.Client.Do(req)
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("get device list: %w", err)
	}
	body, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return ResolvedIPs{}, fmt.Errorf("read device list response: %w", err)
	}

	// Step 5: Parse XML and find device by MAC
	return s.parseDevices(body)
}

// parseDevices parses the ZTE XML response and finds the device matching cfg.DeviceMAC.
// The XML structure is: <OBJ_LAN_INFO_ID><Instance><ParaName>k</ParaName><ParaValue>v</ParaValue>...</Instance></OBJ_LAN_INFO_ID>
func (s *ZTEStarSource) parseDevices(data []byte) (ResolvedIPs, error) {
	targetMAC := normalizeMAC(s.cfg.DeviceMAC)

	dec := xml.NewDecoder(strings.NewReader(string(data)))
	inLanInfo := false
	inInstance := false
	var params map[string]string
	var pendingKey string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ResolvedIPs{}, fmt.Errorf("parse XML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "OBJ_LAN_INFO_ID":
				inLanInfo = true
			case "Instance":
				if inLanInfo {
					inInstance = true
					params = make(map[string]string)
					pendingKey = ""
				}
			case "ParaName":
				if inInstance {
					text, _ := readXMLText(dec)
					pendingKey = text
				}
			case "ParaValue":
				if inInstance && pendingKey != "" {
					text, _ := readXMLText(dec)
					params[pendingKey] = text
					pendingKey = ""
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "OBJ_LAN_INFO_ID":
				inLanInfo = false
			case "Instance":
				if inInstance {
					inInstance = false
					if normalizeMAC(params["MACAddress"]) == targetMAC {
						return extractZTEStarIPs(params, s.cfg.DeviceMAC)
					}
				}
			}
		}
	}

	return ResolvedIPs{}, fmt.Errorf("device %s not found in router response", s.cfg.DeviceMAC)
}

// readXMLText reads concatenated CharData tokens until the next EndElement.
func readXMLText(dec *xml.Decoder) (string, error) {
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return sb.String(), err
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			return strings.TrimSpace(sb.String()), nil
		}
	}
}

func extractZTEStarIPs(params map[string]string, mac string) (ResolvedIPs, error) {
	var result ResolvedIPs

	if v := params["IPAddress"]; v != "" {
		if ip, err := parseMaybeIP(v); err == nil && ip.Is4() {
			result.IPv4 = ip
		}
	}

	// IPV6GuaAddr0, IPV6GuaAddr1, ... are the global unicast IPv6 addresses
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("IPV6GuaAddr%d", i)
		if v := params[key]; v != "" && v != "::" {
			if ip, err := parseMaybeIP(v); err == nil && ip.Is6() && !ip.IsLinkLocalUnicast() {
				result.IPv6 = ip
				break
			}
		}
	}

	if !result.HasIPv4() && !result.HasIPv6() {
		return ResolvedIPs{}, fmt.Errorf("device %s found but no valid IP extracted", mac)
	}
	return result, nil
}
