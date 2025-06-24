# HTTP2Transport 流量监控功能

本功能为 http2transport 代理添加了网络流量监控和连接时长追踪功能，并提供了 Objective-C 兼容的接口。

## 新增功能

### 1. 流量统计
- **上行流量 (Upload)**: 记录发送到服务器的字节数
- **下行流量 (Download)**: 记录从服务器接收的字节数
- **实时统计**: 支持获取实时的流量数据

### 2. 连接时长追踪
- **总连接时长**: 所有已关闭连接的累计时长
- **当前会话时长**: 从第一个连接建立到现在的时长
- **活跃连接数**: 当前活跃的连接数量
- **总连接数**: 历史上建立的连接总数

### 3. HTTP API 接口
可通过 HTTP 请求获取统计信息：

```bash
curl http://localhost:1080/stats
```

返回 JSON 格式的统计数据：
```json
{
    "uploadBytes": 1024,
    "downloadBytes": 2048,
    "outlineConnectionTime": 5000,
    "currentSessionDuration": 30000,
    "activeConnections": 2,
    "totalConnections": 10
}
```

## 使用方法

### 1. 启动代理服务器
```bash
go run github.com/Jigsaw-Code/outline-sdk/x/examples/http2transport@latest \
    -transport "ss://your-shadowsocks-config" \
    -localAddr localhost:1080 \
    -whitelist "*.baidu.com,*.qq.com"
```

### 2. 查看统计信息
```bash
# 获取流量统计
curl http://localhost:1080/stats

# 使用 jq 格式化输出
curl -s http://localhost:1080/stats | jq '.'
```

## Mobile 集成 (Objective-C)

### 新增的 MobileProxy 接口

以下方法已添加到 `MobileproxyProxy` 类中：

```objc
// 获取上行流量字节数
- (long long)getUploadBytes;

// 获取下行流量字节数  
- (long long)getDownloadBytes;

// 获取总连接时长（毫秒）
- (long long)getTotalConnectionTime;

// 获取当前会话时长（毫秒）
- (long long)getCurrentSessionDuration;

// 获取活跃连接数
- (long long)getActiveConnections;

// 获取总连接数
- (long long)getTotalConnections;

// 重置流量统计
- (void)resetTrafficStats;
```

### Objective-C 使用示例

```objc
// 创建并启动代理
MobileproxyStreamDialer *dialer = [[MobileproxyStreamDialer alloc] initFromConfig:@"ss://..."];
MobileproxyProxy *proxy = MobileproxyRunProxy(@"localhost:0", dialer, nil);

// 获取流量统计
long long uploadBytes = [proxy getUploadBytes];
long long downloadBytes = [proxy getDownloadBytes];
long long connectionTime = [proxy getTotalConnectionTime];
long long sessionDuration = [proxy getCurrentSessionDuration];

NSLog(@"Upload: %lld bytes, Download: %lld bytes", uploadBytes, downloadBytes);
NSLog(@"Connection time: %lld ms, Session: %lld ms", connectionTime, sessionDuration);

// 重置统计
[proxy resetTrafficStats];

// 停止代理
[proxy stop:5];
```

### Swift 使用示例

```swift
// 创建并启动代理
let dialer = MobileproxyStreamDialer(fromConfig: "ss://...")
let proxy = MobileproxyRunProxy("localhost:0", dialer, nil)

// 获取流量统计
let uploadBytes = proxy?.getUploadBytes() ?? 0
let downloadBytes = proxy?.getDownloadBytes() ?? 0
let connectionTime = proxy?.getTotalConnectionTime() ?? 0

print("Upload: \(uploadBytes) bytes, Download: \(downloadBytes) bytes")
print("Connection time: \(connectionTime) ms")

// 停止代理
proxy?.stop(5)
```

## 构建 Mobile 库

### 构建 iOS Framework
```bash
cd x/
task build:mobileproxy:ios
```

### 构建 Android AAR
```bash
cd x/
task build:mobileproxy:android
```

### 构建所有平台
```bash
cd x/
task build:mobileproxy
```

## 技术实现

### 监控原理
1. **连接包装**: 使用 `MonitoredConn` 包装所有网络连接
2. **流量追踪**: 在 `Read`/`Write` 操作中记录字节数
3. **时间追踪**: 记录连接建立和关闭时间
4. **原子操作**: 使用原子计数器确保线程安全

### 架构集成
- **WhitelistDialer**: 为直连和代理连接都添加监控
- **MobileProxy**: 在现有 Proxy 结构中集成统计功能
- **HTTP API**: 提供 RESTful 接口查询统计数据

### 性能考虑
- 使用原子操作避免锁竞争
- 最小化监控开销
- 内存高效的统计数据结构

## 注意事项

1. **统计重置**: 调用 `resetTrafficStats()` 会清除所有统计数据
2. **会话定义**: 会话时长从第一个连接建立开始计算
3. **连接分类**: 直连和代理连接都会被统计
4. **线程安全**: 所有统计操作都是线程安全的

## 兼容性

- 向后兼容现有的 mobileproxy API
- 支持 Go Mobile 绑定
- 支持 iOS 11.0+ 和 Android API 21+
- 与现有的传输配置兼容