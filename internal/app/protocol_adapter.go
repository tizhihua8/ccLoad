package app

import (
	"fmt"
	"sync"

	"ccLoad/internal/util"
)

// ProtocolAdapterMode 协议适配器工作模式
type ProtocolAdapterMode string

const (
	// AdapterModeSameOnly 只匹配同协议渠道（当前默认行为）
	AdapterModeSameOnly ProtocolAdapterMode = "same_only"
	// AdapterModePreferSame 优先同协议，无则跨协议转换
	AdapterModePreferSame ProtocolAdapterMode = "prefer_same"
	// AdapterModeAlwaysConvert 总是允许跨协议（只要有模型匹配就转）
	AdapterModeAlwaysConvert ProtocolAdapterMode = "always_convert"
)

// StreamConverter 流式响应转换器接口
type StreamConverter interface {
	// ConvertChunk 转换单个 SSE chunk
	// 返回转换后的数据、是否完成、错误
	ConvertChunk(chunk []byte) ([]byte, bool, error)

	// GetConverterType 返回转换器类型标识
	GetConverterType() string
}

// RequestConverter 请求转换接口
type RequestConverter interface {
	// ConvertRequest 将请求体从源协议转换为目标协议
	// sourceType: 客户端协议类型, targetType: 上游渠道协议类型
	// 返回：转换后的 body、新的 endpoint path、错误
	ConvertRequest(body []byte, sourceType, targetType, targetModel string) ([]byte, string, error)
}

// ResponseConverter 响应转换接口
type ResponseConverter interface {
	// ConvertResponse 将上游响应体从目标协议转换回源协议
	// 用于非流式响应
	ConvertResponse(respBody []byte) ([]byte, error)

	// CreateStreamConverter 创建流式响应转换器
	CreateStreamConverter() StreamConverter
}

// BidirectionalConverter 双向转换器，组合请求和响应转换
type BidirectionalConverter interface {
	RequestConverter
	ResponseConverter
}

// ProtocolConverterRegistry 协议转换器注册表
type ProtocolConverterRegistry struct {
	mu         sync.RWMutex
	converters map[string]BidirectionalConverter // key: "source->target"
}

// NewProtocolConverterRegistry 创建新的注册表
func NewProtocolConverterRegistry() *ProtocolConverterRegistry {
	return &ProtocolConverterRegistry{
		converters: make(map[string]BidirectionalConverter),
	}
}

// Register 注册双向转换器（同时注册 source->target 和 target->source）
func (r *ProtocolConverterRegistry) Register(sourceType, targetType string, converter BidirectionalConverter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeConverterKey(sourceType, targetType)
	r.converters[key] = converter

	// 注册反向转换（使用同一转换器，内部处理双向逻辑）
	reverseKey := makeConverterKey(targetType, sourceType)
	r.converters[reverseKey] = converter
}

