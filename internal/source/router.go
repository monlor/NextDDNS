package source

import (
	"context"
	"fmt"
	"net/http"
)

type RouterConfig struct {
	Family      string
	Mode        string
	BaseURL     string
	Username    string
	Password    string
	DeviceMAC   string
	DeviceTypes []string
	Client      *http.Client
}

type RouterDriver interface {
	ResolveWAN(context.Context) (ResolvedIPs, error)
	ResolveDevice(context.Context, string) (ResolvedIPs, error)
}

type RouterSource struct {
	mode      string
	deviceMAC string
	driver    RouterDriver
}

func NewRouter(cfg RouterConfig) (*RouterSource, error) {
	if cfg.Mode == "" {
		cfg.Mode = "device"
	}
	if cfg.Mode != "device" && cfg.Mode != "wan" {
		return nil, fmt.Errorf("unsupported router mode %q", cfg.Mode)
	}

	var driver RouterDriver
	switch cfg.Family {
	case "hg2201t":
		driver = &hg2201tDriver{cfg: cfg}
	case "zte_star":
		if cfg.Mode == "wan" {
			return nil, fmt.Errorf("router family %q does not support wan mode", cfg.Family)
		}
		driver = &zteStarDriver{cfg: cfg}
	default:
		return nil, fmt.Errorf("unsupported router family %q", cfg.Family)
	}

	if cfg.Mode == "device" && cfg.DeviceMAC == "" {
		return nil, fmt.Errorf("device_mac is required in device mode")
	}
	return &RouterSource{
		mode:      cfg.Mode,
		deviceMAC: cfg.DeviceMAC,
		driver:    driver,
	}, nil
}

func (s *RouterSource) Resolve(ctx context.Context) (ResolvedIPs, error) {
	if s.mode == "wan" {
		return s.driver.ResolveWAN(ctx)
	}
	return s.driver.ResolveDevice(ctx, s.deviceMAC)
}

type hg2201tDriver struct {
	cfg RouterConfig
}

func (d *hg2201tDriver) ResolveWAN(ctx context.Context) (ResolvedIPs, error) {
	return NewHG2201T(HG2201TConfig{
		Mode:        "wan",
		BaseURL:     d.cfg.BaseURL,
		Username:    d.cfg.Username,
		Password:    d.cfg.Password,
		DeviceTypes: d.cfg.DeviceTypes,
		Client:      d.cfg.Client,
	}).Resolve(ctx)
}

func (d *hg2201tDriver) ResolveDevice(ctx context.Context, mac string) (ResolvedIPs, error) {
	return NewHG2201T(HG2201TConfig{
		Mode:        "device",
		BaseURL:     d.cfg.BaseURL,
		Username:    d.cfg.Username,
		Password:    d.cfg.Password,
		DeviceMAC:   mac,
		DeviceTypes: d.cfg.DeviceTypes,
		Client:      d.cfg.Client,
	}).Resolve(ctx)
}

type zteStarDriver struct {
	cfg RouterConfig
}

func (d *zteStarDriver) ResolveWAN(ctx context.Context) (ResolvedIPs, error) {
	return ResolvedIPs{}, fmt.Errorf("router family %q does not support wan mode", d.cfg.Family)
}

func (d *zteStarDriver) ResolveDevice(ctx context.Context, mac string) (ResolvedIPs, error) {
	return NewZTEStar(ZTEStarConfig{
		BaseURL:   d.cfg.BaseURL,
		Password:  d.cfg.Password,
		DeviceMAC: mac,
		Client:    d.cfg.Client,
	}).Resolve(ctx)
}
