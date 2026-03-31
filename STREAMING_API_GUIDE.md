# 流式输出使用指南

## 概述

系统现在支持两种输出模式:
- **非流式模式** (`stream: false`): 等待完成后一次性返回全部内容
- **流式模式** (`stream: true`): 逐词实时输出,类似ChatGPT

## API 端点

### 1. Chat Completions API

**端点**: `POST /v1/chat/completions`

**请求格式**:
```json
{
  "model": "llama-7b",
  "messages": [
    {
      "role": "user",
      "content": "你好"
    }
  ],
  "max_tokens": 2046,
  "temperature": 0.7,
  "stream": true
}
```

**参数说明**:
- `model`: 模型名称
- `messages`: 对话消息数组
- `max_tokens`: 最大输出token数 (影响输出长度)
- `temperature`: 温度参数 (0.0-2.0, 越高越随机)
- `stream`: 是否使用流式输出 (true/false)

### 2. Completions API

**端点**: `POST /v1/completions`

**请求格式**:
```json
{
  "prompt": "你好",
  "max_tokens": 2046,
  "temperature": 0.7,
  "stream": true
}
```

**参数说明**:
- `prompt`: 输入提示
- `max_tokens`: 最大输出token数
- `temperature`: 温度参数
- `stream`: 是否使用流式输出 (true/false)

## 使用示例

### 非流式模式 (一次性输出)

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-7b",
    "messages": [
      {"role": "user", "content": "写一首诗"}
    ],
    "max_tokens": 500,
    "stream": false
  }'
```

**响应** (一次性返回完整内容):
```json
{
  "id": "chatcmpl-123456",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "llama-7b",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "春江潮水连海平，海上明月共潮生..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "total_tokens": 100
  }
}
```

### 流式模式 (逐词输出)

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-7b",
    "messages": [
      {"role": "user", "content": "写一首诗"}
    ],
    "max_tokens": 500,
    "stream": true
  }'
```

**响应** (Server-Sent Events格式):
```
data: {"choices":[{"index":0,"delta":{"content":"春"}}]}

data: {"choices":[{"index":0,"delta":{"content":"江"}}]}

data: {"choices":[{"index":0,"delta":{"content":"潮"}}]}

data: {"choices":[{"index":0,"delta":{"content":"水"}}]}

data: {"choices":[{"index":0,"delta":{"content":"连"}}]}

data: [DONE]
```

## Python 客户端示例

### 非流式模式

```python
import requests

url = "http://localhost:8080/v1/chat/completions"
data = {
    "model": "llama-7b",
    "messages": [
        {"role": "user", "content": "你好"}
    ],
    "max_tokens": 2046,
    "temperature": 0.7,
    "stream": False
}

response = requests.post(url, json=data)
result = response.json()

print(result['choices'][0]['message']['content'])
```

### 流式模式

```python
import requests

url = "http://localhost:8080/v1/chat/completions"
data = {
    "model": "llama-7b",
    "messages": [
        {"role": "user", "content": "你好"}
    ],
    "max_tokens": 2046,
    "temperature": 0.7,
    "stream": True
}

response = requests.post(url, json=data, stream=True)

for line in response.iter_lines():
    if line:
        line = line.decode('utf-8')
        if line.startswith('data: '):
            data_str = line[6:]  # Remove "data: " prefix
            if data_str == '[DONE]':
                break
            try:
                import json
                data_json = json.loads(data_str)
                if 'choices' in data_json and len(data_json['choices']) > 0:
                    delta = data_json['choices'][0].get('delta', {})
                    content = delta.get('content', '')
                    print(content, end='', flush=True)
            except json.JSONDecodeError:
                pass
```

## JavaScript 客户端示例

### 非流式模式

```javascript
async function chatNonStream() {
  const response = await fetch('http://localhost:8080/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model: 'llama-7b',
      messages: [
        { role: 'user', content: '你好' }
      ],
      max_tokens: 2046,
      temperature: 0.7,
      stream: false
    })
  });

  const result = await response.json();
  console.log(result.choices[0].message.content);
}
```