// GetConverter 获取指定方向的转换器
func (r *ProtocolConverterRegistry) GetConverter(sourceType, targetType string) (BidirectionalConverter, bool) {
	// 同协议无需转换
	if sourceType == targetType {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeConverterKey(sourceType, targetType)
	converter, ok := r.converters[key]
	return converter, ok
}

// CanConvert 检查是否支持从 sourceType 到 targetType 的转换
func (r *ProtocolConverterRegistry) CanConvert(sourceType, targetType string) bool {
	if sourceType == targetType {
		return true
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeConverterKey(sourceType, targetType)
	_, ok := r.converters[key]
	return ok
}

// makeConverterKey 创建转换器映射 key
func makeConverterKey(sourceType, targetType string) string {
	return sourceType + "->" + targetType
}

// ProtocolAdapter 协议适配器主结构
type ProtocolAdapter struct {
	registry *ProtocolConverterRegistry
	mapping  *ModelMapping
	enabled  bool
	mode     ProtocolAdapterMode
}

// ConfigGetter 配置获取接口（允许使用 mock 进行测试）
type ConfigGetter interface {
	GetBool(key string, defaultValue bool) bool
	GetString(key, defaultValue string) string
}

// NewProtocolAdapter 创建协议适配器
func NewProtocolAdapter(cfg ConfigGetter) *ProtocolAdapter {
	enabled := cfg.GetBool("protocol_adapter_enabled", false)
	modeStr := cfg.GetString("protocol_adapter_mode", string(AdapterModePreferSame))

	mode := ProtocolAdapterMode(modeStr)
	switch mode {
	case AdapterModeSameOnly, AdapterModePreferSame, AdapterModeAlwaysConvert:
		// valid
	default:
		mode = AdapterModePreferSame
	}

	adapter := &ProtocolAdapter{
		registry: NewProtocolConverterRegistry(),
		mapping:  NewModelMapping(),
		enabled:  enabled,
		mode:     mode,
	}

	// 注册各协议转换器
	if enabled {
		adapter.registerConverters()
	}

	return adapter
}

// registerConverters 注册所有协议转换器
func (pa *ProtocolAdapter) registerConverters() {
	// 注册 OpenAI ↔ Anthropic 转换器
	oaConverter := NewOpenAIAnthropicConverter(pa.mapping)
	pa.registry.Register(util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, oaConverter)

	// TODO: Phase 3 添加更多转换器
	// pa.registry.Register(util.ChannelTypeOpenAI, util.ChannelTypeGemini, NewOpenAIGeminiConverter())
	// pa.registry.Register(util.ChannelTypeAnthropic, util.ChannelTypeGemini, NewAnthropicGeminiConverter())
}

// IsEnabled 返回适配器是否启用
func (pa *ProtocolAdapter) IsEnabled() bool {
	return pa.enabled
}

// GetMode 返回当前工作模式
func (pa *ProtocolAdapter) GetMode() ProtocolAdapterMode {
	return pa.mode
}

// CanConvert 检查是否支持转换
func (pa *ProtocolAdapter) CanConvert(sourceType, targetType string) bool {
	if !pa.enabled {
		return sourceType == targetType
	}
	return pa.registry.CanConvert(sourceType, targetType)
}

// ConvertRequest 转换请求
// 返回：转换后的 body、新的 endpoint path、错误
func (pa *ProtocolAdapter) ConvertRequest(body []byte, sourceType, targetType, targetModel string) ([]byte, string, error) {
	if sourceType == targetType {
		return body, "", nil
	}

	if !pa.enabled {
		return nil, "", fmt.Errorf("protocol adapter is disabled")
	}

	converter, ok := pa.registry.GetConverter(sourceType, targetType)
	if !ok {
		return nil, "", fmt.Errorf("no converter available for %s -> %s", sourceType, targetType)
	}

	return converter.ConvertRequest(body, sourceType, targetType, targetModel)
}

// ConvertResponse 转换非流式响应
func (pa *ProtocolAdapter) ConvertResponse(respBody []byte, sourceType, targetType string) ([]byte, error) {
	if sourceType == targetType {
		return respBody, nil
	}

	if !pa.enabled {
		return nil, fmt.Errorf("protocol adapter is disabled")
	}

	converter, ok := pa.registry.GetConverter(sourceType, targetType)
	if !ok {
		return nil, fmt.Errorf("no converter available for %s -> %s", sourceType, targetType)
	}

	return converter.ConvertResponse(respBody)
}

// CreateStreamConverter 创建流式响应转换器
func (pa *ProtocolAdapter) CreateStreamConverter(sourceType, targetType string) (StreamConverter, error) {
	if sourceType == targetType {
		return nil, nil
	}

	if !pa.enabled {
		return nil, fmt.Errorf("protocol adapter is disabled")
	}

	converter, ok := pa.registry.GetConverter(sourceType, targetType)
	if !ok {
		return nil, fmt.Errorf("no converter available for %s -> %s", sourceType, targetType)
	}

	return converter.CreateStreamConverter(), nil
}

// MapModel 映射模型名称
func (pa *ProtocolAdapter) MapModel(model, sourceType, targetType string) string {
	return pa.mapping.MapModel(model, sourceType, targetType)
}

// ShouldAttemptCrossProtocol 根据当前模式判断是否应尝试跨协议
func (pa *ProtocolAdapter) ShouldAttemptCrossProtocol(sameProtocolChannelsAvailable bool) bool {
	switch pa.mode {
	case AdapterModeSameOnly:
		return false
	case AdapterModeAlwaysConvert:
		return true
	case AdapterModePreferSame:
		return !sameProtocolChannelsAvailable
	default:
		return !sameProtocolChannelsAvailable
	}
}

// GetSupportedEndpoint 获取目标协议支持的 endpoint
func (pa *ProtocolAdapter) GetSupportedEndpoint(targetType string, isStreaming bool) string {
	switch targetType {
	case util.ChannelTypeOpenAI:
		if isStreaming {
			return "/v1/chat/completions"
		}
		return "/v1/chat/completions"
	case util.ChannelTypeAnthropic:
		return "/v1/messages"
	case util.ChannelTypeCodex:
		return "/v1/responses"
	case util.ChannelTypeGemini:
		if isStreaming {
			return "/v1beta/models/{model}:streamGenerateContent"
		}
		return "/v1beta/models/{model}:generateContent"
	default:
		return ""
	}
}
