package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

type Cloudflare struct {
	apiToken string
	client   *http.Client
}

func NewCloudflare(apiToken string, client *http.Client) *Cloudflare {
	if client == nil {
		client = http.DefaultClient
	}
	return &Cloudflare{apiToken: apiToken, client: client}
}

func (c *Cloudflare) GetRecord(ctx context.Context, zone, name string, recordType RecordType) (*Record, error) {
	zoneID, err := c.getZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("type", string(recordType))
	query.Set("name", fqdn(name, zone))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cloudflareAPIBase+"/zones/"+zoneID+"/dns_records?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.applyAuth(req)
	var payload struct {
		cloudflareEnvelope
		Result []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
			TTL     int    `json:"ttl"`
			Proxied *bool  `json:"proxied"`
		} `json:"result"`
	}
	if err := c.doJSON(req, &payload); err != nil {
		return nil, err
	}
	if len(payload.Result) == 0 {
		return nil, nil
	}
	record := payload.Result[0]
	return &Record{
		ID:      record.ID,
		Name:    record.Name,
		Type:    RecordType(record.Type),
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: record.Proxied,
	}, nil
}

func (c *Cloudflare) UpsertRecord(ctx context.Context, params UpsertParams) error {
	zoneID, err := c.getZoneID(ctx, params.Zone)
	if err != nil {
		return err
	}
	current, err := c.GetRecord(ctx, params.Zone, params.Name, params.Type)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"type":    params.Type,
		"name":    fqdn(params.Name, params.Zone),
		"content": params.Content,
		"ttl":     params.TTL,
	}
	if params.Proxied != nil {
		payload["proxied"] = *params.Proxied
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	method := http.MethodPost
	endpoint := cloudflareAPIBase + "/zones/" + zoneID + "/dns_records"
	if current != nil {
		method = http.MethodPut
		endpoint = endpoint + "/" + current.ID
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req)
	return c.doJSON(req, &struct{}{})
}

func (c *Cloudflare) getZoneID(ctx context.Context, zone string) (string, error) {
	query := url.Values{}
	query.Set("name", zone)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cloudflareAPIBase+"/zones?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	c.applyAuth(req)
	var payload struct {
		cloudflareEnvelope
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := c.doJSON(req, &payload); err != nil {
		return "", err
	}
	if len(payload.Result) == 0 {
		return "", fmt.Errorf("cloudflare zone %q not found", zone)
	}
	return payload.Result[0].ID, nil
}

func (c *Cloudflare) applyAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
}

func (c *Cloudflare) doJSON(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare API status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 || out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	if envelope, ok := out.(interface {
		isSuccess() bool
		errorMessages() []string
	}); ok && !envelope.isSuccess() {
		return errors.New(strings.Join(envelope.errorMessages(), "; "))
	}
	if carrier, ok := out.(interface{ GetErrors() []string }); ok {
		messages := carrier.GetErrors()
		if len(messages) > 0 {
			return errors.New(strings.Join(messages, "; "))
		}
	}
	return nil
}

type cloudflareEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (e *cloudflareEnvelope) isSuccess() bool {
	return e.Success
}

func (e *cloudflareEnvelope) errorMessages() []string {
	out := make([]string, 0, len(e.Errors))
	for _, item := range e.Errors {
		out = append(out, item.Message)
	}
	return out
}

func fqdn(name string, zone string) string {
	if name == "@" || name == zone {
		return zone
	}
	if strings.HasSuffix(name, "."+zone) {
		return name
	}
	return name + "." + zone
}
