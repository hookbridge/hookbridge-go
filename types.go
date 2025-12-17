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
	Endpoint       string            `json:"endpoint"`
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
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// dlqDataResponse is the internal structure for DLQ responses.
type dlqDataResponse struct {
	Messages   []DLQMessage `json:"messages"`
	HasMore    bool         `json:"has_more"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}
