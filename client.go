package hookbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.hookbridge.io"
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
	userAgent      = "hookbridge-go/1.0.0"
)

// Client is the HookBridge API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	retries    int
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets a custom base URL for the API.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimSuffix(baseURL, "/")
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithRetries sets the number of retry attempts.
func WithRetries(retries int) Option {
	return func(c *Client) {
		c.retries = retries
	}
}

// NewClient creates a new HookBridge client.
func NewClient(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, &ValidationError{APIError: &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "api_key is required",
		}}
	}

	baseURL := os.Getenv("HOOKBRIDGE_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	c := &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		retries: defaultRetries,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Send sends a webhook to the specified endpoint.
func (c *Client) Send(ctx context.Context, req SendRequest) (*SendResponse, error) {
	var resp apiResponse[SendResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/webhooks/send", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetMessage retrieves a message by ID.
func (c *Client) GetMessage(ctx context.Context, messageID string) (*Message, error) {
	var resp apiResponse[Message]
	if err := c.do(ctx, http.MethodGet, "/v1/messages/"+messageID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Replay replays a message.
func (c *Client) Replay(ctx context.Context, messageID string) (*ReplayResponse, error) {
	var resp apiResponse[ReplayResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/messages/"+messageID+"/replay", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// RetryNow triggers an immediate retry of a message.
func (c *Client) RetryNow(ctx context.Context, messageID string) (*Message, error) {
	var resp apiResponse[Message]
	if err := c.do(ctx, http.MethodPost, "/v1/messages/"+messageID+"/retry-now", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// CancelRetry cancels a pending retry.
func (c *Client) CancelRetry(ctx context.Context, messageID string) (*Message, error) {
	var resp apiResponse[Message]
	if err := c.do(ctx, http.MethodPost, "/v1/messages/"+messageID+"/cancel", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetLogs retrieves delivery logs with optional filtering.
func (c *Client) GetLogs(ctx context.Context, filter *LogsFilter) (*LogsResponse, error) {
	query := url.Values{}
	if filter != nil {
		if filter.Status != nil {
			query.Set("status", string(*filter.Status))
		}
		if filter.StartTime != nil {
			query.Set("start_time", filter.StartTime.Format(time.RFC3339))
		}
		if filter.EndTime != nil {
			query.Set("end_time", filter.EndTime.Format(time.RFC3339))
		}
		if filter.Limit != nil {
			query.Set("limit", fmt.Sprintf("%d", *filter.Limit))
		}
		if filter.Cursor != nil {
			query.Set("cursor", *filter.Cursor)
		}
	}

	path := "/v1/logs"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	// Logs endpoint returns data as array, pagination in meta
	var resp apiResponse[[]MessageSummary]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &LogsResponse{
		Messages:   resp.Data,
		HasMore:    resp.Meta.HasMore,
		NextCursor: resp.Meta.NextCursor,
	}, nil
}

// GetMetrics retrieves aggregated delivery metrics.
func (c *Client) GetMetrics(ctx context.Context, window MetricsWindow) (*Metrics, error) {
	path := "/v1/metrics"
	if window != "" {
		path += "?window=" + string(window)
	}

	var resp apiResponse[Metrics]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetDLQMessages retrieves messages from the Dead Letter Queue.
func (c *Client) GetDLQMessages(ctx context.Context, filter *DLQFilter) (*DLQResponse, error) {
	query := url.Values{}
	if filter != nil {
		if filter.Limit != nil {
			query.Set("limit", fmt.Sprintf("%d", *filter.Limit))
		}
		if filter.Cursor != nil {
			query.Set("cursor", *filter.Cursor)
		}
	}

	path := "/v1/dlq/messages"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	// DLQ endpoint returns data as object with messages array inside
	var resp apiResponse[dlqDataResponse]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	messages := resp.Data.Messages
	if messages == nil {
		messages = []DLQMessage{}
	}
	return &DLQResponse{
		Messages:   messages,
		HasMore:    resp.Data.HasMore,
		NextCursor: resp.Data.NextCursor,
	}, nil
}

// ReplayFromDLQ replays a message from the Dead Letter Queue.
func (c *Client) ReplayFromDLQ(ctx context.Context, messageID string) (*ReplayResponse, error) {
	var resp apiResponse[ReplayResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/dlq/replay/"+messageID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListAPIKeys retrieves all API keys for the project.
func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var resp apiResponse[[]APIKey]
	if err := c.do(ctx, http.MethodGet, "/v1/api-keys", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// CreateAPIKey creates a new API key.
func (c *Client) CreateAPIKey(ctx context.Context, mode APIKeyMode, label *string) (*APIKeyWithSecret, error) {
	req := CreateAPIKeyRequest{
		Mode:  mode,
		Label: label,
	}
	var resp apiResponse[APIKeyWithSecret]
	if err := c.do(ctx, http.MethodPost, "/v1/api-keys", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DeleteAPIKey deletes an API key.
func (c *Client) DeleteAPIKey(ctx context.Context, keyID string) error {
	var resp apiResponse[struct{}]
	return c.do(ctx, http.MethodDelete, "/v1/api-keys/"+keyID, nil, &resp)
}

// do performs an HTTP request with retries.
func (c *Client) do(ctx context.Context, method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return &ValidationError{APIError: &APIError{
				Code:    "VALIDATION_ERROR",
				Message: fmt.Sprintf("failed to marshal request body: %v", err),
			}}
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			// Reset body reader for retry
			if body != nil {
				bodyBytes, _ := json.Marshal(body)
				bodyReader = bytes.NewReader(bodyBytes)
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return &NetworkError{Err: err}
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return &TimeoutError{Err: ctx.Err()}
			}
			lastErr = &NetworkError{Err: err}
			if isRetryable(0, err) {
				continue
			}
			return lastErr
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = &NetworkError{Err: err}
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if result != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, result); err != nil {
					return &ValidationError{APIError: &APIError{
						Code:    "PARSE_ERROR",
						Message: fmt.Sprintf("failed to parse response: %v", err),
					}}
				}
			}
			return nil
		}

		// Parse error response
		var errResp apiResponse[any]
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			lastErr = &APIError{
				Code:       "UNKNOWN_ERROR",
				Message:    string(respBody),
				StatusCode: resp.StatusCode,
			}
		} else if errResp.Error != nil {
			lastErr = newErrorFromResponse(resp.StatusCode, errResp.Error.Code, errResp.Error.Message, errResp.Meta.RequestID)
		} else {
			lastErr = &APIError{
				Code:       "UNKNOWN_ERROR",
				Message:    "unexpected error response",
				StatusCode: resp.StatusCode,
			}
		}

		if !isRetryable(resp.StatusCode, nil) {
			return lastErr
		}
	}

	return lastErr
}

// isRetryable determines if a request should be retried.
func isRetryable(statusCode int, err error) bool {
	if err != nil {
		return true // Network errors are retryable
	}
	// Retry on server errors and specific client errors
	return statusCode >= 500 || statusCode == 408 || statusCode == 429
}
