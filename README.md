# HookBridge Go SDK

Official Go SDK for HookBridge. Send webhooks with guaranteed delivery, automatic retries, and inbound/outbound observability.

## Installation

```bash
go get github.com/hookbridge/hookbridge-go/v2
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	hookbridge "github.com/hookbridge/hookbridge-go/v2"
)

func main() {
	client, _ := hookbridge.NewClient("hb_live_xxxxxxxxxxxxxxxxxxxx")
	ctx := context.Background()

	endpoint, _ := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
		URL: "https://customer.app/webhooks",
	})

	result, _ := client.Send(ctx, hookbridge.SendRequest{
		EndpointID: endpoint.ID,
		Payload: map[string]any{"event": "order.created"},
	})

	fmt.Println(result.MessageID)
}
```

## Outbound Endpoints

```go
endpoint, _ := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
	URL:         "https://customer.app/webhooks",
	Description: stringPtr("Main production webhook"),
	RateLimitRPS: intPtr(10),
	Burst:       intPtr(20),
})

details, _ := client.GetEndpoint(ctx, endpoint.ID)
list, _ := client.ListEndpoints(ctx, &hookbridge.EndpointsFilter{Limit: intPtr(50)})
_, _ = client.RotateEndpointSecret(ctx, endpoint.ID)
_ = client.DeleteEndpoint(ctx, endpoint.ID)
```

## Sending and Observability

```go
result, _ := client.Send(ctx, hookbridge.SendRequest{
	EndpointID: endpoint.ID,
	Payload:    map[string]any{"event": "user.created", "user_id": "usr_123"},
})

message, _ := client.GetMessage(ctx, result.MessageID)
logs, _ := client.GetLogs(ctx, &hookbridge.LogsFilter{Limit: intPtr(100)})
metrics, _ := client.GetMetrics(ctx, hookbridge.Window24Hour)
timeseries, _ := client.GetTimeSeriesMetrics(ctx, hookbridge.Window24Hour, &endpoint.ID)

_, _ = client.Replay(ctx, message.ID)
_, _ = client.ReplayAll(ctx, hookbridge.ReplayAllMessagesRequest{
	Status:     hookbridge.StatusFailedPermanent,
	EndpointID: &endpoint.ID,
	Limit:      intPtr(50),
})
```

## Inbound Webhooks

```go
tokenHeaderName := "X-Webhook-Token"
tokenValue := "my-shared-secret"

inbound, _ := client.CreateInboundEndpoint(ctx, hookbridge.CreateInboundEndpointRequest{
	URL:               stringPtr("https://myapp.com/webhooks/inbound"),
	Name:              stringPtr("Stripe inbound"),
	VerifyStaticToken: boolPtr(true),
	TokenHeaderName:   &tokenHeaderName,
	TokenValue:        &tokenValue,
	SigningEnabled:    boolPtr(true),
})

fmt.Println(inbound.IngestURL)   // Save this
fmt.Println(inbound.SecretToken) // Only shown once

inboundDetails, _ := client.GetInboundEndpoint(ctx, inbound.ID)
_, _ = client.PauseInboundEndpoint(ctx, inbound.ID)
_, _ = client.ResumeInboundEndpoint(ctx, inbound.ID)

_, _ = client.UpdateInboundEndpoint(ctx, inbound.ID, hookbridge.UpdateInboundEndpointRequest{
	VerifyHMAC:     boolPtr(true),
	HMACHeaderName: stringPtr("X-Signature"),
	HMACSecret:     stringPtr("whsec_inbound_secret"),
})
```

## Inbound Observability

```go
inboundList, _ := client.ListInboundEndpoints(ctx, &hookbridge.InboundEndpointsFilter{Limit: intPtr(50)})
inboundLogs, _ := client.GetInboundLogs(ctx, &hookbridge.InboundLogsFilter{
	InboundEndpointID: &inbound.ID,
	Limit:             intPtr(50),
})
inboundMetrics, _ := client.GetInboundMetrics(ctx, hookbridge.Window24Hour, &inbound.ID)
inboundTimeseries, _ := client.GetInboundTimeSeriesMetrics(ctx, hookbridge.Window24Hour, &inbound.ID)
rejections, _ := client.ListInboundRejections(ctx, &hookbridge.InboundRejectionsFilter{
	InboundEndpointID: &inbound.ID,
	Limit:             intPtr(25),
})

_, _, _, _ = inboundList, inboundLogs, inboundMetrics, inboundTimeseries
_ = rejections
```

## Billing and Exports

```go
subscription, _ := client.GetSubscription(ctx)
usage, _ := client.GetUsageHistory(ctx, intPtr(12), intPtr(0))
invoices, _ := client.GetInvoices(ctx)

export, _ := client.CreateExport(ctx, hookbridge.CreateExportRequest{
	StartTime: time.Now().Add(-24 * time.Hour).UTC(),
	EndTime:   time.Now().UTC(),
})

download, _ := client.DownloadExport(ctx, export.ID)

_, _, _, _ = subscription, usage, invoices, download
```
