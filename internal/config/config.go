package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultListenAddr = ":8080"
	defaultInterval   = 5 * time.Minute
	defaultTimeout    = 10 * time.Second
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Tasks    []TaskConfig   `yaml:"tasks"`
}

type ServerConfig struct {
	Listen string `yaml:"listen"`
}

type DefaultsConfig struct {
	Interval Duration `yaml:"interval"`
	Timeout  Duration `yaml:"timeout"`
	LogFormat string  `yaml:"log_format"`
}

type TaskConfig struct {
	Name      string           `yaml:"name"`
	Interval  Duration         `yaml:"interval"`
	Source    SourceConfig     `yaml:"source"`
	Providers []ProviderConfig `yaml:"providers"`
}

type SourceConfig struct {
	Type      string               `yaml:"type"`
	Interface InterfaceSourceConfig `yaml:"interface"`
	Public    PublicSourceConfig    `yaml:"public"`
	DNS       DNSSourceConfig       `yaml:"dns"`
	ZTERouter ZTERouterSourceConfig `yaml:"zte_router"`
	ZTEStar   ZTEStarSourceConfig   `yaml:"zte_star"`
}

type InterfaceSourceConfig struct {
	Name             string `yaml:"name"`
	AllowLinkLocal   bool   `yaml:"allow_link_local"`
	AllowLoopback    bool   `yaml:"allow_loopback"`
	PreferTemporary  bool   `yaml:"prefer_temporary"`
}

type PublicSourceConfig struct {
	IPv4URLs []string `yaml:"ipv4_urls"`
	IPv6URLs []string `yaml:"ipv6_urls"`
}

type DNSSourceConfig struct {
	Hostname string   `yaml:"hostname"`
	Resolver []string `yaml:"resolver"`
}

type ZTERouterSourceConfig struct {
	BaseURL      string            `yaml:"base_url"`
	Username     string            `yaml:"username"`
	Password     string            `yaml:"password"`
	LoginPath    string            `yaml:"login_path"`
	LoginMethod  string            `yaml:"login_method"`
	LoginBody    string            `yaml:"login_body"`
	LoginHeaders map[string]string `yaml:"login_headers"`
	TokenPath    string            `yaml:"token_path"`
	AuthHeader   string            `yaml:"auth_header"`
	AuthScheme   string            `yaml:"auth_scheme"`
	DevicesPath  string            `yaml:"devices_path"`
	DevicesMethod string           `yaml:"devices_method"`
	DevicesBody  string            `yaml:"devices_body"`
	DevicesHeaders map[string]string `yaml:"devices_headers"`
	DeviceListPath string          `yaml:"device_list_path"`
	DeviceMAC    string            `yaml:"device_mac"`
	DeviceMACField string          `yaml:"device_mac_field"`
	IPv4Field    string            `yaml:"ipv4_field"`
	IPv6Field    string            `yaml:"ipv6_field"`
}

type ZTEStarSourceConfig struct {
	BaseURL   string `yaml:"base_url"`
	Password  string `yaml:"password"`
	DeviceMAC string `yaml:"device_mac"`
}

type ProviderConfig struct {
	Type       string                  `yaml:"type"`
	Cloudflare CloudflareProviderConfig `yaml:"cloudflare"`
	DNSPod     DNSPodProviderConfig     `yaml:"dnspod"`
	Records    []RecordConfig          `yaml:"records"`
}

type CloudflareProviderConfig struct {
	APIToken string `yaml:"api_token"`
}

type DNSPodProviderConfig struct {
	SecretID  string `yaml:"secret_id"`
	SecretKey string `yaml:"secret_key"`
}

