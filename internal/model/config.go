package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// ModelEntry 模型配置条目
type ModelEntry struct {
	Model         string `json:"model"`                    // 模型名称
	RedirectModel string `json:"redirect_model,omitempty"` // 重定向目标模型（空表示不重定向）
}

// Validate 验证并规范化模型条目
// 返回 error 如果验证失败，否则返回 nil
// 副作用：会 trim 空白字符并写回 Model 和 RedirectModel 字段
func (e *ModelEntry) Validate() error {
	e.Model = strings.TrimSpace(e.Model)
	if e.Model == "" {
		return errors.New("model cannot be empty")
	}
	if strings.ContainsAny(e.Model, "\x00\r\n") {
		return errors.New("model contains illegal characters")
	}

	e.RedirectModel = strings.TrimSpace(e.RedirectModel)
	if strings.ContainsAny(e.RedirectModel, "\x00\r\n") {
		return errors.New("redirect_model contains illegal characters")
	}
	return nil
}

// Config 渠道配置
type Config struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	ChannelType           string `json:"channel_type"` // 渠道类型: "anthropic" | "codex" | "openai" | "gemini"，默认anthropic
	URL                   string `json:"url"`
	Priority              int    `json:"priority"`
	Enabled               bool   `json:"enabled"`
	ScheduledCheckEnabled bool   `json:"scheduled_check_enabled"`
	ScheduledCheckModel   string `json:"scheduled_check_model"`

	// 模型配置（统一管理模型和重定向）
	ModelEntries []ModelEntry `json:"models"`

	// 渠道级冷却（从cooldowns表迁移）
	CooldownUntil      int64 `json:"cooldown_until"`       // Unix秒时间戳，0表示无冷却
	CooldownDurationMs int64 `json:"cooldown_duration_ms"` // 冷却持续时间（毫秒）

	// 每日成本限额
	DailyCostLimit float64 `json:"daily_cost_limit"` // 每日成本限额（美元），0表示无限制

	// User-Agent 覆写（旧版简单字段，已废弃但保留兼容）
	UARewriteEnabled bool   `json:"ua_rewrite_enabled"` // 是否启用 UA 覆写（默认关闭，透传客户端 UA）
	UAOverride       string `json:"ua_override"`        // 完全覆写 UA（非空时替换客户端 UA）
	UAPrefix         string `json:"ua_prefix"`          // UA 前缀追加（在原有 UA 前添加）
	UASuffix         string `json:"ua_suffix"`          // UA 后缀追加（在原有 UA 后添加）

	// User-Agent 配置（新版 JSON 配置，支持复杂 UA 覆写）
	UAConfig *UAConfig `json:"ua_config,omitempty"` // UA 覆写配置（nil 表示未启用新版配置）

	CreatedAt JSONTime `json:"created_at"` // 使用JSONTime确保序列化格式一致（RFC3339）
	UpdatedAt JSONTime `json:"updated_at"` // 使用JSONTime确保序列化格式一致（RFC3339）

	// 缓存Key数量，避免冷却判断时的N+1查询
	KeyCount int `json:"key_count"` // API Key数量（查询时JOIN计算）

	// 模型查找索引（懒加载，不序列化）
	modelIndex map[string]*ModelEntry `json:"-"`
	indexMu    sync.RWMutex           `json:"-"` // 保护索引的并发访问
}

// GetModels 获取所有支持的模型名称列表
func (c *Config) GetModels() []string {
	models := make([]string, 0, len(c.ModelEntries))
	for _, e := range c.ModelEntries {
		models = append(models, e.Model)
	}
	return models
}

// GetURLs 解析URL字段，返回URL列表
// 支持换行分隔多个URL，向后兼容单URL场景
func (c *Config) GetURLs() []string {
	raw := c.URL
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if !strings.Contains(raw, "\n") {
		return []string{trimmed}
	}
	lines := strings.Split(raw, "\n")
	urls := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		urls = append(urls, line)
	}
	return urls
}

