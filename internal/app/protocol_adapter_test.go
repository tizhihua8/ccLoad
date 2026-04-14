package app

import (
	"testing"

	"ccLoad/internal/util"

	"github.com/bytedance/sonic"
)

// TestModelMapping 测试模型名称映射
func TestModelMapping(t *testing.T) {
	mapping := NewModelMapping()

	tests := []struct {
		name       string
		model      string
		sourceType string
		targetType string
		want       string
	}{
		// OpenAI -> Anthropic
		{"gpt-4o to claude", "gpt-4o", util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, "claude-3-5-sonnet-20241022"},
		{"gpt-4o-mini to claude", "gpt-4o-mini", util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, "claude-3-5-haiku-20241022"},
		{"gpt-4-turbo to claude", "gpt-4-turbo", util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, "claude-3-opus-20240229"},
		{"unknown openai model", "unknown-model", util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, "unknown-model"},

		// Anthropic -> OpenAI
		{"claude-sonnet to openai", "claude-3-5-sonnet", util.ChannelTypeAnthropic, util.ChannelTypeOpenAI, "gpt-4o"},
		{"claude-haiku to openai", "claude-3-5-haiku", util.ChannelTypeAnthropic, util.ChannelTypeOpenAI, "gpt-4o-mini"},
		{"claude-opus to openai", "claude-3-opus", util.ChannelTypeAnthropic, util.ChannelTypeOpenAI, "gpt-4-turbo"},
		{"unknown anthropic model", "unknown-model", util.ChannelTypeAnthropic, util.ChannelTypeOpenAI, "unknown-model"},

		// 同协议映射（应原样返回）
		{"same protocol", "gpt-4o", util.ChannelTypeOpenAI, util.ChannelTypeOpenAI, "gpt-4o"},

		// 空模型名
		{"empty model", "", util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapping.MapModel(tt.model, tt.sourceType, tt.targetType)
			if got != tt.want {
				t.Errorf("MapModel(%q, %q, %q) = %q, want %q",
					tt.model, tt.sourceType, tt.targetType, got, tt.want)
			}
		})
	}
}

// TestProtocolAdapterEnabled 测试适配器启用状态
func TestProtocolAdapterEnabled(t *testing.T) {
	// 创建模拟的 ConfigService
	mockCfg := &mockConfigService{
		boolValues: map[string]bool{
			"protocol_adapter_enabled": true,
		},
		stringValues: map[string]string{
			"protocol_adapter_mode": "prefer_same",
		},
	}

	adapter := NewProtocolAdapter(mockCfg)
	if !adapter.IsEnabled() {
		t.Error("Expected adapter to be enabled")
	}
	if adapter.GetMode() != AdapterModePreferSame {
		t.Errorf("Expected mode to be prefer_same, got %s", adapter.GetMode())
	}
}

// TestProtocolAdapterDisabled 测试适配器禁用状态
func TestProtocolAdapterDisabled(t *testing.T) {
	mockCfg := &mockConfigService{
		boolValues: map[string]bool{
			"protocol_adapter_enabled": false,
		},
		stringValues: map[string]string{
			"protocol_adapter_mode": "prefer_same",
		},
	}

	adapter := NewProtocolAdapter(mockCfg)
	if adapter.IsEnabled() {
		t.Error("Expected adapter to be disabled")
	}
}

