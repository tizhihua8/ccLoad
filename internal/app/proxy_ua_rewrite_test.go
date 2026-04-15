package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"ccLoad/internal/model"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// UA覆写与请求体重写集成测试
// 端到端验证：请求经过代理后，UA/Headers/Body是否正确改写
// ============================================================================

// capturedRequest 记录上游收到的请求信息
type capturedRequest struct {
	mu      sync.Mutex
	headers http.Header
	body    []byte
}

// getHeaders 线程安全地获取捕获的请求头
func (c *capturedRequest) getHeaders() http.Header {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := make(http.Header)
	for k, v := range c.headers {
		h[k] = v
	}
	return h
}

// getBody 线程安全地获取捕获的请求体
func (c *capturedRequest) getBody() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := make([]byte, len(c.body))
	copy(b, c.body)
	return b
}

// getBodyJSON 解析捕获的请求体为 map
func (c *capturedRequest) getBodyJSON(t *testing.T) map[string]any {
	t.Helper()
	body := c.getBody()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("解析上游请求体失败: %v\nbody: %s", err, string(body))
	}
	return m
}

// createCaptureUpstream 创建一个捕获请求的 mock 上游服务器
// 返回服务器和捕获对象，服务器返回标准 OpenAI 格式成功响应
func createCaptureUpstream(t *testing.T) (*httptest.Server, *capturedRequest) {
	t.Helper()
	cap := &capturedRequest{
		headers: make(http.Header),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 捕获请求头
		cap.mu.Lock()
		for k, v := range r.Header {
			cap.headers[k] = v
		}
		// 捕获请求体
		body, _ := io.ReadAll(r.Body)
		cap.body = body
		cap.mu.Unlock()

		// 返回标准成功响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	t.Cleanup(server.Close)
	return server, cap
}

// setupUATestEnv 创建带 UA 配置的测试环境
// 渠道使用 override 模式并携带指定的 uaConfig
func setupUATestEnv(t *testing.T, upstreamURL string, uaConfig *model.UAConfig) *proxyTestEnv {
	t.Helper()

	srv := newInMemoryServer(t)
	store := srv.store
	ctx := context.Background()

	cfg := &model.Config{
		Name:             "ua-test-channel",
		URL:              upstreamURL,
		ChannelType:      util.ChannelTypeOpenAI,
		Priority:         100,
		Enabled:          true,
		UARewriteEnabled: true,
		UAConfig:         uaConfig,
		ModelEntries:     []model.ModelEntry{{Model: "gpt-4"}},
	}
	created, err := store.CreateConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}
	_ = created

	err = store.CreateAPIKeysBatch(ctx, []*model.APIKey{
		{ChannelID: created.ID, KeyIndex: 0, APIKey: "sk-test-ua"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKeysBatch: %v", err)
	}

	injectAPIToken(srv.authService, "test-api-key", 0, 1)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	srv.SetupRoutes(engine)

	return &proxyTestEnv{
		server: srv,
		store:  store,
		engine: engine,
	}
}

// ============================================================================
// UA 覆写模式测试
// ============================================================================

func TestProxy_UA_Override_Mode(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode: model.UAConfigModeOverride,
		Items: []model.UAConfigItem{
			{Field: "User-Agent", Value: "TestAgent/2.0"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, map[string]string{
		"User-Agent": "OriginalClient/1.0",
	})

	ua := cap.getHeaders().Get("User-Agent")
	if ua != "TestAgent/2.0" {
		t.Errorf("UA Override 模式失败: 期望 'TestAgent/2.0', 实际 '%s'", ua)
	}
}

func TestProxy_UA_Append_Mode(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode: model.UAConfigModeAppend,
		Items: []model.UAConfigItem{
			{Field: "User-Agent", Value: "Prefix-"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, map[string]string{
		"User-Agent": "Client/1.0",
	})

	ua := cap.getHeaders().Get("User-Agent")
	if ua != "Prefix-Client/1.0" {
		t.Errorf("UA Append 模式失败: 期望 'Prefix-Client/1.0', 实际 '%s'", ua)
	}
}

func TestProxy_UA_Headers_Mode(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode: model.UAConfigModeHeaders,
		Headers: []model.UAHeaderItem{
			{Name: "X-Custom", Value: "my-value", Action: "set"},
			{Name: "X-To-Remove", Value: "", Action: "remove"},
			{Name: "X-Added", Value: "extra", Action: "add"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, map[string]string{
		"X-To-Remove": "should-be-removed",
	})

	headers := cap.getHeaders()

	// 验证 set 操作
	if v := headers.Get("X-Custom"); v != "my-value" {
		t.Errorf("Headers set 失败: 期望 'my-value', 实际 '%s'", v)
	}

	// 验证 remove 操作
	if v := headers.Get("X-To-Remove"); v != "" {
		t.Errorf("Headers remove 失败: X-To-Remove 应为空, 实际 '%s'", v)
	}

	// 验证 add 操作
	if v := headers.Get("X-Added"); v != "extra" {
		t.Errorf("Headers add 失败: 期望 'extra', 实际 '%s'", v)
	}
}

func TestProxy_UA_Passthrough_Mode(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	// 透传模式 + 无 Items：不应修改任何请求头
	uaConfig := &model.UAConfig{
		Mode: model.UAConfigModePassThrough,
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, map[string]string{
		"User-Agent": "OriginalClient/1.0",
	})

	ua := cap.getHeaders().Get("User-Agent")
	if ua != "OriginalClient/1.0" {
		t.Errorf("UA Passthrough 模式失败: 期望 'OriginalClient/1.0', 实际 '%s'", ua)
	}
}

// ============================================================================
// 请求体重写操作测试
// ============================================================================

func TestProxy_BodyOp_Set(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{Op: string(model.BodyOpSet), Path: "stream", Value: "true"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, nil)

	body := cap.getBodyJSON(t)

	// stream 应被设置为 true
	stream, ok := body["stream"].(bool)
	if !ok || !stream {
		t.Errorf("BodyOp Set 失败: 期望 stream=true, 实际 %v", body["stream"])
	}
}

func TestProxy_BodyOp_Delete(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{Op: string(model.BodyOpDelete), Path: "temperature"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":       "gpt-4",
		"messages":    []map[string]string{{"role": "user", "content": "hi"}},
		"temperature": 0.7,
	}, nil)

	body := cap.getBodyJSON(t)

	// temperature 应被删除
	if _, exists := body["temperature"]; exists {
		t.Errorf("BodyOp Delete 失败: temperature 应不存在, 实际 %v", body["temperature"])
	}

	// model 应仍然存在
	if body["model"] != "gpt-4" {
		t.Errorf("BodyOp Delete 副作用: model 应仍为 'gpt-4', 实际 %v", body["model"])
	}
}

func TestProxy_BodyOp_Rename(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{Op: string(model.BodyOpRename), From: "max_tokens", To: "max_completion_tokens"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":      "gpt-4",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 100,
	}, nil)

	body := cap.getBodyJSON(t)

	// max_tokens 应不存在（已被重命名）
	if _, exists := body["max_tokens"]; exists {
		t.Errorf("BodyOp Rename 失败: max_tokens 应不存在")
	}

	// max_completion_tokens 应存在且值为 100
	maxComp, ok := body["max_completion_tokens"].(float64)
	if !ok || maxComp != 100 {
		t.Errorf("BodyOp Rename 失败: 期望 max_completion_tokens=100, 实际 %v", body["max_completion_tokens"])
	}
}

func TestProxy_BodyOp_Copy(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{Op: string(model.BodyOpCopy), From: "model", To: "original_model"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, nil)

	body := cap.getBodyJSON(t)

	// model 应仍然存在
	if body["model"] != "gpt-4" {
		t.Errorf("BodyOp Copy 失败: model 应仍为 'gpt-4', 实际 %v", body["model"])
	}

	// original_model 应为 "gpt-4"（复制）
	if body["original_model"] != "gpt-4" {
		t.Errorf("BodyOp Copy 失败: 期望 original_model='gpt-4', 实际 %v", body["original_model"])
	}
}

func TestProxy_BodyOp_Set_WithCondition_True(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{
				Op:        string(model.BodyOpSet),
				Path:      "stream",
				Value:     "true",
				Condition: "{{if .Stream}}true{{end}}",
			},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	// 场景A: stream=true → 条件满足，应设置 stream=true
	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
		"stream":   true,
	}, nil)

	body := cap.getBodyJSON(t)
	stream, ok := body["stream"].(bool)
	if !ok || !stream {
		t.Errorf("条件模板(stream=true)失败: 期望 stream=true, 实际 %v", body["stream"])
	}
}

func TestProxy_BodyOp_Set_WithCondition_False(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{
				Op:        string(model.BodyOpSet),
				Path:      "stream",
				Value:     "true",
				Condition: "{{if .Stream}}true{{end}}",
			},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	// 场景B: stream=false 或不存在 → 条件不满足，不应修改
	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
		"stream":   false,
	}, nil)

	body := cap.getBodyJSON(t)

	// stream 应保持 false（条件不满足，操作被跳过）
	stream, ok := body["stream"].(bool)
	if !ok || stream {
		t.Errorf("条件模板(stream=false)失败: stream 应为 false, 实际 %v", body["stream"])
	}
}

