package hookbridge_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hookbridge/hookbridge-go/v2"
)

func newMockClient(t *testing.T, handler http.HandlerFunc) *hookbridge.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	httpClient := server.Client()
	client, err := hookbridge.NewClient(
		"hb_test_mock",
		hookbridge.WithBaseURL(server.URL),
		hookbridge.WithSendURL(server.URL),
		hookbridge.WithHTTPClient(httpClient),
		hookbridge.WithRetries(0),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func readJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return out
}

func TestSpecParitySigningKeys(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/endpoints/ep_1/signing-keys":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"sk_1","key_hint":"abcd","created_at":"2025-12-06T12:10:00Z"}],"meta":{"request_id":"req_4"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/endpoints/ep_1/signing-keys":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"sk_2","signing_secret":"whsec_new","key_hint":"wxyz","created_at":"2025-12-06T12:10:00Z"},"meta":{"request_id":"req_5"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/endpoints/ep_1/signing-keys/sk_2":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{},"meta":{"request_id":"req_6"}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()

	keys, err := client.ListEndpointSigningKeys(ctx, "ep_1")
	if err != nil || len(keys) != 1 || keys[0].ID != "sk_1" {
		t.Fatalf("ListEndpointSigningKeys failed: %#v %v", keys, err)
	}

	createdKey, err := client.RotateEndpointSecret(ctx, "ep_1")
	if err != nil || createdKey.SigningSecret != "whsec_new" {
		t.Fatalf("RotateEndpointSecret failed: %#v %v", createdKey, err)
	}

	if err := client.DeleteEndpointSigningKey(ctx, "ep_1", "sk_2"); err != nil {
		t.Fatalf("DeleteEndpointSigningKey failed: %v", err)
	}
}

func TestSpecParityEndpointPauseState(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/endpoints/ep_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"ep_1","url":"https://customer.app/webhooks","description":"Main production webhook","paused":false,"rate_limit_rps":10,"burst":20,"created_at":"2025-12-01T10:00:00Z","updated_at":"2025-12-06T12:00:00Z"},"meta":{"request_id":"req_1"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/endpoints":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"ep_1","url":"https://customer.app/webhooks","description":"Main production webhook","paused":false,"created_at":"2025-12-01T10:00:00Z"}],"meta":{"request_id":"req_2","has_more":false}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/endpoints/ep_1/pause":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"ep_1","paused":true},"meta":{"request_id":"req_3"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/endpoints/ep_1/resume":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"ep_1","paused":false,"messages_requeued":5},"meta":{"request_id":"req_4"}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()

	endpoint, err := client.GetEndpoint(ctx, "ep_1")
	if err != nil || endpoint.Paused {
		t.Fatalf("GetEndpoint failed: %#v %v", endpoint, err)
	}

	listed, err := client.ListEndpoints(ctx, nil)
	if err != nil || len(listed.Endpoints) != 1 || listed.Endpoints[0].Paused {
		t.Fatalf("ListEndpoints failed: %#v %v", listed, err)
	}

	paused, err := client.PauseEndpoint(ctx, "ep_1")
	if err != nil || !paused.Paused {
		t.Fatalf("PauseEndpoint failed: %#v %v", paused, err)
	}

	resumed, err := client.ResumeEndpoint(ctx, "ep_1")
	if err != nil || resumed.Paused {
		t.Fatalf("ResumeEndpoint failed: %#v %v", resumed, err)
	}
	if resumed.MessagesRequeued == nil || *resumed.MessagesRequeued != 5 {
		t.Fatalf("expected messages requeued, got %#v", resumed)
	}
}

