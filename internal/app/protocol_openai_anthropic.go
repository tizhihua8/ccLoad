package app

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ccLoad/internal/util"

	"github.com/bytedance/sonic"
)

// =============================================================================
// OpenAI 请求/响应结构
// =============================================================================

// OpenAIChatCompletionRequest OpenAI 聊天完成请求
type OpenAIChatCompletionRequest struct {
	Model          string                 `json:"model"`
	Messages       []OpenAIMessage        `json:"messages"`
	Stream         bool                   `json:"stream,omitempty"`
	Temperature    *float64               `json:"temperature,omitempty"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	MaxPromptTokens int                 `json:"max_prompt_tokens,omitempty"`
	TopP           *float64               `json:"top_p,omitempty"`
	Tools          []OpenAITool           `json:"tools,omitempty"`
	ToolChoice     any                    `json:"tool_choice,omitempty"`
	ResponseFormat *OpenAIResponseFormat `json:"response_format,omitempty"`
	StreamOptions  *OpenAIStreamOptions   `json:"stream_options,omitempty"`
}

// OpenAIMessage OpenAI 消息格式
type OpenAIMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"` // string or []OpenAIContentPart
	Name       string     `json:"name,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// OpenAIContentPart 多模态内容部分
type OpenAIContentPart struct {
	Type     string               `json:"type"`
	Text     string               `json:"text,omitempty"`
	ImageURL *OpenAIImageURL      `json:"image_url,omitempty"`
}

// OpenAIImageURL 图片URL
type OpenAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// OpenAITool 工具定义
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction 函数定义
type OpenAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// OpenAIToolCall 工具调用
type OpenAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

// OpenAIFunctionCall 函数调用详情
type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIResponseFormat 响应格式
type OpenAIResponseFormat struct {
	Type       string `json:"type"`
	JSONSchema any    `json:"json_schema,omitempty"`
}

// OpenAIStreamOptions 流式选项
type OpenAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// OpenAIChatCompletionResponse 非流式响应
type OpenAIChatCompletionResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage,omitempty"`
}

// OpenAIChoice 选择项
type OpenAIChoice struct {
	Index        int          `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string       `json:"finish_reason,omitempty"`
}

// OpenAIUsage Token 使用情况
type OpenAIUsage struct {
	PromptTokens            int                    `json:"prompt_tokens"`
	CompletionTokens        int                    `json:"completion_tokens"`
	TotalTokens             int                    `json:"total_tokens"`
	PromptTokensDetails     OpenAIPromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// OpenAIPromptTokensDetails 提示词 token 详情
type OpenAIPromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// OpenAIChatCompletionChunk 流式响应块
type OpenAIChatCompletionChunk struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []OpenAIChunkChoice `json:"choices"`
	Usage   *OpenAIUsage     `json:"usage,omitempty"`
}

// OpenAIChunkChoice 流式选择项
type OpenAIChunkChoice struct {
	Index        int              `json:"index"`
	Delta        OpenAIChoiceDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason,omitempty"`
}