### 流式模式

```javascript
async function chatStream() {
  const response = await fetch('http://localhost:8080/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model: 'llama-7b',
      messages: [
        { role: 'user', content: '你好' }
      ],
      max_tokens: 2046,
      temperature: 0.7,
      stream: true
    })
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    const chunk = decoder.decode(value);
    const lines = chunk.split('\n');

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        const dataStr = line.slice(6);
        if (dataStr === '[DONE]') break;

        try {
          const data = JSON.parse(dataStr);
          if (data.choices && data.choices[0]) {
            const delta = data.choices[0].delta;
            if (delta && delta.content) {
              document.getElementById('output').textContent += delta.content;
            }
          }
        } catch (e) {
          // Skip invalid JSON
        }
      }
    }
  }
}
```

## 参数调优

### max_tokens (最大输出长度)

**影响**: 控制输出的最大token数量

**推荐值**:
- 短对话: 100-500 tokens
- 中等对话: 500-1500 tokens
- 长对话/代码生成: 1500-4096 tokens

**注意**: 
- `max_tokens` 只是最大限制,实际输出可能更少
- 中文一个汉字通常需要 2-3 tokens
- 2046 tokens 大约相当于 800-1000 个汉字

### temperature (温度参数)

**影响**: 控制输出的随机性

**推荐值**:
- 0.0-0.3: 保守、确定性的输出
- 0.4-0.7: 平衡的创意和确定性
- 0.8-2.0: 创意性强、随机性高

**示例**:
```json
{
  "temperature": 0.0   // 确定性输出,适合代码生成
}
{
  "temperature": 0.7   // 平衡输出,适合一般对话
}
{
  "temperature": 1.2   // 创意输出,适合创意写作
}
```

## 性能优化

### 流式模式优势
- ✅ 更好的用户体验,立即看到输出
- ✅ 减少感知延迟
- ✅ 可以提前终止长生成

### 非流式模式优势
- ✅ 更简单的客户端实现
- ✅ 更适合需要完整内容的场景
- ✅ 更容易缓存和重试

### 选择建议
- 交互式对话: 使用流式模式
- 后台处理: 使用非流式模式
- 大批量请求: 根据场景选择

## 故障排除

### 流式输出断断续续
- 检查网络连接稳定性
- 增加HTTP服务器的超时设置
- 确保服务器资源充足

### max_tokens 不生效
- 确认传递的 `max_tokens` 参数正确
- 检查服务器日志中的请求参数
- 注意: `max_tokens` 是上限,实际可能更少

### 流式输出不工作
- 确认 `stream: true` 参数正确设置
- 检查HTTP响应头是否包含 `Content-Type: text/event-stream`
- 确保客户端支持Server-Sent Events

## 高级功能

### 动态调整 max_tokens
根据对话长度动态调整:
```python
def get_max_tokens(prompt_length):
    if prompt_length < 100:
        return 500
    elif prompt_length < 500:
        return 1500
    else:
        return 2046
```

### 流式与非流式混合使用
在同一个会话中根据请求类型切换:
```python
def should_stream(request_type):
    streaming_types = ['chat', 'generation']
    return request_type in streaming_types
```

## 测试

### 测试非流式模式
```bash
curl -X POST http://localhost:8080/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Hello", "max_tokens": 100, "stream": false}'
```

### 测试流式模式
```bash
curl -X POST http://localhost:8080/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Hello", "max_tokens": 100, "stream": true}'
```

## 总结

现在您可以通过以下方式控制输出行为:

1. **使用 `stream: false`**: 获得一次性完整输出
2. **使用 `stream: true`**: 获得逐词实时输出
3. **调整 `max_tokens`**: 控制最大输出长度
4. **调整 `temperature`**: 控制输出随机性

这些功能现在已经完全可用!