// buildIndexIfNeeded 懒加载构建模型查找索引（性能优化：O(n) → O(1)）
// 使用双重检查锁定（DCL）模式保证并发安全
func (c *Config) buildIndexIfNeeded() {
	// 快路径：读锁检查
	c.indexMu.RLock()
	if c.modelIndex != nil {
		c.indexMu.RUnlock()
		return
	}
	c.indexMu.RUnlock()

	// 慢路径：写锁构建
	c.indexMu.Lock()
	defer c.indexMu.Unlock()
	// 双重检查：可能其他 goroutine 已构建
	if c.modelIndex != nil {
		return
	}
	c.modelIndex = make(map[string]*ModelEntry, len(c.ModelEntries))
	for i := range c.ModelEntries {
		c.modelIndex[c.ModelEntries[i].Model] = &c.ModelEntries[i]
	}
}

// GetRedirectModel 获取模型的重定向目标
// 返回 (目标模型, 是否有重定向)
func (c *Config) GetRedirectModel(model string) (string, bool) {
	c.buildIndexIfNeeded()
	c.indexMu.RLock()
	defer c.indexMu.RUnlock()
	if entry, exists := c.modelIndex[model]; exists && entry.RedirectModel != "" {
		return entry.RedirectModel, true
	}
	return "", false
}

// SupportsModel 检查渠道是否支持指定模型
func (c *Config) SupportsModel(model string) bool {
	c.buildIndexIfNeeded()
	c.indexMu.RLock()
	defer c.indexMu.RUnlock()
	_, exists := c.modelIndex[model]
	return exists
}

// GetChannelType 默认返回"anthropic"（Claude API）
func (c *Config) GetChannelType() string {
	if c.ChannelType == "" {
		return "anthropic"
	}
	return c.ChannelType
}

// IsCoolingDown 检查渠道是否处于冷却状态
func (c *Config) IsCoolingDown(now time.Time) bool {
	return c.CooldownUntil > now.Unix()
}

// KeyStrategy 常量定义
const (
	KeyStrategySequential = "sequential"  // 顺序选择：按索引顺序尝试Key
	KeyStrategyRoundRobin = "round_robin" // 轮询选择：均匀分布请求到各个Key
)

// IsValidKeyStrategy 验证KeyStrategy是否有效
func IsValidKeyStrategy(s string) bool {
	return s == "" || s == KeyStrategySequential || s == KeyStrategyRoundRobin
}

// APIKey 表示渠道的 API 密钥配置
type APIKey struct {
	ID        int64  `json:"id"`
	ChannelID int64  `json:"channel_id"`
	KeyIndex  int    `json:"key_index"`
	APIKey    string `json:"api_key"`

	KeyStrategy string `json:"key_strategy"` // "sequential" | "round_robin"

	// Key级冷却（从key_cooldowns表迁移）
	CooldownUntil      int64 `json:"cooldown_until"`
	CooldownDurationMs int64 `json:"cooldown_duration_ms"`

	CreatedAt JSONTime `json:"created_at"`
	UpdatedAt JSONTime `json:"updated_at"`
}

// IsCoolingDown 检查密钥是否处于冷却状态
func (k *APIKey) IsCoolingDown(now time.Time) bool {
	return k.CooldownUntil > now.Unix()
}

// ChannelWithKeys 渠道和API Keys的完整数据
// 用于批量导入导出等需要完整渠道数据的场景
type ChannelWithKeys struct {
	Config  *Config  `json:"config"`
	APIKeys []APIKey `json:"api_keys"` // 不使用指针避免额外分配
}

// FuzzyMatchModel 模糊匹配模型名称
// 当精确匹配失败时，查找包含 query 子串的模型，按版本排序返回最新的
// 返回 (匹配到的模型名, 是否匹配成功)
func (c *Config) FuzzyMatchModel(query string) (string, bool) {
	if query == "" {
		return "", false
	}

	queryLower := strings.ToLower(query)
	var matches []string

	for _, entry := range c.ModelEntries {
		if strings.Contains(strings.ToLower(entry.Model), queryLower) {
			matches = append(matches, entry.Model)
		}
	}

	if len(matches) == 0 {
		return "", false
	}
	if len(matches) == 1 {
		return matches[0], true
	}

	// 多个匹配：按版本排序，取最新
	sortModelsByVersion(matches)
	return matches[0], true
}

