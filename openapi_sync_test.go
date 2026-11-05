package hookbridge_test

import (
	"encoding/json"
	"testing"

	"github.com/hookbridge/hookbridge-go/v2"
)

func marshalMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func unmarshalInto(t *testing.T, raw string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestSyncRequestFields(t *testing.T) {
	m := marshalMap(t, hookbridge.SendRequest{EndpointID: "e", Payload: 1, RetentionHours: ptr(0)})
	if v, ok := m["retention_hours"]; !ok || v != float64(0) {
		t.Fatalf("send retention_hours: %#v", m)
	}

	m = marshalMap(t, hookbridge.CreateEndpointRequest{URL: "u", Ephemeral: ptr(true), TTLMinutes: ptr(30)})
	if m["ephemeral"] != true || m["ttl_minutes"] != float64(30) {
		t.Fatalf("create endpoint: %#v", m)
	}

	m = marshalMap(t, hookbridge.CreateInboundEndpointRequest{Ephemeral: ptr(true), TTLMinutes: ptr(5), HMACSignatureEncoding: ptr("base64")})
	if m["ephemeral"] != true || m["ttl_minutes"] != float64(5) || m["hmac_signature_encoding"] != "base64" {
		t.Fatalf("create inbound: %#v", m)
	}

	m = marshalMap(t, hookbridge.UpdateInboundEndpointRequest{HMACSignatureEncoding: ptr("hex")})
	if m["hmac_signature_encoding"] != "hex" {
		t.Fatalf("update inbound: %#v", m)
	}
	m = marshalMap(t, hookbridge.CreatePullEndpointRequest{HMACSignatureEncoding: ptr("hex")})
	if m["hmac_signature_encoding"] != "hex" {
		t.Fatalf("create pull: %#v", m)
	}
	m = marshalMap(t, hookbridge.UpdatePullEndpointRequest{HMACSignatureEncoding: ptr("base64")})
	if m["hmac_signature_encoding"] != "base64" {
		t.Fatalf("update pull: %#v", m)
	}

	m = marshalMap(t, hookbridge.CreateExportRequest{Direction: ptr("inbound"), InboundEndpointID: ptr("ib_1")})
	if m["direction"] != "inbound" || m["inbound_endpoint_id"] != "ib_1" {
		t.Fatalf("create export: %#v", m)
	}
}

func TestSyncResponseFields(t *testing.T) {
	var msg hookbridge.Message
	unmarshalInto(t, `{"id":"m","first_attempt_at":"2026-01-01T00:00:00Z","delivered_at":"2026-01-01T00:00:01Z","processing_ms":5,"total_delivery_ms":6,"queue_wait_ms":7,"retention_hours":72}`, &msg)
	if msg.FirstAttemptAt == nil || msg.DeliveredAt == nil || *msg.ProcessingMs != 5 || *msg.TotalDeliveryMs != 6 || *msg.QueueWaitMs != 7 || *msg.RetentionHours != 72 {
		t.Fatalf("message: %#v", msg)
	}

	var sum hookbridge.MessageSummary
	unmarshalInto(t, `{"message_id":"m","first_attempt_at":"2026-01-01T00:00:00Z","processing_ms":1,"total_delivery_ms":2,"queue_wait_ms":3}`, &sum)
	if sum.FirstAttemptAt == nil || *sum.ProcessingMs != 1 || *sum.TotalDeliveryMs != 2 || *sum.QueueWaitMs != 3 {
		t.Fatalf("summary: %#v", sum)
	}

	var in hookbridge.InboundMessage
	unmarshalInto(t, `{"id":"i","retention_hours":24}`, &in)
	if in.RetentionHours == nil || *in.RetentionHours != 24 {
		t.Fatalf("inbound message: %#v", in)
	}

	var met hookbridge.Metrics
	unmarshalInto(t, `{"avg_delivery_time_ms":10,"avg_queue_wait_ms":20}`, &met)
	if met.AvgDeliveryTimeMs != 10 || met.AvgQueueWaitMs != 20 {
		t.Fatalf("metrics: %#v", met)
	}

	var sub hookbridge.Subscription
	unmarshalInto(t, `{"limits":{"overage_rate_per_message":0.002},"usage":{"overage_count":3,"overage_rate":0.002,"estimated_overage_charge":0.006}}`, &sub)
	if sub.Limits.OverageRatePerMessage != 0.002 || sub.Usage.OverageCount != 3 || sub.Usage.OverageRate != 0.002 || sub.Usage.EstimatedOverageCharge != 0.006 {
		t.Fatalf("subscription: %#v", sub)
	}

	var rb hookbridge.ReplayResult
	unmarshalInto(t, `{"message_id":"m","status":"failed","error":"x","error_code":"RETENTION_EXPIRED"}`, &rb)
	if rb.ErrorCode == nil || *rb.ErrorCode != hookbridge.ReplayErrorRetentionExpired {
		t.Fatalf("replay result: %#v", rb)
	}
	if hookbridge.ReplayErrorMessageDeleted != "MESSAGE_DELETED" || hookbridge.ReplayErrorPayloadUnavailable != "PAYLOAD_UNAVAILABLE" {
		t.Fatal("replay error constants")
	}

	var ex hookbridge.ExportRecord
	unmarshalInto(t, `{"id":"e","filter_inbound_endpoint_id":"ib_1","direction":"inbound"}`, &ex)
	if ex.FilterInboundEndpointID == nil || *ex.FilterInboundEndpointID != "ib_1" || ex.Direction == nil || *ex.Direction != "inbound" {
		t.Fatalf("export record: %#v", ex)
	}
}

func TestSyncEndpointResponseFields(t *testing.T) {
	var ep hookbridge.Endpoint
	unmarshalInto(t, `{"id":"e","ephemeral":true,"expires_at":"2026-01-01T00:00:00Z"}`, &ep)
	if ep.Ephemeral == nil || !*ep.Ephemeral || ep.ExpiresAt == nil {
		t.Fatalf("endpoint: %#v", ep)
	}
	var es hookbridge.EndpointSummary
	unmarshalInto(t, `{"id":"e","ephemeral":true,"expires_at":"2026-01-01T00:00:00Z"}`, &es)
	if es.Ephemeral == nil || es.ExpiresAt == nil {
		t.Fatalf("endpoint summary: %#v", es)
	}
	var ce hookbridge.CreateEndpointResponse
	unmarshalInto(t, `{"id":"e","ephemeral":true,"expires_at":"2026-01-01T00:00:00Z"}`, &ce)
	if ce.Ephemeral == nil || ce.ExpiresAt == nil {
		t.Fatalf("create endpoint resp: %#v", ce)
	}
	var ib hookbridge.InboundEndpoint
	unmarshalInto(t, `{"id":"i","ephemeral":true,"expires_at":"2026-01-01T00:00:00Z","hmac_signature_encoding":"base64","ingest_url":"https://x"}`, &ib)
	if ib.Ephemeral == nil || ib.ExpiresAt == nil || ib.HMACSignatureEncoding == nil || *ib.HMACSignatureEncoding != "base64" || ib.IngestURL == nil {
		t.Fatalf("inbound endpoint: %#v", ib)
	}
	var is hookbridge.InboundEndpointSummary
	unmarshalInto(t, `{"id":"i","ephemeral":true,"expires_at":"2026-01-01T00:00:00Z"}`, &is)
	if is.Ephemeral == nil || is.ExpiresAt == nil {
		t.Fatalf("inbound summary: %#v", is)
	}
	var cib hookbridge.CreateInboundEndpointResponse
	unmarshalInto(t, `{"id":"i","ephemeral":true,"expires_at":"2026-01-01T00:00:00Z","hmac_signature_encoding":"hex"}`, &cib)
	if cib.Ephemeral == nil || cib.ExpiresAt == nil || cib.HMACSignatureEncoding == nil {
		t.Fatalf("create inbound resp: %#v", cib)
	}
	var pe hookbridge.PullEndpoint
	unmarshalInto(t, `{"id":"p","hmac_signature_encoding":"hex"}`, &pe)
	if pe.HMACSignatureEncoding == nil || *pe.HMACSignatureEncoding != "hex" {
		t.Fatalf("pull endpoint: %#v", pe)
	}
}

func TestSyncZeroValueRequestsOmitNewKeys(t *testing.T) {
	newKeys := []string{"retention_hours", "ephemeral", "ttl_minutes", "hmac_signature_encoding", "direction", "inbound_endpoint_id"}
	cases := map[string]any{
		"SendRequest":                  hookbridge.SendRequest{},
		"CreateEndpointRequest":        hookbridge.CreateEndpointRequest{},
		"CreateInboundEndpointRequest": hookbridge.CreateInboundEndpointRequest{},
		"UpdateInboundEndpointRequest": hookbridge.UpdateInboundEndpointRequest{},
		"CreatePullEndpointRequest":    hookbridge.CreatePullEndpointRequest{},
		"UpdatePullEndpointRequest":    hookbridge.UpdatePullEndpointRequest{},
		"CreateExportRequest":          hookbridge.CreateExportRequest{},
	}
	for name, v := range cases {
		m := marshalMap(t, v)
		for _, k := range newKeys {
			if _, ok := m[k]; ok {
				t.Errorf("%s zero value should not emit %q: %#v", name, k, m)
			}
		}
	}
}

func TestSyncDeleteErrorCodeDecode(t *testing.T) {
	var resp hookbridge.DeleteAllResponse
	unmarshalInto(t, `{"deleted":1,"deleted_message_ids":["a"],"error":{"code":"PARTIAL_DELETE_FAILURE","message":"x"}}`, &resp)
	if resp.Error == nil || resp.Error.Code != hookbridge.DeleteErrorPartialDeleteFailure {
		t.Fatalf("delete all: %#v", resp)
	}
	var pull hookbridge.DeletePullEventsAllResponse
	unmarshalInto(t, `{"deleted":0,"deleted_event_ids":[],"error":{"code":"DELETE_FAILED","message":"x"}}`, &pull)
	if pull.Error == nil || pull.Error.Code != hookbridge.DeleteErrorDeleteFailed {
		t.Fatalf("pull delete all: %#v", pull)
	}
}
