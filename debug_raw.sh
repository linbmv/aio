#!/bin/bash

SERVER="https://llmio.150129.xyz"
TOKEN="sk-LinHome-wo20Fang13145204eVer"

echo "捕获原始响应（流式）"
echo "=========================================="
curl -N -H "Authorization: Bearer $TOKEN" \
  "$SERVER/v1/responses" \
  -d '{"model":"gpt-5.1-codex-max","input":"hi","stream":true}' 2>&1 | xxd | head -50

echo ""
echo ""
echo "捕获原始响应（非流式）"
echo "=========================================="
curl -s -H "Authorization: Bearer $TOKEN" \
  "$SERVER/v1/responses" \
  -d '{"model":"gpt-5.1-codex-max","input":"hi","stream":false}' 2>&1 | xxd

echo ""
echo ""
echo "查看容器日志（最后 30 行）"
echo "=========================================="
ssh root@150129.xyz "docker logs llmio --tail 30"
