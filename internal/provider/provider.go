package provider

import "context"

type RecordType string

const (
	TypeA    RecordType = "A"
	TypeAAAA RecordType = "AAAA"
)

type Record struct {
	ID      string
	Name    string
	Type    RecordType
	Content string
	TTL     int
	Proxied *bool
}

type UpsertParams struct {
	Zone    string
	Name    string
	Type    RecordType
	Content string
	TTL     int
	Proxied *bool
}

type Provider interface {
	GetRecord(ctx context.Context, zone, name string, recordType RecordType) (*Record, error)
	UpsertRecord(ctx context.Context, params UpsertParams) error
}
