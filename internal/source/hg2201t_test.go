package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
)

func TestHG2201TResolveDualStackAcrossDeviceTypes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cgi-bin/luci", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("username"); got != "useradmin" {
			t.Fatalf("unexpected username %q", got)
		}
		if got := r.Form.Get("psd"); got != "pezbm" {
			t.Fatalf("unexpected password %q", got)
		}
		http.SetCookie(w, &http.Cookie{Name: "sysauth", Value: "ok", Path: "/"})
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/cgi-bin/luci/admin/device/devInfo", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sysauth"); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("type") {
		case "0":
			_, _ = w.Write([]byte(`{"count":1,"dev1":{"mac":"7C7D21B945D9","ip":"192.168.1.10","ipv6":"::"}}`))
		case "1":
			_, _ = w.Write([]byte(`{"count":1,"dev1":{"mac":"7C:7D:21:B9:45:D9","ip":"0.0.0.0","ipv6":"240e:36a:6c04:6000:7e7d:21ff:feb9:45d9"}}`))
		default:
			t.Fatalf("unexpected device type %q", r.URL.Query().Get("type"))
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	src := NewHG2201T(HG2201TConfig{
		BaseURL:   server.URL,
		Mode:      "device",
		Password:  "pezbm",
		DeviceMAC: "7c-7d-21-b9-45-d9",
		Client:    &http.Client{Jar: jar},
	})

	got, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.IPv4.String() != "192.168.1.10" {
		t.Fatalf("expected ipv4, got %s", got.IPv4)
	}
	if got.IPv6.String() != "240e:36a:6c04:6000:7e7d:21ff:feb9:45d9" {
		t.Fatalf("expected ipv6, got %s", got.IPv6)
	}
}

func TestHG2201TResolveNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cgi-bin/luci", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sysauth", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/cgi-bin/luci/admin/device/devInfo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"count":0}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	src := NewHG2201T(HG2201TConfig{
		BaseURL:   server.URL,
		Mode:      "device",
		Password:  "pezbm",
		DeviceMAC: "AA:BB:CC:DD:EE:FF",
		Client:    &http.Client{Jar: jar},
	})

	_, err = src.Resolve(context.Background())
	if err == nil || err.Error() != fmt.Sprintf("device %s not found in router response", "AA:BB:CC:DD:EE:FF") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHG2201TResolveWAN(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cgi-bin/luci", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sysauth", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/cgi-bin/luci/admin/settings/gwinfo", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("get"); got != "part" {
			t.Fatalf("unexpected get query %q", got)
		}
		_, _ = w.Write([]byte(`{"WANIP":"203.0.113.8","WANIPv6":"240e:36a:6c04:6000::8"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	src := NewHG2201T(HG2201TConfig{
		BaseURL:  server.URL,
		Mode:     "wan",
		Password: "pezbm",
		Client:   &http.Client{Jar: jar},
	})

	got, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.IPv4.String() != "203.0.113.8" {
		t.Fatalf("expected ipv4, got %s", got.IPv4)
	}
	if got.IPv6.String() != "240e:36a:6c04:6000::8" {
		t.Fatalf("expected ipv6, got %s", got.IPv6)
	}
}
