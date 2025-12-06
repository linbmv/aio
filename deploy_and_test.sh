#!/bin/bash

echo "=========================================="
echo "部署并测试最新代码"
echo "=========================================="
echo ""

cd ~/data/docker/llmio

echo "1. 查看当前 Git 版本"
echo "------------------------------------------"
git log --oneline -1
echo ""

echo "2. 拉取最新代码"
echo "------------------------------------------"
git pull origin fix/security-improvements
echo ""

echo "3. 查看更新后的 Git 版本"
echo "------------------------------------------"
git log --oneline -1
echo "应该显示: fdccc53 debug: 添加SSE行调试日志"
echo ""

echo "4. 停止旧容器"
echo "------------------------------------------"
docker-compose down llmio
echo ""

echo "5. 重新构建镜像"
echo "------------------------------------------"
docker-compose build --no-cache llmio
echo ""

echo "6. 启动新容器"
echo "------------------------------------------"
docker-compose up -d llmio
echo ""

echo "7. 等待容器启动"
echo "------------------------------------------"
sleep 5
docker ps | grep llmio
echo ""

echo "8. 测试 OpenAI-Res 流式"
echo "------------------------------------------"
curl -N -H "Authorization: Bearer sk-LinHome-wo20Fang13145204eVer" \
  https://llmio.150129.xyz/v1/responses \
  -d '{"model":"gpt-5.1-codex-max","input":"count to 3","stream":true}' | head -20
echo ""
echo ""

echo "9. 查看调试日志（最后30行）"
echo "------------------------------------------"
docker logs llmio --tail 30 | grep -E "(OpenAIResponsesAPISSEToOpenAIRes|received line)"
echo ""

echo "=========================================="
echo "部署完成"
echo "=========================================="