// OpenAIChoiceDelta 增量内容
type OpenAIChoiceDelta struct {
	Role       string           `json:"role,omitempty"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// =============================================================================
// Anthropic 请求/响应结构
// =============================================================================

// AnthropicMessagesRequest Anthropic 消息请求
type AnthropicMessagesRequest struct {
	Model         string              `json:"model"`
	Messages      []AnthropicMessage  `json:"messages"`
	System        any                 `json:"system,omitempty"` // string or []AnthropicTextBlock
	MaxTokens     int                 `json:"max_tokens"`
	Metadata      *AnthropicMetadata  `json:"metadata,omitempty"`
	StopSequences []string          `json:"stop_sequences,omitempty"`
	Stream        bool                `json:"stream,omitempty"`
	Temperature   *float64            `json:"temperature,omitempty"`
	TopP          *float64            `json:"top_p,omitempty"`
	TopK          int                 `json:"top_k,omitempty"`
	Tools         []AnthropicTool     `json:"tools,omitempty"`
	ToolChoice    *AnthropicToolChoice `json:"tool_choice,omitempty"`
	Thinking      *AnthropicThinking  `json:"thinking,omitempty"`
	BetaHeaders   []string            `json:"-"` // 内部使用，不序列化
}

// AnthropicMessage Anthropic 消息
type AnthropicMessage struct {
	Role    string                    `json:"role"`
	Content any                       `json:"content"` // string or []AnthropicContentBlock
}

// AnthropicContentBlock 内容块（可以是 text, image, tool_use, tool_result）
type AnthropicContentBlock struct {
	Type         string                    `json:"type"`
	Text         string                    `json:"text,omitempty"`
	Source       *AnthropicImageSource     `json:"source,omitempty"`
	ID           string                    `json:"id,omitempty"`
	Name         string                    `json:"name,omitempty"`
	Input        json.RawMessage           `json:"input,omitempty"`
	Content      any                       `json:"content,omitempty"` // for tool_result
	IsError      bool                      `json:"is_error,omitempty"`
	Thinking     string                    `json:"thinking,omitempty"`
	Signature    string                    `json:"signature,omitempty"`
	PartialJSON  string                    `json:"partial_json,omitempty"` // for tool_use in stream
}

// AnthropicTextBlock 文本块（用于 system prompt）
type AnthropicTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// AnthropicImageSource 图片源
type AnthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// AnthropicTool 工具定义
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// AnthropicToolChoice 工具选择
type AnthropicToolChoice struct {
	Type string `json:"type"` // "auto", "any", "tool"
	Name string `json:"name,omitempty"`
}

// AnthropicMetadata 元数据
type AnthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// AnthropicThinking 扩展思考模式
type AnthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// AnthropicMessagesResponse 非流式响应
type AnthropicMessagesResponse struct {
	ID           string                `json:"id"`
	Type         string                `json:"type"`
	Role         string                `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                `json:"model"`
	StopReason   *string               `json:"stop_reason,omitempty"`
	StopSequence *string               `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage        `json:"usage"`
}

// AnthropicUsage Token 使用
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// AnthropicStreamEvent 流式事件接口
type AnthropicStreamEvent struct {
	Type         string                 `json:"type"`
	Message      *AnthropicMessagesResponse `json:"message,omitempty"`
	Index        int                    `json:"index,omitempty"`
	ContentBlock *AnthropicContentBlock `json:"content_block,omitempty"`
	Delta        *AnthropicContentDelta `json:"delta,omitempty"`
	Usage        *AnthropicUsage        `json:"usage,omitempty"`
	StopReason   *string                `json:"stop_reason,omitempty"`
}

// AnthropicContentDelta 内容增量
type AnthropicContentDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Signature   string `json:"signature,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// =============================================================================
// OpenAI ↔ Anthropic 转换器
// =============================================================================

// OpenAIAnthropicConverter OpenAI 和 Anthropic 协议双向转换器
type OpenAIAnthropicConverter struct {
	mapping *ModelMapping
}

// NewOpenAIAnthropicConverter 创建新的转换器
func NewOpenAIAnthropicConverter(mapping *ModelMapping) *OpenAIAnthropicConverter {
	return &OpenAIAnthropicConverter{
		mapping: mapping,
	}
}

// ConvertRequest 转换请求
func (c *OpenAIAnthropicConverter) ConvertRequest(body []byte, sourceType, targetType, targetModel string) ([]byte, string, error) {
	// 根据 sourceType 和 targetType 决定转换方向
	switch {
	case sourceType == util.ChannelTypeOpenAI && targetType == util.ChannelTypeAnthropic:
		var openAIReq OpenAIChatCompletionRequest
		if err := sonic.Unmarshal(body, &openAIReq); err != nil {
			return nil, "", fmt.Errorf("invalid openai request format: %w", err)
		}
		return c.convertOpenAIToAnthropic(&openAIReq, targetModel)

	case sourceType == util.ChannelTypeAnthropic && targetType == util.ChannelTypeOpenAI:
		var anthropicReq AnthropicMessagesRequest
		if err := sonic.Unmarshal(body, &anthropicReq); err != nil {
			return nil, "", fmt.Errorf("invalid anthropic request format: %w", err)
		}
		return c.convertAnthropicToOpenAI(&anthropicReq, targetModel)

	default:
		return nil, "", fmt.Errorf("unsupported conversion direction: %s -> %s", sourceType, targetType)
	}
}

// convertOpenAIToAnthropic OpenAI -> Anthropic
func (c *OpenAIAnthropicConverter) convertOpenAIToAnthropic(req *OpenAIChatCompletionRequest, targetModel string) ([]byte, string, error) {
	anthropicReq := AnthropicMessagesRequest{
		Model:   targetModel,
		Stream:  req.Stream,
		MaxTokens: req.MaxTokens,
	}

	if anthropicReq.MaxTokens == 0 {
		anthropicReq.MaxTokens = 4096 // 默认值
	}

	// 转换 temperature
	if req.Temperature != nil {
		temp := *req.Temperature
		anthropicReq.Temperature = &temp
	}

	// 转换 top_p
	if req.TopP != nil {
		topP := *req.TopP
		anthropicReq.TopP = &topP
	}

	// 提取 system messages 和转换其他 messages
	var systemParts []AnthropicTextBlock
	var messages []AnthropicMessage

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			// Anthropic 的 system 是独立字段
			content := extractStringContent(msg.Content)
			if content != "" {
				systemParts = append(systemParts, AnthropicTextBlock{
					Type: "text",
					Text: content,
				})
			}

		case "user", "assistant":
			anthropicMsg := c.convertOpenAIMessageToAnthropic(msg)
			messages = append(messages, anthropicMsg)

		case "tool":
			// 工具结果消息
			anthropicMsg := AnthropicMessage{
				Role: "user",
				Content: []AnthropicContentBlock{{
					Type:    "tool_result",
					ID:      msg.ToolCallID, // tool_result uses ID field for tool_use_id
					Content: extractStringContent(msg.Content),
				}},
			}
			messages = append(messages, anthropicMsg)
		}
	}

	// 设置 system
	if len(systemParts) == 1 {
		anthropicReq.System = systemParts[0].Text
	} else if len(systemParts) > 1 {
		anthropicReq.System = systemParts
	}

	anthropicReq.Messages = messages

	// 转换 tools
	if len(req.Tools) > 0 {
		anthropicReq.Tools = c.convertOpenAIToolsToAnthropic(req.Tools)
	}

	// 转换 tool_choice
	if req.ToolChoice != nil {
		anthropicReq.ToolChoice = c.convertOpenAIToolChoiceToAnthropic(req.ToolChoice)
	}

	result, err := sonic.Marshal(anthropicReq)
	if err != nil {
		return nil, "", fmt.Errorf("marshal anthropic request: %w", err)
	}

	return result, "/v1/messages", nil
}

