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

	"github.com/hookbridge/hookbridge-go"
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

func TestSpecParityProjectsAndSigningKeys(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"proj_1","tenant_id":"tenant_1","name":"Main","status":"active","rate_limit_default":1000,"created_at":"2025-12-01T10:00:00Z"}],"meta":{"request_id":"req_1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
			body := readJSONBody(t, r)
			if body["name"] != "New Project" {
				t.Fatalf("expected project name in request, got %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"proj_2","tenant_id":"tenant_1","name":"New Project","status":"active","rate_limit_default":500,"created_at":"2025-12-01T10:00:00Z"},"meta":{"request_id":"req_2"}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/projects/proj_2":
			body := readJSONBody(t, r)
			if body["name"] != "Renamed" {
				t.Fatalf("expected update payload, got %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"id":"proj_2","tenant_id":"tenant_1","name":"Renamed","status":"active","rate_limit_default":500,"created_at":"2025-12-01T10:00:00Z"},"meta":{"request_id":"req_3"}}`)
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

	projects, err := client.ListProjects(ctx)
	if err != nil || len(projects) != 1 || projects[0].ID != "proj_1" {
		t.Fatalf("ListProjects failed: %#v %v", projects, err)
	}

	limit := 500
	project, err := client.CreateProject(ctx, hookbridge.CreateProjectRequest{Name: "New Project", RateLimitDefault: &limit})
	if err != nil || project.Name != "New Project" {
		t.Fatalf("CreateProject failed: %#v %v", project, err)
	}

	renamed := "Renamed"
	updated, err := client.UpdateProject(ctx, "proj_2", hookbridge.UpdateProjectRequest{Name: &renamed})
	if err != nil || updated.Name != "Renamed" {
		t.Fatalf("UpdateProject failed: %#v %v", updated, err)
	}

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

func TestSpecParityBillingAndExports(t *testing.T) {
	start := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 6, 23, 59, 59, 0, time.UTC)
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/billing/checkout":
			body := readJSONBody(t, r)
			if body["plan"] != "pro" || body["interval"] != "monthly" {
				t.Fatalf("unexpected checkout payload %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":{"session_id":"cs_123","checkout_url":"https://checkout.stripe.com/c/pay/cs_123"},"meta":{"request_id":"req_3"}}`)
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

	checkout, err := client.CreateCheckout(ctx, hookbridge.CreateCheckoutRequest{Plan: "pro", Interval: "monthly"})
	if err != nil || checkout.SessionID != "cs_123" {
		t.Fatalf("CreateCheckout failed: %#v %v", checkout, err)
	}

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
	inbound, err := client.CreateInboundEndpoint(ctx, hookbridge.CreateInboundEndpointRequest{
		Name: &name,
		URL:  "https://example.com/inbound",
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
