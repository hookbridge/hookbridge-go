package hookbridge

import "fmt"

// APIError is the base error type for all HookBridge API errors.
type APIError struct {
	Code       string
	Message    string
	RequestID  string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("hookbridge: %s: %s (request_id: %s)", e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("hookbridge: %s: %s", e.Code, e.Message)
}

// AuthenticationError indicates an invalid or missing API key.
type AuthenticationError struct {
	*APIError
}

// NotFoundError indicates the requested resource was not found.
type NotFoundError struct {
	*APIError
}

// ValidationError indicates the request was invalid.
type ValidationError struct {
	*APIError
}

// RateLimitError indicates the rate limit was exceeded.
type RateLimitError struct {
	*APIError
}

// IdempotencyError indicates an idempotency key conflict.
type IdempotencyError struct {
	*APIError
}

// ReplayLimitError indicates the replay limit was exceeded.
type ReplayLimitError struct {
	*APIError
}

// NetworkError indicates a network-level error.
type NetworkError struct {
	Err error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("hookbridge: network error: %v", e.Err)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

// TimeoutError indicates a request timeout.
type TimeoutError struct {
	Err error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("hookbridge: timeout: %v", e.Err)
}

func (e *TimeoutError) Unwrap() error {
	return e.Err
}

// newErrorFromResponse creates the appropriate error type from an API response.
func newErrorFromResponse(statusCode int, code, message, requestID string) error {
	base := &APIError{
		Code:       code,
		Message:    message,
		RequestID:  requestID,
		StatusCode: statusCode,
	}

	switch statusCode {
	case 401:
		return &AuthenticationError{APIError: base}
	case 404:
		return &NotFoundError{APIError: base}
	case 400:
		return &ValidationError{APIError: base}
	case 429:
		if code == "REPLAY_LIMIT_EXCEEDED" {
			return &ReplayLimitError{APIError: base}
		}
		return &RateLimitError{APIError: base}
	case 409:
		return &IdempotencyError{APIError: base}
	default:
		return base
	}
}
