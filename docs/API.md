# llama-go-server API 文档

## 基础信息

- **基础URL**: `http://localhost:8080`
- **Content-Type**: `application/json`

---

## 一、LLM 推理接口 (OpenAI 兼容)

### 1. Chat Completions

生成聊天响应。

**端点**: `POST /v1/chat/completions`

**请求体**:
```json
{
  "model": "Qwen3.5-4B",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "temperature": 0.7,
  "max_tokens": 512,
  "stream": false
}
```

**响应**:
```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "Qwen3.5-4B",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30
  }
}
```

---

### 2. Completions

生成文本补全。

**端点**: `POST /v1/completions`

**请求体**:
```json
{
  "prompt": "Once upon a time",
  "temperature": 0.7,
  "max_tokens": 512
}
```

**响应**:
```json
{
  "id": "cmpl-xxx",
  "object": "text_completion",
  "created": 1234567890,
  "choices": [
    {
      "text": "there was a little village...",
      "index": 0,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 5,
    "completion_tokens": 15,
    "total_tokens": 20
  }
}
```

---

### 3. 流式 Chat Completions

**端点**: `POST /v1/chat/completions_stream`

**请求体**:
```json
{
  "model": "Qwen3.5-4B",
  "messages": [{"role": "user", "content": "Tell me a story"}],
  "max_tokens": 256
}
```

**响应 (SSE)**:
```json
data: {"choices":[{"delta":{"content":"Once"}]}

data: {"choices":[{"delta":{"content":" upon"}]}

data: {"choices":[{"delta":{"content":" a"}]}

data: [DONE]
```

---

### 4. WebSocket 流式

**端点**: `WS /ws/chat`

**客户端示例 (JavaScript)**:
```javascript
const ws = new WebSocket('ws://localhost:8080/ws/chat');

ws.onopen = () => {
  ws.send(JSON.stringify({
    messages: [{role: 'user', content: 'Hello!'}],
    max_tokens: 256
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  if (data.type === 'token') {
    process.stdout.write(data.content);
  } else if (data.type === 'done') {
    console.log('\n[Done]');
  }
};
```

---

## 二、多模态接口 (图像/视频生成)

### 1. 图像生成

使用 Stable Diffusion 生成图像。

**端点**: `POST /v1/image/generate`

**请求体**:
```json
{
  "prompt": "a beautiful landscape with mountains and rivers",
  "negative_prompt": "low quality, blurry, distorted",
  "width": 512,
  "height": 512,
  "steps": 20,
  "cfg_scale": 7.0,
  "seed": 12345,
  "model": "sd15_v1.5.safetensors",
  "sampler": "euler"
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| prompt | string | 是 | - | 正向提示词 |
| negative_prompt | string | 否 | - | 负向提示词 |
| width | int | 否 | 512 | 图像宽度 |
| height | int | 否 | 512 | 图像高度 |
| steps | int | 否 | 20 | 采样步数 |
| cfg_scale | float | 否 | 7.0 | CFG 强度 |
| seed | int | 否 | 随机 | 随机种子 |
| model | string | 否 | - | 模型名称 |
| sampler | string | 否 | euler | 采样器 |

**响应**:
```json
{
  "image_path": "./output/images/sd_123456.png",
  "seed": 12345
}
```

---

### 2. 视频生成

使用 AnimateDiff 生成视频。

**端点**: `POST /v1/video/generate`

**请求体**:
```json
{
  "prompt": "a flowing river in the forest",
  "negative_prompt": "low quality, blurry",
  "width": 512,
  "height": 512,
  "frames": 24,
  "fps": 8,
  "steps": 20,
  "seed": 12345
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| prompt | string | 是 | - | 提示词 |
| width | int | 否 | 512 | 帧宽度 |
| height | int | 否 | 512 | 帧高度 |
| frames | int | 否 | 24 | 帧数量 |
| fps | int | 否 | 8 | 帧率 |
| steps | int | 否 | 20 | 采样步数 |
| seed | int | 否 | 随机 | 随机种子 |

**响应**:
```json
{
  "video_path": "./output/videos/video_123456.mp4",
  "frame_count": 24,
  "fps": 8
}
```

---

### 3. 视频合成

将多张图片合成视频。

**端点**: `POST /v1/video/compose`

**请求体**:
```json
{
  "image_paths": [
    "./output/frame_00000.png",
    "./output/frame_00001.png",
    "./output/frame_00002.png"
  ],
  "audio_path": "./output/audio.mp3",
  "output_name": "my_video",
  "fps": 30
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| image_paths | array | 是 | 图片路径数组 |
| audio_path | string | 否 | 音频路径 |
| output_name | string | 否 | 输出文件名 |
| fps | int | 否 | 帧率 |

**响应**:
```json
{
  "video_path": "./output/videos/my_video.mp4"
}
```

---

### 4. 图像超分

放大图像。

**端点**: `POST /v1/image/upscale`

**请求体**:
```json
{
  "image_path": "./output/image.png",
  "scale": 2,
  "model": "ESRGAN_4x",
  "denoising_strength": 0.5
}
```

**响应**:
```json
{
  "image_path": "./output/images/upscaled_123456.png"
}
```

---

## 三、系统接口

### 1. 健康检查

**端点**: `GET /health`

**响应**:
```json
{
  "status": "ok"
}
```

---

### 2. 统计信息

**端点**: `GET /stats`

**响应**:
```json
{
  "total_instances": 2,
  "busy_instances": 1,
  "free_instances": 1,
  "queue_size": 5,
  "model_stats": {
    "Qwen3.5-4B": {
      "total": 2,
      "busy": 1,
      "free": 1
    }
  }
}
```

---

### 3. 模型列表

**端点**: `GET /models`

**响应**:
```json
{
  "object": "list",
  "data": [
    {
      "id": "Qwen3.5-4B",
      "object": "model",
      "created": 1677610602,
      "owned_by": "meta"
    }
  ]
}
```

---

## 四、错误响应

所有接口错误返回统一格式:

```json
{
  "error": {
    "message": "错误描述",
    "type": "invalid_request_error",
    "code": 400
  }
}
```

### HTTP 状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 400 | 请求参数错误 |
| 404 | 端点不存在 |
| 500 | 服务器内部错误 |
| 504 | 请求超时 |

---

## 五、使用示例

### cURL

```bash
# Chat Completion
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3.5-4B",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Image Generation
curl -X POST http://localhost:8080/v1/image/generate \
  -H "Content-Type: application/json" \
  -d '{"prompt": "a cat", "width": 512, "height": 512}'

# Video Generation
curl -X POST http://localhost:8080/v1/video/generate \
  -H "Content-Type: application/json" \
  -d '{"prompt": "flowing water", "frames": 24, "fps": 8}'

# Health Check
curl http://localhost:8080/health
```

### Python

```python
import requests

# Chat
response = requests.post(
    "http://localhost:8080/v1/chat/completions",
    json={
        "model": "Qwen3.5-4B",
        "messages": [{"role": "user", "content": "Hello!"}]
    }
)
print(response.json())

# Image
response = requests.post(
    "http://localhost:8080/v1/image/generate",
    json={"prompt": "a cat", "width": 512, "height": 512}
)
print(response.json())
```