func TestProxy_BodyOp_MultipleOps(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			// 1. 删除 temperature
			{Op: string(model.BodyOpDelete), Path: "temperature"},
			// 2. 设置 stream=true
			{Op: string(model.BodyOpSet), Path: "stream", Value: "true"},
			// 3. 复制 model → original_model
			{Op: string(model.BodyOpCopy), From: "model", To: "original_model"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":       "gpt-4",
		"messages":    []map[string]string{{"role": "user", "content": "hi"}},
		"temperature": 0.5,
	}, nil)

	body := cap.getBodyJSON(t)

	// temperature 应被删除
	if _, exists := body["temperature"]; exists {
		t.Errorf("多操作(delete)失败: temperature 应不存在")
	}

	// stream 应被设置为 true
	if stream, ok := body["stream"].(bool); !ok || !stream {
		t.Errorf("多操作(set)失败: 期望 stream=true, 实际 %v", body["stream"])
	}

	// original_model 应被复制
	if body["original_model"] != "gpt-4" {
		t.Errorf("多操作(copy)失败: 期望 original_model='gpt-4', 实际 %v", body["original_model"])
	}

	// model 应保持不变
	if body["model"] != "gpt-4" {
		t.Errorf("多操作副作用: model 应为 'gpt-4', 实际 %v", body["model"])
	}
}