// TestProtocolAdapterCanConvert 测试协议转换支持检测
func TestProtocolAdapterCanConvert(t *testing.T) {
	mockCfg := &mockConfigService{
		boolValues: map[string]bool{
			"protocol_adapter_enabled": true,
		},
		stringValues: map[string]string{
			"protocol_adapter_mode": "prefer_same",
		},
	}

	adapter := NewProtocolAdapter(mockCfg)

	tests := []struct {
		sourceType string
		targetType string
		want       bool
	}{
		{util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, true},
		{util.ChannelTypeAnthropic, util.ChannelTypeOpenAI, true},
		{util.ChannelTypeOpenAI, util.ChannelTypeOpenAI, true},  // 同协议总是支持
		{util.ChannelTypeAnthropic, util.ChannelTypeAnthropic, true},
		{util.ChannelTypeOpenAI, util.ChannelTypeGemini, false}, // 尚未实现
		{util.ChannelTypeGemini, util.ChannelTypeOpenAI, false},
	}

	for _, tt := range tests {
		t.Run(tt.sourceType+"->"+tt.targetType, func(t *testing.T) {
			got := adapter.CanConvert(tt.sourceType, tt.targetType)
			if got != tt.want {
				t.Errorf("CanConvert(%q, %q) = %v, want %v", tt.sourceType, tt.targetType, got, tt.want)
			}
		})
	}
}

// TestProtocolAdapterShouldAttemptCrossProtocol 测试跨协议尝试决策
func TestProtocolAdapterShouldAttemptCrossProtocol(t *testing.T) {
	tests := []struct {
		mode                      ProtocolAdapterMode
		sameProtocolChannelsAvail bool
		want                      bool
	}{
		{AdapterModeSameOnly, true, false},
		{AdapterModeSameOnly, false, false},
		{AdapterModePreferSame, true, false},
		{AdapterModePreferSame, false, true},
		{AdapterModeAlwaysConvert, true, true},
		{AdapterModeAlwaysConvert, false, true},
	}

	for _, tt := range tests {
		adapter := &ProtocolAdapter{mode: tt.mode, enabled: true}
		got := adapter.ShouldAttemptCrossProtocol(tt.sameProtocolChannelsAvail)
		if got != tt.want {
			t.Errorf("ShouldAttemptCrossProtocol(mode=%s, sameAvail=%v) = %v, want %v",
				tt.mode, tt.sameProtocolChannelsAvail, got, tt.want)
		}
	}
}

// TestGetSupportedEndpoint 测试支持的 endpoint 获取
func TestGetSupportedEndpoint(t *testing.T) {
	adapter := &ProtocolAdapter{}

	tests := []struct {
		targetType  string
		isStreaming bool
		want        string
	}{
		{util.ChannelTypeOpenAI, false, "/v1/chat/completions"},
		{util.ChannelTypeOpenAI, true, "/v1/chat/completions"},
		{util.ChannelTypeAnthropic, false, "/v1/messages"},
		{util.ChannelTypeAnthropic, true, "/v1/messages"},
		{util.ChannelTypeCodex, false, "/v1/responses"},
		{util.ChannelTypeGemini, false, "/v1beta/models/{model}:generateContent"},
		{util.ChannelTypeGemini, true, "/v1beta/models/{model}:streamGenerateContent"},
		{"unknown", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.targetType+"-streaming-"+string(rune('0'+boolToInt(tt.isStreaming))), func(t *testing.T) {
			got := adapter.GetSupportedEndpoint(tt.targetType, tt.isStreaming)
			if got != tt.want {
				t.Errorf("GetSupportedEndpoint(%q, %v) = %q, want %q",
					tt.targetType, tt.isStreaming, got, tt.want)
			}
		})
	}
}

// mockConfigService 模拟 ConfigService 用于测试
type mockConfigService struct {
	boolValues   map[string]bool
	stringValues map[string]string
}

func (m *mockConfigService) GetBool(key string, defaultValue bool) bool {
	if v, ok := m.boolValues[key]; ok {
		return v
	}
	return defaultValue
}