type RecordConfig struct {
	Zone    string `yaml:"zone"`
	Name    string `yaml:"name"`
	TTL     int    `yaml:"ttl"`
	Proxied *bool  `yaml:"proxied"`
	IPv4    *bool  `yaml:"ipv4"` // nil = default true
	IPv6    *bool  `yaml:"ipv6"` // nil = default true
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" {
		return nil
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	d.Duration = parsed
	return nil
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.Expand(string(raw), func(key string) string {
		return os.Getenv(key)
	})

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = defaultListenAddr
	}
	if c.Defaults.Interval.Duration == 0 {
		c.Defaults.Interval.Duration = defaultInterval
	}
	if c.Defaults.Timeout.Duration == 0 {
		c.Defaults.Timeout.Duration = defaultTimeout
	}
	if c.Defaults.LogFormat == "" {
		c.Defaults.LogFormat = "text"
	}
	for i := range c.Tasks {
		if c.Tasks[i].Interval.Duration == 0 {
			c.Tasks[i].Interval.Duration = c.Defaults.Interval.Duration
		}
		for j := range c.Tasks[i].Providers {
			for k := range c.Tasks[i].Providers[j].Records {
				if c.Tasks[i].Providers[j].Records[k].TTL == 0 {
					c.Tasks[i].Providers[j].Records[k].TTL = 300
				}
			}
		}
	}
}

func (c *Config) Validate() error {
	if len(c.Tasks) == 0 {
		return errors.New("config requires at least one task")
	}

	for i, task := range c.Tasks {
		prefix := fmt.Sprintf("tasks[%d]", i)
		if strings.TrimSpace(task.Name) == "" {
			return fmt.Errorf("%s.name is required", prefix)
		}
		if task.Interval.Duration <= 0 {
			return fmt.Errorf("%s.interval must be > 0", prefix)
		}
		if err := validateSource(prefix, task.Source); err != nil {
			return err
		}
		if len(task.Providers) == 0 {
			return fmt.Errorf("%s.providers must have at least one entry", prefix)
		}
		for j, p := range task.Providers {
			pPrefix := fmt.Sprintf("%s.providers[%d]", prefix, j)
			if err := validateProvider(pPrefix, p); err != nil {
				return err
			}
			if len(p.Records) == 0 {
				return fmt.Errorf("%s.records must have at least one entry", pPrefix)
			}
			for k, r := range p.Records {
				rPrefix := fmt.Sprintf("%s.records[%d]", pPrefix, k)
				if strings.TrimSpace(r.Zone) == "" {
					return fmt.Errorf("%s.zone is required", rPrefix)
				}
				if strings.TrimSpace(r.Name) == "" {
					return fmt.Errorf("%s.name is required", rPrefix)
				}
			}
		}
	}
	return nil
}

func validateSource(prefix string, src SourceConfig) error {
	switch src.Type {
	case "interface":
		if strings.TrimSpace(src.Interface.Name) == "" {
			return fmt.Errorf("%s.source.interface.name is required", prefix)
		}
	case "public":
		if len(src.Public.IPv4URLs) == 0 && len(src.Public.IPv6URLs) == 0 {
			return fmt.Errorf("%s.source.public must define ipv4_urls and/or ipv6_urls", prefix)
		}
	case "dns":
		if strings.TrimSpace(src.DNS.Hostname) == "" {
			return fmt.Errorf("%s.source.dns.hostname is required", prefix)
		}
	case "zte_router":
		zte := src.ZTERouter
		if zte.BaseURL == "" || zte.DevicesPath == "" || zte.DeviceMAC == "" {
			return fmt.Errorf("%s.source.zte_router requires base_url, devices_path, device_mac", prefix)
		}
		if zte.DeviceListPath == "" {
			return fmt.Errorf("%s.source.zte_router.device_list_path is required", prefix)
		}
	case "zte_star":
		zs := src.ZTEStar
		if zs.BaseURL == "" || zs.Password == "" || zs.DeviceMAC == "" {
			return fmt.Errorf("%s.source.zte_star requires base_url, password, device_mac", prefix)
		}
	default:
		return fmt.Errorf("%s.source.type %q is unsupported", prefix, src.Type)
	}
	return nil
}

func validateProvider(prefix string, provider ProviderConfig) error {
	switch provider.Type {
	case "cloudflare":
		if strings.TrimSpace(provider.Cloudflare.APIToken) == "" {
			return fmt.Errorf("%s.provider.cloudflare.api_token is required", prefix)
		}
	case "dnspod":
		if provider.DNSPod.SecretID == "" || provider.DNSPod.SecretKey == "" {
			return fmt.Errorf("%s.provider.dnspod requires secret_id and secret_key", prefix)
		}
	default:
		return fmt.Errorf("%s.provider.type %q is unsupported", prefix, provider.Type)
	}
	return nil
}
