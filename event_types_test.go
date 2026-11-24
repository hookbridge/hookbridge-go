package hookbridge_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/hookbridge/hookbridge-go/v2"
)

func TestSendEvent(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/webhooks/send" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if body["event_type"] != "order.created" || body["idempotency_key"] != "k1" || body["retention_hours"] != float64(0) {
			t.Errorf("body: %#v", body)
		}
		if _, ok := body["endpoint_id"]; ok {
			t.Errorf("endpoint_id must be omitted: %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"data":{"event_id":"ev_1","event_type":"order.created","status":"accepted"},"meta":{"request_id":"r"}}`)
	})
	resp, err := client.SendEvent(context.Background(), hookbridge.SendEventRequest{
		EventType:      "order.created",
		Payload:        map[string]any{"a": 1},
		IdempotencyKey: "k1",
		RetentionHours: ptr(0),
	})
	if err != nil || resp.EventID != "ev_1" || resp.EventType != "order.created" || resp.Status != "accepted" || resp.MessageID != nil {
		t.Fatalf("SendEvent: %#v %v", resp, err)
	}
}

func TestSendEventWithEndpoint(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if body["endpoint_id"] != "ep_1" {
			t.Errorf("body: %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"data":{"event_id":"ev_1","event_type":"order.created","status":"accepted","message_id":"m_1"}}`)
	})
	resp, err := client.SendEvent(context.Background(), hookbridge.SendEventRequest{EventType: "order.created", EndpointID: "ep_1", Payload: 1})
	if err != nil || resp.MessageID == nil || *resp.MessageID != "m_1" {
		t.Fatalf("SendEvent: %#v %v", resp, err)
	}
}

const eventTypeBody = `{"id":"u1","name":"order.created","description":"d","group":"orders","schema":{"type":"object"},"sample_payload":{"a":1},"deprecated":false,"public":true,"auto_created":false,"archived_at":null,"created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-01T00:00:00Z"}`

func checkEventType(t *testing.T, et *hookbridge.EventType, err error) {
	t.Helper()
	if err != nil || et.Name != "order.created" || et.Group == nil || *et.Group != "orders" || et.Schema["type"] != "object" ||
		!et.Public || et.ArchivedAt != nil || et.CreatedAt.IsZero() || et.SamplePayload == nil {
		t.Fatalf("event type: %#v %v", et, err)
	}
}

func TestCreateEventType(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/event-types" || body["name"] != "order.created" || body["group"] != "orders" {
			t.Errorf("unexpected request %s %s %#v", r.Method, r.URL.Path, body)
		}
		if _, ok := body["schema"]; ok {
			t.Errorf("schema must be omitted: %#v", body)
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"data":`+eventTypeBody+`,"meta":{"request_id":"r"}}`)
	})
	et, err := client.CreateEventType(context.Background(), hookbridge.CreateEventTypeRequest{Name: "order.created", Group: ptr("orders")})
	checkEventType(t, et, err)
}

func TestCreateEventTypeExists(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"error":{"code":"EVENT_TYPE_EXISTS","message":"exists"},"meta":{"request_id":"r"}}`)
	})
	_, err := client.CreateEventType(context.Background(), hookbridge.CreateEventTypeRequest{Name: "x"})
	var apiErr *hookbridge.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "EVENT_TYPE_EXISTS" || apiErr.StatusCode != 409 {
		t.Fatalf("err: %#v", err)
	}
}

func TestListEventTypes(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/event-types" || q.Get("limit") != "10" || q.Get("cursor") != "c1" ||
			q.Get("group") != "orders" || q.Get("name_prefix") != "order." || q.Get("include_archived") != "true" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		io.WriteString(w, `{"data":[`+eventTypeBody+`],"meta":{"request_id":"r","next_cursor":"c2"}}`)
	})
	resp, err := client.ListEventTypes(context.Background(), &hookbridge.EventTypesFilter{
		Limit: ptr(10), Cursor: ptr("c1"), Group: ptr("orders"), NamePrefix: ptr("order."), IncludeArchived: ptr(true),
	})
	if err != nil || len(resp.EventTypes) != 1 || !resp.HasMore || resp.NextCursor == nil || *resp.NextCursor != "c2" {
		t.Fatalf("ListEventTypes: %#v %v", resp, err)
	}
	checkEventType(t, &resp.EventTypes[0], nil)
}

