# Gemini API 参考

## 概述

AxonHub 原生支持 Gemini API，可访问 Gemini 强大的多模态功能。您可以使用 Gemini SDK 访问 Gemini 模型，也可以访问 OpenAI、Anthropic 和其他支持的模型。

GenerateContent 请求也接受 `system_instruction`、`inline_data` 和 `mime_type`，分别作为 `systemInstruction`、`inlineData` 和 `mimeType` 的兼容别名。两种写法都有值时，优先使用 camelCase 字段的值。Gemini 输出仍使用标准的 camelCase 字段名。

## 核心优势

- **API 互操作性**：使用 Gemini API 调用 OpenAI、Anthropic 和其他支持的模型
- **零代码变更**：继续使用现有的 Gemini 客户端 SDK，无需修改
- **自动转换**：AxonHub 在需要时自动在 API 格式之间进行转换
- **多模态支持**：通过 Gemini API 格式访问文本和图像功能

## 支持的端点

**端点：**
- `POST /gemini/v1beta/models/{model}:generateContent` - 文本和多模态内容生成
- `POST /v1beta/models/{model}:generateContent` - 文本和多模态内容生成 (可选)
- `GET /gemini/v1beta/models` - 列出可用模型
- `GET /v1beta/models` - 列出可用模型 (可选)
- `GET /gemini/v1beta/v1/models` - 以 OpenAI 形状列出可用模型及发现字段
- `GET /v1beta/v1/models` - 以 OpenAI 形状列出可用模型及发现字段 (可选)

**示例请求：**
```go
import (
    "context"
    "google.golang.org/genai"
)

// 使用 AxonHub 配置创建 Gemini 客户端
ctx := context.Background()
client, err := genai.NewClient(ctx, &genai.ClientConfig{
    APIKey:  "your-axonhub-api-key",
    Backend: genai.Backend(genai.APIBackendUnspecified), // 使用默认后端
    HTTPOptions: genai.HTTPOptions{
			BaseURL: "http://localhost:8090/gemini",
	},
})
if err != nil {
    // 适当处理错误
    panic(err)
}

// 使用 Gemini API 格式调用 OpenAI 模型
modelName := "gpt-4o"  // 通过 Gemini API 格式访问 OpenAI 模型
content := &genai.Content{
    Parts: []*genai.Part{
        {Text: genai.Ptr("Hello, GPT!")},
    },
}

// 可选：配置生成参数
config := &genai.GenerateContentConfig{
    Temperature: genai.Ptr(float32(0.7)),
    MaxOutputTokens: genai.Ptr(int32(1024)),
}

response, err := client.Models.GenerateContent(ctx, modelName, []*genai.Content{content}, config)
if err != nil {
    // 适当处理错误
    panic(err)
}

// 从响应中提取文本
if len(response.Candidates) > 0 &&
   len(response.Candidates[0].Content.Parts) > 0 {
    responseText := response.Candidates[0].Content.Parts[0].Text
    fmt.Println(*responseText)
}
```

**示例：多轮对话**
```go
// 创建带有对话历史的聊天会话
modelName := "claude-3-5-sonnet"
config := &genai.GenerateContentConfig{
    Temperature: genai.Ptr(float32(0.5)),
}

chat, err := client.Chats.Create(ctx, modelName, config, nil)
if err != nil {
    panic(err)
}

// 第一条消息
response1, err := chat.SendMessage(ctx, genai.Part{Text: genai.Ptr("My name is Alice")})
if err != nil {
    panic(err)
}

// 后续消息（模型记住上下文）
response2, err := chat.SendMessage(ctx, genai.Part{Text: genai.Ptr("What is my name?")})
if err != nil {
    panic(err)
}

// 提取响应
if len(response2.Candidates) > 0 {
    text := response2.Candidates[0].Content.Parts[0].Text
    fmt.Println(*text)  // 应该包含 "Alice"
}
```

### 模型发现列表

`GET /gemini/v1beta/models` 按 Gemini 原生形状返回 `models[]`。`GET /gemini/v1beta/v1/models` 按 OpenAI 列表形状返回模型，供在 Gemini 推理根后请求 `/v1/models` 的客户端（如 OMP 的 `openai-models-list`）发现模型，每行附带模型卡记录的发现字段：

- `context_length` - 上下文窗口的最大令牌数。
- `max_output_tokens` - 输出的最大令牌数。
- `input_modalities` - 输入模态，如 `["text", "image"]`。
- `supported_reasoning_levels` - 推理档位。模型卡标记支持推理时才返回。

