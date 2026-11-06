// Package openaicompat implements usecase/conversation.LLM against any
// endpoint speaking the OpenAI chat-completions wire protocol — OpenAI
// itself, Azure OpenAI, or a self-hosted OpenAI-compatible server. This is
// what makes the LLM side of a trunk provider-agnostic: there is one adapter,
// selected by BaseURL, not one adapter per vendor.
package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

type LLM struct {
	httpClient *http.Client
	debug      bool
}

func New(debug bool) *LLM {
	return &LLM{
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // total timeout for LLM stream, prevents hanging call forever
		},
		debug: debug,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type tool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Tools    []tool        `json:"tools,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int `json:"index"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (l *LLM) StreamChat(ctx context.Context, cfg conversation.LLMConfig, messages []conversation.LLMMessage, tools []conversation.ToolSpec) (<-chan conversation.LLMDelta, error) {
	reqBody := chatRequest{
		Model:  cfg.Model,
		Stream: true,
	}
	for _, m := range messages {
		reqBody.Messages = append(reqBody.Messages, chatMessage{Role: m.Role, Content: m.Content})
	}
	for _, t := range tools {
		reqBody.Tools = append(reqBody.Tools, tool{
			Type: "function",
			Function: toolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  json.RawMessage(t.ParametersJSONSchema),
			},
		})
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openaicompat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := l.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openaicompat: unexpected status %d: %s", resp.StatusCode, errBody)
	}

	out := make(chan conversation.LLMDelta)
	go streamSSE(resp.Body, out, l.debug)
	return out, nil
}

// toolCallAccum holds one tool call's name and arguments while they stream in
// across multiple chunks, keyed by the index the API assigns it — a response
// may interleave argument fragments for more than one call.
type toolCallAccum struct {
	name string
	args strings.Builder
}

func streamSSE(body io.ReadCloser, out chan<- conversation.LLMDelta, debug bool) {
	defer close(out)
	defer body.Close()

	toolCalls := map[int]*toolCallAccum{}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		if data == "[DONE]" {
			emitToolCalls(out, toolCalls)
			out <- conversation.LLMDelta{Done: true}
			return
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			if debug {
				logging.For("agentsip", "llm").Debug().Str("content", delta.Content).Msg("delta")
			}
			out <- conversation.LLMDelta{ContentDelta: delta.Content}
		}
		for _, tc := range delta.ToolCalls {
			acc, ok := toolCalls[tc.Index]
			if !ok {
				acc = &toolCallAccum{}
				toolCalls[tc.Index] = acc
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.args.WriteString(tc.Function.Arguments)
		}
	}
	// Stream ended without a [DONE] sentinel (connection closed early, or a
	// scanner error). Log error and still emit accumulated tool calls.
	if err := scanner.Err(); err != nil {
		logging.For("agentsip", "llm").Warn().Err(err).Msg("llm sse scanner error")
	}
	emitToolCalls(out, toolCalls)
	out <- conversation.LLMDelta{Done: true}
}

func emitToolCalls(out chan<- conversation.LLMDelta, toolCalls map[int]*toolCallAccum) {
	for _, acc := range toolCalls {
		if acc.name == "" {
			continue
		}
		out <- conversation.LLMDelta{ToolCall: &conversation.ToolCall{Name: acc.name, Arguments: acc.args.String()}}
	}
}