// convertAnthropicToOpenAI Anthropic -> OpenAI
func (c *OpenAIAnthropicConverter) convertAnthropicToOpenAI(req *AnthropicMessagesRequest, targetModel string) ([]byte, string, error) {
	openAIReq := OpenAIChatCompletionRequest{
		Model:     targetModel,
		Stream:    req.Stream,
		MaxTokens: req.MaxTokens,
	}

	// 转换 temperature
	if req.Temperature != nil {
		temp := *req.Temperature
		openAIReq.Temperature = &temp
	}

	// 转换 top_p
	if req.TopP != nil {
		topP := *req.TopP
		openAIReq.TopP = &topP
	}

	var messages []OpenAIMessage

	// 处理 system prompt
	if req.System != nil {
		switch s := req.System.(type) {
		case string:
			if s != "" {
				messages = append(messages, OpenAIMessage{
					Role:    "system",
					Content: s,
				})
			}
		case []any:
			// 多个 system blocks
			for _, block := range s {
				if blockMap, ok := block.(map[string]any); ok {
					if text, ok := blockMap["text"].(string); ok {
						messages = append(messages, OpenAIMessage{
							Role:    "system",
							Content: text,
						})
					}
				}
			}
		}
	}

	// 转换 messages
	for _, msg := range req.Messages {
		openAIMsg := c.convertAnthropicMessageToOpenAI(msg)
		messages = append(messages, openAIMsg)
	}

	openAIReq.Messages = messages

	// 转换 tools
	if len(req.Tools) > 0 {
		openAIReq.Tools = c.convertAnthropicToolsToOpenAI(req.Tools)
	}

	// 转换 tool_choice
	if req.ToolChoice != nil {
		openAIReq.ToolChoice = c.convertAnthropicToolChoiceToOpenAI(req.ToolChoice)
	}

	result, err := sonic.Marshal(openAIReq)
	if err != nil {
		return nil, "", fmt.Errorf("marshal openai request: %w", err)
	}

	return result, "/v1/chat/completions", nil
}

// =============================================================================
// 消息转换辅助函数
// =============================================================================

// convertOpenAIMessageToAnthropic 转换 OpenAI 消息到 Anthropic
func (c *OpenAIAnthropicConverter) convertOpenAIMessageToAnthropic(msg OpenAIMessage) AnthropicMessage {
	var content any

	switch v := msg.Content.(type) {
	case string:
		if v != "" {
			content = []AnthropicContentBlock{{
				Type: "text",
				Text: v,
			}}
		}
	case []any:
		// 多模态内容
		var blocks []AnthropicContentBlock
		for _, part := range v {
			if partMap, ok := part.(map[string]any); ok {
				block := c.convertOpenAIContentPartToAnthropic(partMap)
				if block.Type != "" {
					blocks = append(blocks, block)
				}
			}
		}
		content = blocks
	}

	// 处理 assistant 的 tool_calls
	if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
		var blocks []AnthropicContentBlock

		// 先添加文本内容
		if text := extractStringContent(msg.Content); text != "" {
			blocks = append(blocks, AnthropicContentBlock{
				Type: "text",
				Text: text,
			})
		}

		// 添加 tool_use
		for _, tc := range msg.ToolCalls {
			blocks = append(blocks, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(tc.Function.Arguments),
			})
		}

		content = blocks
	}

	return AnthropicMessage{
		Role:    msg.Role,
		Content: content,
	}
}

