package syncer

import (
	"context"
	"net/netip"
	"testing"

	"nextddns/internal/provider"
	"nextddns/internal/source"
)

type fakeProvider struct {
	record  *provider.Record
	upserts int
}

func (f *fakeProvider) GetRecord(ctx context.Context, zone, name string, recordType provider.RecordType) (*provider.Record, error) {
	return f.record, nil
}

func (f *fakeProvider) UpsertRecord(ctx context.Context, params provider.UpsertParams) error {
	f.upserts++
	return nil
}

type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

func TestSyncSkipsUnchangedRecord(t *testing.T) {
	p := &fakeProvider{
		record: &provider.Record{
			Content: "203.0.113.7",
			Type:    provider.TypeA,
		},
	}
	s := New([]Target{{
		Provider: p,
		Records:  []Record{{Zone: "example.com", Name: "home", TTL: 300, IPv4: true, IPv6: false}},
	}}, noopLogger{})

	err := s.Sync(context.Background(), "task", source.ResolvedIPs{
		IPv4: netip.MustParseAddr("203.0.113.7"),
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if p.upserts != 0 {
		t.Fatalf("expected no upsert, got %d", p.upserts)
	}
}