func TestListEventTypesLastPage(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[],"meta":{"request_id":"r","next_cursor":""}}`)
	})
	resp, err := client.ListEventTypes(context.Background(), nil)
	if err != nil || resp.HasMore || resp.NextCursor != nil {
		t.Fatalf("ListEventTypes: %#v %v", resp, err)
	}
}

func TestEventTypeNameEscaped(t *testing.T) {
	var calls []string
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		io.WriteString(w, `{"data":`+eventTypeBody+`}`)
	})
	ctx := context.Background()
	for _, call := range []func() (*hookbridge.EventType, error){
		func() (*hookbridge.EventType, error) { return client.GetEventType(ctx, "a.b/c*") },
		func() (*hookbridge.EventType, error) { return client.ArchiveEventType(ctx, "a.b/c*") },
		func() (*hookbridge.EventType, error) { return client.UnarchiveEventType(ctx, "a.b/c*") },
		func() (*hookbridge.EventType, error) {
			return client.UpdateEventType(ctx, "a.b/c*", hookbridge.UpdateEventTypeRequest{Description: ptr("x")})
		},
	} {
		et, err := call()
		checkEventType(t, et, err)
	}
	want := []string{
		"GET /v1/event-types/a.b%2Fc%2A",
		"DELETE /v1/event-types/a.b%2Fc%2A",
		"POST /v1/event-types/a.b%2Fc%2A/unarchive",
		"PATCH /v1/event-types/a.b%2Fc%2A",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls: %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

func TestUpdateEventTypeOmitNullValue(t *testing.T) {
	var bodies []map[string]any
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		bodies = append(bodies, readJSONBody(t, r))
		io.WriteString(w, `{"data":`+eventTypeBody+`}`)
	})
	ctx := context.Background()
	if _, err := client.UpdateEventType(ctx, "order.created", hookbridge.UpdateEventTypeRequest{Deprecated: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateEventType(ctx, "order.created", hookbridge.UpdateEventTypeRequest{
		Group:         hookbridge.Null[string](),
		Schema:        hookbridge.Null[map[string]any](),
		SamplePayload: hookbridge.Some[any](map[string]any{"a": 1}),
	}); err != nil {
		t.Fatal(err)
	}
	if len(bodies[0]) != 1 || bodies[0]["deprecated"] != false {
		t.Errorf("omitted body: %#v", bodies[0])
	}
	b := bodies[1]
	if v, ok := b["group"]; !ok || v != nil {
		t.Errorf("group must be null: %#v", b)
	}
	if v, ok := b["schema"]; !ok || v != nil {
		t.Errorf("schema must be null: %#v", b)
	}
	if sp, ok := b["sample_payload"].(map[string]any); !ok || sp["a"] != float64(1) {
		t.Errorf("sample_payload: %#v", b)
	}
	if _, ok := b["description"]; ok {
		t.Errorf("description must be omitted: %#v", b)
	}
}

func TestListEventTypeGroups(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/event-type-groups" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"data":[{"name":"orders","count":4}],"meta":{"request_id":"r"}}`)
	})
	groups, err := client.ListEventTypeGroups(context.Background())
	if err != nil || len(groups) != 1 || groups[0].Name != "orders" || groups[0].Count != 4 {
		t.Fatalf("ListEventTypeGroups: %#v %v", groups, err)
	}
}

const eventBody = `{"id":"ev_1","project_id":"p","event_type":"order.created","payload_sha256":"abc","content_type":"application/json","size_bytes":12,"fanout_status":"done","matched_count":2,"headers":{"X-A":"b"},"idempotency_key":"k","retention_hours":72,"target_endpoint_id":"t","fanned_out_at":"2026-06-01T00:00:01Z","created_at":"2026-06-01T00:00:00Z","payload_available":true}`

func TestListEvents(t *testing.T) {
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/events" || q.Get("limit") != "5" || q.Get("cursor") != "c1" ||
			q.Get("event_type") != "order.created" || q.Get("since") != "2026-06-01T00:00:00Z" || q.Get("until") != "2026-06-01T01:00:00Z" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		io.WriteString(w, `{"data":[`+eventBody+`],"meta":{"request_id":"r","has_more":true,"next_cursor":"c2"}}`)
	})
	resp, err := client.ListEvents(context.Background(), &hookbridge.EventsFilter{
		Limit: ptr(5), Cursor: ptr("c1"), EventType: ptr("order.created"), Since: &since, Until: &until,
	})
	if err != nil || len(resp.Events) != 1 || !resp.HasMore || resp.NextCursor == nil || *resp.NextCursor != "c2" {
		t.Fatalf("ListEvents: %#v %v", resp, err)
	}
	e := resp.Events[0]
	if e.ID != "ev_1" || e.FanoutStatus != "done" || e.MatchedCount != 2 || e.SizeBytes != 12 || e.RetentionHours == nil || *e.RetentionHours != 72 ||
		e.FannedOutAt == nil || e.DeletedAt != nil || !e.PayloadAvailable || e.Headers["X-A"] != "b" {
		t.Fatalf("event: %#v", e)
	}
}