// convertOpenAIContentPartToAnthropic 转换内容部分
func (c *OpenAIAnthropicConverter) convertOpenAIContentPartToAnthropic(part map[string]any) AnthropicContentBlock {
	blockType, _ := part["type"].(string)

	switch blockType {
	case "text":
		text, _ := part["text"].(string)
		return AnthropicContentBlock{Type: "text", Text: text}

	case "image_url":
		if imageURL, ok := part["image_url"].(map[string]any); ok {
			url, _ := imageURL["url"].(string)
			// 解析 data URL
			if strings.HasPrefix(url, "data:") {
				mediaType, data := parseDataURL(url)
				return AnthropicContentBlock{
					Type: "image",
					Source: &AnthropicImageSource{
						Type:      "base64",
						MediaType: mediaType,
						Data:      data,
					},
				}
			}
		}
	}

	return AnthropicContentBlock{}
}

// convertAnthropicMessageToOpenAI 转换 Anthropic 消息到 OpenAI
func (c *OpenAIAnthropicConverter) convertAnthropicMessageToOpenAI(msg AnthropicMessage) OpenAIMessage {
	openAIMsg := OpenAIMessage{
		Role: msg.Role,
	}

	switch v := msg.Content.(type) {
	case string:
		openAIMsg.Content = v

	case []any:
		var contentParts []OpenAIContentPart
		var toolCalls []OpenAIToolCall
		var textContent strings.Builder

		for _, block := range v {
			if blockMap, ok := block.(map[string]any); ok {
				blockType, _ := blockMap["type"].(string)

				switch blockType {
				case "text":
					if text, ok := blockMap["text"].(string); ok && text != "" {
						textContent.WriteString(text)
					}

				case "image":
					if source, ok := blockMap["source"].(map[string]any); ok {
						mediaType, _ := source["media_type"].(string)
						data, _ := source["data"].(string)
						contentParts = append(contentParts, OpenAIContentPart{
							Type: "image_url",
							ImageURL: &OpenAIImageURL{
								URL: fmt.Sprintf("data:%s;base64,%s", mediaType, data),
							},
						})
					}

				case "tool_use":
					id, _ := blockMap["id"].(string)
					name, _ := blockMap["name"].(string)
					input, _ := blockMap["input"].(map[string]any)
					inputJSON, _ := json.Marshal(input)

					toolCalls = append(toolCalls, OpenAIToolCall{
						ID:   id,
						Type: "function",
						Function: OpenAIFunctionCall{
							Name:      name,
							Arguments: string(inputJSON),
						},
					})

				case "tool_result":
					// tool_result 在 Anthropic 中是 user 消息的一部分
					// 在 OpenAI 中是单独的 tool 角色消息
					// 这里只提取内容，外层需要特殊处理
					if content, ok := blockMap["content"].(string); ok {
						textContent.WriteString(content)
					}
				}
			}
		}

		// 组装内容
		if len(toolCalls) > 0 {
			openAIMsg.ToolCalls = toolCalls
			if textContent.Len() > 0 {
				openAIMsg.Content = textContent.String()
			}
		} else if len(contentParts) > 0 {
			// 有多模态内容
			if textContent.Len() > 0 {
				contentParts = append([]OpenAIContentPart{{Type: "text", Text: textContent.String()}}, contentParts...)
			}
			openAIMsg.Content = contentParts
		} else {
			openAIMsg.Content = textContent.String()
		}
	}

	return openAIMsg
}

// =============================================================================
// Tools 转换
// =============================================================================

func (c *OpenAIAnthropicConverter) convertOpenAIToolsToAnthropic(tools []OpenAITool) []AnthropicTool {
	var result []AnthropicTool
	for _, tool := range tools {
		var inputSchema json.RawMessage
		if tool.Function.Parameters != nil {
			inputSchema, _ = json.Marshal(tool.Function.Parameters)
		}

		result = append(result, AnthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: inputSchema,
		})
	}
	return result
}

func (c *OpenAIAnthropicConverter) convertAnthropicToolsToOpenAI(tools []AnthropicTool) []OpenAITool {
	var result []OpenAITool
	for _, tool := range tools {
		var params any
		if len(tool.InputSchema) > 0 {
			_ = json.Unmarshal(tool.InputSchema, &params)
		}

		result = append(result, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			},
		})
	}
	return result
}

func (c *OpenAIAnthropicConverter) convertOpenAIToolChoiceToAnthropic(choice any) *AnthropicToolChoice {
	switch v := choice.(type) {
	case string:
		switch v {
		case "auto":
			return &AnthropicToolChoice{Type: "auto"}
		case "none":
			// Anthropic 没有 direct equivalent，使用 auto
			return &AnthropicToolChoice{Type: "auto"}
		}
	case map[string]any:
		// {"type": "function", "function": {"name": "xxx"}}
		if fn, ok := v["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				return &AnthropicToolChoice{
					Type: "tool",
					Name: name,
				}
			}
		}
	}
	return nil
}

