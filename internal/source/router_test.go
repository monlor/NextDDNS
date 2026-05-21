package source

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
)

func TestRouterSourceHG2201TWAN(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cgi-bin/luci", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sysauth", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/cgi-bin/luci/admin/settings/gwinfo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"WANIP":"203.0.113.8","WANIPv6":"240e:36a:6c04:6000::8"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	src, err := NewRouter(RouterConfig{
		Family:   "hg2201t",
		Mode:     "wan",
		BaseURL:  server.URL,
		Password: "pezbm",
		Client:   &http.Client{Jar: jar},
	})
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	got, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.IPv4.String() != "203.0.113.8" || got.IPv6.String() != "240e:36a:6c04:6000::8" {
		t.Fatalf("unexpected ips: %+v", got)
	}
}

func TestRouterSourceRejectsUnsupportedMode(t *testing.T) {
	_, err := NewRouter(RouterConfig{
		Family:   "zte_star",
		Mode:     "wan",
		BaseURL:  "http://zte.home",
		Password: "x",
	})
	if err == nil {
		t.Fatal("expected unsupported mode error")
	}
}
