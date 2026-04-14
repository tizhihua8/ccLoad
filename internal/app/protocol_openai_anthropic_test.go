package app

import (
	"testing"

	"github.com/bytedance/sonic"
)
func TestOpenAIToAnthropicRequestConversion(t *testing.T) {
	converter := NewOpenAIAnthropicConverter(NewModelMapping())

	tests := []struct {
		name           string
		openAIReq      string
		expectedModel  string
		checkFields    func(t *testing.T, req *AnthropicMessagesRequest)
	}{
		{
			name: "basic text conversion",
			openAIReq: `{
				"model": "gpt-4o",
				"messages": [
					{"role": "user", "content": "Hello Claude"}
				]
			}`,
			expectedModel: "claude-3-5-sonnet-20241022",
			checkFields: func(t *testing.T, req *AnthropicMessagesRequest) {
				if len(req.Messages) != 1 {
					t.Errorf("expected 1 message, got %d", len(req.Messages))
				}
				if req.Messages[0].Role != "user" {
					t.Errorf("expected role 'user', got %s", req.Messages[0].Role)
				}
			},
		},
		{
			name: "system message extraction",
			openAIReq: `{
				"model": "gpt-4o",
				"messages": [
					{"role": "system", "content": "You are helpful"},
					{"role": "user", "content": "Hello"}
				]
			}`,
			expectedModel: "claude-3-5-sonnet-20241022",
			checkFields: func(t *testing.T, req *AnthropicMessagesRequest) {
				// System should be in top-level system field
				if req.System != "You are helpful" {
					t.Errorf("expected system prompt 'You are helpful', got %v", req.System)
				}
				// Should only have user message in messages array
				if len(req.Messages) != 1 {
					t.Errorf("expected 1 message (user only), got %d", len(req.Messages))
				}
			},
		},
		{
			name: "streaming request",
			openAIReq: `{
				"model": "gpt-4o",
				"messages": [{"role": "user", "content": "Hi"}],
				"stream": true
			}`,
			expectedModel: "claude-3-5-sonnet-20241022",
			checkFields: func(t *testing.T, req *AnthropicMessagesRequest) {
				if !req.Stream {
					t.Error("expected stream=true")
				}
			},
		},
		{
			name: "temperature and top_p conversion",
			openAIReq: `{
				"model": "gpt-4o",
				"messages": [{"role": "user", "content": "Hi"}],
				"temperature": 0.7,
				"top_p": 0.9
			}`,
			expectedModel: "claude-3-5-sonnet-20241022",
			checkFields: func(t *testing.T, req *AnthropicMessagesRequest) {
				if req.Temperature == nil || *req.Temperature != 0.7 {
					t.Error("expected temperature=0.7")
				}
				if req.TopP == nil || *req.TopP != 0.9 {
					t.Error("expected top_p=0.9")
				}
			},
		},
		{
			name: "max_tokens conversion",
			openAIReq: `{
				"model": "gpt-4o",
				"messages": [{"role": "user", "content": "Hi"}],
				"max_tokens": 1000
			}`,
			expectedModel: "claude-3-5-sonnet-20241022",
			checkFields: func(t *testing.T, req *AnthropicMessagesRequest) {
				if req.MaxTokens != 1000 {
					t.Errorf("expected max_tokens=1000, got %d", req.MaxTokens)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, path, err := converter.ConvertRequest([]byte(tt.openAIReq), "openai", "anthropic", tt.expectedModel)
			if err != nil {
				t.Fatalf("ConvertRequest failed: %v", err)
			}

			if path != "/v1/messages" {
				t.Errorf("expected path '/v1/messages', got %s", path)
			}

			var anthropicReq AnthropicMessagesRequest
			if err := sonic.Unmarshal(body, &anthropicReq); err != nil {
				t.Fatalf("Failed to unmarshal Anthropic request: %v", err)
			}

			if anthropicReq.Model != tt.expectedModel {
				t.Errorf("expected model %s, got %s", tt.expectedModel, anthropicReq.Model)
			}

			tt.checkFields(t, &anthropicReq)
		})
	}
}

func TestAnthropicToOpenAIRequestConversion(t *testing.T) {
	converter := NewOpenAIAnthropicConverter(NewModelMapping())

	tests := []struct {
		name          string
		anthropicReq  string
		expectedModel string
		checkFields   func(t *testing.T, req *OpenAIChatCompletionRequest)
	}{
		{
			name: "basic text conversion",
			anthropicReq: `{
				"model": "claude-3-5-sonnet",
				"messages": [
					{"role": "user", "content": "Hello GPT"}
				]
			}`,
			expectedModel: "gpt-4o",
			checkFields: func(t *testing.T, req *OpenAIChatCompletionRequest) {
				if len(req.Messages) != 1 {
					t.Errorf("expected 1 message, got %d", len(req.Messages))
				}
				if req.Messages[0].Role != "user" {
					t.Errorf("expected role 'user', got %s", req.Messages[0].Role)
				}
			},
		},
		{
			name: "system message conversion",
			anthropicReq: `{
				"model": "claude-3-5-sonnet",
				"system": "You are helpful",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			expectedModel: "gpt-4o",
			checkFields: func(t *testing.T, req *OpenAIChatCompletionRequest) {
				// Should have system message as first message
				if len(req.Messages) < 1 {
					t.Fatal("expected at least 1 message")
				}
				if req.Messages[0].Role != "system" {
					t.Errorf("expected first message role 'system', got %s", req.Messages[0].Role)
				}
			},
		},
		{
			name: "streaming request",
			anthropicReq: `{
				"model": "claude-3-5-sonnet",
				"messages": [{"role": "user", "content": "Hi"}],
				"stream": true
			}`,
			expectedModel: "gpt-4o",
			checkFields: func(t *testing.T, req *OpenAIChatCompletionRequest) {
				if !req.Stream {
					t.Error("expected stream=true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, path, err := converter.ConvertRequest([]byte(tt.anthropicReq), "anthropic", "openai", tt.expectedModel)
			if err != nil {
				t.Fatalf("ConvertRequest failed: %v", err)
			}

			if path != "/v1/chat/completions" {
				t.Errorf("expected path '/v1/chat/completions', got %s", path)
			}

			var openAIReq OpenAIChatCompletionRequest
			if err := sonic.Unmarshal(body, &openAIReq); err != nil {
				t.Fatalf("Failed to unmarshal OpenAI request: %v", err)
			}

			if openAIReq.Model != tt.expectedModel {
				t.Errorf("expected model %s, got %s", tt.expectedModel, openAIReq.Model)
			}

			tt.checkFields(t, &openAIReq)
		})
	}
}

// =============================================================================
// Response Conversion Tests
// =============================================================================

func TestAnthropicToOpenAIResponseConversion(t *testing.T) {
	converter := NewOpenAIAnthropicConverter(NewModelMapping())

	anthropicResp := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [
			{"type": "text", "text": "Hello! How can I help you today?"}
		],
		"model": "claude-3-5-sonnet-20241022",
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 10,
			"output_tokens": 20
		}
	}`

	result, err := converter.ConvertResponse([]byte(anthropicResp))
	if err != nil {
		t.Fatalf("ConvertResponse failed: %v", err)
	}

	var openAIResp OpenAIChatCompletionResponse
	if err := sonic.Unmarshal(result, &openAIResp); err != nil {
		t.Fatalf("Failed to unmarshal OpenAI response: %v", err)
	}

	// Check basic fields
	if openAIResp.Object != "chat.completion" {
		t.Errorf("expected object='chat.completion', got %s", openAIResp.Object)
	}
	if openAIResp.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("unexpected model: %s", openAIResp.Model)
	}

	// Check usage conversion
	if openAIResp.Usage.PromptTokens != 10 {
		t.Errorf("expected prompt_tokens=10, got %d", openAIResp.Usage.PromptTokens)
	}
	if openAIResp.Usage.CompletionTokens != 20 {
		t.Errorf("expected completion_tokens=20, got %d", openAIResp.Usage.CompletionTokens)
	}
	if openAIResp.Usage.TotalTokens != 30 {
		t.Errorf("expected total_tokens=30, got %d", openAIResp.Usage.TotalTokens)
	}

	// Check message content
	if len(openAIResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(openAIResp.Choices))
	}
	if openAIResp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason='stop', got %s", openAIResp.Choices[0].FinishReason)
	}
}