func TestSpecParityPullEndpointsAndObservability(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pull-endpoints":
			body := readJSONBody(t, r)
			if body["name"] != "Stripe Pull" || body["retention_days"] != float64(14) {
				t.Fatalf("unexpected pull create payload %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"pull_1","name":"Stripe Pull","description":"Stores provider events for polling","mode":"pull","ingest_url":"https://ingest.hookbridge.io/pull/secret-token","secret_token":"secret-token","active":true,"paused":false,"retention_days":14,"event_type_source":"body","event_type_path":"type","verify_static_token":true,"token_header_name":"X-Webhook-Token","verify_hmac":false,"verify_ip_allowlist":false,"ingest_response_code":202,"idempotency_header_names":["X-Idempotency-Key"],"created_at":"2025-12-06T12:00:00Z","updated_at":"2025-12-06T12:00:00Z"},"meta":{"request_id":"req_1"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pull-endpoints":
			if r.URL.Query().Get("limit") != "10" {
				t.Fatalf("unexpected pull endpoint query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"pull_1","name":"Stripe Pull","active":true,"paused":false,"ingest_url":"https://ingest.hookbridge.io/pull","created_at":"2025-12-06T12:00:00Z"}],"meta":{"request_id":"req_2","has_more":false}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pull-endpoints/pull_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"pull_1","name":"Stripe Pull","description":"Stores provider events for polling","mode":"pull","ingest_url":"https://ingest.hookbridge.io/pull","active":true,"paused":false,"retention_days":14,"event_type_source":"body","event_type_path":"type","counts":{"stored":1,"fetched":0,"delivered":0,"total":1},"verify_static_token":true,"token_header_name":"X-Webhook-Token","verify_hmac":false,"verify_ip_allowlist":false,"ingest_response_code":202,"idempotency_header_names":["X-Idempotency-Key"],"created_at":"2025-12-06T12:00:00Z","updated_at":"2025-12-06T12:05:00Z"},"meta":{"request_id":"req_3"}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/pull-endpoints/pull_1":
			body := readJSONBody(t, r)
			if body["name"] != "Stripe Pull Renamed" || body["retention_days"] != float64(21) {
				t.Fatalf("unexpected pull update payload %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"pull_1","name":"Stripe Pull Renamed","description":"Stores provider events for polling","mode":"pull","ingest_url":"https://ingest.hookbridge.io/pull","active":true,"paused":false,"retention_days":21,"event_type_source":"body","event_type_path":"type","counts":{"stored":1,"fetched":0,"delivered":0,"total":1},"verify_static_token":true,"token_header_name":"X-Webhook-Token","verify_hmac":false,"verify_ip_allowlist":false,"ingest_response_code":202,"idempotency_header_names":["X-Idempotency-Key"],"created_at":"2025-12-06T12:00:00Z","updated_at":"2025-12-06T12:10:00Z"},"meta":{"request_id":"req_4"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pull-endpoints/pull_1/pause":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"pull_1","paused":true},"meta":{"request_id":"req_5"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pull-endpoints/pull_1/resume":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"pull_1","paused":false},"meta":{"request_id":"req_6"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pull-endpoints/pull_1/events":
			if r.URL.Query().Get("status") != "stored" || r.URL.Query().Get("event_type") != "payment_intent.succeeded" {
				t.Fatalf("unexpected pull events query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"evt_1","event_type":"payment_intent.succeeded","status":"stored","size_bytes":256,"received_at":"2025-12-06T12:01:00Z","fetched_at":null}],"meta":{"request_id":"req_7","has_more":false,"next_cursor":""}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pull-endpoints/pull_1/events/evt_1":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("preview") == "true" {
				io.WriteString(w, `{"data":{"id":"evt_1","event_type":"payment_intent.succeeded","status":"stored","content_type":"application/json","payload":{"type":"payment_intent.succeeded","id":"evt_123"},"headers":{"content-type":"application/json"},"size_bytes":256,"received_at":"2025-12-06T12:01:00Z","timing":{"ingest_processing_ms":12}},"meta":{"request_id":"req_8p"}}`)
			} else {
				io.WriteString(w, `{"data":{"id":"evt_1","event_type":"payment_intent.succeeded","status":"fetched","content_type":"application/json","payload":{"type":"payment_intent.succeeded","id":"evt_123"},"headers":{"content-type":"application/json"},"size_bytes":256,"received_at":"2025-12-06T12:01:00Z","fetched_at":"2025-12-06T12:01:30Z","timing":{"ingest_processing_ms":12,"time_to_fetch_ms":30000}},"meta":{"request_id":"req_8"}}`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pull-endpoints/pull_1/events/ack":
			body := readJSONBody(t, r)
			items, ok := body["event_ids"].([]any)
			if !ok || len(items) != 1 || items[0] != "evt_1" {
				t.Fatalf("unexpected pull ack payload %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"acknowledged":1},"meta":{"request_id":"req_9"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/pull-endpoints/pull_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"pull_1","deleted":true},"meta":{"request_id":"req_10"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pull-logs":
			if r.URL.Query().Get("pull_endpoint_id") != "pull_1" || r.URL.Query().Get("status") != "delivered" {
				t.Fatalf("unexpected pull logs query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"event_id":"evt_1","pull_endpoint_id":"pull_1","endpoint_name":"Stripe Pull","event_type":"payment_intent.succeeded","status":"fetched","size_bytes":256,"received_at":"2025-12-06T12:01:00Z","fetched_at":"2025-12-06T12:01:30Z","timing":{"ingest_processing_ms":12,"time_to_fetch_ms":30000}}],"meta":{"request_id":"req_11","has_more":false}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pull-metrics":
			if r.URL.Query().Get("pull_endpoint_id") != "pull_1" || r.URL.Query().Get("window") != "24h" {
				t.Fatalf("unexpected pull metrics query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"window":"24h","total_messages":10,"succeeded":4,"failed":0,"retries":0,"success_rate":0.4,"avg_latency_ms":15},"meta":{"request_id":"req_12"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pull-metrics/timeseries":
			if r.URL.Query().Get("pull_endpoint_id") != "pull_1" || r.URL.Query().Get("window") != "24h" {
				t.Fatalf("unexpected pull timeseries query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"window":"24h","buckets":[{"timestamp":"2025-12-06T12:00:00Z","succeeded":4,"stored":4,"fetched":2,"total":10}]},"meta":{"request_id":"req_13"}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()
	name := "Stripe Pull"
	description := "Stores provider events for polling"
	retention := 14
	eventTypeSource := "body"
	eventTypePath := "type"
	verifyStaticToken := true
	tokenHeaderName := "X-Webhook-Token"
	tokenValue := "secret-token-value"
	idempotencyHeaders := []string{"X-Idempotency-Key"}
	ingestResponseCode := 202

	created, err := client.CreatePullEndpoint(ctx, hookbridge.CreatePullEndpointRequest{
		Name:                   &name,
		Description:            &description,
		RetentionDays:          &retention,
		EventTypeSource:        &eventTypeSource,
		EventTypePath:          &eventTypePath,
		VerifyStaticToken:      &verifyStaticToken,
		TokenHeaderName:        &tokenHeaderName,
		TokenValue:             &tokenValue,
		IdempotencyHeaderNames: &idempotencyHeaders,
		IngestResponseCode:     &ingestResponseCode,
	})
	if err != nil || created.ID != "pull_1" || created.SecretToken == nil || *created.SecretToken != "secret-token" {
		t.Fatalf("CreatePullEndpoint failed: %#v %v", created, err)
	}

	limit := 10
	listed, err := client.ListPullEndpoints(ctx, &hookbridge.PullEndpointsFilter{Limit: &limit})
	if err != nil || len(listed.Endpoints) != 1 || listed.Endpoints[0].ID != "pull_1" {
		t.Fatalf("ListPullEndpoints failed: %#v %v", listed, err)
	}

	fetched, err := client.GetPullEndpoint(ctx, "pull_1")
	if err != nil || fetched.Counts == nil || fetched.Counts.Stored == nil || *fetched.Counts.Stored != 1 || fetched.Counts.Fetched == nil || *fetched.Counts.Fetched != 0 {
		t.Fatalf("GetPullEndpoint failed: %#v %v", fetched, err)
	}

	updatedName := "Stripe Pull Renamed"
	updatedRetention := 21
	updated, err := client.UpdatePullEndpoint(ctx, "pull_1", hookbridge.UpdatePullEndpointRequest{
		Name:          &updatedName,
		RetentionDays: &updatedRetention,
	})
	if err != nil || updated.Name == nil || *updated.Name != "Stripe Pull Renamed" {
		t.Fatalf("UpdatePullEndpoint failed: %#v %v", updated, err)
	}

	paused, err := client.PausePullEndpoint(ctx, "pull_1")
	if err != nil || !paused.Paused {
		t.Fatalf("PausePullEndpoint failed: %#v %v", paused, err)
	}

	resumed, err := client.ResumePullEndpoint(ctx, "pull_1")
	if err != nil || resumed.Paused {
		t.Fatalf("ResumePullEndpoint failed: %#v %v", resumed, err)
	}

	status := "stored"
	eventType := "payment_intent.succeeded"
	eventLimit := 5
	events, err := client.ListPullEvents(ctx, "pull_1", &hookbridge.PullEventsFilter{
		Status:    &status,
		EventType: &eventType,
		Limit:     &eventLimit,
	})
	if err != nil || len(events.Events) != 1 || events.Events[0].ID != "evt_1" {
		t.Fatalf("ListPullEvents failed: %#v %v", events, err)
	}

	preview, err := client.GetPullEvent(ctx, "pull_1", "evt_1", true)
	if err != nil || preview.Status != "stored" || preview.Timing == nil || preview.Timing.IngestProcessingMs == nil || *preview.Timing.IngestProcessingMs != 12 {
		t.Fatalf("GetPullEvent preview failed: %#v %v", preview, err)
	}

	event, err := client.GetPullEvent(ctx, "pull_1", "evt_1", false)
	if err != nil || event.ContentType != "application/json" || event.Status != "fetched" || event.FetchedAt == nil {
		t.Fatalf("GetPullEvent failed: %#v %v", event, err)
	}
	if event.Timing == nil || event.Timing.TimeToFetchMs == nil || *event.Timing.TimeToFetchMs != 30000 {
		t.Fatalf("GetPullEvent timing failed: %#v", event.Timing)
	}

	acked, err := client.AckPullEvents(ctx, "pull_1", []string{"evt_1"})
	if err != nil || acked.Acknowledged != 1 {
		t.Fatalf("AckPullEvents failed: %#v %v", acked, err)
	}

	deleted, err := client.DeletePullEndpoint(ctx, "pull_1")
	if err != nil || !deleted.Deleted {
		t.Fatalf("DeletePullEndpoint failed: %#v %v", deleted, err)
	}

	delivered := "delivered"
	logLimit := 10
	logs, err := client.GetPullLogs(ctx, &hookbridge.PullLogsFilter{
		PullEndpointID: &created.ID,
		Status:         &delivered,
		EventType:      &eventType,
		Limit:          &logLimit,
	})
	if err != nil || len(logs.Entries) != 1 || logs.Entries[0].PullEndpointID != "pull_1" || logs.Entries[0].FetchedAt == nil {
		t.Fatalf("GetPullLogs failed: %#v %v", logs, err)
	}
	if logs.Entries[0].Timing == nil || logs.Entries[0].Timing.IngestProcessingMs == nil || *logs.Entries[0].Timing.IngestProcessingMs != 12 {
		t.Fatalf("GetPullLogs timing failed: %#v", logs.Entries[0].Timing)
	}

	metrics, err := client.GetPullMetrics(ctx, hookbridge.Window24Hour, &created.ID)
	if err != nil || metrics.TotalMessages != 10 {
		t.Fatalf("GetPullMetrics failed: %#v %v", metrics, err)
	}

	timeseries, err := client.GetPullTimeSeriesMetrics(ctx, hookbridge.Window24Hour, &created.ID)
	if err != nil || len(timeseries.Buckets) != 1 || timeseries.Buckets[0].Total != 10 || timeseries.Buckets[0].Fetched != 2 {
		t.Fatalf("GetPullTimeSeriesMetrics failed: %#v %v", timeseries, err)
	}
}

func TestSpecParityBillingAndExports(t *testing.T) {
	start := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 6, 23, 59, 59, 0, time.UTC)
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/billing/usage-history"):
			if r.URL.Query().Get("limit") != "10" || r.URL.Query().Get("offset") != "20" {
				t.Fatalf("unexpected usage history query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"period_start":"2026-02-01","period_end":"2026-02-28","message_count":6102,"overage_count":1102,"plan_limit":5000}],"meta":{"request_id":"req_4","total":1,"limit":10,"offset":20,"has_more":false}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exports":
			body := readJSONBody(t, r)
			if body["start_time"] != start.Format(time.RFC3339) || body["end_time"] != end.Format(time.RFC3339) {
				t.Fatalf("unexpected export payload %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"exp_1","project_id":"proj_1","status":"pending","filter_start_time":"2025-12-01T00:00:00Z","filter_end_time":"2025-12-06T23:59:59Z","created_at":"2025-12-06T12:00:00Z"},"meta":{"request_id":"req_5"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/exports/exp_1/download":
			w.Header().Set("Location", "https://downloads.example.com/export.csv")
			w.WriteHeader(http.StatusFound)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()

	limit := 10
	offset := 20
	usage, err := client.GetUsageHistory(ctx, &limit, &offset)
	if err != nil || usage.Total != 1 || len(usage.Rows) != 1 {
		t.Fatalf("GetUsageHistory failed: %#v %v", usage, err)
	}

	exportRecord, err := client.CreateExport(ctx, hookbridge.CreateExportRequest{StartTime: start, EndTime: end})
	if err != nil || exportRecord.ID != "exp_1" {
		t.Fatalf("CreateExport failed: %#v %v", exportRecord, err)
	}
	if !exportRecord.FilterStartTime.Equal(start) || !exportRecord.FilterEndTime.Equal(end) {
		t.Fatalf("CreateExport should preserve serialized times: %#v", exportRecord)
	}

	download, err := client.DownloadExport(ctx, "exp_1")
	if err != nil || download.URL != "https://downloads.example.com/export.csv" {
		t.Fatalf("DownloadExport failed: %#v %v", download, err)
	}
}

func TestSpecParityReplayMetricsAndInbound(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/messages/replay-all"):
			if r.URL.Query().Get("status") != "failed_permanent" || r.URL.Query().Get("limit") != "3" {
				t.Fatalf("unexpected replay-all query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"message":"3 messages queued for replay","data":{"replayed":3,"failed":0,"stuck":0,"replayed_message_ids":["m1","m2","m3"]},"meta":{"request_id":"req_1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/replay-batch":
			body := readJSONBody(t, r)
			if len(body["message_ids"].([]any)) != 2 {
				t.Fatalf("unexpected replay-batch body %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"message":"2 messages replayed","data":{"replayed":2,"failed":0,"stuck":0,"results":[{"message_id":"m1","status":"replayed"},{"message_id":"m2","status":"replayed"}]},"meta":{"request_id":"req_2"}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/metrics/timeseries"):
			if r.URL.Query().Get("window") != "24h" || r.URL.Query().Get("endpoint_id") != "ep_1" {
				t.Fatalf("unexpected timeseries query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"window":"24h","buckets":[{"timestamp":"2025-12-06T00:00:00Z","succeeded":10,"failed":1,"retrying":2,"total":13,"avg_latency_ms":180}]},"meta":{"request_id":"req_3"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-endpoints":
			body := readJSONBody(t, r)
			if body["url"] != "https://example.com/inbound" {
				t.Fatalf("unexpected inbound create body %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"in_1","name":"Stripe","url":"https://example.com/inbound","ingest_url":"https://receive.hookbridge.io/v1/webhooks/receive/in_1/token","secret_token":"token","created_at":"2025-12-06T12:00:00Z"},"meta":{"request_id":"req_4"}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/inbound-endpoints/in_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"in_1","updated":true},"meta":{"request_id":"req_5"}}`)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/inbound-messages/replay-all"):
			if r.URL.Query().Get("inbound_endpoint_id") != "in_1" {
				t.Fatalf("unexpected inbound replay-all query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"message":"1 inbound messages queued for replay","data":{"replayed":1,"failed":0,"stuck":0,"replayed_message_ids":["inm_1"]},"meta":{"request_id":"req_6"}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/inbound-logs"):
			if r.URL.Query().Get("status") != "succeeded" || r.URL.Query().Get("inbound_endpoint_id") != "in_1" {
				t.Fatalf("unexpected inbound logs query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"message_id":"inm_1","inbound_endpoint_id":"in_1","endpoint":"https://example.com/inbound","status":"succeeded","attempt_count":1,"received_at":"2025-12-06T12:00:00Z","delivered_at":"2025-12-06T12:00:05Z","response_status":200,"response_latency_ms":120,"total_delivery_ms":5000}],"meta":{"request_id":"req_7","has_more":false}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/inbound-metrics/timeseries"):
			if r.URL.Query().Get("inbound_endpoint_id") != "in_1" {
				t.Fatalf("unexpected inbound metrics query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"window":"24h","buckets":[{"timestamp":"2025-12-06T00:00:00Z","succeeded":20,"failed":2,"retrying":1,"total":23,"avg_latency_ms":150}]},"meta":{"request_id":"req_8"}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/inbound-rejections"):
			if r.URL.Query().Get("inbound_endpoint_id") != "in_1" {
				t.Fatalf("unexpected inbound rejections query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"rej_1","reason_code":"hmac_failed","received_at":"2025-12-06T12:00:00Z","inbound_endpoint_id":"in_1","reason_detail":"HMAC signature mismatch","source_ip":"203.0.113.42"}],"meta":{"request_id":"req_9","has_more":false}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()
	limit := 3
	endpointID := "ep_1"
	replayedAll, err := client.ReplayAll(ctx, hookbridge.ReplayAllMessagesRequest{
		Status:     hookbridge.StatusFailedPermanent,
		EndpointID: &endpointID,
		Limit:      &limit,
	})
	if err != nil || replayedAll.Data.Replayed != 3 {
		t.Fatalf("ReplayAll failed: %#v %v", replayedAll, err)
	}

	batch, err := client.ReplayBatch(ctx, []string{"m1", "m2"})
	if err != nil || batch.Data.Replayed != 2 {
		t.Fatalf("ReplayBatch failed: %#v %v", batch, err)
	}

	metrics, err := client.GetTimeSeriesMetrics(ctx, hookbridge.Window24Hour, &endpointID)
	if err != nil || len(metrics.Buckets) != 1 {
		t.Fatalf("GetTimeSeriesMetrics failed: %#v %v", metrics, err)
	}

	name := "Stripe"
	inboundURL := "https://example.com/inbound"
	inbound, err := client.CreateInboundEndpoint(ctx, hookbridge.CreateInboundEndpointRequest{
		Name: &name,
		URL:  &inboundURL,
	})
	if err != nil || inbound.ID != "in_1" {
		t.Fatalf("CreateInboundEndpoint failed: %#v %v", inbound, err)
	}

	updatedName := "Stripe Updated"
	updatedInbound, err := client.UpdateInboundEndpoint(ctx, "in_1", hookbridge.UpdateInboundEndpointRequest{Name: &updatedName})
	if err != nil || !updatedInbound.Updated {
		t.Fatalf("UpdateInboundEndpoint failed: %#v %v", updatedInbound, err)
	}

	inboundID := "in_1"
	inboundReplay, err := client.ReplayAllInboundMessages(ctx, hookbridge.InboundReplayAllRequest{
		Status:            hookbridge.StatusFailedPermanent,
		InboundEndpointID: &inboundID,
	})
	if err != nil || inboundReplay.Data.Replayed != 1 {
		t.Fatalf("ReplayAllInboundMessages failed: %#v %v", inboundReplay, err)
	}

	status := hookbridge.StatusSucceeded
	logs, err := client.GetInboundLogs(ctx, &hookbridge.InboundLogsFilter{Status: &status, InboundEndpointID: &inboundID})
	if err != nil || len(logs.Logs) != 1 {
		t.Fatalf("GetInboundLogs failed: %#v %v", logs, err)
	}

	inboundTS, err := client.GetInboundTimeSeriesMetrics(ctx, hookbridge.Window24Hour, &inboundID)
	if err != nil || len(inboundTS.Buckets) != 1 {
		t.Fatalf("GetInboundTimeSeriesMetrics failed: %#v %v", inboundTS, err)
	}

	rejections, err := client.ListInboundRejections(ctx, &hookbridge.InboundRejectionsFilter{InboundEndpointID: &inboundID})
	if err != nil || len(rejections.Rejections) != 1 {
		t.Fatalf("ListInboundRejections failed: %#v %v", rejections, err)
	}
}

func TestSpecParityMessageControlsAndBilling(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/m_1/replay":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"message_id":"m_1","status":"queued"},"meta":{"request_id":"req_1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/m_1/retry-now":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"m_1","project_id":"proj_1","endpoint_id":"ep_1","status":"pending_retry","attempt_count":2,"replay_count":0,"content_type":"application/json","size_bytes":128,"payload_sha256":"abc123","created_at":"2025-12-06T12:00:00Z","updated_at":"2025-12-06T12:00:01Z"},"meta":{"request_id":"req_2"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/m_1/cancel":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"m_1","project_id":"proj_1","endpoint_id":"ep_1","status":"failed_permanent","attempt_count":2,"replay_count":0,"content_type":"application/json","size_bytes":128,"payload_sha256":"abc123","created_at":"2025-12-06T12:00:00Z","updated_at":"2025-12-06T12:00:02Z"},"meta":{"request_id":"req_3"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/dlq/replay/m_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"message_id":"m_1","status":"queued"},"meta":{"request_id":"req_4"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/endpoints/ep_1/signing-keys":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"sk_1","signing_secret":"whsec_new","key_hint":"abcd","created_at":"2025-12-06T12:10:00Z"},"meta":{"request_id":"req_7"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/billing/subscription":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"plan":"starter","status":"active","limits":{"plan":"starter","messages_per_month":5000,"max_projects":3,"max_endpoints":25,"retention_days":30},"usage":{"messages_used":123,"period_start":"2026-02-01T00:00:00Z","period_end":"2026-02-28T23:59:59Z"},"cancel_at_period_end":false,"current_period_end":"2026-03-01T00:00:00Z"},"meta":{"request_id":"req_9"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/billing/invoices":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"in_1","status":"paid","amount_due":1000,"amount_paid":1000,"currency":"usd","period_start":"2026-02-05T00:00:00Z","period_end":"2026-03-05T00:00:00Z","created":"2026-03-05T06:00:00Z","lines":[{"description":"Starter Plan","amount":1000,"quantity":1}]}],"meta":{"request_id":"req_10","has_more":false}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()

	replayed, err := client.Replay(ctx, "m_1")
	if err != nil || replayed.Status != hookbridge.StatusQueued {
		t.Fatalf("Replay failed: %#v %v", replayed, err)
	}

	retried, err := client.RetryNow(ctx, "m_1")
	if err != nil || retried.Status != hookbridge.StatusPendingRetry {
		t.Fatalf("RetryNow failed: %#v %v", retried, err)
	}

	canceled, err := client.CancelRetry(ctx, "m_1")
	if err != nil || canceled.Status != hookbridge.StatusFailedPermanent {
		t.Fatalf("CancelRetry failed: %#v %v", canceled, err)
	}

	fromDLQ, err := client.ReplayFromDLQ(ctx, "m_1")
	if err != nil || fromDLQ.MessageID != "m_1" {
		t.Fatalf("ReplayFromDLQ failed: %#v %v", fromDLQ, err)
	}

	signingKey, err := client.CreateEndpointSigningKey(ctx, "ep_1")
	if err != nil || signingKey.SigningSecret != "whsec_new" {
		t.Fatalf("CreateEndpointSigningKey failed: %#v %v", signingKey, err)
	}

	subscription, err := client.GetSubscription(ctx)
	if err != nil || subscription.Plan != "starter" || subscription.Usage.MessagesUsed != 123 {
		t.Fatalf("GetSubscription failed: %#v %v", subscription, err)
	}

	invoices, err := client.GetInvoices(ctx)
	if err != nil || len(invoices.Invoices) != 1 || invoices.Invoices[0].Lines[0].Quantity != 1 {
		t.Fatalf("GetInvoices failed: %#v %v", invoices, err)
	}
}

func TestSpecParityInboundManagementAndExports(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/inbound-endpoints":
			if r.URL.Query().Get("limit") != "10" {
				t.Fatalf("unexpected inbound endpoint query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"in_1","name":"Stripe","url":"https://example.com/inbound","active":true,"paused":false,"created_at":"2025-12-06T12:00:00Z"}],"meta":{"request_id":"req_1","has_more":false}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/inbound-endpoints/in_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"in_1","name":"Stripe","description":"Receives Stripe events","url":"https://example.com/inbound","active":true,"paused":false,"verify_static_token":false,"verify_hmac":true,"verify_ip_allowlist":false,"ingest_response_code":202,"idempotency_header_names":["stripe-signature"],"signing_enabled":true,"created_at":"2025-12-06T12:00:00Z","updated_at":"2025-12-06T12:05:00Z"},"meta":{"request_id":"req_2"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-endpoints/in_1/pause":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"in_1","paused":true},"meta":{"request_id":"req_3"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-endpoints/in_1/resume":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"in_1","paused":false},"meta":{"request_id":"req_4"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-endpoints/in_1/signing-keys":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"data":{"id":"sk_inbound_1","signing_secret":"whsec_inbound123456789","key_hint":"6789"},"meta":{"request_id":"req_sk1"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/inbound-endpoints/in_1/signing-keys":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"sk_inbound_1","key_hint":"6789","created_at":"2025-12-06T12:10:00Z"},{"id":"sk_inbound_2","key_hint":"abcd","created_at":"2025-12-06T12:00:00Z"}],"meta":{"request_id":"req_sk2"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/inbound-endpoints/in_1/signing-keys/sk_inbound_1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/inbound-endpoints/in_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"in_1","deleted":true},"meta":{"request_id":"req_5"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-messages/inm_1/replay":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"message":"1 inbound message queued for replay","data":{"replayed":1,"failed":0,"stuck":0,"results":[{"message_id":"inm_1","status":"replayed"}]}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-messages/replay-batch":
			body := readJSONBody(t, r)
			if len(body["message_ids"].([]any)) != 2 {
				t.Fatalf("unexpected replay batch body %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"message":"2 inbound messages replayed","data":{"replayed":1,"failed":1,"stuck":0,"results":[{"message_id":"inm_1","status":"replayed"},{"message_id":"inm_2","status":"failed","error":"missing"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/inbound-metrics":
			if r.URL.Query().Get("window") != "24h" || r.URL.Query().Get("inbound_endpoint_id") != "in_1" {
				t.Fatalf("unexpected inbound metrics query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"window":"24h","total_messages":5000,"succeeded":4900,"failed":20,"retries":80,"success_rate":0.98,"avg_latency_ms":150,"avg_delivery_time_ms":3200},"meta":{"request_id":"req_8"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/exports":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"exp_1","project_id":"proj_1","status":"completed","filter_start_time":"2025-12-01T00:00:00Z","filter_end_time":"2025-12-06T23:59:59Z","row_count":125,"created_at":"2025-12-06T12:00:00Z"}],"meta":{"request_id":"req_9"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/exports/exp_1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"exp_1","project_id":"proj_1","status":"completed","filter_start_time":"2025-12-01T00:00:00Z","filter_end_time":"2025-12-06T23:59:59Z","file_size_bytes":2048,"created_at":"2025-12-06T12:00:00Z"},"meta":{"request_id":"req_10"}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()
	limit := 10
	listed, err := client.ListInboundEndpoints(ctx, &hookbridge.InboundEndpointsFilter{Limit: &limit})
	if err != nil || len(listed.Endpoints) != 1 || listed.Endpoints[0].ID != "in_1" {
		t.Fatalf("ListInboundEndpoints failed: %#v %v", listed, err)
	}

	inbound, err := client.GetInboundEndpoint(ctx, "in_1")
	if err != nil || !inbound.VerifyHMAC {
		t.Fatalf("GetInboundEndpoint failed: %#v %v", inbound, err)
	}

	paused, err := client.PauseInboundEndpoint(ctx, "in_1")
	if err != nil || !paused.Paused {
		t.Fatalf("PauseInboundEndpoint failed: %#v %v", paused, err)
	}

	resumed, err := client.ResumeInboundEndpoint(ctx, "in_1")
	if err != nil || resumed.Paused {
		t.Fatalf("ResumeInboundEndpoint failed: %#v %v", resumed, err)
	}

	signingKey, err := client.CreateInboundSigningKey(ctx, "in_1")
	if err != nil || signingKey.ID != "sk_inbound_1" || signingKey.SigningSecret != "whsec_inbound123456789" {
		t.Fatalf("CreateInboundSigningKey failed: %#v %v", signingKey, err)
	}

	signingKeys, err := client.ListInboundSigningKeys(ctx, "in_1")
	if err != nil || len(signingKeys) != 2 || signingKeys[0].ID != "sk_inbound_1" {
		t.Fatalf("ListInboundSigningKeys failed: %#v %v", signingKeys, err)
	}

	if err := client.DeleteInboundSigningKey(ctx, "in_1", "sk_inbound_1"); err != nil {
		t.Fatalf("DeleteInboundSigningKey failed: %v", err)
	}

	deleted, err := client.DeleteInboundEndpoint(ctx, "in_1")
	if err != nil || !deleted.Deleted {
		t.Fatalf("DeleteInboundEndpoint failed: %#v %v", deleted, err)
	}

	replayed, err := client.ReplayInboundMessage(ctx, "inm_1")
	if err != nil || replayed.Data.Replayed != 1 {
		t.Fatalf("ReplayInboundMessage failed: %#v %v", replayed, err)
	}

	replayBatch, err := client.ReplayBatchInboundMessages(ctx, []string{"inm_1", "inm_2"})
	if err != nil || replayBatch.Data.Results[1].Error == nil || *replayBatch.Data.Results[1].Error != "missing" {
		t.Fatalf("ReplayBatchInboundMessages failed: %#v %v", replayBatch, err)
	}

	inboundID := "in_1"
	metrics, err := client.GetInboundMetrics(ctx, hookbridge.Window24Hour, &inboundID)
	if err != nil || metrics.AvgDeliveryTimeMs != 3200 {
		t.Fatalf("GetInboundMetrics failed: %#v %v", metrics, err)
	}

	exports, err := client.ListExports(ctx)
	if err != nil || len(exports) != 1 || exports[0].RowCount == nil || *exports[0].RowCount != 125 {
		t.Fatalf("ListExports failed: %#v %v", exports, err)
	}

	exportRecord, err := client.GetExport(ctx, "exp_1")
	if err != nil || exportRecord.FileSizeBytes == nil || *exportRecord.FileSizeBytes != 2048 {
		t.Fatalf("GetExport failed: %#v %v", exportRecord, err)
	}
}

func TestSpecParityDeleteMessagesAndActors(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/messages/m_1":
			io.WriteString(w, `{"data":{"message_id":"m_1","deleted_at":"2026-04-05T14:23:11.123Z","already_deleted":false},"meta":{"request_id":"req_1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/delete-batch":
			body := readJSONBody(t, r)
			ids, _ := body["message_ids"].([]any)
			if len(ids) != 2 {
				t.Fatalf("expected 2 ids, got %#v", body)
			}
			io.WriteString(w, `{"data":{"results":[{"message_id":"m_1","outcome":"deleted","deleted_at":"2026-04-05T14:23:11.123Z"},{"message_id":"m_2","outcome":"not_found"}],"deleted_count":1,"already_deleted_count":0,"not_found_count":1},"meta":{"request_id":"req_2"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/delete-all":
			if r.URL.Query().Get("status") != "succeeded" || r.URL.Query().Get("limit") != "500" {
				t.Fatalf("unexpected delete-all query: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"data":{"deleted":2,"deleted_message_ids":["m_1","m_2"]},"meta":{"request_id":"req_3"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/inbound-messages/inm_1":
			io.WriteString(w, `{"data":{"message_id":"inm_1","deleted_at":"2026-04-05T14:23:11.123Z","already_deleted":true},"meta":{"request_id":"req_4"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-messages/delete-batch":
			io.WriteString(w, `{"data":{"results":[{"message_id":"inm_1","outcome":"deleted","deleted_at":"2026-04-05T14:23:11.123Z"}],"deleted_count":1,"already_deleted_count":0,"not_found_count":0},"meta":{"request_id":"req_5"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/inbound-messages/delete-all":
			if r.URL.Query().Get("inbound_endpoint_id") != "in_1" {
				t.Fatalf("unexpected inbound delete-all query: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"data":{"deleted":0,"deleted_message_ids":[]},"meta":{"request_id":"req_6"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/pull-endpoints/pe_1/events/ev_1":
			io.WriteString(w, `{"data":{"event_id":"ev_1","deleted_at":"2026-04-05T14:23:11.123Z","already_deleted":false},"meta":{"request_id":"req_7"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pull-endpoints/pe_1/events/delete-batch":
			io.WriteString(w, `{"data":{"results":[{"event_id":"ev_1","outcome":"deleted","deleted_at":"2026-04-05T14:23:11.123Z"}],"deleted_count":1,"already_deleted_count":0,"not_found_count":0},"meta":{"request_id":"req_8"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pull-endpoints/pe_1/events/delete-all":
			if r.URL.Query().Get("event_type") != "user.created" {
				t.Fatalf("unexpected pull delete-all query: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"data":{"deleted":3,"deleted_event_ids":["ev_1","ev_2","ev_3"]},"meta":{"request_id":"req_9"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/actors/lookup":
			if r.URL.Query().Get("user_id") != "user_1,user_2" || r.URL.Query().Get("api_key_id") != "key_1" {
				t.Fatalf("unexpected actor lookup query: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"data":{"users":{"user_1":{"email":"alice@example.com"},"user_2":{"email":"bob@example.com"}},"api_keys":{"key_1":{"label":"Production"}}},"meta":{"request_id":"req_10"}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})

	ctx := context.Background()

	del, err := client.DeleteMessage(ctx, "m_1")
	if err != nil || del.MessageID != "m_1" || del.AlreadyDeleted {
		t.Fatalf("DeleteMessage failed: %#v %v", del, err)
	}

	batch, err := client.DeleteMessagesBatch(ctx, []string{"m_1", "m_2"})
	if err != nil || batch.DeletedCount != 1 || batch.NotFoundCount != 1 || batch.Results[0].Outcome != hookbridge.DeleteOutcomeDeleted {
		t.Fatalf("DeleteMessagesBatch failed: %#v %v", batch, err)
	}

	status := "succeeded"
	limit := 500
	all, err := client.DeleteMessagesAll(ctx, hookbridge.DeleteMessagesAllRequest{Status: &status, Limit: &limit})
	if err != nil || all.Deleted != 2 || len(all.DeletedMessageIDs) != 2 {
		t.Fatalf("DeleteMessagesAll failed: %#v %v", all, err)
	}

	inDel, err := client.DeleteInboundMessage(ctx, "inm_1")
	if err != nil || !inDel.AlreadyDeleted {
		t.Fatalf("DeleteInboundMessage failed: %#v %v", inDel, err)
	}

	inBatch, err := client.DeleteInboundMessagesBatch(ctx, []string{"inm_1"})
	if err != nil || inBatch.DeletedCount != 1 {
		t.Fatalf("DeleteInboundMessagesBatch failed: %#v %v", inBatch, err)
	}

	inboundID := "in_1"
	inAll, err := client.DeleteInboundMessagesAll(ctx, hookbridge.DeleteInboundMessagesAllRequest{InboundEndpointID: &inboundID})
	if err != nil || inAll.Deleted != 0 {
		t.Fatalf("DeleteInboundMessagesAll failed: %#v %v", inAll, err)
	}

	pDel, err := client.DeletePullEvent(ctx, "pe_1", "ev_1")
	if err != nil || pDel.EventID != "ev_1" {
		t.Fatalf("DeletePullEvent failed: %#v %v", pDel, err)
	}

	pBatch, err := client.DeletePullEventsBatch(ctx, "pe_1", []string{"ev_1"})
	if err != nil || pBatch.DeletedCount != 1 {
		t.Fatalf("DeletePullEventsBatch failed: %#v %v", pBatch, err)
	}

	eventType := "user.created"
	pAll, err := client.DeletePullEventsAll(ctx, "pe_1", hookbridge.DeletePullEventsAllRequest{EventType: &eventType})
	if err != nil || pAll.Deleted != 3 || len(pAll.DeletedEventIDs) != 3 {
		t.Fatalf("DeletePullEventsAll failed: %#v %v", pAll, err)
	}

	actors, err := client.LookupActors(ctx, hookbridge.ActorLookupRequest{UserIDs: []string{"user_1", "user_2"}, APIKeyIDs: []string{"key_1"}})
	if err != nil || actors.Users["user_1"].Email != "alice@example.com" || actors.APIKeys["key_1"].Label != "Production" {
		t.Fatalf("LookupActors failed: %#v %v", actors, err)
	}
}
