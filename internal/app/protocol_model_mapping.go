package app

import (
	"strings"
	"sync"

	"ccLoad/internal/util"
)

// ModelMapping 模型名称映射表
// 支持跨协议模型名称转换，如 gpt-4o -> claude-3-5-sonnet
type ModelMapping struct {
	mu       sync.RWMutex
	mappings map[string]map[string]map[string]string // sourceType -> targetType -> sourceModel -> targetModel
}

// NewModelMapping 创建默认模型映射
func NewModelMapping() *ModelMapping {
	mm := &ModelMapping{
		mappings: make(map[string]map[string]map[string]string),
	}
	mm.initDefaultMappings()
	return mm
}

// initDefaultMappings 初始化默认映射表
// 这些映射关系基于模型能力对等原则
func (mm *ModelMapping) initDefaultMappings() {
	// OpenAI -> Anthropic
	mm.addMapping(util.ChannelTypeOpenAI, util.ChannelTypeAnthropic, map[string]string{
		"gpt-4o":               "claude-3-5-sonnet-20241022",
		"gpt-4o-2024-08-06":    "claude-3-5-sonnet-20241022",
		"gpt-4o-2024-05-13":    "claude-3-5-sonnet-20241022",
		"gpt-4o-latest":        "claude-3-5-sonnet-20241022",
		"gpt-4o-mini":          "claude-3-5-haiku-20241022",
		"gpt-4o-mini-2024-07-18": "claude-3-5-haiku-20241022",
		"gpt-4-turbo":          "claude-3-opus-20240229",
		"gpt-4-turbo-2024-04-09": "claude-3-opus-20240229",
		"gpt-4":                "claude-3-opus-20240229",
		"gpt-4-0125-preview":   "claude-3-opus-20240229",
		"gpt-4-1106-preview":   "claude-3-opus-20240229",
		"gpt-4-vision-preview": "claude-3-opus-20240229",
		"gpt-3.5-turbo":        "claude-3-5-haiku-20241022",
		"gpt-3.5-turbo-0125":   "claude-3-5-haiku-20241022",
		"o1":                   "claude-3-opus-20240229",
		"o1-preview":           "claude-3-opus-20240229",
		"o1-mini":              "claude-3-5-sonnet-20241022",
		"o3-mini":              "claude-3-5-sonnet-20241022",
	})

	// Anthropic -> OpenAI
	mm.addMapping(util.ChannelTypeAnthropic, util.ChannelTypeOpenAI, map[string]string{
		"claude-3-5-sonnet":         "gpt-4o",
		"claude-3-5-sonnet-20241022": "gpt-4o",
		"claude-3-5-sonnet-latest":  "gpt-4o",
		"claude-3-5-haiku":          "gpt-4o-mini",
		"claude-3-5-haiku-20241022": "gpt-4o-mini",
		"claude-3-5-haiku-latest":    "gpt-4o-mini",
		"claude-3-opus":             "gpt-4-turbo",
		"claude-3-opus-20240229":    "gpt-4-turbo",
		"claude-3-opus-latest":      "gpt-4-turbo",
		"claude-3-sonnet":           "gpt-4",
		"claude-3-sonnet-20240229":  "gpt-4",
		"claude-3-haiku":            "gpt-3.5-turbo",
		"claude-3-haiku-20240307":   "gpt-3.5-turbo",
		"claude-2":                  "gpt-4",
		"claude-2.1":                "gpt-4",
		"claude-instant":            "gpt-3.5-turbo",
		"claude-instant-1.2":        "gpt-3.5-turbo",
	})
}

// addMapping 添加单向映射
func (mm *ModelMapping) addMapping(sourceType, targetType string, mapping map[string]string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.mappings[sourceType] == nil {
		mm.mappings[sourceType] = make(map[string]map[string]string)
	}
	if mm.mappings[sourceType][targetType] == nil {
		mm.mappings[sourceType][targetType] = make(map[string]string)
	}

	for sourceModel, targetModel := range mapping {
		mm.mappings[sourceType][targetType][normalizeModelName(sourceModel)] = targetModel
	}
}

// MapModel 映射模型名称
// 如果找不到映射关系，返回原始模型名（允许渠道自身处理模型别名）
func (mm *ModelMapping) MapModel(model, sourceType, targetType string) string {
	if sourceType == targetType || model == "" {
		return model
	}

	mm.mu.RLock()
	defer mm.mu.RUnlock()

	normalized := normalizeModelName(model)

	// 查找精确映射
	if targetMap, ok := mm.mappings[sourceType]; ok {
		if modelMap, ok := targetMap[targetType]; ok {
			if targetModel, ok := modelMap[normalized]; ok {
				return targetModel
			}
		}
	}

	// 尝试前缀匹配（如 claude-3-5-sonnet 匹配到带日期后缀的版本）
	if targetMap, ok := mm.mappings[sourceType]; ok {
		if modelMap, ok := targetMap[targetType]; ok {
			for sourcePattern, targetModel := range modelMap {
				if strings.HasPrefix(normalized, sourcePattern) || strings.HasPrefix(sourcePattern, normalized) {
					return targetModel
				}
			}
		}
	}

	// 无映射关系，返回原始模型名
	// 由上游渠道自身的重定向/模糊匹配处理
	return model
}

// AddCustomMapping 动态添加映射（支持运行时配置/数据库加载）
func (mm *ModelMapping) AddCustomMapping(sourceType, targetType, sourceModel, targetModel string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.mappings[sourceType] == nil {
		mm.mappings[sourceType] = make(map[string]map[string]string)
	}
	if mm.mappings[sourceType][targetType] == nil {
		mm.mappings[sourceType][targetType] = make(map[string]string)
	}

	mm.mappings[sourceType][targetType][normalizeModelName(sourceModel)] = targetModel
}

// normalizeModelName 规范化模型名（小写、去空格）
func normalizeModelName(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

// GetAllMappings 获取指定方向的完整映射表（用于管理界面展示）
func (mm *ModelMapping) GetAllMappings(sourceType, targetType string) map[string]string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	result := make(map[string]string)
	if targetMap, ok := mm.mappings[sourceType]; ok {
		if modelMap, ok := targetMap[targetType]; ok {
			for k, v := range modelMap {
				result[k] = v
			}
		}
	}
	return result
}

