package source

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestRouterSourceSerializesSameRouter(t *testing.T) {
	t.Parallel()

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	started := make(chan struct{}, 2)
	release := make(chan struct{})

	driver := &blockingDriver{
		resolveDevice: func(context.Context, string) (ResolvedIPs, error) {
			current := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				seen := maxInFlight.Load()
				if current <= seen || maxInFlight.CompareAndSwap(seen, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			return ResolvedIPs{}, nil
		},
	}

	src1 := &RouterSource{mode: "device", deviceMAC: "aa", serialKey: "zte_star|http://router.local", driver: driver}
	src2 := &RouterSource{mode: "device", deviceMAC: "bb", serialKey: "zte_star|http://router.local", driver: driver}

	var wg sync.WaitGroup
	wg.Add(2)
	for _, src := range []*RouterSource{src1, src2} {
		go func(src *RouterSource) {
			defer wg.Done()
			if _, err := src.Resolve(context.Background()); err != nil {
				t.Errorf("Resolve() error = %v", err)
			}
		}(src)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first resolve did not start")
	}

	select {
	case <-started:
		t.Fatal("second resolve started before first finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	wg.Wait()

	if got := maxInFlight.Load(); got != 1 {
		t.Fatalf("expected max in-flight resolve to be 1, got %d", got)
	}
}

type blockingDriver struct {
	resolveWAN    func(context.Context) (ResolvedIPs, error)
	resolveDevice func(context.Context, string) (ResolvedIPs, error)
}

func (d *blockingDriver) ResolveWAN(ctx context.Context) (ResolvedIPs, error) {
	if d.resolveWAN == nil {
		return ResolvedIPs{}, nil
	}
	return d.resolveWAN(ctx)
}

func (d *blockingDriver) ResolveDevice(ctx context.Context, mac string) (ResolvedIPs, error) {
	if d.resolveDevice == nil {
		return ResolvedIPs{}, nil
	}
	return d.resolveDevice(ctx, mac)
}
