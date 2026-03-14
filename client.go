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
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.hookbridge.io"
	defaultSendURL = "https://send.hookbridge.io"
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
	userAgent      = "hookbridge-go/1.4.0"
)

// Client is the HookBridge API client.
type Client struct {
	apiKey     string
	baseURL    string
	sendURL    string
	httpClient *http.Client
	retries    int
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets a custom base URL for the management API.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimSuffix(baseURL, "/")
	}
}

// WithSendURL sets a custom base URL for the webhook send API.
func WithSendURL(sendURL string) Option {
	return func(c *Client) {
		c.sendURL = strings.TrimSuffix(sendURL, "/")
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

	baseURL := strings.TrimSuffix(envOrDefault("HOOKBRIDGE_BASE_URL", defaultBaseURL), "/")
	sendURL := strings.TrimSuffix(envOrDefault("HOOKBRIDGE_SEND_URL", defaultSendURL), "/")

	c := &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		sendURL: sendURL,
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

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// Send sends a webhook to the specified endpoint.
func (c *Client) Send(ctx context.Context, req SendRequest) (*SendResponse, error) {
	var resp apiResponse[SendResponse]
	if err := c.doSend(ctx, http.MethodPost, "/v1/webhooks/send", req, &resp); err != nil {
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

// Replay replays a single outbound message.
func (c *Client) Replay(ctx context.Context, messageID string) (*ReplayResponse, error) {
	var resp apiResponse[ReplayResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/messages/"+messageID+"/replay", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ReplayAll replays matching outbound messages.
func (c *Client) ReplayAll(ctx context.Context, req ReplayAllMessagesRequest) (*ReplayAllMessagesResponse, error) {
	path := "/v1/messages/replay-all?" + buildReplayAllQuery(req, "endpoint_id")
	var resp ReplayAllMessagesResponse
	if err := c.do(ctx, http.MethodPost, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ReplayBatch replays specific outbound messages.
func (c *Client) ReplayBatch(ctx context.Context, messageIDs []string) (*ReplayBatchMessagesResponse, error) {
	var resp ReplayBatchMessagesResponse
	if err := c.do(ctx, http.MethodPost, "/v1/messages/replay-batch", ReplayBatchRequest{MessageIDs: messageIDs}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
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
	path := appendQuery("/v1/logs", func(query url.Values) {
		if filter == nil {
			return
		}
		if filter.Status != nil {
			query.Set("status", string(*filter.Status))
		}
		if filter.StartTime != nil {
			query.Set("start_time", filter.StartTime.Format(time.RFC3339))
		}
		if filter.EndTime != nil {
			query.Set("end_time", filter.EndTime.Format(time.RFC3339))
		}
		setInt(query, "limit", filter.Limit)
		setString(query, "cursor", filter.Cursor)
	})

	var resp apiResponse[[]MessageSummary]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &LogsResponse{Messages: resp.Data, HasMore: resp.Meta.HasMore, NextCursor: resp.Meta.NextCursor}, nil
}

// GetMetrics retrieves aggregated delivery metrics.
func (c *Client) GetMetrics(ctx context.Context, window MetricsWindow) (*Metrics, error) {
	path := buildWindowPath("/v1/metrics", window, nil, "endpoint_id")
	var resp apiResponse[Metrics]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetTimeSeriesMetrics retrieves chartable outbound metrics.
func (c *Client) GetTimeSeriesMetrics(ctx context.Context, window MetricsWindow, endpointID *string) (*TimeSeriesMetrics, error) {
	path := buildWindowPath("/v1/metrics/timeseries", window, endpointID, "endpoint_id")
	var resp apiResponse[TimeSeriesMetrics]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetDLQMessages retrieves messages from the Dead Letter Queue.
func (c *Client) GetDLQMessages(ctx context.Context, filter *DLQFilter) (*DLQResponse, error) {
	path := appendQuery("/v1/dlq/messages", func(query url.Values) {
		if filter == nil {
			return
		}
		setInt(query, "limit", filter.Limit)
		setString(query, "cursor", filter.Cursor)
	})

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
	var resp apiResponse[APIKeyWithSecret]
	if err := c.do(ctx, http.MethodPost, "/v1/api-keys", CreateAPIKeyRequest{Mode: mode, Label: label}, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DeleteAPIKey deletes an API key.
func (c *Client) DeleteAPIKey(ctx context.Context, keyID string) error {
	var resp apiResponse[struct{}]
	return c.do(ctx, http.MethodDelete, "/v1/api-keys/"+keyID, nil, &resp)
}

// ListProjects retrieves all projects for the authenticated user.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var resp apiResponse[[]Project]
	if err := c.do(ctx, http.MethodGet, "/v1/projects", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// CreateProject creates a project.
func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error) {
	var resp apiResponse[Project]
	if err := c.do(ctx, http.MethodPost, "/v1/projects", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetProject retrieves a project.
func (c *Client) GetProject(ctx context.Context, projectID string) (*Project, error) {
	var resp apiResponse[Project]
	if err := c.do(ctx, http.MethodGet, "/v1/projects/"+projectID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateProject updates a project.
func (c *Client) UpdateProject(ctx context.Context, projectID string, req UpdateProjectRequest) (*Project, error) {
	var resp apiResponse[Project]
	if err := c.do(ctx, http.MethodPatch, "/v1/projects/"+projectID, req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DeleteProject deletes a project.
func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	var resp apiResponse[struct{}]
	return c.do(ctx, http.MethodDelete, "/v1/projects/"+projectID, nil, &resp)
}

// CreateEndpoint creates a new webhook endpoint.
func (c *Client) CreateEndpoint(ctx context.Context, req CreateEndpointRequest) (*CreateEndpointResponse, error) {
	var resp apiResponse[CreateEndpointResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/endpoints", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetEndpoint retrieves an endpoint by ID.
func (c *Client) GetEndpoint(ctx context.Context, endpointID string) (*Endpoint, error) {
	var resp apiResponse[Endpoint]
	if err := c.do(ctx, http.MethodGet, "/v1/endpoints/"+endpointID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListEndpoints retrieves all endpoints for the project.
func (c *Client) ListEndpoints(ctx context.Context, filter *EndpointsFilter) (*ListEndpointsResponse, error) {
	path := appendQuery("/v1/endpoints", func(query url.Values) {
		if filter == nil {
			return
		}
		setInt(query, "limit", filter.Limit)
		setString(query, "cursor", filter.Cursor)
	})

	var resp apiResponse[[]EndpointSummary]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &ListEndpointsResponse{Endpoints: resp.Data, HasMore: resp.Meta.HasMore, NextCursor: resp.Meta.NextCursor}, nil
}

// UpdateEndpoint updates an existing endpoint.
func (c *Client) UpdateEndpoint(ctx context.Context, endpointID string, req UpdateEndpointRequest) (*UpdateResourceResponse, error) {
	var resp apiResponse[UpdateResourceResponse]
	if err := c.do(ctx, http.MethodPatch, "/v1/endpoints/"+endpointID, req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DeleteEndpoint deletes an endpoint (soft-delete).
func (c *Client) DeleteEndpoint(ctx context.Context, endpointID string) error {
	var resp apiResponse[struct{}]
	return c.do(ctx, http.MethodDelete, "/v1/endpoints/"+endpointID, nil, &resp)
}

// PauseEndpoint pauses an outbound endpoint.
func (c *Client) PauseEndpoint(ctx context.Context, endpointID string) (*ToggleResourceResponse, error) {
	var resp apiResponse[ToggleResourceResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/endpoints/"+endpointID+"/pause", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ResumeEndpoint resumes an outbound endpoint.
func (c *Client) ResumeEndpoint(ctx context.Context, endpointID string) (*ToggleResourceResponse, error) {
	var resp apiResponse[ToggleResourceResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/endpoints/"+endpointID+"/resume", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// CreateEndpointSigningKey creates a new signing key for an endpoint.
func (c *Client) CreateEndpointSigningKey(ctx context.Context, endpointID string) (*RotateSecretResponse, error) {
	var resp apiResponse[RotateSecretResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/endpoints/"+endpointID+"/signing-keys", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListEndpointSigningKeys lists the signing keys for an endpoint.
func (c *Client) ListEndpointSigningKeys(ctx context.Context, endpointID string) ([]SigningKey, error) {
	var resp apiResponse[[]SigningKey]
	if err := c.do(ctx, http.MethodGet, "/v1/endpoints/"+endpointID+"/signing-keys", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// DeleteEndpointSigningKey deletes a signing key.
func (c *Client) DeleteEndpointSigningKey(ctx context.Context, endpointID, keyID string) error {
	var resp apiResponse[struct{}]
	return c.do(ctx, http.MethodDelete, "/v1/endpoints/"+endpointID+"/signing-keys/"+keyID, nil, &resp)
}

// RotateEndpointSecret rotates the signing secret for an endpoint.
func (c *Client) RotateEndpointSecret(ctx context.Context, endpointID string) (*RotateSecretResponse, error) {
	return c.CreateEndpointSigningKey(ctx, endpointID)
}

// CreateCheckout creates a Stripe checkout session.
func (c *Client) CreateCheckout(ctx context.Context, req CreateCheckoutRequest) (*CheckoutSession, error) {
	var resp apiResponse[CheckoutSession]
	if err := c.do(ctx, http.MethodPost, "/v1/billing/checkout", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// CreatePortal creates a Stripe customer portal session.
func (c *Client) CreatePortal(ctx context.Context, req *CreatePortalRequest) (*PortalSession, error) {
	var body any
	if req != nil {
		body = req
	}
	var resp apiResponse[PortalSession]
	if err := c.do(ctx, http.MethodPost, "/v1/billing/portal", body, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetSubscription retrieves subscription details.
func (c *Client) GetSubscription(ctx context.Context) (*Subscription, error) {
	var resp apiResponse[Subscription]
	if err := c.do(ctx, http.MethodGet, "/v1/billing/subscription", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetUsageHistory retrieves usage history.
func (c *Client) GetUsageHistory(ctx context.Context, limit, offset *int) (*UsageHistoryResponse, error) {
	path := appendQuery("/v1/billing/usage-history", func(query url.Values) {
		setInt(query, "limit", limit)
		setInt(query, "offset", offset)
	})
	var resp apiResponse[[]UsageHistoryRow]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &UsageHistoryResponse{
		Rows:    resp.Data,
		Total:   resp.Meta.Total,
		Limit:   resp.Meta.Limit,
		Offset:  resp.Meta.Offset,
		HasMore: resp.Meta.HasMore,
	}, nil
}

// GetInvoices retrieves invoice history.
func (c *Client) GetInvoices(ctx context.Context) (*InvoicesResponse, error) {
	var resp apiResponse[[]Invoice]
	if err := c.do(ctx, http.MethodGet, "/v1/billing/invoices", nil, &resp); err != nil {
		return nil, err
	}
	return &InvoicesResponse{Invoices: resp.Data, HasMore: resp.Meta.HasMore}, nil
}

// CreateInboundEndpoint creates a new inbound endpoint.
func (c *Client) CreateInboundEndpoint(ctx context.Context, req CreateInboundEndpointRequest) (*CreateInboundEndpointResponse, error) {
	var resp apiResponse[CreateInboundEndpointResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/inbound-endpoints", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListInboundEndpoints lists inbound endpoints.
func (c *Client) ListInboundEndpoints(ctx context.Context, filter *InboundEndpointsFilter) (*ListInboundEndpointsResponse, error) {
	path := appendQuery("/v1/inbound-endpoints", func(query url.Values) {
		if filter == nil {
			return
		}
		setInt(query, "limit", filter.Limit)
		setString(query, "cursor", filter.Cursor)
	})
	var resp apiResponse[[]InboundEndpointSummary]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &ListInboundEndpointsResponse{Endpoints: resp.Data, HasMore: resp.Meta.HasMore, NextCursor: resp.Meta.NextCursor}, nil
}

// GetInboundEndpoint retrieves an inbound endpoint.
func (c *Client) GetInboundEndpoint(ctx context.Context, endpointID string) (*InboundEndpoint, error) {
	var resp apiResponse[InboundEndpoint]
	if err := c.do(ctx, http.MethodGet, "/v1/inbound-endpoints/"+endpointID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateInboundEndpoint updates an inbound endpoint.
func (c *Client) UpdateInboundEndpoint(ctx context.Context, endpointID string, req UpdateInboundEndpointRequest) (*UpdateResourceResponse, error) {
	var resp apiResponse[UpdateResourceResponse]
	if err := c.do(ctx, http.MethodPatch, "/v1/inbound-endpoints/"+endpointID, req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DeleteInboundEndpoint deletes an inbound endpoint.
func (c *Client) DeleteInboundEndpoint(ctx context.Context, endpointID string) (*ToggleResourceResponse, error) {
	var resp apiResponse[ToggleResourceResponse]
	if err := c.do(ctx, http.MethodDelete, "/v1/inbound-endpoints/"+endpointID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// PauseInboundEndpoint pauses an inbound endpoint.
func (c *Client) PauseInboundEndpoint(ctx context.Context, endpointID string) (*ToggleResourceResponse, error) {
	var resp apiResponse[ToggleResourceResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/inbound-endpoints/"+endpointID+"/pause", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ResumeInboundEndpoint resumes an inbound endpoint.
func (c *Client) ResumeInboundEndpoint(ctx context.Context, endpointID string) (*ToggleResourceResponse, error) {
	var resp apiResponse[ToggleResourceResponse]
	if err := c.do(ctx, http.MethodPost, "/v1/inbound-endpoints/"+endpointID+"/resume", nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ReplayInboundMessage replays a single inbound message.
func (c *Client) ReplayInboundMessage(ctx context.Context, messageID string) (*ReplayBatchMessagesResponse, error) {
	var resp ReplayBatchMessagesResponse
	if err := c.do(ctx, http.MethodPost, "/v1/inbound-messages/"+messageID+"/replay", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ReplayAllInboundMessages replays matching inbound messages.
func (c *Client) ReplayAllInboundMessages(ctx context.Context, req InboundReplayAllRequest) (*ReplayAllMessagesResponse, error) {
	path := "/v1/inbound-messages/replay-all?" + buildReplayAllQuery(ReplayAllMessagesRequest{
		Status:     req.Status,
		EndpointID: req.InboundEndpointID,
		Limit:      req.Limit,
	}, "inbound_endpoint_id")
	var resp ReplayAllMessagesResponse
	if err := c.do(ctx, http.MethodPost, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ReplayBatchInboundMessages replays specific inbound messages.
func (c *Client) ReplayBatchInboundMessages(ctx context.Context, messageIDs []string) (*ReplayBatchMessagesResponse, error) {
	var resp ReplayBatchMessagesResponse
	if err := c.do(ctx, http.MethodPost, "/v1/inbound-messages/replay-batch", ReplayBatchRequest{MessageIDs: messageIDs}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetInboundLogs retrieves inbound delivery logs.
func (c *Client) GetInboundLogs(ctx context.Context, filter *InboundLogsFilter) (*InboundLogsResponse, error) {
	path := appendQuery("/v1/inbound-logs", func(query url.Values) {
		if filter == nil {
			return
		}
		if filter.Status != nil {
			query.Set("status", string(*filter.Status))
		}
		setString(query, "inbound_endpoint_id", filter.InboundEndpointID)
		if filter.StartTime != nil {
			query.Set("start_time", filter.StartTime.Format(time.RFC3339))
		}
		if filter.EndTime != nil {
			query.Set("end_time", filter.EndTime.Format(time.RFC3339))
		}
		setInt(query, "limit", filter.Limit)
		setString(query, "cursor", filter.Cursor)
	})
	var resp apiResponse[[]InboundLogEntry]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &InboundLogsResponse{Logs: resp.Data, HasMore: resp.Meta.HasMore, NextCursor: resp.Meta.NextCursor}, nil
}

// GetInboundMetrics retrieves aggregated inbound metrics.
func (c *Client) GetInboundMetrics(ctx context.Context, window MetricsWindow, inboundEndpointID *string) (*InboundMetrics, error) {
	path := buildWindowPath("/v1/inbound-metrics", window, inboundEndpointID, "inbound_endpoint_id")
	var resp apiResponse[InboundMetrics]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetInboundTimeSeriesMetrics retrieves chartable inbound metrics.
func (c *Client) GetInboundTimeSeriesMetrics(ctx context.Context, window MetricsWindow, inboundEndpointID *string) (*TimeSeriesMetrics, error) {
	path := buildWindowPath("/v1/inbound-metrics/timeseries", window, inboundEndpointID, "inbound_endpoint_id")
	var resp apiResponse[TimeSeriesMetrics]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListInboundRejections retrieves rejected inbound requests.
func (c *Client) ListInboundRejections(ctx context.Context, filter *InboundRejectionsFilter) (*InboundRejectionsResponse, error) {
	path := appendQuery("/v1/inbound-rejections", func(query url.Values) {
		if filter == nil {
			return
		}
		setString(query, "inbound_endpoint_id", filter.InboundEndpointID)
		if filter.StartTime != nil {
			query.Set("start_time", filter.StartTime.Format(time.RFC3339))
		}
		if filter.EndTime != nil {
			query.Set("end_time", filter.EndTime.Format(time.RFC3339))
		}
		setInt(query, "limit", filter.Limit)
		setString(query, "cursor", filter.Cursor)
	})
	var resp apiResponse[[]InboundRejection]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &InboundRejectionsResponse{Rejections: resp.Data, HasMore: resp.Meta.HasMore, NextCursor: resp.Meta.NextCursor}, nil
}

// CreateExport creates an export job.
func (c *Client) CreateExport(ctx context.Context, req CreateExportRequest) (*ExportRecord, error) {
	var resp apiResponse[ExportRecord]
	if err := c.do(ctx, http.MethodPost, "/v1/exports", req, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// ListExports lists export jobs.
func (c *Client) ListExports(ctx context.Context) ([]ExportRecord, error) {
	var resp apiResponse[[]ExportRecord]
	if err := c.do(ctx, http.MethodGet, "/v1/exports", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetExport retrieves an export job.
func (c *Client) GetExport(ctx context.Context, exportID string) (*ExportRecord, error) {
	var resp apiResponse[ExportRecord]
	if err := c.do(ctx, http.MethodGet, "/v1/exports/"+exportID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DownloadExport returns the presigned download URL for an export.
func (c *Client) DownloadExport(ctx context.Context, exportID string) (*DownloadExportResponse, error) {
	cloned := *c.httpClient
	cloned.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/exports/"+exportID+"/download", nil)
	if err != nil {
		return nil, &NetworkError{Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", userAgent)

	resp, err := cloned.Do(req)
	if err != nil {
		return nil, &NetworkError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if location == "" {
			return nil, &ValidationError{APIError: &APIError{Code: "PARSE_ERROR", Message: "missing Location header"}}
		}
		return &DownloadExportResponse{URL: location}, nil
	}

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, &NetworkError{Err: readErr}
	}
	return nil, c.parseErrorResponse(resp.StatusCode, body)
}

func buildReplayAllQuery(req ReplayAllMessagesRequest, endpointKey string) string {
	query := url.Values{}
	query.Set("status", string(req.Status))
	setString(query, endpointKey, req.EndpointID)
	setInt(query, "limit", req.Limit)
	return query.Encode()
}

func buildWindowPath(path string, window MetricsWindow, resourceID *string, resourceKey string) string {
	return appendQuery(path, func(query url.Values) {
		if window != "" {
			query.Set("window", string(window))
		}
		setString(query, resourceKey, resourceID)
	})
}

func appendQuery(path string, mutate func(url.Values)) string {
	query := url.Values{}
	mutate(query)
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

func setString(query url.Values, key string, value *string) {
	if value != nil {
		query.Set(key, *value)
	}
}

func setInt(query url.Values, key string, value *int) {
	if value != nil {
		query.Set(key, strconv.Itoa(*value))
	}
}

// doSend performs an HTTP request to the send URL with retries.
func (c *Client) doSend(ctx context.Context, method, path string, body any, result any) error {
	return c.doWithBaseURL(ctx, c.sendURL, method, path, body, result)
}

// do performs an HTTP request with retries.
func (c *Client) do(ctx context.Context, method, path string, body any, result any) error {
	return c.doWithBaseURL(ctx, c.baseURL, method, path, body, result)
}

// doWithBaseURL performs an HTTP request with retries using the specified base URL.
func (c *Client) doWithBaseURL(ctx context.Context, baseURL, method, path string, body any, result any) error {
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return &ValidationError{APIError: &APIError{
				Code:    "VALIDATION_ERROR",
				Message: fmt.Sprintf("failed to marshal request body: %v", err),
			}}
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
		if err != nil {
			return &NetworkError{Err: err}
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("User-Agent", userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

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

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = &NetworkError{Err: readErr}
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

		lastErr = c.parseErrorResponse(resp.StatusCode, respBody)
		if !isRetryable(resp.StatusCode, nil) {
			return lastErr
		}
	}

	return lastErr
}

func (c *Client) parseErrorResponse(statusCode int, body []byte) error {
	var errResp apiResponse[any]
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		return newErrorFromResponse(statusCode, errResp.Error.Code, errResp.Error.Message, errResp.Meta.RequestID)
	}
	return &APIError{
		Code:       "UNKNOWN_ERROR",
		Message:    strings.TrimSpace(string(body)),
		StatusCode: statusCode,
	}
}

// isRetryable determines if a request should be retried.
func isRetryable(statusCode int, err error) bool {
	if err != nil {
		return true
	}
	return statusCode >= 500 || statusCode == 408 || statusCode == 429
}
