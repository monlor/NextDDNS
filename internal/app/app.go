package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync"
	"time"

	"nextddns/internal/config"
	"nextddns/internal/health"
	"nextddns/internal/provider"
	"nextddns/internal/source"
	"nextddns/internal/syncer"
)

type App struct {
	cfg    *config.Config
	logger *slog.Logger
	health *health.Server
	tasks  []taskRunner
}

type taskRunner struct {
	name     string
	interval time.Duration
	run      func(context.Context)
}

func New(configPath string, logger *slog.Logger) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	if cfg.Defaults.LogFormat == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))
	}

	app := &App{
		cfg:    cfg,
		logger: logger,
		health: health.New(cfg.Server.Listen),
	}

	for _, task := range cfg.Tasks {
		runner, err := app.buildTask(task)
		if err != nil {
			return nil, fmt.Errorf("build task %s: %w", task.Name, err)
		}
		app.tasks = append(app.tasks, runner)
	}

	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := a.health.Run(); err != nil {
			errCh <- err
		}
	}()

	var wg sync.WaitGroup
	for _, task := range a.tasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			task.run(ctx)
			ticker := time.NewTicker(task.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					task.run(ctx)
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.health.Shutdown(shutdownCtx)
		wg.Wait()
		return nil
	case err := <-errCh:
		return err
	}
}

func (a *App) buildTask(task config.TaskConfig) (taskRunner, error) {
	timeout := a.cfg.Defaults.Timeout.Duration
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	httpClient := newHTTPClient(timeout)

	src, err := buildSource(task.Source, httpClient)
	if err != nil {
		return taskRunner{}, err
	}
	var targets []syncer.Target
	for _, pc := range task.Providers {
		p, err := buildProvider(pc)
		if err != nil {
			return taskRunner{}, err
		}
		var records []syncer.Record
		for _, r := range pc.Records {
			records = append(records, syncer.Record{
				Zone:    r.Zone,
				Name:    r.Name,
				TTL:     r.TTL,
				Proxied: r.Proxied,
				IPv4:    derefBool(r.IPv4, true),
				IPv6:    derefBool(r.IPv6, true),
			})
		}
		targets = append(targets, syncer.Target{Provider: p, Records: records})
	}
	s := syncer.New(targets, a.logger)
	return taskRunner{
		name:     task.Name,
		interval: task.Interval.Duration,
		run: func(parent context.Context) {
			defer func() {
				if r := recover(); r != nil {
					a.logger.Error("task panicked", "task", task.Name, "panic", r)
				}
			}()
			ctx, cancel := context.WithTimeout(parent, timeout)
			defer cancel()

			ips, err := src.Resolve(ctx)
			if err != nil {
				a.logger.Error("resolve source failed", "task", task.Name, "error", err)
				return
			}
			_ = s.Sync(ctx, task.Name, ips)
		},
	}, nil
}

func buildSource(cfg config.SourceConfig, client *http.Client) (source.Source, error) {
	switch cfg.Type {
	case "interface":
		return source.NewInterface(source.InterfaceConfig{
			Name:           cfg.Interface.Name,
			AllowLinkLocal: cfg.Interface.AllowLinkLocal,
			AllowLoopback:  cfg.Interface.AllowLoopback,
		}), nil
	case "public":
		return source.NewPublic(source.PublicConfig{
			IPv4URLs: cfg.Public.IPv4URLs,
			IPv6URLs: cfg.Public.IPv6URLs,
			Client:   client,
		}), nil
	case "dns":
		return source.NewDNS(source.DNSConfig{
			Hostname: cfg.DNS.Hostname,
			Resolver: cfg.DNS.Resolver,
		}), nil
	case "router":
		return buildRouterSource(cfg.Router, client)
	default:
		return nil, fmt.Errorf("unsupported source type %q", cfg.Type)
	}
}

func buildRouterSource(cfg config.RouterSourceConfig, client *http.Client) (source.Source, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	copied := *client
	copied.Jar = jar
	return source.NewRouter(source.RouterConfig{
		Family:      cfg.Family,
		Mode:        cfg.Mode,
		BaseURL:     cfg.BaseURL,
		Username:    cfg.Username,
		Password:    cfg.Password,
		DeviceMAC:   cfg.DeviceMAC,
		DeviceTypes: cfg.DeviceTypes,
		Client:      &copied,
	})
}

func buildProvider(cfg config.ProviderConfig) (provider.Provider, error) {
	switch cfg.Type {
	case "cloudflare":
		return provider.NewCloudflare(cfg.Cloudflare.APIToken, nil), nil
	case "dnspod":
		return provider.NewDNSPod(cfg.DNSPod.SecretID, cfg.DNSPod.SecretKey)
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cfg.Type)
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func derefBool(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
