package hookbridge

import "time"

// MessageStatus represents the status of a webhook message.
type MessageStatus string

const (
	StatusQueued          MessageStatus = "queued"
	StatusDelivering      MessageStatus = "delivering"
	StatusSucceeded       MessageStatus = "succeeded"
	StatusPendingRetry    MessageStatus = "pending_retry"
	StatusFailedPermanent MessageStatus = "failed_permanent"
)

// MetricsWindow represents the time window for metrics aggregation.
type MetricsWindow string

const (
	Window1Hour  MetricsWindow = "1h"
	Window24Hour MetricsWindow = "24h"
	Window7Day   MetricsWindow = "7d"
	Window30Day  MetricsWindow = "30d"
)

// APIKeyMode represents the mode for an API key.
type APIKeyMode string

const (
	ModeLive APIKeyMode = "live"
	ModeTest APIKeyMode = "test"
)

// SendRequest represents a request to send a webhook.
type SendRequest struct {
	EndpointID     string            `json:"endpoint_id"`
	Payload        any               `json:"payload"`
	Headers        map[string]string `json:"headers,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

// SendResponse represents the response from sending a webhook.
type SendResponse struct {
	MessageID string        `json:"message_id"`
	Status    MessageStatus `json:"status"`
}

// Message represents a webhook message.
type Message struct {
	ID                string        `json:"id"`
	ProjectID         string        `json:"project_id"`
	EndpointID        string        `json:"endpoint_id"`
	Status            MessageStatus `json:"status"`
	AttemptCount      int           `json:"attempt_count"`
	ReplayCount       int           `json:"replay_count"`
	ContentType       string        `json:"content_type"`
	SizeBytes         int           `json:"size_bytes"`
	PayloadSHA256     string        `json:"payload_sha256"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	IdempotencyKey    *string       `json:"idempotency_key,omitempty"`
	NextAttemptAt     *time.Time    `json:"next_attempt_at,omitempty"`
	LastError         *string       `json:"last_error,omitempty"`
	ResponseStatus    *int          `json:"response_status,omitempty"`
	ResponseLatencyMs *int          `json:"response_latency_ms,omitempty"`
}

// MessageSummary represents a summary of a message for log listings.
type MessageSummary struct {
	MessageID         string        `json:"message_id"`
	Endpoint          string        `json:"endpoint"`
	Status            MessageStatus `json:"status"`
	AttemptCount      int           `json:"attempt_count"`
	CreatedAt         string        `json:"created_at"`
	DeliveredAt       *string       `json:"delivered_at,omitempty"`
	ResponseStatus    *int          `json:"response_status,omitempty"`
	ResponseLatencyMs *int          `json:"response_latency_ms,omitempty"`
	LastError         *string       `json:"last_error,omitempty"`
}

// LogsResponse represents the response from querying logs.
type LogsResponse struct {
	Messages   []MessageSummary
	HasMore    bool
	NextCursor *string
}

// LogsFilter represents filtering options for log queries.
type LogsFilter struct {
	Status    *MessageStatus
	StartTime *time.Time
	EndTime   *time.Time
	Limit     *int
	Cursor    *string
}

// Metrics represents aggregated delivery metrics.
type Metrics struct {
	Window        MetricsWindow `json:"window"`
	TotalMessages int           `json:"total_messages"`
	Succeeded     int           `json:"succeeded"`
	Failed        int           `json:"failed"`
	Retries       int           `json:"retries"`
	SuccessRate   float64       `json:"success_rate"`
	AvgLatencyMs  int           `json:"avg_latency_ms"`
}

// TimeSeriesBucket represents a single metrics bucket.
type TimeSeriesBucket struct {
	Timestamp    time.Time `json:"timestamp"`
	Succeeded    int       `json:"succeeded"`
	Failed       int       `json:"failed"`
	Retrying     int       `json:"retrying"`
	Total        int       `json:"total"`
	AvgLatencyMs int       `json:"avg_latency_ms"`
}

// TimeSeriesMetrics represents chartable delivery metrics.
type TimeSeriesMetrics struct {
	Window  MetricsWindow      `json:"window"`
	Buckets []TimeSeriesBucket `json:"buckets"`
}

// DLQMessage represents a message in the Dead Letter Queue.
type DLQMessage struct {
	MessageID    string `json:"message_id"`
	Endpoint     string `json:"endpoint"`
	AttemptCount int    `json:"attempt_count"`
	LastError    string `json:"last_error"`
	CreatedAt    string `json:"created_at"`
	FailedAt     string `json:"failed_at"`
}

