package hookbridge_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hookbridge/hookbridge-go/v2"
)

type recordedRequest struct {
	header http.Header
	path   string
}

type recorder struct {
	mu   sync.Mutex
	reqs []recordedRequest
}

func (r *recorder) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, recordedRequest{header: req.Header.Clone(), path: req.URL.Path})
}

func (r *recorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.reqs...)
}

func newRecordingServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, n int)) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		handler(w, r, int(atomic.AddInt32(&n, 1)))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func okHandler(w http.ResponseWriter, _ *http.Request, _ int) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":{},"meta":{"request_id":"req_1"}}`))
}

func staticToken(tok string) hookbridge.TokenSource {
	return hookbridge.TokenSourceFunc(func(context.Context) (string, error) { return tok, nil })
}

func TestTokenSourceUsedForAuthorization(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	c, err := hookbridge.NewClientWithTokenSource(staticToken("oauth-tok-1"), hookbridge.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetExport(context.Background(), "exp_1"); err != nil {
		t.Fatal(err)
	}
	reqs := rec.all()
	if len(reqs) != 1 || reqs[0].header.Get("Authorization") != "Bearer oauth-tok-1" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}
}

func TestTokenFetchedPerRetryAttempt(t *testing.T) {
	srv, rec := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		okHandler(w, r, n)
	})
	var calls int32
	ts := hookbridge.TokenSourceFunc(func(context.Context) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return "old-token", nil
		}
		return "new-token", nil
	})
	c, err := hookbridge.NewClientWithTokenSource(ts, hookbridge.WithBaseURL(srv.URL), hookbridge.WithRetries(2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetExport(context.Background(), "exp_1"); err != nil {
		t.Fatal(err)
	}
	reqs := rec.all()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if got := reqs[0].header.Get("Authorization"); got != "Bearer old-token" {
		t.Errorf("first attempt auth = %q", got)
	}
	if got := reqs[1].header.Get("Authorization"); got != "Bearer new-token" {
		t.Errorf("second attempt auth = %q", got)
	}
}

func TestTokenSourceErrorStopsRequest(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	sentinel := errors.New("refresh failed for secret-refresh-token")
	var calls int32
	ts := hookbridge.TokenSourceFunc(func(context.Context) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", sentinel
	})
	c, err := hookbridge.NewClientWithTokenSource(ts, hookbridge.WithBaseURL(srv.URL), hookbridge.WithRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetExport(context.Background(), "exp_1")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(rec.all()) != 0 {
		t.Fatal("no request should reach the server")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("token source called %d times, want 1", got)
	}
	if !errors.Is(err, sentinel) {
		t.Error("error should unwrap to the original")
	}
	var authErr *hookbridge.AuthenticationError
	if !errors.As(err, &authErr) || authErr.Code != "TOKEN_SOURCE_ERROR" {
		t.Errorf("expected AuthenticationError TOKEN_SOURCE_ERROR, got %#v", err)
	}
	if strings.Contains(err.Error(), "secret-refresh-token") {
		t.Error("error message leaks token source detail")
	}
}

func TestEmptyTokenRejected(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	c, err := hookbridge.NewClientWithTokenSource(staticToken(""), hookbridge.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetExport(context.Background(), "exp_1")
	var authErr *hookbridge.AuthenticationError
	if !errors.As(err, &authErr) || authErr.Code != "TOKEN_SOURCE_ERROR" {
		t.Fatalf("expected TOKEN_SOURCE_ERROR, got %#v", err)
	}
	if len(rec.all()) != 0 {
		t.Fatal("no request should be sent with an empty token")
	}
}

func TestNilTokenSourceRejected(t *testing.T) {
	var nilFunc hookbridge.TokenSourceFunc
	for name, ts := range map[string]hookbridge.TokenSource{"nil": nil, "nil func": nilFunc} {
		c, err := hookbridge.NewClientWithTokenSource(ts)
		var vErr *hookbridge.ValidationError
		if c != nil || !errors.As(err, &vErr) || !strings.Contains(err.Error(), "token_source is required") {
			t.Errorf("%s: expected validation error, got %v", name, err)
		}
	}
}

func TestTokenNeverAppearsInErrors(t *testing.T) {
	const secret = "super-secret-token-value"
	srv, _ := newRecordingServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"bad token"},"meta":{}}`))
	})
	c, err := hookbridge.NewClientWithTokenSource(staticToken(secret), hookbridge.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetExport(context.Background(), "exp_1")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("failed request error must not contain token: %v", err)
	}

	failing := hookbridge.TokenSourceFunc(func(context.Context) (string, error) { return secret, errors.New("boom") })
	c2, _ := hookbridge.NewClientWithTokenSource(failing, hookbridge.WithBaseURL(srv.URL))
	_, err = c2.GetExport(context.Background(), "exp_1")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("token source error must not contain token: %v", err)
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	c3, _ := hookbridge.NewClientWithTokenSource(staticToken(secret), hookbridge.WithBaseURL(closed.URL), hookbridge.WithRetries(0))
	_, err = c3.GetExport(context.Background(), "exp_1")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("network error must not contain token: %v", err)
	}
}

func TestAPIKeyPathUnchanged(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	c, err := hookbridge.NewClient("hb_live_key", hookbridge.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetExport(context.Background(), "exp_1"); err != nil {
		t.Fatal(err)
	}
	h := rec.all()[0].header
	if h.Get("Authorization") != "Bearer hb_live_key" {
		t.Errorf("auth = %q", h.Get("Authorization"))
	}
	if _, ok := h["X-Hookbridge-Project"]; ok {
		t.Error("project header must be absent")
	}
	if _, ok := h["X-Hookbridge-Client"]; ok {
		t.Error("client header must be absent")
	}
	if _, err := hookbridge.NewClient(""); err == nil || !strings.Contains(err.Error(), "api_key is required") {
		t.Errorf("empty key error = %v", err)
	}
}

func TestContextHeadersOnAllPaths(t *testing.T) {
	srv, rec := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("Location", "https://example.com/file")
			w.WriteHeader(http.StatusFound)
			return
		}
		okHandler(w, r, n)
	})
	c, err := hookbridge.NewClientWithTokenSource(staticToken("t"), hookbridge.WithBaseURL(srv.URL), hookbridge.WithSendURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx := hookbridge.ContextWithProject(context.Background(), "proj_123")
	ctx = hookbridge.ContextWithClientName(ctx, "hb-mcp/0.1.0; agent=Claude Code")

	if _, err := c.GetExport(ctx, "exp_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Send(ctx, hookbridge.SendRequest{EndpointID: "ep_1", Payload: map[string]any{"a": 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DownloadExport(ctx, "exp_1"); err != nil {
		t.Fatal(err)
	}
	reqs := rec.all()
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}
	for _, r := range reqs {
		if r.header.Get("X-Hookbridge-Project") != "proj_123" {
			t.Errorf("%s: project header = %q", r.path, r.header.Get("X-Hookbridge-Project"))
		}
		if r.header.Get("X-Hookbridge-Client") != "hb-mcp/0.1.0; agent=Claude Code" {
			t.Errorf("%s: client header = %q", r.path, r.header.Get("X-Hookbridge-Client"))
		}
	}
}

func TestContextHeadersOnEveryRetry(t *testing.T) {
	srv, rec := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		okHandler(w, r, n)
	})
	c, _ := hookbridge.NewClient("k", hookbridge.WithBaseURL(srv.URL), hookbridge.WithRetries(1))
	ctx := hookbridge.ContextWithProject(context.Background(), "proj_9")
	if _, err := c.GetExport(ctx, "exp_1"); err != nil {
		t.Fatal(err)
	}
	for i, r := range rec.all() {
		if r.header.Get("X-Hookbridge-Project") != "proj_9" {
			t.Errorf("attempt %d missing project header", i)
		}
	}
	if len(rec.all()) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(rec.all()))
	}
}

func TestContextHeadersAbsentWhenEmpty(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	c, _ := hookbridge.NewClient("k", hookbridge.WithBaseURL(srv.URL))
	ctx := hookbridge.ContextWithProject(context.Background(), "")
	ctx = hookbridge.ContextWithClientName(ctx, "")
	if _, err := c.GetExport(ctx, "exp_1"); err != nil {
		t.Fatal(err)
	}
	h := rec.all()[0].header
	if _, ok := h["X-Hookbridge-Project"]; ok {
		t.Error("project header must be absent")
	}
	if _, ok := h["X-Hookbridge-Client"]; ok {
		t.Error("client header must be absent")
	}
}

func TestControlCharsRejectedBeforeNetwork(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	c, _ := hookbridge.NewClient("k", hookbridge.WithBaseURL(srv.URL), hookbridge.WithSendURL(srv.URL))
	bad := []string{"a\r\nX-Evil: 1", "a\nb", "a\rb", "a\x00b", "a\tb", "a\x7fb", "a\x1fb"}
	for _, v := range bad {
		ctxs := map[string]context.Context{
			"project": hookbridge.ContextWithProject(context.Background(), v),
			"client":  hookbridge.ContextWithClientName(context.Background(), v),
		}
		for name, ctx := range ctxs {
			calls := map[string]func() error{
				"do":       func() error { _, err := c.GetExport(ctx, "e"); return err },
				"doSend":   func() error { _, err := c.Send(ctx, hookbridge.SendRequest{EndpointID: "e", Payload: 1}); return err },
				"download": func() error { _, err := c.DownloadExport(ctx, "e"); return err },
			}
			for path, call := range calls {
				err := call()
				var vErr *hookbridge.ValidationError
				if !errors.As(err, &vErr) {
					t.Errorf("%s/%s/%q: expected ValidationError, got %v", name, path, v, err)
				}
			}
		}
	}
	if len(rec.all()) != 0 {
		t.Fatalf("no request should be sent, got %d", len(rec.all()))
	}
}

func TestUserAgentPinned(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	c, _ := hookbridge.NewClient("k", hookbridge.WithBaseURL(srv.URL))
	if _, err := c.GetExport(context.Background(), "exp_1"); err != nil {
		t.Fatal(err)
	}
	if got := rec.all()[0].header.Get("User-Agent"); got != "hookbridge-go/2.3.0" {
		t.Errorf("User-Agent = %q", got)
	}
	if hookbridge.Version != "2.3.0" {
		t.Errorf("Version = %q", hookbridge.Version)
	}
}

func TestControlCharTokenRejected(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	for _, tok := range []string{"abc\r\nX-Evil: 1", "abc\n", "a\x00b", "a\x7fb"} {
		var calls int32
		ts := hookbridge.TokenSourceFunc(func(context.Context) (string, error) {
			atomic.AddInt32(&calls, 1)
			return tok, nil
		})
		c, _ := hookbridge.NewClientWithTokenSource(ts, hookbridge.WithBaseURL(srv.URL), hookbridge.WithRetries(3))
		_, err := c.GetExport(context.Background(), "exp_1")
		var authErr *hookbridge.AuthenticationError
		if !errors.As(err, &authErr) || authErr.Code != "TOKEN_SOURCE_ERROR" {
			t.Fatalf("%q: expected TOKEN_SOURCE_ERROR, got %#v", tok, err)
		}
		if strings.Contains(err.Error(), "abc") || strings.Contains(err.Error(), "X-Evil") {
			t.Errorf("%q: error leaks token: %v", tok, err)
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("%q: token source called %d times, want 1", tok, got)
		}
	}
	if len(rec.all()) != 0 {
		t.Fatal("no request should be sent")
	}
}

func TestTokenSourceErrorWithCancelledContextIsTimeout(t *testing.T) {
	srv, rec := newRecordingServer(t, okHandler)
	ts := hookbridge.TokenSourceFunc(func(ctx context.Context) (string, error) { return "", ctx.Err() })
	c, _ := hookbridge.NewClientWithTokenSource(ts, hookbridge.WithBaseURL(srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetExport(ctx, "exp_1")
	var tErr *hookbridge.TimeoutError
	if !errors.As(err, &tErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected TimeoutError wrapping context.Canceled, got %#v", err)
	}
	if len(rec.all()) != 0 {
		t.Fatal("no request should be sent")
	}
}
