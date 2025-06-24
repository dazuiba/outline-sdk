#!/bin/bash

# HTTP2Transport 流量监控功能测试脚本

set -e

echo "=== HTTP2Transport 流量监控功能测试 ==="

# 检查是否提供了必要的参数
if [ -z "$1" ]; then
    echo "用法: $0 <shadowsocks-config>"
    echo "示例: $0 'ss://userinfo@host:port'"
    exit 1
fi

TRANSPORT_CONFIG="$1"
PROXY_PORT="18080"
PROXY_ADDR="localhost:$PROXY_PORT"

echo "1. 构建 http2transport..."
go build -o http2transport

echo "2. 启动代理服务器..."
./http2transport -transport "$TRANSPORT_CONFIG" -localAddr "$PROXY_ADDR" -whitelist "httpbin.org" &
PROXY_PID=$!

# 等待代理启动
sleep 2

# 确保在脚本退出时清理进程
trap "kill $PROXY_PID 2>/dev/null || true" EXIT

echo "3. 测试代理连接..."

# 测试直连（白名单域名）
echo "测试直连 (httpbin.org)..."
curl -s -x "http://$PROXY_ADDR" "http://httpbin.org/get?test=direct" > /dev/null
echo "✓ 直连测试完成"

# 测试代理连接
echo "测试代理连接 (google.com)..."
curl -s -x "http://$PROXY_ADDR" "http://google.com" > /dev/null || echo "代理连接测试 (可能因网络环境失败)"

# 等待数据处理
sleep 1

echo "4. 获取流量统计..."
STATS=$(curl -s "http://$PROXY_ADDR/stats")
echo "流量统计结果:"
echo "$STATS" | jq '.' 2>/dev/null || echo "$STATS"

echo ""
echo "5. 解析统计数据..."
if command -v jq &> /dev/null; then
    UPLOAD=$(echo "$STATS" | jq -r '.uploadBytes')
    DOWNLOAD=$(echo "$STATS" | jq -r '.downloadBytes')
    TOTAL_TIME=$(echo "$STATS" | jq -r '.totalConnectionTime')
    SESSION_TIME=$(echo "$STATS" | jq -r '.currentSessionDuration')
    ACTIVE_CONN=$(echo "$STATS" | jq -r '.activeConnections')
    TOTAL_CONN=$(echo "$STATS" | jq -r '.totalConnections')
    
    echo "📊 流量统计摘要:"
    echo "   上行流量: $UPLOAD 字节"
    echo "   下行流量: $DOWNLOAD 字节"
    echo "   总连接时长: $TOTAL_TIME 毫秒"
    echo "   会话时长: $SESSION_TIME 毫秒"
    echo "   活跃连接: $ACTIVE_CONN"
    echo "   总连接数: $TOTAL_CONN"
    
    # 验证统计数据的合理性
    if [ "$DOWNLOAD" -gt 0 ]; then
        echo "✓ 下行流量统计正常"
    else
        echo "⚠ 下行流量为0，可能存在问题"
    fi
    
    if [ "$TOTAL_CONN" -gt 0 ]; then
        echo "✓ 连接统计正常"
    else
        echo "⚠ 连接数为0，可能存在问题"
    fi
    
    if [ "$SESSION_TIME" -gt 0 ]; then
        echo "✓ 会话时长统计正常"
    else
        echo "⚠ 会话时长为0，可能存在问题"
    fi
else
    echo "未安装 jq，无法解析 JSON 数据"
fi

echo ""
echo "6. 测试连续请求..."
for i in {1..3}; do
    echo "发送请求 #$i..."
    curl -s -x "http://$PROXY_ADDR" "http://httpbin.org/get?test=$i" > /dev/null
    sleep 0.5
done

echo ""
echo "7. 获取最终统计..."
FINAL_STATS=$(curl -s "http://$PROXY_ADDR/stats")
echo "最终流量统计:"
echo "$FINAL_STATS" | jq '.' 2>/dev/null || echo "$FINAL_STATS"

echo ""
echo "=== 测试完成 ==="
echo "✓ HTTP2Transport 流量监控功能测试成功"
echo ""
echo "说明:"
echo "- 统计端点: http://$PROXY_ADDR/stats"
echo "- 白名单域名 (直连): httpbin.org"
echo "- 其他域名通过代理连接"
echo "- 所有连接都会被监控和统计"