// sortModelsByVersion 按版本排序模型列表（最新优先）
// 排序优先级：1.日期后缀 2.版本数字 3.字典序
// 使用标准库 slices.SortFunc，O(n log n) 复杂度
func sortModelsByVersion(models []string) {
	slices.SortFunc(models, func(a, b string) int {
		return -compareModelVersion(a, b) // 降序（最新优先）
	})
}

// compareModelVersion 比较两个模型版本
// 返回 >0 表示 a 更新，<0 表示 b 更新，0 表示相同
func compareModelVersion(a, b string) int {
	// 1. 日期后缀优先（YYYYMMDD）
	dateA := extractDateSuffix(a)
	dateB := extractDateSuffix(b)
	if dateA != dateB {
		if dateA > dateB {
			return 1
		}
		return -1
	}

	// 2. 版本数字序列比较
	verA := extractVersionNumbers(a)
	verB := extractVersionNumbers(b)
	maxLen := len(verA)
	if len(verB) > maxLen {
		maxLen = len(verB)
	}
	for i := 0; i < maxLen; i++ {
		va, vb := 0, 0
		if i < len(verA) {
			va = verA[i]
		}
		if i < len(verB) {
			vb = verB[i]
		}
		if va != vb {
			return va - vb
		}
	}

	// 3. 兜底：字典序
	if a > b {
		return 1
	} else if a < b {
		return -1
	}
	return 0
}

// extractDateSuffix 提取模型名称末尾的日期后缀（YYYYMMDD）
// 返回日期字符串，无日期返回空串
func extractDateSuffix(model string) string {
	// 查找最后一个分隔符
	lastDash := strings.LastIndexByte(model, '-')
	lastDot := strings.LastIndexByte(model, '.')
	lastSep := lastDash
	if lastDot > lastSep {
		lastSep = lastDot
	}
	if lastSep < 0 {
		return ""
	}

	suffix := model[lastSep+1:]
	if len(suffix) != 8 {
		return ""
	}

	// 验证是否全数字
	for i := 0; i < len(suffix); i++ {
		if suffix[i] < '0' || suffix[i] > '9' {
			return ""
		}
	}

	// 简单验证年份范围
	year := (int(suffix[0]-'0') * 1000) + (int(suffix[1]-'0') * 100) +
		(int(suffix[2]-'0') * 10) + int(suffix[3]-'0')
	if year < 2000 || year > 2100 {
		return ""
	}

	return suffix
}

// extractVersionNumbers 提取模型名称中的版本数字
// 例如：gpt-5.2 → [5,2], claude-sonnet-4-5-20250929 → [4,5]
func extractVersionNumbers(model string) []int {
	// 移除日期后缀避免干扰
	if date := extractDateSuffix(model); date != "" {
		model = model[:len(model)-len(date)-1]
	}

	var nums []int
	var current int
	inNumber := false

	for i := 0; i < len(model); i++ {
		c := model[i]
		if c >= '0' && c <= '9' {
			current = current*10 + int(c-'0')
			inNumber = true
		} else {
			if inNumber {
				nums = append(nums, current)
				current = 0
				inNumber = false
			}
		}
	}
	if inNumber {
		nums = append(nums, current)
	}

	return nums
}

// BodyOperationType 请求体操作类型
type BodyOperationType string

const (
	// BodyOpSet 设置字段值
	BodyOpSet BodyOperationType = "set"
	// BodyOpDelete 删除字段
	BodyOpDelete BodyOperationType = "delete"
	// BodyOpRename 重命名字段
	BodyOpRename BodyOperationType = "rename"
	// BodyOpCopy 复制字段
	BodyOpCopy BodyOperationType = "copy"
)

// BodyOperation 请求体重写操作定义
// 支持条件模板（Go text/template语法），条件为 true 时执行
// 可用变量: Model, OriginalModel, MaxTokens, Temperature, Stream 等
type BodyOperation struct {
	Op        string `json:"op"`                  // 操作类型: set/delete/rename/copy
	Path      string `json:"path,omitempty"`      // JSON 路径，如 "stream", "max_tokens"
	From      string `json:"from,omitempty"`      // rename/copy 的源路径
	To        string `json:"to,omitempty"`        // rename/copy 的目标路径
	Value     string `json:"value,omitempty"`     // set 的值（支持模板语法，如 "{{if gt .MaxTokens 4096}}true{{else}}false{{end}}"）
	Condition string `json:"condition,omitempty"` // 条件模板，为空时总是执行
}