func (c *OpenAIAnthropicConverter) convertAnthropicToolChoiceToOpenAI(choice *AnthropicToolChoice) any {
	switch choice.Type {
	case "auto":
		return "auto"
	case "any":
		return "auto" // OpenAI 没有 exact equivalent
	case "tool":
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": choice.Name,
			},
		}
	}
	return nil
}

// =============================================================================
// 响应转换
// =============================================================================

// ConvertResponse 转换响应
func (c *OpenAIAnthropicConverter) ConvertResponse(respBody []byte) ([]byte, error) {
	// 尝试检测响应格式
	// Anthropic 响应有 "type": "message" 字段
	// OpenAI 响应有 "object": "chat.completion" 字段

	var rawResp map[string]any
	if err := sonic.Unmarshal(respBody, &rawResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// 检测 Anthropic 格式
	if respType, ok := rawResp["type"].(string); ok && respType == "message" {
		// Anthropic -> OpenAI
		return c.convertAnthropicResponseToOpenAI(respBody)
	}

	// 检测 OpenAI 格式
	if objType, ok := rawResp["object"].(string); ok && (objType == "chat.completion" || objType == "chat.completion.chunk") {
		// OpenAI -> Anthropic
		return c.convertOpenAIResponseToAnthropic(respBody)
	}

	// 无法检测，尝试根据结构推断
	if _, ok := rawResp["content"]; ok {
		// 可能是 Anthropic
		return c.convertAnthropicResponseToOpenAI(respBody)
	}

	if _, ok := rawResp["choices"]; ok {
		// 可能是 OpenAI
		return c.convertOpenAIResponseToAnthropic(respBody)
	}

	return nil, fmt.Errorf("unable to detect response format")
}

// convertAnthropicResponseToOpenAI Anthropic 响应 -> OpenAI
func (c *OpenAIAnthropicConverter) convertAnthropicResponseToOpenAI(respBody []byte) ([]byte, error) {
	var anthropicResp AnthropicMessagesResponse
	if err := sonic.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
	}

	// 转换 content blocks 到 OpenAI message
	content := c.convertAnthropicContentToOpenAI(anthropicResp.Content)

	// 确定 finish_reason
	finishReason := ""
	if anthropicResp.StopReason != nil {
		switch *anthropicResp.StopReason {
		case "end_turn":
			finishReason = "stop"
		case "max_tokens":
			finishReason = "length"
		case "stop_sequence":
			finishReason = "stop"
		case "tool_use":
			finishReason = "tool_calls"
		}
	}

	openAIResp := OpenAIChatCompletionResponse{
		ID:      anthropicResp.ID,
		Object:  "chat.completion",
		Created: 0, // Anthropic 没有 created，设为 0
		Model:   anthropicResp.Model,
		Choices: []OpenAIChoice{{
			Index: 0,
			Message: OpenAIMessage{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: finishReason,
		}},
		Usage: OpenAIUsage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}

	// 如果有 tool_use，需要转换
	toolCalls := c.extractToolCallsFromAnthropic(anthropicResp.Content)
	if len(toolCalls) > 0 {
		openAIResp.Choices[0].Message.ToolCalls = toolCalls
		openAIResp.Choices[0].FinishReason = "tool_calls"
	}

	return sonic.Marshal(openAIResp)
}

// convertOpenAIResponseToAnthropic OpenAI 响应 -> Anthropic
func (c *OpenAIAnthropicConverter) convertOpenAIResponseToAnthropic(respBody []byte) ([]byte, error) {
	var openAIResp OpenAIChatCompletionResponse
	if err := sonic.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("unmarshal openai response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in openai response")
	}

	choice := openAIResp.Choices[0]
	msg := choice.Message

	// 转换 content
	var content []AnthropicContentBlock

	switch v := msg.Content.(type) {
	case string:
		if v != "" {
			content = append(content, AnthropicContentBlock{Type: "text", Text: v})
		}
	case []OpenAIContentPart:
		for _, part := range v {
			switch part.Type {
			case "text":
				if part.Text != "" {
					content = append(content, AnthropicContentBlock{Type: "text", Text: part.Text})
				}
			case "image_url":
				if part.ImageURL != nil {
					mediaType, data := parseDataURL(part.ImageURL.URL)
					content = append(content, AnthropicContentBlock{
						Type: "image",
						Source: &AnthropicImageSource{
							Type:      "base64",
							MediaType: mediaType,
							Data:      data,
						},
					})
				}
			}
		}
	}

	// 转换 tool_calls 到 tool_use
	for _, tc := range msg.ToolCalls {
		var input map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		inputJSON, _ := json.Marshal(input)

		content = append(content, AnthropicContentBlock{
			Type:    "tool_use",
			ID:      tc.ID,
			Name:    tc.Function.Name,
			Input:   inputJSON,
		})
	}

	// 确定 stop_reason
	stopReason := (*string)(nil)
	switch choice.FinishReason {
	case "stop":
		s := "end_turn"
		stopReason = &s
	case "length":
		s := "max_tokens"
		stopReason = &s
	case "tool_calls":
		s := "tool_use"
		stopReason = &s
	}

	anthropicResp := AnthropicMessagesResponse{
		ID:      openAIResp.ID,
		Type:    "message",
		Role:    "assistant",
		Content: content,
		Model:   openAIResp.Model,
		Usage: AnthropicUsage{
			InputTokens:  openAIResp.Usage.PromptTokens,
			OutputTokens: openAIResp.Usage.CompletionTokens,
		},
	}

	if stopReason != nil {
		anthropicResp.StopReason = stopReason
	}

	return sonic.Marshal(anthropicResp)
}

