package syncer

import (
	"context"
	"fmt"
	"net/netip"

	"nextddns/internal/provider"
	"nextddns/internal/source"
)

type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Target pairs a provider with the DNS records it should manage.
type Target struct {
	Provider provider.Provider
	Records  []Record
}

// Record holds the DNS record parameters for one entry.
type Record struct {
	Zone    string
	Name    string
	TTL     int
	Proxied *bool
	IPv4    bool // sync A records
	IPv6    bool // sync AAAA records
}

type Syncer struct {
	targets []Target
	logger  Logger
}

func New(targets []Target, logger Logger) *Syncer {
	return &Syncer{targets: targets, logger: logger}
}

func (s *Syncer) Sync(ctx context.Context, taskName string, ips source.ResolvedIPs) error {
	for _, t := range s.targets {
		for _, rec := range t.Records {
			if rec.IPv4 {
				if err := s.syncOne(ctx, taskName, rec, provider.TypeA, ips.IPv4, t.Provider); err != nil {
					s.logger.Error("sync failed", "task", taskName, "family", "ipv4", "record", rec.Name+"."+rec.Zone, "error", err)
				}
			}
			if rec.IPv6 {
				if err := s.syncOne(ctx, taskName, rec, provider.TypeAAAA, ips.IPv6, t.Provider); err != nil {
					s.logger.Error("sync failed", "task", taskName, "family", "ipv6", "record", rec.Name+"."+rec.Zone, "error", err)
				}
			}
		}
	}
	return nil
}

func (s *Syncer) syncOne(ctx context.Context, taskName string, rec Record, recordType provider.RecordType, ip netip.Addr, p provider.Provider) error {
	if !ip.IsValid() {
		s.logger.Warn("skip family because source has no address", "task", taskName, "type", recordType)
		return nil
	}
	current, err := p.GetRecord(ctx, rec.Zone, rec.Name, recordType)
	if err != nil {
		return fmt.Errorf("get record: %w", err)
	}
	if current != nil && current.Content == ip.String() {
		s.logger.Info("record already up to date", "task", taskName, "type", recordType, "record", rec.Name+"."+rec.Zone, "value", ip.String())
		return nil
	}
	if err := p.UpsertRecord(ctx, provider.UpsertParams{
		Zone:    rec.Zone,
		Name:    rec.Name,
		Type:    recordType,
		Content: ip.String(),
		TTL:     rec.TTL,
		Proxied: rec.Proxied,
	}); err != nil {
		return fmt.Errorf("upsert record: %w", err)
	}
	s.logger.Info("record updated", "task", taskName, "type", recordType, "record", rec.Name+"."+rec.Zone, "value", ip.String())
	return nil
}