// ============================================================================
// UA + BodyOp 组合测试
// ============================================================================

func TestProxy_UA_BodyOp_Combined(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode: model.UAConfigModeOverride,
		Items: []model.UAConfigItem{
			{Field: "User-Agent", Value: "CombinedTest/1.0"},
		},
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{Op: string(model.BodyOpSet), Path: "stream", Value: "true"},
			{Op: string(model.BodyOpDelete), Path: "temperature"},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":       "gpt-4",
		"messages":    []map[string]string{{"role": "user", "content": "hi"}},
		"temperature": 0.7,
	}, map[string]string{
		"User-Agent": "Original/1.0",
	})

	// 验证 UA 覆写生效
	ua := cap.getHeaders().Get("User-Agent")
	if ua != "CombinedTest/1.0" {
		t.Errorf("组合测试 UA 失败: 期望 'CombinedTest/1.0', 实际 '%s'", ua)
	}

	// 验证 Body 操作生效
	body := cap.getBodyJSON(t)

	if stream, ok := body["stream"].(bool); !ok || !stream {
		t.Errorf("组合测试 BodyOp(set)失败: 期望 stream=true, 实际 %v", body["stream"])
	}

	if _, exists := body["temperature"]; exists {
		t.Errorf("组合测试 BodyOp(delete)失败: temperature 应不存在")
	}

	if body["model"] != "gpt-4" {
		t.Errorf("组合测试副作用: model 应为 'gpt-4', 实际 %v", body["model"])
	}
}

// ============================================================================
// BodyOp 条件模板高级测试
// ============================================================================

func TestProxy_BodyOp_Set_WithMaxTokensCondition(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	// 模拟 Fireworks AI 场景: max_tokens > 4096 时设置 stream_options
	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{
				Op:        string(model.BodyOpSet),
				Path:      "stream_options.include_usage",
				Value:     "true",
				Condition: "{{if gt .MaxTokens 4096}}true{{end}}",
			},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	// 场景: max_tokens=8192 → 条件满足
	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":      "gpt-4",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 8192,
	}, nil)

	body := cap.getBodyJSON(t)

	// stream_options.include_usage 应为 true
	streamOpts, ok := body["stream_options"].(map[string]any)
	if !ok {
		t.Errorf("条件模板(max_tokens=8192)失败: stream_options 不存在或类型错误, 实际 %v", body["stream_options"])
	} else if includeUsage, ok := streamOpts["include_usage"].(bool); !ok || !includeUsage {
		t.Errorf("条件模板(max_tokens=8192)失败: 期望 include_usage=true, 实际 %v", streamOpts["include_usage"])
	}
}

func TestProxy_BodyOp_Set_WithMaxTokensCondition_NotMet(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	uaConfig := &model.UAConfig{
		Mode:               model.UAConfigModePassThrough,
		BodyRewriteEnabled: true,
		BodyOperations: []model.BodyOperation{
			{
				Op:        string(model.BodyOpSet),
				Path:      "stream_options.include_usage",
				Value:     "true",
				Condition: "{{if gt .MaxTokens 4096}}true{{end}}",
			},
		},
	}

	env := setupUATestEnv(t, upstream.URL, uaConfig)

	// 场景: max_tokens=1024 → 条件不满足
	doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":      "gpt-4",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1024,
	}, nil)

	body := cap.getBodyJSON(t)

	// stream_options 不应存在
	if _, exists := body["stream_options"]; exists {
		t.Errorf("条件模板(max_tokens=1024)失败: stream_options 不应存在, 实际 %v", body["stream_options"])
	}
}