func (m *mockConfigService) GetString(key, defaultValue string) string {
	if v, ok := m.stringValues[key]; ok {
		return v
	}
	return defaultValue
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestOpenAIAnthropicConverterRequest 测试 OpenAI -> Anthropic 请求转换
func TestOpenAIAnthropicConverterRequest(t *testing.T) {
	converter := NewOpenAIAnthropicConverter(NewModelMapping())

	tests := []struct {
		name        string
		input       string
		sourceType  string
		targetType  string
		targetModel string
		wantPath    string
		checkFunc   func(t *testing.T, body []byte)
	}{
		{
			name: "basic chat request",
			input: `{
				"model": "gpt-4o",
				"messages": [
					{"role": "system", "content": "You are helpful"},
					{"role": "user", "content": "Hello"}
				]
			}`,
			sourceType:  "openai",
			targetType:  "anthropic",
			targetModel: "claude-3-5-sonnet-20241022",
			wantPath:    "/v1/messages",
			checkFunc: func(t *testing.T, body []byte) {
				var req AnthropicMessagesRequest
				if err := sonic.Unmarshal(body, &req); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				if req.Model != "claude-3-5-sonnet-20241022" {
					t.Errorf("Model = %q, want claude-3-5-sonnet-20241022", req.Model)
				}
				if req.System != "You are helpful" {
					t.Errorf("System = %v, want 'You are helpful'", req.System)
				}
				if len(req.Messages) != 1 {
					t.Errorf("Messages count = %d, want 1", len(req.Messages))
				}
				if req.Messages[0].Role != "user" {
					t.Errorf("First message role = %q, want user", req.Messages[0].Role)
				}
			},
		},
		{
			name: "with stream flag",
			input: `{
				"model": "gpt-4o",
				"messages": [{"role": "user", "content": "Hi"}],
				"stream": true
			}`,
			sourceType:  "openai",
			targetType:  "anthropic",
			targetModel: "claude-3-5-sonnet-20241022",
			wantPath:    "/v1/messages",
			checkFunc: func(t *testing.T, body []byte) {
				var req AnthropicMessagesRequest
				if err := sonic.Unmarshal(body, &req); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				if !req.Stream {
					t.Error("Stream = false, want true")
				}
			},
		},
		{
			name: "with max_tokens",
			input: `{
				"model": "gpt-4o",
				"messages": [{"role": "user", "content": "Hi"}],
				"max_tokens": 1000
			}`,
			sourceType:  "openai",
			targetType:  "anthropic",
			targetModel: "claude-3-5-sonnet-20241022",
			wantPath:    "/v1/messages",
			checkFunc: func(t *testing.T, body []byte) {
				var req AnthropicMessagesRequest
				if err := sonic.Unmarshal(body, &req); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				if req.MaxTokens != 1000 {
					t.Errorf("MaxTokens = %d, want 1000", req.MaxTokens)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, path, err := converter.ConvertRequest([]byte(tt.input), tt.sourceType, tt.targetType, tt.targetModel)
			if err != nil {
				t.Fatalf("ConvertRequest failed: %v", err)
			}
			if path != tt.wantPath {
				t.Errorf("Path = %q, want %q", path, tt.wantPath)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, body)
			}
		})
	}
}

// TestAnthropicOpenAIConverterRequest 测试 Anthropic -> OpenAI 请求转换
func TestAnthropicOpenAIConverterRequest(t *testing.T) {
	converter := NewOpenAIAnthropicConverter(NewModelMapping())

	tests := []struct {
		name        string
		input       string
		sourceType  string
		targetType  string
		targetModel string
		wantPath    string
		checkFunc   func(t *testing.T, body []byte)
	}{
		{
			name: "basic anthropic request",
			input: `{
				"model": "claude-3-5-sonnet",
				"system": "You are helpful",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			sourceType:  "anthropic",
			targetType:  "openai",
			targetModel: "gpt-4o",
			wantPath:    "/v1/chat/completions",
			checkFunc: func(t *testing.T, body []byte) {
				var req OpenAIChatCompletionRequest
				if err := sonic.Unmarshal(body, &req); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				if req.Model != "gpt-4o" {
					t.Errorf("Model = %q, want gpt-4o", req.Model)
				}
				if len(req.Messages) != 2 {
					t.Errorf("Messages count = %d, want 2", len(req.Messages))
				}
				if req.Messages[0].Role != "system" {
					t.Errorf("First message role = %q, want system", req.Messages[0].Role)
				}
				if req.Messages[1].Role != "user" {
					t.Errorf("Second message role = %q, want user", req.Messages[1].Role)
				}
			},
		},
		{
			name: "anthropic with stream",
			input: `{
				"model": "claude-3-5-sonnet",
				"messages": [{"role": "user", "content": "Hi"}],
				"stream": true
			}`,
			sourceType:  "anthropic",
			targetType:  "openai",
			targetModel: "gpt-4o",
			wantPath:    "/v1/chat/completions",
			checkFunc: func(t *testing.T, body []byte) {
				var req OpenAIChatCompletionRequest
				if err := sonic.Unmarshal(body, &req); err != nil {
					t.Fatalf("Failed to unmarshal: %v", err)
				}
				if !req.Stream {
					t.Error("Stream = false, want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, path, err := converter.ConvertRequest([]byte(tt.input), tt.sourceType, tt.targetType, tt.targetModel)
			if err != nil {
				t.Fatalf("ConvertRequest failed: %v", err)
			}
			if path != tt.wantPath {
				t.Errorf("Path = %q, want %q", path, tt.wantPath)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, body)
			}
		})
	}
}

// TestResponseConversion 测试响应转换
func TestResponseConversion(t *testing.T) {
	converter := NewOpenAIAnthropicConverter(NewModelMapping())

	t.Run("anthropic to openai response", func(t *testing.T) {
		anthropicResp := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "Hello!"}],
			"model": "claude-3-5-sonnet-20241022",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 10,
				"output_tokens": 5
			}
		}`

		converted, err := converter.ConvertResponse([]byte(anthropicResp))
		if err != nil {
			t.Fatalf("ConvertResponse failed: %v", err)
		}

		var openAIResp OpenAIChatCompletionResponse
		if err := sonic.Unmarshal(converted, &openAIResp); err != nil {
			t.Fatalf("Failed to unmarshal converted response: %v", err)
		}

		if openAIResp.Object != "chat.completion" {
			t.Errorf("Object = %q, want chat.completion", openAIResp.Object)
		}
		if len(openAIResp.Choices) != 1 {
			t.Errorf("Choices count = %d, want 1", len(openAIResp.Choices))
		}
		if openAIResp.Usage.PromptTokens != 10 {
			t.Errorf("PromptTokens = %d, want 10", openAIResp.Usage.PromptTokens)
		}
		if openAIResp.Usage.CompletionTokens != 5 {
			t.Errorf("CompletionTokens = %d, want 5", openAIResp.Usage.CompletionTokens)
		}
	})

	t.Run("openai to anthropic response", func(t *testing.T) {
		openAIResp := `{
			"id": "chatcmpl_123",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-4o",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello!"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15
			}
		}`

		converted, err := converter.ConvertResponse([]byte(openAIResp))
		if err != nil {
			t.Fatalf("ConvertResponse failed: %v", err)
		}

		var anthropicResp AnthropicMessagesResponse
		if err := sonic.Unmarshal(converted, &anthropicResp); err != nil {
			t.Fatalf("Failed to unmarshal converted response: %v", err)
		}

		if anthropicResp.Type != "message" {
			t.Errorf("Type = %q, want message", anthropicResp.Type)
		}
		if anthropicResp.Role != "assistant" {
			t.Errorf("Role = %q, want assistant", anthropicResp.Role)
		}
		if anthropicResp.Usage.InputTokens != 10 {
			t.Errorf("InputTokens = %d, want 10", anthropicResp.Usage.InputTokens)
		}
		if anthropicResp.Usage.OutputTokens != 5 {
			t.Errorf("OutputTokens = %d, want 5", anthropicResp.Usage.OutputTokens)
		}
	})
}