func TestOpenAIToAnthropicResponseConversion(t *testing.T) {
	converter := NewOpenAIAnthropicConverter(NewModelMapping())

	openAIResp := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-4o",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello! How can I help?"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 15,
			"completion_tokens": 25,
			"total_tokens": 40
		}
	}`

	result, err := converter.ConvertResponse([]byte(openAIResp))
	if err != nil {
		t.Fatalf("ConvertResponse failed: %v", err)
	}

	var anthropicResp AnthropicMessagesResponse
	if err := sonic.Unmarshal(result, &anthropicResp); err != nil {
		t.Fatalf("Failed to unmarshal Anthropic response: %v", err)
	}

	// Check basic fields
	if anthropicResp.Type != "message" {
		t.Errorf("expected type='message', got %s", anthropicResp.Type)
	}
	if anthropicResp.Role != "assistant" {
		t.Errorf("expected role='assistant', got %s", anthropicResp.Role)
	}

	// Check usage conversion
	if anthropicResp.Usage.InputTokens != 15 {
		t.Errorf("expected input_tokens=15, got %d", anthropicResp.Usage.InputTokens)
	}
	if anthropicResp.Usage.OutputTokens != 25 {
		t.Errorf("expected output_tokens=25, got %d", anthropicResp.Usage.OutputTokens)
	}

	// Check content
	if len(anthropicResp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(anthropicResp.Content))
	}
	if anthropicResp.Content[0].Type != "text" {
		t.Errorf("expected content type='text', got %s", anthropicResp.Content[0].Type)
	}

	// Check stop_reason conversion (stop -> end_turn)
	if anthropicResp.StopReason == nil || *anthropicResp.StopReason != "end_turn" {
		stopReason := "nil"
		if anthropicResp.StopReason != nil {
			stopReason = *anthropicResp.StopReason
		}
		t.Errorf("expected stop_reason='end_turn', got %s", stopReason)
	}
}

// =============================================================================
// Model Mapping Tests
// =============================================================================

