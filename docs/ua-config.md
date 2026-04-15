# UA 覆写与请求体重写配置指南

本文档介绍如何在 ccLoad 中配置 User-Agent 覆写和请求体重写，用于解决上游 API 兼容性问题。

## 概述

UA 覆写配置位于渠道（Channel）级别，通过 `ua_config` 字段中的 `body_operations` 数组定义请求体重写规则。

## 操作类型

| 操作 | 说明 | 必填字段 |
|------|------|----------|
| `set` | 设置/修改字段值 | `path`, `value` |
| `delete` | 删除字段 | `path` |
| `rename` | 重命名字段（移动） | `from`, `to` |
| `copy` | 复制字段 | `from`, `to` |

## 常用字段路径

| 路径 | 说明 |
|------|------|
| `model` | 模型名称 |
| `max_tokens` | 最大 token 数 |
| `temperature` | 温度参数 |
| `stream` | 是否流式输出 |
| `json_mode` | JSON 模式 |
| `response_format` | 响应格式 |

## 模板变量

在 `value` 和 `condition` 中可以使用以下变量：

| 变量 | 类型 | 说明 |
|------|------|------|
| `.Model` | string | 当前模型（可能已重定向） |
| `.OriginalModel` | string | 原始请求的模型 |
| `.MaxTokens` | int | 最大 token 数 |
| `.Temperature` | float64 | 温度参数 |
| `.Stream` | bool | 是否流式 |

## 条件模板语法

| 语法 | 说明 |
|------|------|
| `{{if gt .MaxTokens 4096}}true{{end}}` | 大于 4096 |
| `{{if eq .Model "gpt-4"}}true{{end}}` | 等于指定模型 |
| `{{if and (gt .MaxTokens 4096) (eq .Stream false)}}true{{end}}` | 组合条件 |

## 常见问题解决方案

### 1. 删除不支持的字段（如 json_mode）

**错误：** `unexpected keyword argument 'json_mode'`

**配置：**
```json
{
  "body_operations": [
    {
      "op": "delete",
      "path": "json_mode"
    }
  ]
}
```

### 2. 条件设置 stream 为 true

**错误：** `max_tokens > 4096 must have stream=true`

**配置：**
```json
{
  "body_operations": [
    {
      "op": "set",
      "path": "stream",
      "value": "true",
      "condition": "{{if gt .MaxTokens 4096}}true{{end}}"
    }
  ]
}
```

### 3. 组合配置（推荐）

同时解决 json_mode 和 max_tokens 问题：

```json
{
  "body_operations": [
    {
      "op": "delete",
      "path": "json_mode"
    },
    {
      "op": "set",
      "path": "stream",
      "value": "true",
      "condition": "{{if gt .MaxTokens 4096}}true{{end}}"
    }
  ]
}
```

## 高级示例

### 根据模型设置不同参数

```json
{
  "body_operations": [
    {
      "op": "set",
      "path": "temperature",
      "value": "0.5",
      "condition": "{{if eq .Model \"gpt-4\"}}true{{end}}"
    }
  ]
}
```

### 重命名字段

将 `max_tokens` 改为 `max_completion_tokens`（兼容某些 OpenAI 兼容层）：

```json
{
  "body_operations": [
    {
      "op": "rename",
      "from": "max_tokens",
      "to": "max_completion_tokens"
    }
  ]
}
```

### 复制字段

```json
{
  "body_operations": [
    {
      "op": "copy",
      "from": "model",
      "to": "model_override"
    }
  ]
}
```

## 配置位置

1. 打开 Admin 管理界面
2. 进入「渠道管理」
3. 编辑目标渠道
4. 找到 **UA 覆写配置** (UA Config) 字段
5. 填入上述 JSON 配置
6. 保存即可生效

## 调试

启用调试日志查看重写前后的请求体：

```bash
go run -tags go_json .  # 或 ./ccload
```

日志中会输出：
```
[DEBUG] [BodyRewrite] 渠道ID=123 重写前: {...}
[DEBUG] [BodyRewrite] 渠道ID=123 重写后: {...}
```

## 注意事项

1. `condition` 为空时，操作总是执行
2. 条件模板结果为 `"true"` 时才执行
3. 操作按数组顺序依次执行
4. 单个操作失败不会中断后续操作（但会记录错误）
5. 支持嵌套路径，如 `response_format.type`
