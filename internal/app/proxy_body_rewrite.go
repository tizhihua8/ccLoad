// Package app 提供请求体重写功能
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"ccLoad/internal/model"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// BodyRewriteContext 用于渲染条件模板的上下文
type BodyRewriteContext struct {
	Model         string                 `json:"model"`          // 当前使用的模型（可能已重定向）
	OriginalModel string                 `json:"original_model"` // 原始请求的模型
	MaxTokens     int                    `json:"max_tokens"`     // max_tokens 参数值
	Temperature   float64                `json:"temperature"`    // temperature 参数值
	Stream        bool                   `json:"stream"`         // stream 参数值
	Extra         map[string]interface{} `json:"extra"`          // 其他原始请求字段
}

// BuildBodyRewriteContext 从请求体构建重写上下文
func BuildBodyRewriteContext(body []byte) BodyRewriteContext {
	ctx := BodyRewriteContext{
		Extra: make(map[string]interface{}),
	}

	// 解析基本字段
	if model := gjson.GetBytes(body, "model"); model.Exists() {
		ctx.Model = model.String()
		ctx.OriginalModel = model.String()
	}
	if maxTokens := gjson.GetBytes(body, "max_tokens"); maxTokens.Exists() {
		ctx.MaxTokens = int(maxTokens.Int())
	}
	if temp := gjson.GetBytes(body, "temperature"); temp.Exists() {
		ctx.Temperature = temp.Float()
	}
	if stream := gjson.GetBytes(body, "stream"); stream.Exists() {
		ctx.Stream = stream.Bool()
	}

	return ctx
}

// StripDeferLoading 删除 tools 数组中所有元素的 defer_loading 字段
// 用于兼容不支持该 Anthropic 专属字段的上游 API（如 Fireworks、Gemini）
func StripDeferLoading(body []byte) []byte {
	// 先检查是否有 tools 字段
	toolsResult := gjson.GetBytes(body, "tools")
	if !toolsResult.Exists() || !toolsResult.IsArray() {
		return body
	}

	// 获取数组长度
	arrayLen := toolsResult.Array()
	if len(arrayLen) == 0 {
		return body
	}

	result := body
	// 从后往前删除，避免索引变化问题
	for i := len(arrayLen) - 1; i >= 0; i-- {
		path := fmt.Sprintf("tools.%d.defer_loading", i)
		// 检查字段是否存在
		if gjson.GetBytes(result, path).Exists() {
			result, _ = sjson.DeleteBytes(result, path)
		}
	}

	return result
}

// applyBodyOperations 应用请求体重写操作
// 返回修改后的 body 和可能的错误
func applyBodyOperations(body []byte, ops []model.BodyOperation, ctx BodyRewriteContext) ([]byte, error) {
	if len(ops) == 0 {
		return body, nil
	}

	result := body
	for _, op := range ops {
		// 评估条件，不满足则跳过
		if !evaluateBodyOpCondition(op.Condition, ctx) {
			continue
		}

		var err error
		switch model.BodyOperationType(op.Op) {
		case model.BodyOpSet:
			result, err = applyBodyOpSet(result, op, ctx)
		case model.BodyOpDelete:
			result, err = applyBodyOpDelete(result, op)
		case model.BodyOpRename:
			result, err = applyBodyOpRename(result, op)
		case model.BodyOpCopy:
			result, err = applyBodyOpCopy(result, op)
		default:
			err = fmt.Errorf("unknown operation: %s", op.Op)
		}

		if err != nil {
			// 记录警告但继续处理其他操作
			// 调用方可以选择是否中止
			return result, fmt.Errorf("operation %s on path %s failed: %w", op.Op, op.Path, err)
		}
	}

	return result, nil
}

// evaluateBodyOpCondition 评估条件模板
// 空条件视为 true
// 模板结果为 "true" 时视为 true
func evaluateBodyOpCondition(condition string, ctx BodyRewriteContext) bool {
	if strings.TrimSpace(condition) == "" {
		return true
	}

	tmpl, err := template.New("condition").Parse(condition)
	if err != nil {
		// 模板解析失败，视为 true（保守策略）
		return true
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		// 执行失败，视为 true
		return true
	}

	return strings.TrimSpace(buf.String()) == "true"
}

// renderBodyOpValue 渲染值模板
// 如果值不包含 {{ 则直接返回原值
// 否则使用 Go template 渲染
func renderBodyOpValue(value string, ctx BodyRewriteContext) (interface{}, error) {
	if !strings.Contains(value, "{{") {
		// 纯字符串值，尝试解析为 JSON
		var jsonVal interface{}
		if err := json.Unmarshal([]byte(value), &jsonVal); err == nil {
			return jsonVal, nil
		}
		// 不是有效的 JSON，返回字符串
		return value, nil
	}

	tmpl, err := template.New("value").Parse(value)
	if err != nil {
		return value, nil // 解析失败返回原值
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return value, nil // 执行失败返回原值
	}

	rendered := buf.String()

	// 尝试解析为 JSON
	var jsonVal interface{}
	if err := json.Unmarshal([]byte(rendered), &jsonVal); err == nil {
		return jsonVal, nil
	}

	return rendered, nil
}

// applyBodyOpSet 设置字段值
func applyBodyOpSet(body []byte, op model.BodyOperation, ctx BodyRewriteContext) ([]byte, error) {
	value, err := renderBodyOpValue(op.Value, ctx)
	if err != nil {
		return body, err
	}

	return sjson.SetBytes(body, op.Path, value)
}

// applyBodyOpDelete 删除字段
func applyBodyOpDelete(body []byte, op model.BodyOperation) ([]byte, error) {
	return sjson.DeleteBytes(body, op.Path)
}

// applyBodyOpRename 重命名字段（从 From 移动到 To）
func applyBodyOpRename(body []byte, op model.BodyOperation) ([]byte, error) {
	// 获取源值
	src := gjson.GetBytes(body, op.From)
	if !src.Exists() {
		// 源字段不存在，静默跳过
		return body, nil
	}

	// 设置到目标位置
	result, err := sjson.SetBytes(body, op.To, src.Value())
	if err != nil {
		return body, err
	}

	// 删除源字段
	return sjson.DeleteBytes(result, op.From)
}

// applyBodyOpCopy 复制字段（从 From 复制到 To）
func applyBodyOpCopy(body []byte, op model.BodyOperation) ([]byte, error) {
	// 获取源值
	src := gjson.GetBytes(body, op.From)
	if !src.Exists() {
		// 源字段不存在，静默跳过
		return body, nil
	}

	// 设置到目标位置
	return sjson.SetBytes(body, op.To, src.Value())
}
