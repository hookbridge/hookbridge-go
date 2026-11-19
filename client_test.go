package hookbridge_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/hookbridge/hookbridge-go/v2"
)

// Test configuration from environment variables
var (
	apiKey          = os.Getenv("HOOKBRIDGE_API_KEY")
	baseURL         = os.Getenv("HOOKBRIDGE_BASE_URL")
	sendURL         = os.Getenv("HOOKBRIDGE_SEND_URL")
	testEndpointURL = os.Getenv("HOOKBRIDGE_TEST_ENDPOINT_URL")
)

func init() {
	if baseURL == "" {
		baseURL = "https://api.hookbridge.io"
	}
	if sendURL == "" {
		sendURL = "https://send.hookbridge.io"
	}
	if testEndpointURL == "" {
		testEndpointURL = "https://example.com/webhooks/test"
	}
}

func skipIfNoCredentials(t *testing.T) {
	t.Helper()
	if apiKey == "" {
		t.Skip("Skipping: HOOKBRIDGE_API_KEY not set")
	}
}

func newTestClient(t *testing.T) *hookbridge.Client {
	t.Helper()
	skipIfNoCredentials(t)
	client, err := hookbridge.NewClient(apiKey, hookbridge.WithBaseURL(baseURL), hookbridge.WithSendURL(sendURL))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	return client
}

// TestSessionOnlyMethodsRemoved verifies the session-cookie-only console
// methods (which always 401 against an app API key) were removed, while the
// API-key-compatible billing methods remain.
func TestSessionOnlyMethodsRemoved(t *testing.T) {
	c := &hookbridge.Client{}
	for _, name := range []string{"ListProjects", "CreateProject", "GetProject", "UpdateProject", "DeleteProject", "CreateCheckout", "CreatePortal"} {
		if _, ok := reflect.TypeOf(c).MethodByName(name); ok {
			t.Errorf("%s should have been removed", name)
		}
	}
	for _, name := range []string{"GetSubscription", "GetUsageHistory", "GetInvoices"} {
		if _, ok := reflect.TypeOf(c).MethodByName(name); !ok {
			t.Errorf("%s must still exist", name)
		}
	}
}

// TestClientConfiguration tests client creation and configuration
func TestClientConfiguration(t *testing.T) {
	t.Run("should create client with API key", func(t *testing.T) {
		skipIfNoCredentials(t)
		client, err := hookbridge.NewClient(apiKey)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		if client == nil {
			t.Fatal("Expected client to be non-nil")
		}
	})

	t.Run("should return ValidationError when API key is empty", func(t *testing.T) {
		_, err := hookbridge.NewClient("")
		if err == nil {
			t.Fatal("Expected error for empty API key")
		}
		var validationErr *hookbridge.ValidationError
		if !errors.As(err, &validationErr) {
			t.Errorf("Expected ValidationError, got %T", err)
		}
	})

	t.Run("should create client with custom base URL", func(t *testing.T) {
		skipIfNoCredentials(t)
		client, err := hookbridge.NewClient(apiKey, hookbridge.WithBaseURL("https://custom.example.com/"))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		if client == nil {
			t.Fatal("Expected client to be non-nil")
		}
	})

	t.Run("should create client with custom timeout", func(t *testing.T) {
		skipIfNoCredentials(t)
		client, err := hookbridge.NewClient(apiKey, hookbridge.WithTimeout(60*time.Second))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		if client == nil {
			t.Fatal("Expected client to be non-nil")
		}
	})
}