// convertAnthropicContentToOpenAI 转换 Anthropic content 到 OpenAI
func (c *OpenAIAnthropicConverter) convertAnthropicContentToOpenAI(content []AnthropicContentBlock) any {
	var textParts []string
	var hasNonText bool

	for _, block := range content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "image":
			hasNonText = true
		case "tool_use":
			// 工具调用单独处理
		}
	}

	// 如果有图片等非文本内容，需要返回数组格式
	if hasNonText {
		var parts []OpenAIContentPart
		for _, block := range content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					parts = append(parts, OpenAIContentPart{Type: "text", Text: block.Text})
				}
			case "image":
				if block.Source != nil {
					parts = append(parts, OpenAIContentPart{
						Type: "image_url",
						ImageURL: &OpenAIImageURL{
							URL: fmt.Sprintf("data:%s;base64,%s", block.Source.MediaType, block.Source.Data),
						},
					})
				}
			}
		}
		return parts
	}

	// 纯文本返回字符串
	return strings.Join(textParts, "")
}

// extractToolCallsFromAnthropic 从 Anthropic content 提取 tool calls
func (c *OpenAIAnthropicConverter) extractToolCallsFromAnthropic(content []AnthropicContentBlock) []OpenAIToolCall {
	var toolCalls []OpenAIToolCall

	for _, block := range content {
		if block.Type == "tool_use" {
			var inputJSON string
			if len(block.Input) > 0 {
				inputJSON = string(block.Input)
			}

			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:   block.ID,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      block.Name,
					Arguments: inputJSON,
				},
			})
		}
	}

	return toolCalls
}

// CreateStreamConverter 创建流式转换器
func (c *OpenAIAnthropicConverter) CreateStreamConverter() StreamConverter {
	return newOpenAIAnthropicStreamConverter()
}

// =============================================================================
// 工具函数
// =============================================================================

// extractStringContent 从 any 类型提取字符串内容
func extractStringContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

// parseDataURL 解析 data URL，返回 media type 和 base64 数据
func parseDataURL(url string) (mediaType, data string) {
	if !strings.HasPrefix(url, "data:") {
		return "", ""
	}

	// data:image/jpeg;base64,xxxx
	url = strings.TrimPrefix(url, "data:")
	parts := strings.SplitN(url, ",", 2)
	if len(parts) != 2 {
		return "", ""
	}

	meta := parts[0]
	data = parts[1]

	// 解析 media type
	if idx := strings.Index(meta, ";"); idx != -1 {
		mediaType = meta[:idx]
	} else {
		mediaType = meta
	}

	return mediaType, data
}

// =============================================================================
// 流式转换器
// =============================================================================

// openAIAnthropicStreamConverter OpenAI ↔ Anthropic 流式转换器
type openAIAnthropicStreamConverter struct {
	isAnthropicToOpenAI bool // 转换方向
	messageID           string
	model               string
	created             int64
	index               int
	buffer              []byte
	toolUseBuffer       map[int]*anthropicToolUseBuffer
}

type anthropicToolUseBuffer struct {
	id       string
	name     string
	inputBuf strings.Builder
}

// newOpenAIAnthropicStreamConverter 创建新的流式转换器
func newOpenAIAnthropicStreamConverter() *openAIAnthropicStreamConverter {
	return &openAIAnthropicStreamConverter{
		messageID:     generateMessageID(),
		created:       nowUnix(),
		toolUseBuffer: make(map[int]*anthropicToolUseBuffer),
	}
}

// GetConverterType 返回转换器类型
func (sc *openAIAnthropicStreamConverter) GetConverterType() string {
	if sc.isAnthropicToOpenAI {
		return "anthropic->openai"
	}
	return "openai->anthropic"
}

