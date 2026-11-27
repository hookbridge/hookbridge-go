package hookbridge_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hookbridge/hookbridge-go/v2"
)

func TestMessageEventInfo(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"id":"m1","event_type":"order.created","event_id":"6f1c2a7e-0000-4000-8000-000000000001","is_test":true},"meta":{"request_id":"r"}}`)
	})
	m, err := client.GetMessage(context.Background(), "m1")
	if err != nil || m.EventType == nil || *m.EventType != "order.created" || m.EventID == nil || *m.EventID != "6f1c2a7e-0000-4000-8000-000000000001" || !m.IsTest {
		t.Fatalf("GetMessage: %#v %v", m, err)
	}
}

func TestMessageEventInfoAbsent(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"id":"m1"},"meta":{"request_id":"r"}}`)
	})
	m, err := client.GetMessage(context.Background(), "m1")
	if err != nil || m.EventType != nil || m.EventID != nil || m.IsTest {
		t.Fatalf("GetMessage: %#v %v", m, err)
	}
}

func TestLogsEventInfoAndEndpointFilter(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/logs" || r.URL.Query().Get("endpoint_id") != "ep_abc" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `{"data":[{"message_id":"m1","event_type":"order.created","event_id":"e1","is_test":true},{"message_id":"m2"}],"meta":{"request_id":"r"}}`)
	})
	resp, err := client.GetLogs(context.Background(), &hookbridge.LogsFilter{EndpointID: ptr("ep_abc")})
	if err != nil || len(resp.Messages) != 2 {
		t.Fatalf("GetLogs: %#v %v", resp, err)
	}
	a, b := resp.Messages[0], resp.Messages[1]
	if a.EventType == nil || *a.EventType != "order.created" || a.EventID == nil || *a.EventID != "e1" || !a.IsTest {
		t.Fatalf("first: %#v", a)
	}
	if b.EventType != nil || b.EventID != nil || b.IsTest {
		t.Fatalf("second: %#v", b)
	}
}

func TestLogsNoEndpointFilterOmitsParam(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["endpoint_id"]; ok {
			t.Errorf("endpoint_id must be omitted: %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `{"data":[],"meta":{"request_id":"r"}}`)
	})
	if _, err := client.GetLogs(context.Background(), &hookbridge.LogsFilter{}); err != nil {
		t.Fatal(err)
	}
}

func TestEventStatusCounts(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/events" {
			_, _ = io.WriteString(w, `{"data":[{"id":"ev_1","status_counts":{"delivered":2,"deleted":1}},{"id":"ev_2","status_counts":{}},{"id":"ev_3"}],"meta":{"request_id":"r"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"ev_1","status_counts":{"delivered":2,"failed":1},"messages":[]},"meta":{"request_id":"r"}}`)
	})
	list, err := client.ListEvents(context.Background(), nil)
	if err != nil || list.Events[0].StatusCounts["delivered"] != 2 || list.Events[0].StatusCounts["deleted"] != 1 ||
		list.Events[1].StatusCounts == nil || len(list.Events[1].StatusCounts) != 0 || list.Events[2].StatusCounts != nil {
		t.Fatalf("ListEvents: %#v %v", list, err)
	}
	e, err := client.GetEvent(context.Background(), "ev_1", nil)
	if err != nil || e.StatusCounts["delivered"] != 2 || e.StatusCounts["failed"] != 1 {
		t.Fatalf("GetEvent: %#v %v", e, err)
	}
}