// DLQResponse represents the response from querying the DLQ.
type DLQResponse struct {
	Messages   []DLQMessage
	HasMore    bool
	NextCursor *string
}

// DLQFilter represents filtering options for DLQ queries.
type DLQFilter struct {
	Limit  *int
	Cursor *string
}

// APIKey represents an API key.
type APIKey struct {
	KeyID      string  `json:"key_id"`
	Label      *string `json:"label,omitempty"`
	Prefix     string  `json:"prefix"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

// APIKeyWithSecret represents an API key with its full secret (returned on creation).
type APIKeyWithSecret struct {
	KeyID     string  `json:"key_id"`
	Key       string  `json:"key"`
	Label     *string `json:"label,omitempty"`
	Prefix    string  `json:"prefix"`
	CreatedAt string  `json:"created_at"`
}

// CreateAPIKeyRequest represents a request to create an API key.
type CreateAPIKeyRequest struct {
	Mode  APIKeyMode `json:"mode"`
	Label *string    `json:"label,omitempty"`
}

// ReplayResponse represents the response from replaying a message.
type ReplayResponse struct {
	MessageID string        `json:"message_id"`
	Status    MessageStatus `json:"status"`
}

// ReplayBatchRequest represents a request to replay multiple messages.
type ReplayBatchRequest struct {
	MessageIDs []string `json:"message_ids"`
}

// ReplayAllMessagesRequest represents query parameters for replay-all.
type ReplayAllMessagesRequest struct {
	Status     MessageStatus
	EndpointID *string
	Limit      *int
}

// ReplayResult represents the result for a single replayed message.
type ReplayResult struct {
	MessageID string  `json:"message_id"`
	Status    string  `json:"status"`
	Error     *string `json:"error,omitempty"`
}

// ReplayAllResult contains a bulk replay summary.
type ReplayAllResult struct {
	Replayed           int      `json:"replayed"`
	Failed             int      `json:"failed"`
	Stuck              int      `json:"stuck"`
	ReplayedMessageIDs []string `json:"replayed_message_ids,omitempty"`
	StuckMessageIDs    []string `json:"stuck_message_ids,omitempty"`
}

// ReplayAllMessagesResponse represents replay-all output.
type ReplayAllMessagesResponse struct {
	Message string          `json:"message"`
	Data    ReplayAllResult `json:"data"`
}

// ReplayBatchResult contains a batch replay summary.
type ReplayBatchResult struct {
	Replayed int            `json:"replayed"`
	Failed   int            `json:"failed"`
	Stuck    int            `json:"stuck"`
	Results  []ReplayResult `json:"results"`
}

// ReplayBatchMessagesResponse represents replay-batch output.
type ReplayBatchMessagesResponse struct {
	Message string            `json:"message"`
	Data    ReplayBatchResult `json:"data"`
}

// Endpoint represents a webhook endpoint.
type Endpoint struct {
	ID           string             `json:"id"`
	URL          string             `json:"url"`
	Description  *string            `json:"description,omitempty"`
	RateLimitRPS *int               `json:"rate_limit_rps,omitempty"`
	Burst        *int               `json:"burst,omitempty"`
	Headers      *map[string]string `json:"headers,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// EndpointSummary represents a summary of an endpoint for list responses.
type EndpointSummary struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateEndpointRequest represents a request to create an endpoint.
type CreateEndpointRequest struct {
	URL          string             `json:"url"`
	Description  *string            `json:"description,omitempty"`
	RateLimitRPS *int               `json:"rate_limit_rps,omitempty"`
	Burst        *int               `json:"burst,omitempty"`
	Headers      *map[string]string `json:"headers,omitempty"`
}

// CreateEndpointResponse represents the response from creating an endpoint.
type CreateEndpointResponse struct {
	ID            string    `json:"id"`
	URL           string    `json:"url"`
	Description   *string   `json:"description,omitempty"`
	SigningKeyID  string    `json:"signing_key_id"`
	SigningSecret string    `json:"signing_secret"`
	KeyHint       string    `json:"key_hint"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpdateEndpointRequest represents a request to update an endpoint.
type UpdateEndpointRequest struct {
	URL          *string            `json:"url,omitempty"`
	Description  *string            `json:"description,omitempty"`
	RateLimitRPS *int               `json:"rate_limit_rps,omitempty"`
	Burst        *int               `json:"burst,omitempty"`
	Headers      *map[string]string `json:"headers,omitempty"`
}

// UpdateResourceResponse indicates whether a resource update succeeded.
type UpdateResourceResponse struct {
	ID      string `json:"id"`
	Updated bool   `json:"updated"`
}

// ToggleResourceResponse indicates whether a resource state was changed.
type ToggleResourceResponse struct {
	ID      string `json:"id"`
	Paused  bool   `json:"paused,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// ListEndpointsResponse represents the response from listing endpoints.
type ListEndpointsResponse struct {
	Endpoints  []EndpointSummary
	HasMore    bool
	NextCursor *string
}

// EndpointsFilter represents filtering options for listing endpoints.
type EndpointsFilter struct {
	Limit  *int
	Cursor *string
}

// SigningKey represents an endpoint signing key.
type SigningKey struct {
	ID        string    `json:"id"`
	KeyHint   string    `json:"key_hint"`
	CreatedAt time.Time `json:"created_at"`
}

// RotateSecretResponse is retained as a compatibility alias for created signing keys.
type RotateSecretResponse struct {
	ID            string    `json:"id"`
	SigningSecret string    `json:"signing_secret"`
	KeyHint       string    `json:"key_hint"`
	CreatedAt     time.Time `json:"created_at"`
}

// Project represents a project.
type Project struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	RateLimitDefault int       `json:"rate_limit_default"`
	CreatedAt        time.Time `json:"created_at"`
}

// CreateProjectRequest represents a request to create a project.
type CreateProjectRequest struct {
	Name             string `json:"name"`
	RateLimitDefault *int   `json:"rate_limit_default,omitempty"`
}

// UpdateProjectRequest represents a request to update a project.
type UpdateProjectRequest struct {
	Name             *string `json:"name,omitempty"`
	RateLimitDefault *int    `json:"rate_limit_default,omitempty"`
}

// CreateCheckoutRequest represents a billing checkout request.
type CreateCheckoutRequest struct {
	Plan     string `json:"plan"`
	Interval string `json:"interval"`
}

// CheckoutSession represents a checkout session response.
type CheckoutSession struct {
	SessionID   string `json:"session_id"`
	CheckoutURL string `json:"checkout_url"`
}

// CreatePortalRequest represents a billing portal request.
type CreatePortalRequest struct {
	ReturnURL *string `json:"return_url,omitempty"`
}

// PortalSession represents a customer portal session response.
type PortalSession struct {
	PortalURL string `json:"portal_url"`
}

// SubscriptionLimits describes plan limits.
type SubscriptionLimits struct {
	Plan             string `json:"plan"`
	MessagesPerMonth int    `json:"messages_per_month"`
	MaxProjects      int    `json:"max_projects"`
	MaxEndpoints     int    `json:"max_endpoints"`
	RetentionDays    int    `json:"retention_days"`
}

// SubscriptionUsage describes current billing usage.
type SubscriptionUsage struct {
	MessagesUsed int       `json:"messages_used"`
	PeriodStart  time.Time `json:"period_start"`
	PeriodEnd    time.Time `json:"period_end"`
}

// Subscription represents subscription details.
type Subscription struct {
	Plan              string             `json:"plan"`
	Status            string             `json:"status"`
	Limits            SubscriptionLimits `json:"limits"`
	Usage             SubscriptionUsage  `json:"usage"`
	CancelAtPeriodEnd bool               `json:"cancel_at_period_end"`
	CurrentPeriodEnd  time.Time          `json:"current_period_end"`
}

// UsageHistoryRow represents a billing usage history row.
type UsageHistoryRow struct {
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
	MessageCount int    `json:"message_count"`
	OverageCount int    `json:"overage_count"`
	PlanLimit    *int   `json:"plan_limit,omitempty"`
}

// UsageHistoryResponse represents paginated usage history.
type UsageHistoryResponse struct {
	Rows    []UsageHistoryRow
	Total   int
	Limit   int
	Offset  int
	HasMore bool
}

// InvoiceLine represents a single invoice line.
type InvoiceLine struct {
	Description string `json:"description"`
	Amount      int    `json:"amount"`
	Quantity    int    `json:"quantity"`
}

// Invoice represents an invoice.
type Invoice struct {
	ID               string        `json:"id"`
	Status           string        `json:"status"`
	AmountDue        int           `json:"amount_due"`
	AmountPaid       int           `json:"amount_paid"`
	Currency         string        `json:"currency"`
	PeriodStart      time.Time     `json:"period_start"`
	PeriodEnd        time.Time     `json:"period_end"`
	Created          time.Time     `json:"created"`
	InvoicePDF       *string       `json:"invoice_pdf,omitempty"`
	HostedInvoiceURL *string       `json:"hosted_invoice_url,omitempty"`
	Lines            []InvoiceLine `json:"lines"`
}

// InvoicesResponse represents invoice history.
type InvoicesResponse struct {
	Invoices []Invoice
	HasMore  bool
}

// InboundEndpoint represents a full inbound endpoint.
type InboundEndpoint struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Description            *string   `json:"description,omitempty"`
	URL                    string    `json:"url"`
	Active                 bool      `json:"active"`
	Paused                 bool      `json:"paused"`
	VerifyStaticToken      bool      `json:"verify_static_token"`
	VerifyHMAC             bool      `json:"verify_hmac"`
	VerifyIPAllowlist      bool      `json:"verify_ip_allowlist"`
	IngestResponseCode     int       `json:"ingest_response_code"`
	IdempotencyHeaderNames []string  `json:"idempotency_header_names"`
	SigningEnabled         bool      `json:"signing_enabled"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// InboundEndpointSummary represents a summary inbound endpoint.
type InboundEndpointSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Active    bool      `json:"active"`
	Paused    bool      `json:"paused"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateInboundEndpointRequest represents a request to create an inbound endpoint.
type CreateInboundEndpointRequest struct {
	Name                   *string   `json:"name,omitempty"`
	Description            *string   `json:"description,omitempty"`
	URL                    string    `json:"url"`
	VerifyStaticToken      *bool     `json:"verify_static_token,omitempty"`
	TokenHeaderName        *string   `json:"token_header_name,omitempty"`
	TokenQueryParam        *string   `json:"token_query_param,omitempty"`
	TokenValue             *string   `json:"token_value,omitempty"`
	VerifyHMAC             *bool     `json:"verify_hmac,omitempty"`
	HMACHeaderName         *string   `json:"hmac_header_name,omitempty"`
	HMACSecret             *string   `json:"hmac_secret,omitempty"`
	TimestampHeaderName    *string   `json:"timestamp_header_name,omitempty"`
	TimestampTTLSeconds    *int      `json:"timestamp_ttl_seconds,omitempty"`
	VerifyIPAllowlist      *bool     `json:"verify_ip_allowlist,omitempty"`
	AllowedCIDRs           *[]string `json:"allowed_cidrs,omitempty"`
	IdempotencyHeaderNames *[]string `json:"idempotency_header_names,omitempty"`
	IngestResponseCode     *int      `json:"ingest_response_code,omitempty"`
	SigningEnabled         *bool     `json:"signing_enabled,omitempty"`
}

// CreateInboundEndpointResponse represents the one-time inbound endpoint creation payload.
type CreateInboundEndpointResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	IngestURL   string    `json:"ingest_url"`
	SecretToken string    `json:"secret_token"`
	CreatedAt   time.Time `json:"created_at"`
}

