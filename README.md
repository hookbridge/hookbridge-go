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

## OAuth Tokens and Per-Request Headers

Instead of a static API key, a client can fetch its bearer token from a `TokenSource` (for example a refreshing OAuth access token). The token is requested on every request attempt, including retries.

```go
ts := hookbridge.TokenSourceFunc(func(ctx context.Context) (string, error) {
	return myOAuthSource.AccessToken(ctx)
})
client, err := hookbridge.NewClientWithTokenSource(ts)

ctx := hookbridge.ContextWithProject(context.Background(), "proj_123")
ctx = hookbridge.ContextWithClientName(ctx, "my-tool/1.0")
endpoints, err := client.ListEndpoints(ctx, nil)
```

`ContextWithProject` sets `X-Hookbridge-Project` (the project an OAuth token acts on) and `ContextWithClientName` sets `X-Hookbridge-Client` (a caller name recorded in the audit log). Both work with any client and any method. Values containing control characters are rejected before a request is sent.

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

## Event Types and Events

Define event types, subscribe endpoints to them, and send an event without naming an endpoint: it is fanned out to every endpoint whose subscriptions match. `Send` is unchanged.

```go
_, _ = client.CreateEventType(ctx, hookbridge.CreateEventTypeRequest{Name: "order.created", Group: stringPtr("orders")})

endpoint, _ := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
	URL:           "https://customer.app/webhooks",
	Subscriptions: &hookbridge.EventSubscriptions{EventTypes: []string{"order.*"}},
})
matching, _ := client.ListEndpoints(ctx, &hookbridge.EndpointsFilter{EventType: stringPtr("order.created")})

sent, _ := client.SendEvent(ctx, hookbridge.SendEventRequest{
	EventType: "order.created",
	Payload:   map[string]any{"order_id": "ord_123"},
})
event, _ := client.GetEvent(ctx, sent.EventID, nil) // FanoutStatus, MatchedCount, Messages
events, _ := client.ListEvents(ctx, &hookbridge.EventsFilter{EventType: stringPtr("order.created")})
_ = client.DeleteEvent(ctx, sent.EventID)

// Omitted fields stay unchanged on update; Null clears one.
_, _ = client.UpdateEventType(ctx, "order.created", hookbridge.UpdateEventTypeRequest{
	Description: stringPtr("Sent when an order is placed"),
	Group:       hookbridge.Null[string](),
})
_, _ = client.ArchiveEventType(ctx, "order.created")
_, _ = client.UnarchiveEventType(ctx, "order.created")
types, _ := client.ListEventTypes(ctx, &hookbridge.EventTypesFilter{NamePrefix: stringPtr("order.")})
groups, _ := client.ListEventTypeGroups(ctx)

// Give an endpoint the events it missed, then poll the job.
job, _ := client.RecoverEndpoint(ctx, endpoint.ID, hookbridge.RecoverRequest{Since: time.Now().Add(-time.Hour)})
status, _ := client.GetRecoverJob(ctx, endpoint.ID, job.RecoverJobID)

// Send one test message of an existing event type to check your receiver.
_, _ = client.SendTestEvent(ctx, endpoint.ID, hookbridge.SendTestEventRequest{EventType: "order.created"})
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

// Preview how many messages ReplayAll would attempt, without replaying any.
dryRun, _ := client.ReplayAllDryRun(ctx, hookbridge.ReplayAllMessagesRequest{
	Status: hookbridge.StatusFailedPermanent,
})
_ = dryRun.Data.WouldReplay

// Audit who replayed, retried or cancelled a message.
actions, _ := client.ListMessageActions(ctx, message.ID, hookbridge.ListMessageActionsParams{Limit: 50})
_ = actions

// Inbound equivalents: ReplayAllInboundMessagesDryRun and ListInboundMessageActions.
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
