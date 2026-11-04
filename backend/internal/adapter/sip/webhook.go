package sip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// webhookDeliveryTimeout bounds only getting the notification to the
// external service, not how long the call then waits for a decision — that's
// the trunk's WebhookTimeoutSeconds, a separate and usually much longer
// window.
const webhookDeliveryTimeout = 5 * time.Second

// IncomingCallWebhookPayload is what a webhook-mode trunk's WebhookURL
// receives for an incoming call — the same role as OpenAI's
// realtime.call.incoming event, scoped to what the receiver needs to decide
// whether to accept it and to call the admin API's accept/reject endpoints
// (usecase/call.Decisions) with the right IDs.
type IncomingCallWebhookPayload struct {
	PendingCallID string `json:"pending_call_id"`
	TrunkID       string `json:"trunk_id"`
	SIPCallID     string `json:"sip_call_id"`
	CallerNumber  string `json:"caller_number"`
	CalleeNumber  string `json:"callee_number"`
}

// notifyIncomingCallWebhook POSTs the incoming-call notification and reports
// only whether delivery succeeded (2xx). The receiving service communicates
// its actual accept/reject decision back separately, through the admin API's
// accept/reject endpoints — not through this request's response body, which
// mirrors OpenAI's split between the webhook delivery and the accept/reject
// API calls.
func notifyIncomingCallWebhook(ctx context.Context, url string, payload IncomingCallWebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sip: marshal webhook payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, webhookDeliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sip: build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sip: deliver webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sip: webhook endpoint returned status %d", resp.StatusCode)
	}
	return nil
}
