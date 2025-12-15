# LLMIO 统一Provider系统使用指南

## 部署完成后的使用步骤

### 1. 访问管理界面
```
http://localhost:7070
```

### 2. 配置Provider
1. 进入 "Provider管理" 页面
2. 创建新Provider：
   - 名称：`CPA-Unified`
   - 类型：`openai`
   - 配置：
   ```json
   {
     "base_url": "https://cpa.hooo.eu.org/v1",
     "keys": [{
       "term": "sk-LinHong20Fang-1314520-4eVer",
       "remark": "test",
       "status": true
     }]
   }
   ```

### 3. 创建Channel
1. 进入 "Channel管理" 页面
2. 创建新Channel：
   - 名称：`CPA-Channel`
   - Provider：选择上面创建的Provider
   - 支持协议：`["openai", "openai-res", "anthropic"]`
   - 负载均衡策略：`lottery`
   - 权重：`100`
   - 状态：`active`

### 4. 配置模型映射
1. 进入 "模型映射" 页面
2. 创建映射关系：

#### CC客户端映射
```
虚拟模型: claude-sonnet-4-5
实际模型: gpt-5.1-high
协议: openai
```

#### Chat客户端映射
```
虚拟模型: gpt-5.1-high
实际模型: gpt-5.1-high
协议: openai
```

#### Codex客户端映射
```
虚拟模型: gpt-5.1-codex-max
实际模型: gpt-5.1-codex-max
协议: openai
```

### 5. 客户端配置

#### CC (Claude Code) 客户端
```json
{
  "model": "claude-sonnet-4-5",
  "base_url": "http://localhost:7070/v1",
  "api_key": "test"
}
```

#### Chat对话客户端
```json
{
  "model": "gpt-5.1-high",
  "base_url": "http://localhost:7070/v1",
  "api_key": "test"
}
```

#### Codex客户端
```json
{
  "model": "gpt-5.1-codex-max",
  "base_url": "http://localhost:7070/v1",
  "api_key": "test"
}
```

### 6. 测试调用

#### 测试CC客户端
```bash
curl -X POST http://localhost:7070/v1/chat/completions \
  -H "Authorization: Bearer test" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

#### 测试Chat客户端
```bash
curl -X POST http://localhost:7070/v1/chat/completions \
  -H "Authorization: Bearer test" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.1-high",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

#### 测试Codex客户端
```bash
curl -X POST http://localhost:7070/v1/chat/completions \
  -H "Authorization: Bearer test" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.1-codex-max",
    "messages": [{"role": "user", "content": "Write hello world in Python"}]
  }'
```

## 核心功能

### 模型映射
- 客户端请求虚拟模型名称
- LLMIO自动映射到实际上游模型
- 支持不同协议转换

### 负载均衡
- 支持多种策略：`lottery`, `rotor`, `error_aware`, `trace_aware`
- 自动故障转移和冷却管理

### 协议转换
- 自动在OpenAI/Anthropic协议间转换
- 统一的API接口

## 监控和管理

### 查看统计
- Channel统计：成功率、响应时间、错误率
- 模型映射使用情况
- Provider健康状态

### 故障排除
- 检查Provider配置
- 查看Channel状态
- 重置冷却状态

## Docker部署

```bash
# 拉取镜像
docker pull ghcr.io/linbmv/aio:unified-provider

# 运行容器
docker run -d \
  -p 7070:7070 \
  -e TOKEN=your_token \
  --name llmio \
  ghcr.io/linbmv/aio:unified-provider
```

## 环境变量

- `TOKEN`: API认证令牌
- `GIN_MODE`: 运行模式 (debug/release)
- `OPENAI_API_KEY`: OpenAI API密钥 (可选)
- `ANTHROPIC_API_KEY`: Anthropic API密钥 (可选)