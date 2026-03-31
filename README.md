# llama-go-server

一个专为 **NVIDIA GPU** 优化的轻量级 LLM 推理服务，支持多GPU、多模型、多模态生成。

## 功能特性

- **多GPU支持**: 自动检测、负载均衡、智能调度
- **显存自适应**: 8GB~96GB 自动优化配置
- **多实例池**: 多 llama.cpp 实例并行推理
- **动态 batching**: 请求自动合并，TPS 提升 2-5 倍
- **多模型支持**: 7B + 13B 混合部署
- **KV Cache 复用**: 相同前缀 prompt 自动缓存
- **WebSocket 流式**: 实时流式输出
- **OpenAI 兼容**: 兼容 OpenAI API 格式
- **多模态生成**: 支持图像/视频生成 (ComfyUI + FFmpeg)

## 快速开始

### 1. 准备环境

```bash
# 下载 llama.cpp 并编译
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp
make
# 生成的 llama-server 放到项目根目录
```

### 2. 配置文件

编辑 `configs/config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

llama:
  binary: "./llama.cpp/llama-server.exe"
  context_size: 2048
  prompt_batch: 512
  gpu_layers: 100

# GPU 配置
gpu:
  memory_gb: 8
  auto_detect: true
  gpu_ids: [0]

# 模型配置
models:
  - name: "Qwen3.5-4B"
    path: "./models/Qwen3.5-4B-Q4_K_M.gguf"
    instances: 1
    context_size: 2048
```

### 3. 启动服务

```bash
.\bin\llama-go-server.exe
```

---

## 配置说明

### GPU 配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| memory_gb | int | 8 | 显存大小 (8/12/16/24/32/48/64/96) |
| auto_detect | bool | false | 自动检测GPU |
| gpu_ids | []int | - | 指定GPU ID列表 |
| scheduling | string | least_used | 调度策略 (least_used/coolest/round_robin) |

### 显存预设

| 显存 | 典型显卡 | 实例数 | Context | Batch |
|------|----------|--------|---------|-------|
| 8GB | RTX 2070/3060 | 2 | 2048 | 512 |
| 12GB | RTX 3060/4070 | 2 | 3072 | 768 |
| 16GB | RTX 4070 Ti | 3 | 4096 | 1024 |
| 24GB | RTX 3090/4090 | 4 | 4096 | 1536 |
| 32GB | A4000/RTX 6000 | 4 | 8192 | 2048 |
| 48GB | A5000/A6000 | 6 | 8192 | 3072 |
| 64GB+ | H100 | 8+ | 16384+ | 4096+ |

### 模型配置

```yaml
models:
  - name: "llama-7b"
    path: "./models/llama-7b-chat.Q4_K_M.gguf"
    instances: 2        # 实例数量
    context_size: 2048  # 上下文长度
    priority: 1        # 调度优先级 (数字越小越高)
    gpu_layers: 100    # GPU层数
    batch_size: 512   # 批处理大小
    temperature: 0.7   # 默认温度
    max_tokens: 512    # 最大生成token
```

### Batcher 配置

```yaml
batcher:
  max_wait_ms: 100    # 最大等待时间 (ms)
  max_batch_size: 512 # 最大批大小
  queue_size: 100     # 队列大小
```

### KV Cache 配置

```yaml
kv_cache:
  enabled: true
  cache_dir: "./cache"
  max_size_mb: 1024
  max_age_hours: 24
```

### 多模态配置

```yaml
multimodal:
  comfyui:
    enabled: false
    host: "127.0.0.1"
    port: 8188
    output_dir: "./output/images"
  ffmpeg:
    enabled: false
    binary_path: "ffmpeg"
    output_dir: "./output/videos"
```

---

## API 接口

### 一、LLM 推理接口

#### 1. Chat Completions

```bash
POST /v1/chat/completions
```

```json
{
  "model": "Qwen3.5-4B",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "temperature": 0.7,
  "max_tokens": 512
}
```

#### 2. Completions

```bash
POST /v1/completions
```

```json
{
  "prompt": "Once upon a time",
  "temperature": 0.7,
  "max_tokens": 512
}
```

#### 3. 流式 Chat Completions

```bash
POST /v1/chat/completions_stream
```

```bash
curl -X POST http://localhost:8080/v1/chat/completions_stream \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"Hello"}]}'
```

#### 4. WebSocket 流式

