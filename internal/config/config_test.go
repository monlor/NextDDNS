package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadExpandsEnvAndDefaults(t *testing.T) {
	t.Setenv("CF_API_TOKEN", "token-123")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
tasks:
  - name: test
    families:
      ipv4: true
      ipv6: false
    source:
      type: public
      public:
        ipv4_urls: ["https://example.com/ip"]
    providers:
      - type: cloudflare
        cloudflare:
          api_token: ${CF_API_TOKEN}
        records:
          - zone: example.com
            name: home
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Listen != defaultListenAddr {
		t.Fatalf("expected default listen %s, got %s", defaultListenAddr, cfg.Server.Listen)
	}
	if cfg.Tasks[0].Providers[0].Cloudflare.APIToken != "token-123" {
		t.Fatalf("expected env expansion")
	}
	if cfg.Tasks[0].Interval.Duration != defaultInterval {
		t.Fatalf("expected default interval")
	}
}

func TestValidateRequiresRecords(t *testing.T) {
	cfg := &Config{
		Defaults: DefaultsConfig{Interval: Duration{Duration: defaultInterval}, Timeout: Duration{Duration: defaultTimeout}},
		Tasks: []TaskConfig{{
			Name:     "bad",
			Interval: Duration{Duration: time.Minute},
			Source:   SourceConfig{Type: "public", Public: PublicSourceConfig{IPv4URLs: []string{"https://example.com"}}},
			Providers: []ProviderConfig{{
				Type:       "cloudflare",
				Cloudflare: CloudflareProviderConfig{APIToken: "x"},
				Records:    nil, // no records — should fail
			}},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error when records is empty")
	}
}