func TestGetEvent(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/events/ev_1" || q.Get("messages_limit") != "1" || q.Get("messages_cursor") != "mc" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		io.WriteString(w, `{"data":`+eventBody[:len(eventBody)-1]+`,"payload_url":"https://x/p","messages":[{"id":"m1","endpoint_id":"e1","status":"delivered","attempt_count":1,"created_at":"2026-06-01T00:00:02Z"}],"messages_has_more":true,"messages_next_cursor":"mc2"}}`)
	})
	e, err := client.GetEvent(context.Background(), "ev_1", &hookbridge.GetEventParams{MessagesLimit: ptr(1), MessagesCursor: ptr("mc")})
	if err != nil || e.MatchedCount != 2 || e.PayloadURL == nil || len(e.Messages) != 1 || e.Messages[0].Status != "delivered" ||
		!e.MessagesHasMore || e.MessagesNextCursor == nil || *e.MessagesNextCursor != "mc2" {
		t.Fatalf("GetEvent: %#v %v", e, err)
	}
}

func TestGetEventNotFound(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":{"code":"EVENT_NOT_FOUND","message":"nope"}}`)
	})
	_, err := client.GetEvent(context.Background(), "ev_1", nil)
	var nf *hookbridge.NotFoundError
	if !errors.As(err, &nf) || nf.Code != "EVENT_NOT_FOUND" {
		t.Fatalf("err: %#v", err)
	}
}

func TestDeleteEvent(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/events/ev_1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := client.DeleteEvent(context.Background(), "ev_1"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
}

func TestEndpointSubscriptions(t *testing.T) {
	var bodies []map[string]any
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			bodies = append(bodies, readJSONBody(t, r))
		}
		switch {
		case r.Method == http.MethodPost:
			io.WriteString(w, `{"data":{"id":"ep_1","url":"https://x","signing_key_id":"k","signing_secret":"s","key_hint":"h","created_at":"2026-06-01T00:00:00Z","subscriptions":{"event_types":["a.b"],"groups":[]}}}`)
		case r.Method == http.MethodPatch:
			io.WriteString(w, `{"data":{"id":"ep_1","updated":true,"subscriptions":{"event_types":[],"groups":[]}}}`)
		default:
			io.WriteString(w, `{"data":{"id":"ep_1","url":"https://x","paused":false,"created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-01T00:00:00Z","subscriptions":{"event_types":["a.*"],"groups":["g"]}}}`)
		}
	})
	ctx := context.Background()
	created, err := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{URL: "https://x", Subscriptions: &hookbridge.EventSubscriptions{EventTypes: []string{"a.b"}}})
	if err != nil || created.Subscriptions == nil || created.Subscriptions.EventTypes[0] != "a.b" {
		t.Fatalf("create: %#v %v", created, err)
	}
	if _, err := client.UpdateEndpoint(ctx, "ep_1", hookbridge.UpdateEndpointRequest{Description: ptr("x")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateEndpoint(ctx, "ep_1", hookbridge.UpdateEndpointRequest{Subscriptions: &hookbridge.EventSubscriptions{}}); err != nil {
		t.Fatal(err)
	}
	got, err := client.GetEndpoint(ctx, "ep_1")
	if err != nil || got.Subscriptions == nil || got.Subscriptions.Groups[0] != "g" {
		t.Fatalf("get: %#v %v", got, err)
	}

	sub, _ := bodies[0]["subscriptions"].(map[string]any)
	if et, _ := sub["event_types"].([]any); len(et) != 1 || et[0] != "a.b" {
		t.Errorf("create subscriptions: %#v", bodies[0])
	}
	if groups, ok := sub["groups"].([]any); !ok || len(groups) != 0 {
		t.Errorf("create groups must be []: %#v", sub)
	}
	if _, ok := bodies[1]["subscriptions"]; ok {
		t.Errorf("unset subscriptions must be omitted: %#v", bodies[1])
	}
	sub, ok := bodies[2]["subscriptions"].(map[string]any)
	if !ok {
		t.Fatalf("explicit empty subscriptions must be sent: %#v", bodies[2])
	}
	if et, ok := sub["event_types"].([]any); !ok || len(et) != 0 {
		t.Errorf("event_types must be []: %#v", sub)
	}
	if groups, ok := sub["groups"].([]any); !ok || len(groups) != 0 {
		t.Errorf("groups must be []: %#v", sub)
	}
}

func TestListEndpointsEventTypeFilter(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("event_type") != "order.created" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		io.WriteString(w, `{"data":[{"id":"ep_1","url":"https://x","paused":false,"created_at":"2026-06-01T00:00:00Z","subscriptions":{"event_types":["order.*"],"groups":[]}}],"meta":{}}`)
	})
	resp, err := client.ListEndpoints(context.Background(), &hookbridge.EndpointsFilter{EventType: ptr("order.created")})
	if err != nil || len(resp.Endpoints) != 1 || resp.Endpoints[0].Subscriptions == nil {
		t.Fatalf("ListEndpoints: %#v %v", resp, err)
	}
}