// TestWebhookSending tests the Send endpoint
func TestWebhookSending(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Create a test endpoint for webhook sending tests
	desc := "Integration test endpoint (Go)"
	endpoint, err := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
		URL:         testEndpointURL,
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("Failed to create test endpoint: %v", err)
	}
	defer func() {
		_ = client.DeleteEndpoint(ctx, endpoint.ID)
	}()

	t.Run("should send a webhook and return message ID", func(t *testing.T) {
		result, err := client.Send(ctx, hookbridge.SendRequest{
			EndpointID: endpoint.ID,
			Payload: map[string]any{
				"event":     "test.integration.go",
				"timestamp": time.Now().Format(time.RFC3339),
				"data":      map[string]bool{"test": true},
			},
		})
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		if result.MessageID == "" {
			t.Error("Expected message ID to be non-empty")
		}
		if result.Status != hookbridge.StatusQueued {
			t.Errorf("Expected status 'queued', got '%s'", result.Status)
		}
	})

	t.Run("should send a webhook with custom headers", func(t *testing.T) {
		result, err := client.Send(ctx, hookbridge.SendRequest{
			EndpointID: endpoint.ID,
			Payload:    map[string]string{"event": "test.headers"},
			Headers: map[string]string{
				"X-Custom-Header": "test-value",
				"X-Request-Id":    "integration-test-go-123",
			},
		})
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		if result.MessageID == "" {
			t.Error("Expected message ID to be non-empty")
		}
	})

	t.Run("should send a webhook with idempotency key", func(t *testing.T) {
		idempotencyKey := fmt.Sprintf("test-go-%d-%d", time.Now().UnixNano(), rand.Int())

		result1, err := client.Send(ctx, hookbridge.SendRequest{
			EndpointID:     endpoint.ID,
			Payload:        map[string]string{"event": "test.idempotent", "key": idempotencyKey},
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			t.Fatalf("First send failed: %v", err)
		}

		// Same request with same idempotency key should return same message ID
		result2, err := client.Send(ctx, hookbridge.SendRequest{
			EndpointID:     endpoint.ID,
			Payload:        map[string]string{"event": "test.idempotent", "key": idempotencyKey},
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			t.Fatalf("Second send failed: %v", err)
		}

		if result1.MessageID != result2.MessageID {
			t.Errorf("Expected same message ID for idempotent request, got %s and %s", result1.MessageID, result2.MessageID)
		}
	})

	t.Run("should reject invalid endpoint ID", func(t *testing.T) {
		_, err := client.Send(ctx, hookbridge.SendRequest{
			EndpointID: "ep_nonexistent_12345",
			Payload:    map[string]string{"event": "test.invalid"},
		})
		if err == nil {
			t.Fatal("Expected error for invalid endpoint ID")
		}
		var notFoundErr *hookbridge.NotFoundError
		if !errors.As(err, &notFoundErr) {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// TestMessageOperations tests message retrieval and operations
func TestMessageOperations(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Create a test endpoint
	desc := "Message ops test endpoint (Go)"
	endpoint, err := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
		URL:         testEndpointURL,
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("Failed to create test endpoint: %v", err)
	}
	defer func() {
		_ = client.DeleteEndpoint(ctx, endpoint.ID)
	}()

	// Create a test message
	sendResult, err := client.Send(ctx, hookbridge.SendRequest{
		EndpointID: endpoint.ID,
		Payload: map[string]any{
			"event":     "test.message-ops.go",
			"timestamp": time.Now().UnixNano(),
		},
	})
	if err != nil {
		t.Fatalf("Failed to create test message: %v", err)
	}
	testMessageID := sendResult.MessageID

	// Give it a moment to be processed
	time.Sleep(1 * time.Second)

	t.Run("should get message details", func(t *testing.T) {
		message, err := client.GetMessage(ctx, testMessageID)
		if err != nil {
			t.Fatalf("GetMessage failed: %v", err)
		}

		if message.ID != testMessageID {
			t.Errorf("Expected message ID %s, got %s", testMessageID, message.ID)
		}
		if message.ProjectID == "" {
			t.Error("Expected project ID to be non-empty")
		}
		validStatuses := []hookbridge.MessageStatus{
			hookbridge.StatusQueued,
			hookbridge.StatusDelivering,
			hookbridge.StatusSucceeded,
			hookbridge.StatusPendingRetry,
			hookbridge.StatusFailedPermanent,
		}
		statusValid := false
		for _, s := range validStatuses {
			if message.Status == s {
				statusValid = true
				break
			}
		}
		if !statusValid {
			t.Errorf("Got invalid status: %s", message.Status)
		}
	})

	t.Run("should return NotFoundError for non-existent message", func(t *testing.T) {
		fakeID := "01935abc-def0-7123-4567-890abcdef012"
		_, err := client.GetMessage(ctx, fakeID)
		if err == nil {
			t.Fatal("Expected error for non-existent message")
		}
		var notFoundErr *hookbridge.NotFoundError
		if !errors.As(err, &notFoundErr) {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// TestLogs tests the logs endpoint
func TestLogs(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	t.Run("should get delivery logs", func(t *testing.T) {
		logs, err := client.GetLogs(ctx, nil)
		if err != nil {
			t.Fatalf("GetLogs failed: %v", err)
		}

		if logs.Messages == nil {
			t.Error("Expected messages to be non-nil")
		}
	})

	t.Run("should filter logs by status", func(t *testing.T) {
		status := hookbridge.StatusSucceeded
		logs, err := client.GetLogs(ctx, &hookbridge.LogsFilter{
			Status: &status,
		})
		if err != nil {
			t.Fatalf("GetLogs failed: %v", err)
		}

		for _, msg := range logs.Messages {
			if msg.Status != hookbridge.StatusSucceeded {
				t.Errorf("Expected status 'succeeded', got '%s'", msg.Status)
			}
		}
	})

	t.Run("should filter logs by time range", func(t *testing.T) {
		now := time.Now()
		oneDayAgo := now.Add(-24 * time.Hour)
		limit := 10

		logs, err := client.GetLogs(ctx, &hookbridge.LogsFilter{
			StartTime: &oneDayAgo,
			EndTime:   &now,
			Limit:     &limit,
		})
		if err != nil {
			t.Fatalf("GetLogs failed: %v", err)
		}

		if len(logs.Messages) > 10 {
			t.Errorf("Expected at most 10 messages, got %d", len(logs.Messages))
		}
	})

	t.Run("should paginate logs", func(t *testing.T) {
		limit := 5
		firstPage, err := client.GetLogs(ctx, &hookbridge.LogsFilter{
			Limit: &limit,
		})
		if err != nil {
			t.Fatalf("GetLogs (first page) failed: %v", err)
		}

		if firstPage.HasMore && firstPage.NextCursor != nil {
			secondPage, err := client.GetLogs(ctx, &hookbridge.LogsFilter{
				Limit:  &limit,
				Cursor: firstPage.NextCursor,
			})
			if err != nil {
				t.Fatalf("GetLogs (second page) failed: %v", err)
			}

			// Messages should be different
			if len(firstPage.Messages) > 0 && len(secondPage.Messages) > 0 {
				if firstPage.Messages[0].MessageID == secondPage.Messages[0].MessageID {
					t.Error("Expected different messages on second page")
				}
			}
		}
	})
}

// TestMetrics tests the metrics endpoint
func TestMetrics(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	t.Run("should get metrics with default window", func(t *testing.T) {
		metrics, err := client.GetMetrics(ctx, "")
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}

		if metrics.Window != hookbridge.Window24Hour {
			t.Errorf("Expected window '24h', got '%s'", metrics.Window)
		}
	})

	t.Run("should get metrics for different time windows", func(t *testing.T) {
		windows := []hookbridge.MetricsWindow{
			hookbridge.Window1Hour,
			hookbridge.Window24Hour,
			hookbridge.Window7Day,
			hookbridge.Window30Day,
		}

		for _, window := range windows {
			metrics, err := client.GetMetrics(ctx, window)
			if err != nil {
				t.Fatalf("GetMetrics(%s) failed: %v", window, err)
			}
			if metrics.Window != window {
				t.Errorf("Expected window '%s', got '%s'", window, metrics.Window)
			}
		}
	})
}

// TestDLQ tests the Dead Letter Queue endpoints
func TestDLQ(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	t.Run("should get DLQ messages", func(t *testing.T) {
		dlq, err := client.GetDLQMessages(ctx, nil)
		if err != nil {
			t.Fatalf("GetDLQMessages failed: %v", err)
		}

		if dlq.Messages == nil {
			t.Error("Expected messages to be non-nil")
		}
	})

	t.Run("should get DLQ messages with limit", func(t *testing.T) {
		limit := 10
		dlq, err := client.GetDLQMessages(ctx, &hookbridge.DLQFilter{
			Limit: &limit,
		})
		if err != nil {
			t.Fatalf("GetDLQMessages failed: %v", err)
		}

		if len(dlq.Messages) > 10 {
			t.Errorf("Expected at most 10 messages, got %d", len(dlq.Messages))
		}
	})
}

// TestAPIKeyManagement tests API key CRUD operations
func TestAPIKeyManagement(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	t.Run("should list API keys", func(t *testing.T) {
		keys, err := client.ListAPIKeys(ctx)
		if err != nil {
			t.Fatalf("ListAPIKeys failed: %v", err)
		}

		if len(keys) == 0 {
			t.Error("Expected at least one API key")
		}

		key := keys[0]
		if key.KeyID == "" {
			t.Error("Expected key ID to be non-empty")
		}
		if key.Prefix == "" {
			t.Error("Expected key prefix to be non-empty")
		}
	})

	t.Run("should create and delete an API key", func(t *testing.T) {
		label := "Integration Test Key (Go)"
		newKey, err := client.CreateAPIKey(ctx, hookbridge.ModeTest, &label)
		if err != nil {
			t.Fatalf("CreateAPIKey failed: %v", err)
		}

		if newKey.KeyID == "" {
			t.Error("Expected key ID to be non-empty")
		}
		if newKey.Key == "" {
			t.Error("Expected key secret to be non-empty")
		}
		if newKey.Label == nil || *newKey.Label != label {
			t.Errorf("Expected label '%s', got %v", label, newKey.Label)
		}

		// Delete the key we just created
		err = client.DeleteAPIKey(ctx, newKey.KeyID)
		if err != nil {
			t.Fatalf("DeleteAPIKey failed: %v", err)
		}

		// Verify it's deleted by checking the list
		keysAfterDelete, err := client.ListAPIKeys(ctx)
		if err != nil {
			t.Fatalf("ListAPIKeys after delete failed: %v", err)
		}

		for _, k := range keysAfterDelete {
			if k.KeyID == newKey.KeyID {
				t.Error("Expected key to be deleted")
			}
		}
	})

	t.Run("should return NotFoundError when deleting non-existent key", func(t *testing.T) {
		err := client.DeleteAPIKey(ctx, "key_nonexistent_12345")
		if err == nil {
			t.Fatal("Expected error for non-existent key")
		}
		var notFoundErr *hookbridge.NotFoundError
		if !errors.As(err, &notFoundErr) {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// TestEndpointManagement tests endpoint CRUD operations
func TestEndpointManagement(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	t.Run("should create, get, update, and delete an endpoint", func(t *testing.T) {
		// Create
		desc := "Test endpoint for CRUD operations"
		endpoint, err := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
			URL:         "https://example.com/webhooks/test-endpoint",
			Description: &desc,
		})
		if err != nil {
			t.Fatalf("CreateEndpoint failed: %v", err)
		}

		if endpoint.ID == "" {
			t.Error("Expected endpoint ID to be non-empty")
		}
		if endpoint.URL != "https://example.com/webhooks/test-endpoint" {
			t.Errorf("Expected URL to match, got %s", endpoint.URL)
		}
		if endpoint.SigningSecret == "" {
			t.Error("Expected signing secret to be non-empty")
		}

		// Get
		retrieved, err := client.GetEndpoint(ctx, endpoint.ID)
		if err != nil {
			t.Fatalf("GetEndpoint failed: %v", err)
		}
		if retrieved.ID != endpoint.ID {
			t.Errorf("Expected ID %s, got %s", endpoint.ID, retrieved.ID)
		}

		// Update
		newDesc := "Updated description"
		updated, err := client.UpdateEndpoint(ctx, endpoint.ID, hookbridge.UpdateEndpointRequest{
			Description: &newDesc,
		})
		if err != nil {
			t.Fatalf("UpdateEndpoint failed: %v", err)
		}
		if updated.ID != endpoint.ID || !updated.Updated {
			t.Errorf("Expected update confirmation, got %+v", updated)
		}

		// Delete
		err = client.DeleteEndpoint(ctx, endpoint.ID)
		if err != nil {
			t.Fatalf("DeleteEndpoint failed: %v", err)
		}

		// Verify deleted
		_, err = client.GetEndpoint(ctx, endpoint.ID)
		if err == nil {
			t.Fatal("Expected error when getting deleted endpoint")
		}
		var notFoundErr *hookbridge.NotFoundError
		if !errors.As(err, &notFoundErr) {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("should list endpoints", func(t *testing.T) {
		// Create a test endpoint
		desc := "List test endpoint"
		endpoint, err := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
			URL:         "https://example.com/webhooks/list-test",
			Description: &desc,
		})
		if err != nil {
			t.Fatalf("CreateEndpoint failed: %v", err)
		}
		defer func() {
			_ = client.DeleteEndpoint(ctx, endpoint.ID)
		}()

		list, err := client.ListEndpoints(ctx, nil)
		if err != nil {
			t.Fatalf("ListEndpoints failed: %v", err)
		}

		found := false
		for _, ep := range list.Endpoints {
			if ep.ID == endpoint.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find created endpoint in list")
		}
	})

	t.Run("should rotate endpoint secret", func(t *testing.T) {
		desc := "Rotate test endpoint"
		endpoint, err := client.CreateEndpoint(ctx, hookbridge.CreateEndpointRequest{
			URL:         "https://example.com/webhooks/rotate-test",
			Description: &desc,
		})
		if err != nil {
			t.Fatalf("CreateEndpoint failed: %v", err)
		}
		defer func() {
			_ = client.DeleteEndpoint(ctx, endpoint.ID)
		}()

		originalSecret := endpoint.SigningSecret

		rotated, err := client.RotateEndpointSecret(ctx, endpoint.ID)
		if err != nil {
			t.Fatalf("RotateEndpointSecret failed: %v", err)
		}

		if rotated.ID != endpoint.ID {
			t.Errorf("Expected ID %s, got %s", endpoint.ID, rotated.ID)
		}
		if rotated.SigningSecret == "" {
			t.Error("Expected new signing secret to be non-empty")
		}
		if rotated.SigningSecret == originalSecret {
			t.Error("Expected signing secret to change after rotation")
		}
	})
}

// TestAuthentication tests authentication error handling
func TestAuthentication(t *testing.T) {
	skipIfNoCredentials(t)
	ctx := context.Background()

	t.Run("should reject invalid API key", func(t *testing.T) {
		badClient, err := hookbridge.NewClient("hb_test_invalid_key_12345", hookbridge.WithBaseURL(baseURL))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		_, err = badClient.GetLogs(ctx, nil)
		if err == nil {
			t.Fatal("Expected error for invalid API key")
		}
		var authErr *hookbridge.AuthenticationError
		if !errors.As(err, &authErr) {
			t.Errorf("Expected AuthenticationError, got %T: %v", err, err)
		}
	})
}
