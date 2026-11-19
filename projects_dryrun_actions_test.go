package hookbridge_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/hookbridge/hookbridge-go/v2"
)

func TestListProjects(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/projects" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"data":[{"id":"p1","tenant_id":"t1","name":"Prod","status":"active","rate_limit_default":1000,"created_at":"2025-12-01T10:00:00Z","role":"developer"}],"meta":{"request_id":"req_1"}}`)
	})
	projects, err := client.ListProjects(context.Background())
	if err != nil || len(projects) != 1 {
		t.Fatalf("ListProjects: %#v %v", projects, err)
	}
	p := projects[0]
	if p.ID != "p1" || p.TenantID != "t1" || p.Name != "Prod" || p.Status != "active" || p.RateLimitDefault != 1000 || p.Role != "developer" || p.CreatedAt.IsZero() {
		t.Fatalf("project: %#v", p)
	}
}

func TestReplayAllDryRun(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages/replay-all" ||
			q.Get("dry_run") != "true" || q.Get("status") != "failed_permanent" || q.Get("endpoint_id") != "ep_1" || q.Get("limit") != "50" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
		io.WriteString(w, `{"data":{"would_replay":7},"meta":{"request_id":"req_1"}}`)
	})
	limit := 50
	ep := "ep_1"
	resp, err := client.ReplayAllDryRun(context.Background(), hookbridge.ReplayAllMessagesRequest{Status: hookbridge.StatusFailedPermanent, EndpointID: &ep, Limit: &limit})
	if err != nil || resp.Data.WouldReplay != 7 || resp.Meta.RequestID != "req_1" {
		t.Fatalf("ReplayAllDryRun: %#v %v", resp, err)
	}
}

func TestReplayAllInboundMessagesDryRun(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodPost || r.URL.Path != "/v1/inbound-messages/replay-all" ||
			q.Get("dry_run") != "true" || q.Get("status") != "failed_permanent" || q.Get("inbound_endpoint_id") != "ib_1" || q.Get("limit") != "5" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
		io.WriteString(w, `{"data":{"would_replay":3},"meta":{"request_id":"req_2"}}`)
	})
	limit := 5
	ib := "ib_1"
	resp, err := client.ReplayAllInboundMessagesDryRun(context.Background(), hookbridge.InboundReplayAllRequest{Status: hookbridge.StatusFailedPermanent, InboundEndpointID: &ib, Limit: &limit})
	if err != nil || resp.Data.WouldReplay != 3 {
		t.Fatalf("ReplayAllInboundMessagesDryRun: %#v %v", resp, err)
	}
}

const actionsBody = `{"data":[
{"id":"a1","action":"replay","user_id":null,"api_key_id":"k1","via":"mcp-local","client_name":"Claude Desktop","created_at":"2026-11-18T10:11:12.345Z"},
{"id":"a2","action":"retry_now","user_id":"u1","api_key_id":null,"via":"console","client_name":null,"created_at":"2026-11-18T10:00:00Z"}
],"meta":{"request_id":"req_3","has_more":true,"next_cursor":"cur_2"}}`

func checkActions(t *testing.T, resp *hookbridge.ListMessageActionsResponse, err error) {
	t.Helper()
	if err != nil || len(resp.Actions) != 2 || !resp.HasMore || resp.NextCursor == nil || *resp.NextCursor != "cur_2" {
		t.Fatalf("actions: %#v %v", resp, err)
	}
	a, b := resp.Actions[0], resp.Actions[1]
	if a.ID != "a1" || a.Action != "replay" || a.UserID != nil || a.APIKeyID == nil || *a.APIKeyID != "k1" || a.Via != "mcp-local" || a.ClientName == nil || *a.ClientName != "Claude Desktop" || a.CreatedAt.IsZero() {
		t.Fatalf("action a: %#v", a)
	}
	if b.ID != "a2" || b.Action != "retry_now" || b.UserID == nil || *b.UserID != "u1" || b.APIKeyID != nil || b.Via != "console" || b.ClientName != nil {
		t.Fatalf("action b: %#v", b)
	}
}

func TestListMessageActions(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/v1/messages/msg%2F1/actions" || q.Get("cursor") != "cur 1" || q.Get("limit") != "25" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
		io.WriteString(w, actionsBody)
	})
	resp, err := client.ListMessageActions(context.Background(), "msg/1", hookbridge.ListMessageActionsParams{Cursor: "cur 1", Limit: 25})
	checkActions(t, resp, err)
}

func TestListInboundMessageActions(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/inbound-messages/in_1/actions" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
		io.WriteString(w, actionsBody)
	})
	resp, err := client.ListInboundMessageActions(context.Background(), "in_1", hookbridge.ListMessageActionsParams{})
	checkActions(t, resp, err)
}

func TestReplayAllDryRunOmitsUnsetParams(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if len(q) != 2 || q.Get("status") != "failed_permanent" || q.Get("dry_run") != "true" {
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
		io.WriteString(w, `{"data":{"would_replay":0},"meta":{"request_id":"r"}}`)
	})
	if _, err := client.ReplayAllDryRun(context.Background(), hookbridge.ReplayAllMessagesRequest{Status: hookbridge.StatusFailedPermanent}); err != nil {
		t.Fatalf("ReplayAllDryRun: %v", err)
	}
}

func TestListMessageActionsEmptyParams(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/m1/actions" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		io.WriteString(w, `{"data":[],"meta":{"request_id":"r","has_more":false}}`)
	})
	resp, err := client.ListMessageActions(context.Background(), "m1", hookbridge.ListMessageActionsParams{})
	if err != nil || len(resp.Actions) != 0 || resp.HasMore || resp.NextCursor != nil {
		t.Fatalf("ListMessageActions: %#v %v", resp, err)
	}
}

func TestListProjectsUnauthorized(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"code":"UNAUTHORIZED","message":"API keys cannot list projects"}}`)
	})
	_, err := client.ListProjects(context.Background())
	var authErr *hookbridge.AuthenticationError
	if !errors.As(err, &authErr) || authErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected AuthenticationError, got %#v", err)
	}
}