// ConvertChunk 转换 SSE chunk
// chunk 应该已经是去掉 "data: " 前缀的原始数据
func (sc *openAIAnthropicStreamConverter) ConvertChunk(chunk []byte) ([]byte, bool, error) {
	// 去除前缀空白
	chunk = bytes.TrimSpace(chunk)

	// 跳过空行和注释行
	if len(chunk) == 0 || bytes.HasPrefix(chunk, []byte(":")) {
		return nil, false, nil
	}

	// 解析 data: 前缀
	dataPrefix := []byte("data: ")
	if bytes.HasPrefix(chunk, dataPrefix) {
		chunk = bytes.TrimPrefix(chunk, dataPrefix)
	}

	// 检查是否是 [DONE]
	if bytes.Equal(chunk, []byte("[DONE]")) {
		return []byte("data: [DONE]\n\n"), true, nil
	}

	// 尝试解析为 Anthropic 事件
	var event anthropicStreamEvent
	if err := sonic.Unmarshal(chunk, &event); err == nil && event.Type != "" {
		// 成功解析为 Anthropic 格式，转换为 OpenAI
		return sc.convertAnthropicEventToOpenAI(&event)
	}

	// 尝试解析为 OpenAI chunk
	var openAIChunk OpenAIChatCompletionChunk
	if err := sonic.Unmarshal(chunk, &openAIChunk); err == nil && openAIChunk.Object != "" {
		// 成功解析为 OpenAI 格式，转换为 Anthropic
		return sc.convertOpenAIChunkToAnthropic(&openAIChunk)
	}

	// 无法识别，原样返回（可能是代理错误信息等）
	return append([]byte("data: "), append(chunk, []byte("\n\n")...)...), false, nil
}

// anthropicStreamEvent 解析 Anthropic SSE 事件
type anthropicStreamEvent struct {
	Type         string                 `json:"type"`
	Message      *AnthropicMessagesResponse `json:"message,omitempty"`
	Index        int                    `json:"index,omitempty"`
	ContentBlock *AnthropicContentBlock `json:"content_block,omitempty"`
	Delta        *AnthropicContentDelta `json:"delta,omitempty"`
	Usage        *AnthropicUsage        `json:"usage,omitempty"`
}

// convertAnthropicEventToOpenAI 转换 Anthropic 事件到 OpenAI 格式
func (sc *openAIAnthropicStreamConverter) convertAnthropicEventToOpenAI(event *anthropicStreamEvent) ([]byte, bool, error) {
	switch event.Type {
	case "message_start":
		if event.Message != nil {
			sc.model = event.Message.Model
		}
		return nil, false, nil // message_start 不产生 OpenAI 输出

	case "content_block_start":
		if event.ContentBlock != nil {
			switch event.ContentBlock.Type {
			case "text":
				// 开始文本块，产生一个空的 delta
				chunk := OpenAIChatCompletionChunk{
					ID:      sc.messageID,
					Object:  "chat.completion.chunk",
					Created: sc.created,
					Model:   sc.model,
					Choices: []OpenAIChunkChoice{{
						Index: sc.index,
						Delta: OpenAIChoiceDelta{
							Role: "assistant",
						},
					}},
				}
				sc.index++
				data, _ := sonic.Marshal(chunk)
				return append([]byte("data: "), append(data, []byte("\n\n")...)...), false, nil

			case "tool_use":
				// 开始 tool_use，初始化 buffer
				sc.toolUseBuffer[event.Index] = &anthropicToolUseBuffer{
					id:   event.ContentBlock.ID,
					name: event.ContentBlock.Name,
				}
				// 不立即输出，等待 partial_json
				return nil, false, nil
			}
		}
		return nil, false, nil

	case "content_block_delta":
		if event.Delta != nil {
			switch event.Delta.Type {
			case "text_delta":
				chunk := OpenAIChatCompletionChunk{
					ID:      sc.messageID,
					Object:  "chat.completion.chunk",
					Created: sc.created,
					Model:   sc.model,
					Choices: []OpenAIChunkChoice{{
						Index: 0,
						Delta: OpenAIChoiceDelta{
							Content: event.Delta.Text,
						},
					}},
				}
				data, _ := sonic.Marshal(chunk)
				return append([]byte("data: "), append(data, []byte("\n\n")...)...), false, nil

			case "input_json_delta":
				// 累积 tool_use 的 JSON
				if buf, ok := sc.toolUseBuffer[event.Index]; ok {
					buf.inputBuf.WriteString(event.Delta.PartialJSON)
				}
				return nil, false, nil
			}
		}
		return nil, false, nil

	case "content_block_stop":
		// 检查是否是 tool_use 完成
		if buf, ok := sc.toolUseBuffer[event.Index]; ok {
			// 输出完整的 tool_calls
			chunk := OpenAIChatCompletionChunk{
				ID:      sc.messageID,
				Object:  "chat.completion.chunk",
				Created: sc.created,
				Model:   sc.model,
				Choices: []OpenAIChunkChoice{{
					Index: 0,
					Delta: OpenAIChoiceDelta{
						ToolCalls: []OpenAIToolCall{{
							ID:   buf.id,
							Type: "function",
							Function: OpenAIFunctionCall{
								Name:      buf.name,
								Arguments: buf.inputBuf.String(),
							},
						}},
					},
				}},
			}
			delete(sc.toolUseBuffer, event.Index)
			data, _ := sonic.Marshal(chunk)
			return append([]byte("data: "), append(data, []byte("\n\n")...)...), false, nil
		}
		return nil, false, nil

	case "message_delta":
		// 处理停止原因
		if event.Delta != nil && event.Delta.StopReason != "" {
			finishReason := ""
			switch event.Delta.StopReason {
			case "end_turn":
				finishReason = "stop"
			case "max_tokens":
				finishReason = "length"
			case "tool_use":
				finishReason = "tool_calls"
			case "stop_sequence":
				finishReason = "stop"
			}

			if finishReason != "" {
				chunk := OpenAIChatCompletionChunk{
					ID:      sc.messageID,
					Object:  "chat.completion.chunk",
					Created: sc.created,
					Model:   sc.model,
					Choices: []OpenAIChunkChoice{{
						Index:        0,
						Delta:        OpenAIChoiceDelta{},
						FinishReason: &finishReason,
					}},
				}

				// 如果有 usage，添加进去
				if event.Usage != nil {
					chunk.Usage = &OpenAIUsage{
						PromptTokens:     event.Usage.InputTokens,
						CompletionTokens: event.Usage.OutputTokens,
						TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
					}
				}

				data, _ := sonic.Marshal(chunk)
				return append([]byte("data: "), append(data, []byte("\n\n")...)...), false, nil
			}
		}
		return nil, false, nil

	case "message_stop":
		// 消息结束，返回 [DONE]
		return []byte("data: [DONE]\n\n"), true, nil

	case "ping":
		// ping 事件忽略
		return nil, false, nil
	}

	return nil, false, nil
}