// UpdateInboundEndpointRequest represents a request to update an inbound endpoint.
type UpdateInboundEndpointRequest struct {
	Name                   *string   `json:"name,omitempty"`
	Description            *string   `json:"description,omitempty"`
	URL                    *string   `json:"url,omitempty"`
	VerifyStaticToken      *bool     `json:"verify_static_token,omitempty"`
	TokenHeaderName        *string   `json:"token_header_name,omitempty"`
	TokenQueryParam        *string   `json:"token_query_param,omitempty"`
	TokenValue             *string   `json:"token_value,omitempty"`
	VerifyHMAC             *bool     `json:"verify_hmac,omitempty"`
	HMACHeaderName         *string   `json:"hmac_header_name,omitempty"`
	HMACSecret             *string   `json:"hmac_secret,omitempty"`
	TimestampHeaderName    *string   `json:"timestamp_header_name,omitempty"`
	TimestampTTLSeconds    *int      `json:"timestamp_ttl_seconds,omitempty"`
	VerifyIPAllowlist      *bool     `json:"verify_ip_allowlist,omitempty"`
	AllowedCIDRs           *[]string `json:"allowed_cidrs,omitempty"`
	IdempotencyHeaderNames *[]string `json:"idempotency_header_names,omitempty"`
	IngestResponseCode     *int      `json:"ingest_response_code,omitempty"`
	SigningEnabled         *bool     `json:"signing_enabled,omitempty"`
}