```bash
WS /ws/chat
```

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/chat');
ws.onopen = () => ws.send(JSON.stringify({messages: [{role: 'user', content: 'Hello'}]}));
ws.onmessage = (e) => console.log(JSON.parse(e.data));
```

---

### 二、多模态接口

#### 1. 图像生成

```bash
POST /v1/image/generate
```

```json
{
  "prompt": "a beautiful landscape",
  "negative_prompt": "low quality, blurry",
  "width": 512,
  "height": 512,
  "steps": 20,
  "cfg_scale": 7.0,
  "seed": 12345,
  "sampler": "euler"
}
```

#### 2. 视频生成

```bash
POST /v1/video/generate
```

```json
{
  "prompt": "a flowing river",
  "width": 512,
  "height": 512,
  "frames": 24,
  "fps": 8,
  "seed": 12345
}
```

#### 3. 视频合成

```bash
POST /v1/video/compose
```

```json
{
  "image_paths": ["./frame1.png", "./frame2.png"],
  "fps": 30,
  "audio_path": "./music.mp3"
}
```

#### 4. 图像超分

```bash
POST /v1/image/upscale
```

```json
{
  "image_path": "./image.png",
  "scale": 2,
  "model": "ESRGAN_4x"
}
```

---

### 三、系统接口

#### 1. 健康检查

```bash
GET /health
```

#### 2. 统计信息

```bash
GET /stats
```

响应:
```json
{
  "total_instances": 4,
  "busy_instances": 2,
  "free_instances": 2,
  "queue_size": 5,
  "gpu_stats": {
    "count": 2,
    "total_memory_mb": 49152,
    "gpus": [
      {"id": 0, "name": "RTX 4090", "memory_used_mb": 12288, "utilization_percent": 75}
    ]
  }
}
```

#### 3. 模型列表

```bash
GET /models
```

---

## 端点清单

| 类型 | 端点 | 说明 |
|------|------|------|
| LLM | POST /v1/chat/completions | 聊天补全 |
| LLM | POST /v1/completions | 文本补全 |
| LLM | POST /v1/chat/completions_stream | 流式聊天 |
| LLM | WS /ws/chat | WebSocket聊天 |
| 图像 | POST /v1/image/generate | SD图像生成 |
| 视频 | POST /v1/video/generate | 视频生成 |
| 视频 | POST /v1/video/compose | 视频合成 |
| 超分 | POST /v1/image/upscale | 图像超分 |
| 系统 | GET /health | 健康检查 |
| 系统 | GET /stats | 统计信息 |
| 系统 | GET /models | 模型列表 |

---

## 性能对比

| 方案 | TPS (7B) | 显存占用 | 启动时间 |
|------|----------|----------|----------|
| Ollama | ~15 | ~7GB | 3s |
| vLLM | ~25 | ~7.5GB | 10s |
| **llama-go-server** | ~30 | ~6GB | 2s |

*注: 测试环境 RTX 2070, Q4_K_M 量化模型*

---

## 多GPU使用示例

### 双卡 RTX 4090

```yaml
gpu:
  memory_gb: 24
  auto_detect: true
  gpu_ids: [0, 1]
  scheduling: "least_used"

models:
  - name: "llama-7b"
    instances: 2
```

### 四卡 RTX 3090

```yaml
gpu:
  memory_gb: 24
  auto_detect: true
  gpu_ids: [0, 1, 2, 3]
  scheduling: "least_used"

models:
  - name: "llama-7b"
    instances: 4
  - name: "llama-13b"
    instances: 2
```

---

## 多模态工作流

```
LLM (llama.cpp) ← 文本理解
   ↓
ComfyUI (图像/视频)
   ├── Stable Diffusion（图像）
   ├── ControlNet（控制）
   └── AnimateDiff（视频）
   ↓
FFmpeg（合成/编辑）
```

---

## 项目结构

```
llama-go-server/
├── cmd/server/main.go       # 入口
├── internal/
│   ├── api/                # HTTP API
│   ├── scheduler/          # 调度器
│   ├── batcher/            # 动态批处理
│   ├── runner/             # llama.cpp 管理
│   ├── pool/              # 实例池
│   ├── gpu/                # GPU 管理
│   ├── multimodal/        # 多模态生成
│   └── config/             # 配置管理
├── configs/config.yaml     # 配置文件
└── bin/llama-go-server     # 编译产物
```

---

## 常见问题

### Q: 显存不够怎么办?
A: 减少 `instances` 数量，或降低 `context_size`

### Q: 如何支持更多模型?
A: 在 `models` 数组中添加配置

### Q: 多模态功能怎么开启?
A: 设置 `multimodal.comfyui.enabled: true` 并配置 ComfyUI 地址