// ============================================================================
// UA 旧版兼容性测试
// ============================================================================

func TestProxy_UA_Legacy_Override(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	srv := newInMemoryServer(t)
	store := srv.store
	ctx := context.Background()

	// 使用旧版简单字段（无 UAConfig）
	cfg := &model.Config{
		Name:             "legacy-ua-channel",
		URL:              upstream.URL,
		ChannelType:      util.ChannelTypeOpenAI,
		Priority:         100,
		Enabled:          true,
		UARewriteEnabled: true,
		UAOverride:       "LegacyAgent/3.0",
		ModelEntries:     []model.ModelEntry{{Model: "gpt-4"}},
	}
	created, err := store.CreateConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}
	err = store.CreateAPIKeysBatch(ctx, []*model.APIKey{
		{ChannelID: created.ID, KeyIndex: 0, APIKey: "sk-test-legacy"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKeysBatch: %v", err)
	}

	injectAPIToken(srv.authService, "test-api-key", 0, 1)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	srv.SetupRoutes(engine)

	doProxyRequest(t, engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, map[string]string{
		"User-Agent": "Original/1.0",
	})

	ua := cap.getHeaders().Get("User-Agent")
	if ua != "LegacyAgent/3.0" {
		t.Errorf("旧版UA覆写失败: 期望 'LegacyAgent/3.0', 实际 '%s'", ua)
	}
}

func TestProxy_UA_Legacy_PrefixSuffix(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	srv := newInMemoryServer(t)
	store := srv.store
	ctx := context.Background()

	cfg := &model.Config{
		Name:             "legacy-prefix-channel",
		URL:              upstream.URL,
		ChannelType:      util.ChannelTypeOpenAI,
		Priority:         100,
		Enabled:          true,
		UARewriteEnabled: true,
		UAPrefix:         "Prefix-",
		UASuffix:         "-Suffix",
		ModelEntries:     []model.ModelEntry{{Model: "gpt-4"}},
	}
	created, err := store.CreateConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}
	err = store.CreateAPIKeysBatch(ctx, []*model.APIKey{
		{ChannelID: created.ID, KeyIndex: 0, APIKey: "sk-test-prefix"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKeysBatch: %v", err)
	}

	injectAPIToken(srv.authService, "test-api-key", 0, 1)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	srv.SetupRoutes(engine)

	doProxyRequest(t, engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, map[string]string{
		"User-Agent": "Client/1.0",
	})

	ua := cap.getHeaders().Get("User-Agent")
	expected := "Prefix-Client/1.0-Suffix"
	if ua != expected {
		t.Errorf("旧版UA前缀后缀失败: 期望 '%s', 实际 '%s'", expected, ua)
	}
}

// ============================================================================
// UA 禁用状态测试
// ============================================================================

func TestProxy_UA_Disabled_NoRewrite(t *testing.T) {
	t.Parallel()

	upstream, cap := createCaptureUpstream(t)

	srv := newInMemoryServer(t)
	store := srv.store
	ctx := context.Background()

	// UARewriteEnabled=false → 不应修改 UA
	cfg := &model.Config{
		Name:             "disabled-ua-channel",
		URL:              upstream.URL,
		ChannelType:      util.ChannelTypeOpenAI,
		Priority:         100,
		Enabled:          true,
		UARewriteEnabled: false,
		UAOverride:       "ShouldNotAppear",
		ModelEntries:     []model.ModelEntry{{Model: "gpt-4"}},
	}
	created, err := store.CreateConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}
	err = store.CreateAPIKeysBatch(ctx, []*model.APIKey{
		{ChannelID: created.ID, KeyIndex: 0, APIKey: "sk-test-disabled"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKeysBatch: %v", err)
	}

	injectAPIToken(srv.authService, "test-api-key", 0, 1)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	srv.SetupRoutes(engine)

	doProxyRequest(t, engine, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, map[string]string{
		"User-Agent": "Original/1.0",
	})

	ua := cap.getHeaders().Get("User-Agent")
	// UA覆写禁用时，应透传客户端原始 UA
	if ua == "ShouldNotAppear" {
		t.Errorf("UA禁用失败: UA不应被覆写为 'ShouldNotAppear'")
	}
	if ua != "Original/1.0" {
		t.Errorf("UA禁用失败: 期望透传原始UA 'Original/1.0', 实际 '%s'", ua)
	}
}
