package hookbridge

import (
	"context"
	"net/http"
)

const (
	projectHeader    = "X-Hookbridge-Project"
	clientNameHeader = "X-Hookbridge-Client"
)

// TokenSource supplies the bearer token for each request (e.g. a refreshing OAuth access token).
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// TokenSourceFunc adapts a function to TokenSource.
type TokenSourceFunc func(ctx context.Context) (string, error)

// Token calls f.
func (f TokenSourceFunc) Token(ctx context.Context) (string, error) {
	return f(ctx)
}

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) {
	return string(s), nil
}

type (
	projectCtxKey    struct{}
	clientNameCtxKey struct{}
)

// ContextWithProject returns a context that makes requests act on the given project
// by sending the X-Hookbridge-Project header. An empty value sends no header.
func ContextWithProject(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, projectCtxKey{}, projectID)
}

// ContextWithClientName returns a context that identifies the caller in the audit log
// by sending the X-Hookbridge-Client header. An empty value sends no header.
func ContextWithClientName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, clientNameCtxKey{}, name)
}

func hasControlChar(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

func contextHeader(ctx context.Context, key any, header string) (string, error) {
	value, _ := ctx.Value(key).(string)
	if hasControlChar(value) {
		return "", &ValidationError{APIError: &APIError{
			Code:    "VALIDATION_ERROR",
			Message: header + " value contains control characters",
		}}
	}
	return value, nil
}

// authorize sets the per-request authentication, project and client headers.
// It runs on every attempt so a refreshed token is picked up by retries.
func (c *Client) authorize(ctx context.Context, req *http.Request) error {
	project, err := contextHeader(ctx, projectCtxKey{}, projectHeader)
	if err != nil {
		return err
	}
	clientName, err := contextHeader(ctx, clientNameCtxKey{}, clientNameHeader)
	if err != nil {
		return err
	}

	token, err := c.tokens.Token(ctx)
	if err != nil || token == "" {
		return &AuthenticationError{APIError: &APIError{
			Code:    "TOKEN_SOURCE_ERROR",
			Message: "failed to get access token",
			cause:   err,
		}}
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", userAgent)
	if project != "" {
		req.Header.Set(projectHeader, project)
	}
	if clientName != "" {
		req.Header.Set(clientNameHeader, clientName)
	}
	return nil
}

// NewClientWithTokenSource creates a client that fetches its bearer token from ts on every request attempt.
func NewClientWithTokenSource(ts TokenSource, opts ...Option) (*Client, error) {
	if f, ok := ts.(TokenSourceFunc); ts == nil || (ok && f == nil) {
		return nil, &ValidationError{APIError: &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "token_source is required",
		}}
	}
	return newClient(ts, opts), nil
}
