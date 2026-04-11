package provider

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	profile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

type DNSPod struct {
	client *dnspod.Client
}

func NewDNSPod(secretID string, secretKey string) (*DNSPod, error) {
	credential := common.NewCredential(secretID, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "dnspod.tencentcloudapi.com"

	client, err := dnspod.NewClient(credential, "", cpf)
	if err != nil {
		return nil, err
	}
	return &DNSPod{client: client}, nil
}

func (d *DNSPod) GetRecord(ctx context.Context, zone, name string, recordType RecordType) (*Record, error) {
	request := dnspod.NewDescribeRecordListRequest()
	request.Domain = common.StringPtr(zone)
	request.Subdomain = common.StringPtr(formatSubdomain(name, zone))
	request.RecordType = common.StringPtr(string(recordType))

	response, err := d.client.DescribeRecordListWithContext(ctx, request)
	if err != nil {
		var sdkErr *tcerrors.TencentCloudSDKError
		if stderrors.As(err, &sdkErr) && strings.HasPrefix(sdkErr.Code, "ResourceNotFound") {
			return nil, nil
		}
		return nil, err
	}
	if response.Response == nil || len(response.Response.RecordList) == 0 {
		return nil, nil
	}
	record := response.Response.RecordList[0]
	return &Record{
		ID:      fmt.Sprintf("%d", *record.RecordId),
		Name:    derefString(record.Name),
		Type:    RecordType(derefString(record.Type)),
		Content: derefString(record.Value),
		TTL:     int(derefUint64(record.TTL)),
	}, nil
}

func (d *DNSPod) UpsertRecord(ctx context.Context, params UpsertParams) error {
	current, err := d.GetRecord(ctx, params.Zone, params.Name, params.Type)
	if err != nil {
		return err
	}
	if current == nil {
		request := dnspod.NewCreateRecordRequest()
		request.Domain = common.StringPtr(params.Zone)
		request.SubDomain = common.StringPtr(formatSubdomain(params.Name, params.Zone))
		request.RecordType = common.StringPtr(string(params.Type))
		request.RecordLine = common.StringPtr("默认")
		request.Value = common.StringPtr(params.Content)
		request.TTL = common.Uint64Ptr(uint64(params.TTL))
		_, err := d.client.CreateRecordWithContext(ctx, request)
		return err
	}

	request := dnspod.NewModifyRecordRequest()
	request.Domain = common.StringPtr(params.Zone)
	request.RecordId = common.Uint64Ptr(parseUint64(current.ID))
	request.SubDomain = common.StringPtr(formatSubdomain(params.Name, params.Zone))
	request.RecordType = common.StringPtr(string(params.Type))
	request.RecordLine = common.StringPtr("默认")
	request.Value = common.StringPtr(params.Content)
	request.TTL = common.Uint64Ptr(uint64(params.TTL))
	_, err = d.client.ModifyRecordWithContext(ctx, request)
	return err
}

func formatSubdomain(name string, zone string) string {
	if name == zone || name == "@" {
		return "@"
	}
	suffix := "." + zone
	if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
		return name[:len(name)-len(suffix)]
	}
	return name
}

func parseUint64(value string) uint64 {
	var result uint64
	fmt.Sscanf(value, "%d", &result)
	return result
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefUint64(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}