// UAConfigMode UA 覆写配置模式
type UAConfigMode string

const (
	// UAConfigModePassThrough 透传模式 - 原样传递客户端 UA
	UAConfigModePassThrough UAConfigMode = "passthrough"
	// UAConfigModeOverride 覆写模式 - 完全替换为指定 UA
	UAConfigModeOverride UAConfigMode = "override"
	// UAConfigModeAppend 追加模式 - 在原有 UA 前后添加内容
	UAConfigModeAppend UAConfigMode = "append"
	// UAConfigModeHeaders Headers模式 - 修改请求头
	UAConfigModeHeaders UAConfigMode = "headers"
)

// UAConfigItem UA 覆写配置条目（用于覆写模式和追加模式）
type UAConfigItem struct {
	Field string `json:"field"` // 字段名，如 "User-Agent", "X-Custom-Header"
	Value string `json:"value"` // 字段值
}

// UAHeaderItem Headers 模式下的请求头配置条目
type UAHeaderItem struct {
	Name   string `json:"name"`   // Header 名称
	Value  string `json:"value"`  // Header 值
	Action string `json:"action"` // 动作: "add" | "set" | "remove"
}

// UAConfig UA 覆写完整配置结构
type UAConfig struct {
	Mode               UAConfigMode      `json:"mode"`                        // 工作模式: passthrough | override | append | headers
	Items              []UAConfigItem    `json:"items,omitempty"`             // 覆写/追加模式的字段列表
	Headers            []UAHeaderItem    `json:"headers,omitempty"`           // Headers 模式的请求头列表
	BodyOperations     []BodyOperation   `json:"body_operations,omitempty"`   // 请求体重写操作列表
	BodyRewriteEnabled bool              `json:"body_rewrite_enabled,omitempty"` // 请求体重写独立开关
}

// IsEnabled 检查 UA 配置是否启用（非透传模式且有内容，或有请求体操作）
func (u *UAConfig) IsEnabled() bool {
	if u == nil {
		return false
	}
	// 有请求体重写操作时也视为启用
	if len(u.BodyOperations) > 0 {
		return true
	}
	switch u.Mode {
	case UAConfigModeOverride, UAConfigModeAppend:
		return len(u.Items) > 0
	case UAConfigModeHeaders:
		return len(u.Headers) > 0
	default:
		return false
	}
}

// Validate 验证 UA 配置是否有效
func (u *UAConfig) Validate() error {
	if u == nil {
		return nil
	}
	// 验证 BodyOperations
	for i, op := range u.BodyOperations {
		if err := op.Validate(); err != nil {
			return fmt.Errorf("body operation[%d]: %w", i, err)
		}
	}
	switch u.Mode {
	case UAConfigModePassThrough:
		return nil
	case UAConfigModeOverride, UAConfigModeAppend:
		if len(u.Items) == 0 {
			return errors.New("override/append mode requires at least one item")
		}
		for _, item := range u.Items {
			if item.Field == "" {
				return errors.New("item field cannot be empty")
			}
		}
		return nil
	case UAConfigModeHeaders:
		if len(u.Headers) == 0 {
			return errors.New("headers mode requires at least one header")
		}
		for _, h := range u.Headers {
			if h.Name == "" {
				return errors.New("header name cannot be empty")
			}
			if h.Action != "add" && h.Action != "set" && h.Action != "remove" {
				return errors.New("header action must be add, set, or remove")
			}
		}
		return nil
	default:
		return errors.New("invalid mode")
	}
}

// Validate 验证 BodyOperation 是否有效
func (b BodyOperation) Validate() error {
	switch BodyOperationType(b.Op) {
	case BodyOpSet:
		if b.Path == "" {
			return errors.New("set operation requires path")
		}
		return nil
	case BodyOpDelete:
		if b.Path == "" {
			return errors.New("delete operation requires path")
		}
		return nil
	case BodyOpRename:
		if b.From == "" || b.To == "" {
			return errors.New("rename operation requires from and to")
		}
		return nil
	case BodyOpCopy:
		if b.From == "" || b.To == "" {
			return errors.New("copy operation requires from and to")
		}
		return nil
	default:
		return fmt.Errorf("invalid operation type: %s", b.Op)
	}
}