// ListInboundEndpointsResponse represents paginated inbound endpoints.
type ListInboundEndpointsResponse struct {
	Endpoints  []InboundEndpointSummary
	HasMore    bool
	NextCursor *string
}

// InboundEndpointsFilter represents inbound endpoint list filters.
type InboundEndpointsFilter struct {
	Limit  *int
	Cursor *string
}

// InboundReplayAllRequest represents replay-all filters for inbound messages.
type InboundReplayAllRequest struct {
	Status            MessageStatus
	InboundEndpointID *string
	Limit             *int
}

// InboundLogsFilter represents filters for inbound logs.
type InboundLogsFilter struct {
	Status            *MessageStatus
	InboundEndpointID *string
	StartTime         *time.Time
	EndTime           *time.Time
	Limit             *int
	Cursor            *string
}

// InboundLogEntry represents a single inbound log entry.
type InboundLogEntry struct {
	MessageID         string        `json:"message_id"`
	InboundEndpointID string        `json:"inbound_endpoint_id"`
	Endpoint          string        `json:"endpoint"`
	Status            MessageStatus `json:"status"`
	AttemptCount      int           `json:"attempt_count"`
	ReceivedAt        time.Time     `json:"received_at"`
	DeliveredAt       *time.Time    `json:"delivered_at,omitempty"`
	ResponseStatus    *int          `json:"response_status,omitempty"`
	ResponseLatencyMs *int          `json:"response_latency_ms,omitempty"`
	LastError         *string       `json:"last_error,omitempty"`
	TotalDeliveryMs   *int          `json:"total_delivery_ms,omitempty"`
}