// convertOpenAIChunkToAnthropic 转换 OpenAI chunk 到 Anthropic 格式
func (sc *openAIAnthropicStreamConverter) convertOpenAIChunkToAnthropic(chunk *OpenAIChatCompletionChunk) ([]byte, bool, error) {
	var events []anthropicStreamEvent

	for _, choice := range chunk.Choices {
		// 处理增量内容
		if choice.Delta.Content != "" {
			events = append(events, anthropicStreamEvent{
				Type: "content_block_delta",
				Delta: &AnthropicContentDelta{
					Type: "text_delta",
					Text: choice.Delta.Content,
				},
			})
		}

		// 处理 tool_calls
		for _, tc := range choice.Delta.ToolCalls {
			// content_block_start for tool_use
			events = append(events, anthropicStreamEvent{
				Type: "content_block_start",
				ContentBlock: &AnthropicContentBlock{
					Type: "tool_use",
					ID:   tc.ID,
					Name: tc.Function.Name,
				},
			})

			// input_json_delta
			if tc.Function.Arguments != "" {
				events = append(events, anthropicStreamEvent{
					Type: "content_block_delta",
					Delta: &AnthropicContentDelta{
						Type:        "input_json_delta",
						PartialJSON: tc.Function.Arguments,
					},
				})
			}

			// content_block_stop
			events = append(events, anthropicStreamEvent{
				Type: "content_block_stop",
			})
		}

		// 处理完成原因
		if choice.FinishReason != nil {
			stopReason := ""
			switch *choice.FinishReason {
			case "stop":
				stopReason = "end_turn"
			case "length":
				stopReason = "max_tokens"
			case "tool_calls":
				stopReason = "tool_use"
			}

			if stopReason != "" {
				events = append(events, anthropicStreamEvent{
					Type: "message_delta",
					Delta: &AnthropicContentDelta{
						StopReason: stopReason,
					},
				})
			}
		}
	}

	// 处理 usage
	if chunk.Usage != nil {
		events = append(events, anthropicStreamEvent{
			Type: "message_delta",
			Usage: &AnthropicUsage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			},
		})
	}

	// 如果没有事件但 chunk 是终止信号，发送 message_stop
	if len(events) == 0 {
		return []byte("event: message_stop\ndata: {\"type\": \"message_stop\"}\n\n"), true, nil
	}

	// 序列化所有事件
	var buf bytes.Buffer
	for _, event := range events {
		data, _ := sonic.Marshal(event)
		// Anthropic 格式通常有 event: 行，但这里简化处理
		buf.WriteString("data: ")
		buf.Write(data)
		buf.WriteString("\n")
	}
	buf.WriteString("\n")

	return buf.Bytes(), false, nil
}

// generateMessageID 生成消息ID（msg_ 前缀 + 随机字符）
func generateMessageID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b[:12])
}

// nowUnix 返回当前 Unix 时间戳（秒）
func nowUnix() int64 {
	return time.Now().Unix()
}