func TestRecoverEndpoint(t *testing.T) {
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/endpoints/ep_1/recover" || body["since"] != "2026-06-01T00:00:00Z" {
			t.Errorf("unexpected request %s %s %#v", r.Method, r.URL.Path, body)
		}
		if _, ok := body["until"]; ok {
			t.Errorf("until must be omitted: %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"data":{"recover_job_id":"j1","status":"pending"},"meta":{"request_id":"r"}}`)
	})
	resp, err := client.RecoverEndpoint(context.Background(), "ep_1", hookbridge.RecoverRequest{Since: since})
	if err != nil || resp.RecoverJobID != "j1" || resp.Status != "pending" {
		t.Fatalf("RecoverEndpoint: %#v %v", resp, err)
	}
}

func TestRecoverEndpointActive(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"error":{"code":"RECOVER_JOB_ACTIVE","message":"active"}}`)
	})
	_, err := client.RecoverEndpoint(context.Background(), "ep_1", hookbridge.RecoverRequest{Since: time.Now()})
	var apiErr *hookbridge.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "RECOVER_JOB_ACTIVE" {
		t.Fatalf("err: %#v", err)
	}
}

func TestGetRecoverJob(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/endpoints/ep_1/recover/j1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"data":{"recover_job_id":"j1","endpoint_id":"ep_1","status":"done","created_count":3,"requeued_count":1,"since":"2026-06-01T00:00:00Z","until":"2026-06-01T01:00:00Z","created_at":"2026-06-01T02:00:00Z","finished_at":"2026-06-01T02:00:05Z"}}`)
	})
	job, err := client.GetRecoverJob(context.Background(), "ep_1", "j1")
	if err != nil || job.Status != "done" || job.CreatedCount != 3 || job.RequeuedCount != 1 || job.FinishedAt == nil || job.Since.IsZero() {
		t.Fatalf("GetRecoverJob: %#v %v", job, err)
	}
}

func TestSendTestEvent(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := readJSONBody(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/endpoints/ep_1/test" || body["event_type"] != "order.created" {
			t.Errorf("unexpected request %s %s %#v", r.Method, r.URL.Path, body)
		}
		if _, ok := body["payload"]; ok {
			t.Errorf("payload must be omitted: %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"data":{"message_id":"m1","status":"queued","event_type":"order.created","is_test":true}}`)
	})
	resp, err := client.SendTestEvent(context.Background(), "ep_1", hookbridge.SendTestEventRequest{EventType: "order.created"})
	if err != nil || resp.MessageID != "m1" || resp.Status != "queued" || !resp.IsTest || resp.EventType != "order.created" {
		t.Fatalf("SendTestEvent: %#v %v", resp, err)
	}
}

func TestSendEventIdempotencyMismatch(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"error":{"code":"IDEMPOTENCY_MISMATCH","message":"mismatch"}}`)
	})
	_, err := client.SendEvent(context.Background(), hookbridge.SendEventRequest{EventType: "a.b", Payload: 1, IdempotencyKey: "k"})
	var idem *hookbridge.IdempotencyError
	if !errors.As(err, &idem) || idem.Code != "IDEMPOTENCY_MISMATCH" {
		t.Fatalf("err: %#v", err)
	}
}