// InboundLogsResponse represents paginated inbound logs.
type InboundLogsResponse struct {
	Logs       []InboundLogEntry
	HasMore    bool
	NextCursor *string
}

// InboundMetrics represents aggregated inbound metrics.
type InboundMetrics struct {
	Window            MetricsWindow `json:"window"`
	TotalMessages     int           `json:"total_messages"`
	Succeeded         int           `json:"succeeded"`
	Failed            int           `json:"failed"`
	Retries           int           `json:"retries"`
	SuccessRate       float64       `json:"success_rate"`
	AvgLatencyMs      int           `json:"avg_latency_ms"`
	AvgDeliveryTimeMs int           `json:"avg_delivery_time_ms"`
}

// InboundRejection represents a rejected inbound request.
type InboundRejection struct {
	ID                string    `json:"id"`
	ReasonCode        string    `json:"reason_code"`
	ReceivedAt        time.Time `json:"received_at"`
	InboundEndpointID *string   `json:"inbound_endpoint_id,omitempty"`
	ReasonDetail      *string   `json:"reason_detail,omitempty"`
	SourceIP          *string   `json:"source_ip,omitempty"`
	HeadersRedacted   *string   `json:"headers_redacted,omitempty"`
}

// InboundRejectionsFilter represents filters for inbound rejections.
type InboundRejectionsFilter struct {
	InboundEndpointID *string
	StartTime         *time.Time
	EndTime           *time.Time
	Limit             *int
	Cursor            *string
}

// InboundRejectionsResponse represents paginated inbound rejections.
type InboundRejectionsResponse struct {
	Rejections []InboundRejection
	HasMore    bool
	NextCursor *string
}

// CreateExportRequest represents an export creation request.
type CreateExportRequest struct {
	StartTime  time.Time      `json:"start_time"`
	EndTime    time.Time      `json:"end_time"`
	Status     *MessageStatus `json:"status,omitempty"`
	EndpointID *string        `json:"endpoint_id,omitempty"`
}

// ExportRecord represents an export job.
type ExportRecord struct {
	ID               string         `json:"id"`
	ProjectID        string         `json:"project_id"`
	Status           string         `json:"status"`
	FilterStartTime  time.Time      `json:"filter_start_time"`
	FilterEndTime    time.Time      `json:"filter_end_time"`
	FilterStatus     *MessageStatus `json:"filter_status,omitempty"`
	FilterEndpointID *string        `json:"filter_endpoint_id,omitempty"`
	RowCount         *int           `json:"row_count,omitempty"`
	FileSizeBytes    *int           `json:"file_size_bytes,omitempty"`
	ErrorMessage     *string        `json:"error_message,omitempty"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	ExpiresAt        *time.Time     `json:"expires_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

// DownloadExportResponse contains the presigned download URL returned by the redirect.
type DownloadExportResponse struct {
	URL string
}

// apiResponse wraps all API responses.
type apiResponse[T any] struct {
	Data  T            `json:"data"`
	Meta  responseMeta `json:"meta"`
	Error *apiError    `json:"error,omitempty"`
}

type responseMeta struct {
	RequestID  string  `json:"request_id"`
	HasMore    bool    `json:"has_more,omitempty"`
	NextCursor *string `json:"next_cursor,omitempty"`
	Total      int     `json:"total,omitempty"`
	Limit      int     `json:"limit,omitempty"`
	Offset     int     `json:"offset,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type dlqDataResponse struct {
	Messages   []DLQMessage `json:"messages"`
	HasMore    bool         `json:"has_more"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
