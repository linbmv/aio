#!/bin/bash

SERVER="https://llmio.150129.xyz"
TOKEN="your_token_here"

echo "=========================================="
echo "LLMIO 完整测试套件"
echo "=========================================="
echo ""

# 第一部分：检查 Provider 配置
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "第 1 部分：Provider 配置检查"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "1.1 所有 Provider 列表"
curl -s "$SERVER/api/providers" -H "Authorization: Bearer $TOKEN" | jq '.data[] | {name: .name, type: .type, enabled: .enabled}'
echo ""
echo "1.2 CliProxyzApi Provider 状态（导致错误的 Provider）"
curl -s "$SERVER/api/providers" -H "Authorization: Bearer $TOKEN" | jq '.data[] | select(.name == "CliProxyzApi")'
echo ""
echo ""

# 第二部分：测试可用 Provider
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "第 2 部分：测试可用 Provider (Amazonq)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "2.1 Anthropic 格式 (流式)"
curl -s -N "$SERVER/v1/messages" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"Say hi"}],"max_tokens":50,"stream":true}' | head -10
echo ""
echo ""

echo "2.2 Anthropic 格式 (非流式)"
curl -s "$SERVER/v1/messages" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"Say hi"}],"max_tokens":50,"stream":false}' | jq '.content[0].text'
echo ""
echo ""

echo "2.3 OpenAI 客户端 → Anthropic Provider (流式转换)"
curl -s -N "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"Say hi"}],"stream":true}' | head -10
echo ""
echo ""

# 第三部分：测试 OpenAI-Res 修复
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "第 3 部分：OpenAI-Res 格式转换修复测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "3.1 OpenAI-Res 流式（期望: data: {\"model\":\"...\",\"output\":\"...\"}）"
curl -s -N "$SERVER/v1/responses" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.1-codex-max","input":"Say hi","stream":true}' 2>&1 | head -15
echo ""
echo ""

echo "3.2 OpenAI-Res 非流式（期望: {\"output\":\"...\"}，output 是字符串）"
curl -s "$SERVER/v1/responses" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.1-codex-max","input":"Say hi","stream":false}' | jq '.'
echo ""
echo ""

echo "3.3 Anthropic Provider → OpenAI-Res 格式转换"
curl -s -N "$SERVER/v1/responses" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5","input":"Say hi","stream":true}' 2>&1 | head -15
echo ""
echo ""

# 诊断总结
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "诊断总结"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "✅ 修复成功的标志："
echo "   - 流式: data: {\"model\":\"...\",\"output\":\"Hello\"}"
echo "   - 非流式: {\"output\":\"Hello\"} (output 是字符串)"
echo ""
echo "❌ 仍有问题的标志："
echo "   - 空响应或只有 data: [DONE]"
echo "   - output 是数组: {\"output\":[{\"type\":\"...\"}]}"
echo "   - 原始 SSE: event: response.output_text.delta"
echo ""
echo "⚠️  Provider 错误："
echo "   - 'no provide items': 没有配置 Provider"
echo "   - 'maximum retry attempts reached': Provider 连接失败"
echo "   → 需要修复 CliProxyzApi 配置或使用其他 Provider"
echo ""