字段值为空或为 0 时省略。同一 API Key 请求时，模型集合与 `GET /gemini/v1beta/models`、未带 `include` 的 `/v1/models`、`/anthropic/v1/models` 相同，上述发现字段的名称与取值和后两者相同。

认证方式与其他 Gemini 端点相同：`?key=` 优先，其次读请求头，`Authorization: Bearer <your-api-key>` 也可用。

```json
{
  "object": "list",
  "data": [
    {
      "id": "gemini-2.5-pro",
      "object": "model",
      "created": 1767225600,
      "owned_by": "configured",
      "context_length": 1048576,
      "max_output_tokens": 65536,
      "input_modalities": ["text", "image"],
      "supported_reasoning_levels": ["low", "medium", "high"]
    }
  ]
}
```

## API 转换能力

AxonHub 自动在 API 格式之间进行转换，实现以下强大场景：

### 使用 Gemini SDK 调用 OpenAI 模型
```go
// Gemini SDK 调用 OpenAI 模型
content := &genai.Content{
    Parts: []*genai.Part{
        {Text: genai.Ptr("什么是人工智能？")},
    },
}

response, err := client.Models.GenerateContent(
    ctx,
    "gpt-4o",  // OpenAI 模型
    []*genai.Content{content},
    nil,
)

// 访问响应
if len(response.Candidates) > 0 &&
   len(response.Candidates[0].Content.Parts) > 0 {
    text := response.Candidates[0].Content.Parts[0].Text
    fmt.Println(*text)
}
// AxonHub 自动转换 Gemini 格式 → OpenAI 格式
```

### 使用 Gemini SDK 调用 Anthropic 模型
```go
// Gemini SDK 调用 Anthropic 模型
content := &genai.Content{
    Parts: []*genai.Part{
        {Text: genai.Ptr("解释神经网络")},
    },
}

response, err := client.Models.GenerateContent(
    ctx,
    "claude-3-5-sonnet",  // Anthropic 模型
    []*genai.Content{content},
    nil,
)

// 访问响应
if len(response.Candidates) > 0 &&
   len(response.Candidates[0].Content.Parts) > 0 {
    text := response.Candidates[0].Content.Parts[0].Text
    fmt.Println(*text)
}
// AxonHub 自动转换 Gemini 格式 → Anthropic 格式
```

## 认证

Gemini API 格式使用以下认证方式：

- **头部**：`X-Goog-API-Key: <your-api-key>`

API 密钥通过 AxonHub 的 API 密钥管理系统进行管理，无论使用哪种 API 格式，都提供相同的权限。

## 流式支持

Gemini API 格式支持流式响应以进行实时内容生成。

## 错误处理

Gemini 格式错误响应遵循标准 Gemini API 错误格式。

## 工具支持

AxonHub 通过 Gemini API 格式支持**函数工具**（自定义函数调用）。但是，**不支持**各提供商特有的工具：

| 工具类型 | 支持状态 | 说明 |
| -------- | -------- | ---- |
| **函数工具（Function Tools）** | ✅ 支持 | 自定义函数定义可跨所有提供商使用 |
| **网页搜索（Web Search）** | ❌ 不支持 | 提供商特有功能 |
| **代码解释器（Code Interpreter）** | ❌ 不支持 | 提供商特有功能 |
| **文件搜索（File Search）** | ❌ 不支持 | 提供商特有功能 |
| **计算机使用（Computer Use）** | ❌ 不支持 | Anthropic 特有功能 |

> **注意**：仅支持可跨提供商转换的通用函数工具。提供商特有工具需要直接访问提供商的基础设施，无法通过 AxonHub 代理。

## 最佳实践

1. **使用追踪头部**：包含 `AH-Trace-Id` 和 `AH-Thread-Id` 头部以获得更好的可观测性
2. **模型选择**：在请求中明确指定目标模型
3. **错误处理**：为 API 响应实现适当的错误处理
4. **流式处理**：对于长响应使用流式处理以获得更好的用户体验
5. **多模态内容**：在处理图像时利用 Gemini API 的多模态功能

## 迁移指南

### 从 Gemini 迁移到 AxonHub
```go
// 之前：直接 Gemini
ctx := context.Background()
client, err := genai.NewClient(ctx, &genai.ClientConfig{
    APIKey: "gemini-api-key",
})

// 之后：使用 Gemini API 的 AxonHub
ctx := context.Background()
client, err := genai.NewClient(ctx, &genai.ClientConfig{
    APIKey: "your-axonhub-api-key",
    HTTPOptions: genai.HTTPOptions{
        BaseURL: "http://localhost:8090/gemini",
    },
})
// 您的现有代码继续工作！
```
