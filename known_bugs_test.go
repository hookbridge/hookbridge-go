package hookbridge_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/hookbridge/hookbridge-go/v2"
)

func TestReplayInboundMessageDecodesServerShape(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/inbound-messages/msg_1/replay" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"message":"Inbound message queued for replay","data":{"message_id":"msg_1","status":"queued"},"meta":{"request_id":"req_1"}}`)
	})

	resp, err := client.ReplayInboundMessage(context.Background(), "msg_1")
	if err != nil {
		t.Fatalf("ReplayInboundMessage: %v", err)
	}
	if resp.Message != "Inbound message queued for replay" {
		t.Errorf("Message = %q", resp.Message)
	}
	if resp.Data.Replayed != 1 || len(resp.Data.Results) != 1 {
		t.Fatalf("Data = %+v", resp.Data)
	}
	if got := resp.Data.Results[0]; got.MessageID != "msg_1" || got.Status != "queued" {
		t.Errorf("Results[0] = %+v", got)
	}
}

func TestRetryNowAndCancelRetryDecodeServerShape(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/messages/msg_1/retry-now":
			io.WriteString(w, `{"message":"Retry scheduled for immediate pickup","data":{"message_id":"msg_1","status":"pending_retry"},"meta":{"request_id":"req_1"}}`)
		case "/v1/messages/msg_1/cancel":
			io.WriteString(w, `{"data":{"message_id":"msg_1","status":"failed_permanent"},"meta":{"request_id":"req_2"}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	got, err := client.RetryNow(context.Background(), "msg_1")
	if err != nil {
		t.Fatalf("RetryNow: %v", err)
	}
	if got.ID != "msg_1" || got.Status != hookbridge.MessageStatus("pending_retry") {
		t.Errorf("RetryNow = id %q status %q", got.ID, got.Status)
	}

	got, err = client.CancelRetry(context.Background(), "msg_1")
	if err != nil {
		t.Fatalf("CancelRetry: %v", err)
	}
	if got.ID != "msg_1" || got.Status != hookbridge.MessageStatus("failed_permanent") {
		t.Errorf("CancelRetry = id %q status %q", got.ID, got.Status)
	}
}

func TestGetEndpointMetricsSendsEndpointID(t *testing.T) {
	var gotQuery string
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		io.WriteString(w, `{"data":{"window":"24h","total_messages":5,"delivered":4,"failed":1},"meta":{"request_id":"req_1"}}`)
	})

	endpointID := "ep_1"
	m, err := client.GetEndpointMetrics(context.Background(), hookbridge.Window24Hour, &endpointID)
	if err != nil {
		t.Fatalf("GetEndpointMetrics: %v", err)
	}
	if gotQuery != "endpoint_id=ep_1&window=24h" {
		t.Errorf("query = %q", gotQuery)
	}
	if m.TotalMessages != 5 {
		t.Errorf("TotalMessages = %d", m.TotalMessages)
	}
}

func TestDLQMessageDecodesMessageSummary(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"messages":[{"message_id":"msg_1","endpoint":"https://example.com/hook","status":"failed_permanent","attempt_count":5,"created_at":"2026-01-02T03:04:05Z","delivered_at":null,"first_attempt_at":"2026-01-02T03:04:06Z","response_status":500,"response_latency_ms":120,"processing_ms":130,"total_delivery_ms":9000,"queue_wait_ms":12,"last_error":"HTTP 500","deleted_at":"2026-01-03T00:00:00Z","event_type":"order.created","event_id":"evt_1","is_test":true}],"has_more":true,"next_cursor":"c1"},"meta":{"request_id":"req_1"}}`)
	})

	resp, err := client.GetDLQMessages(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetDLQMessages: %v", err)
	}
	if !resp.HasMore || resp.NextCursor == nil || *resp.NextCursor != "c1" || len(resp.Messages) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	m := resp.Messages[0]
	if m.MessageID != "msg_1" || m.Endpoint != "https://example.com/hook" || m.AttemptCount != 5 || m.LastError != "HTTP 500" || m.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("existing fields = %+v", m)
	}
	if m.Status != hookbridge.MessageStatus("failed_permanent") {
		t.Errorf("Status = %q", m.Status)
	}
	if m.DeliveredAt != nil {
		t.Errorf("DeliveredAt = %v", m.DeliveredAt)
	}
	wantFirst := time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC)
	if m.FirstAttemptAt == nil || !m.FirstAttemptAt.Equal(wantFirst) {
		t.Errorf("FirstAttemptAt = %v", m.FirstAttemptAt)
	}
	if m.DeletedAt == nil || !m.DeletedAt.Equal(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("DeletedAt = %v", m.DeletedAt)
	}
	if m.ResponseStatus == nil || *m.ResponseStatus != 500 ||
		m.ResponseLatencyMs == nil || *m.ResponseLatencyMs != 120 ||
		m.ProcessingMs == nil || *m.ProcessingMs != 130 ||
		m.TotalDeliveryMs == nil || *m.TotalDeliveryMs != 9000 ||
		m.QueueWaitMs == nil || *m.QueueWaitMs != 12 {
		t.Errorf("numeric fields = %+v", m)
	}
	if m.EventType == nil || *m.EventType != "order.created" || m.EventID == nil || *m.EventID != "evt_1" || !m.IsTest {
		t.Errorf("event fields = %+v", m)
	}
}

func TestListInvoicesSendsPagination(t *testing.T) {
	var gotQuery string
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/billing/invoices" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		io.WriteString(w, `{"data":[],"meta":{"request_id":"req_1","has_more":true}}`)
	})

	limit := 5
	after := "in_1abc"
	resp, err := client.ListInvoices(context.Background(), &hookbridge.InvoicesFilter{Limit: &limit, StartingAfter: &after})
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	if gotQuery != "limit=5&starting_after=in_1abc" {
		t.Errorf("query = %q", gotQuery)
	}
	if !resp.HasMore {
		t.Error("HasMore = false")
	}

	if _, err := client.GetInvoices(context.Background()); err != nil {
		t.Fatalf("GetInvoices: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("GetInvoices query = %q", gotQuery)
	}
}
