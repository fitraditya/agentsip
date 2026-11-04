package openaicompat_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fitraditya/agentsip/internal/adapter/llm/openaicompat"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

func sseServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer test-key")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		for _, line := range lines {
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
	}))
}

func drain(t *testing.T, ch <-chan conversation.LLMDelta) []conversation.LLMDelta {
	t.Helper()
	var out []conversation.LLMDelta
	for {
		select {
		case d, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, d)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for delta stream")
		}
	}
}

func TestStreamChat_ContentDeltas(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"content":"Hel"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":"lo"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	llm := openaicompat.New()
	deltas, err := llm.StreamChat(t.Context(), conversation.LLMConfig{BaseURL: srv.URL, APIKey: "test-key", Model: "gpt-4o-mini"}, []conversation.LLMMessage{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	got := drain(t, deltas)
	want := "Hello"
	var gotText string
	doneSeen := false
	for _, d := range got {
		gotText += d.ContentDelta
		if d.Done {
			doneSeen = true
		}
	}
	if gotText != want {
		t.Fatalf("content = %q, want %q", gotText, want)
	}
	if !doneSeen {
		t.Fatalf("expected a terminal Done delta, got %+v", got)
	}
}

func TestStreamChat_ToolCallAccumulatesAcrossChunks(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"search_knowledge_base","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"refund policy\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	llm := openaicompat.New()
	deltas, err := llm.StreamChat(t.Context(), conversation.LLMConfig{BaseURL: srv.URL, APIKey: "test-key"}, []conversation.LLMMessage{{Role: "user", Content: "what's your refund policy?"}}, []conversation.ToolSpec{{Name: "search_knowledge_base"}})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	got := drain(t, deltas)
	var toolCall *conversation.ToolCall
	for _, d := range got {
		if d.ToolCall != nil {
			toolCall = d.ToolCall
		}
	}
	if toolCall == nil {
		t.Fatalf("expected a tool call delta, got %+v", got)
	}
	if toolCall.Name != "search_knowledge_base" {
		t.Errorf("tool call name = %q, want %q", toolCall.Name, "search_knowledge_base")
	}
	wantArgs := `{"query":"refund policy"}`
	if toolCall.Arguments != wantArgs {
		t.Errorf("tool call arguments = %q, want %q", toolCall.Arguments, wantArgs)
	}
}

func TestStreamChat_NonOKStatusReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	llm := openaicompat.New()
	_, err := llm.StreamChat(t.Context(), conversation.LLMConfig{BaseURL: srv.URL, APIKey: "bad-key"}, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a 401 response, got nil")
	}
}
