# HTTP2Transport - HTTP 和 SOCKS5 代理服务器

本工具提供一个本地 HTTP CONNECT 代理，并可选开启 SOCKS5 代理。可基于 Outline/Shadowsocks 等传输配置进行拨号，并支持直连/主代理/备用代理的域名名单与统计接口。

## 构建与运行

```
go build
./http2transport [选项]
```

也可直接使用 `go run`：

```
MAIN_KEY=ss://ENCRYPTION_KEY@HOST:PORT/
go run . -main-proxy "$MAIN_KEY" -localAddr 0.0.0.0:1080
```

## 用法

```
./http2transport [选项]
```

常用选项（与 `-h` 输出一致）：
- `-main-proxy` string: 主代理传输配置（必填，例：`ss://...`）
- `-second-proxy` string: 备用代理传输配置（可选）
- `-localAddr` string: 本地 HTTP 代理监听地址，默认 `localhost:1080`
- `-socket-port` string: 启动 SOCKS5 代理的端口（例如 `1082`，留空则不启用）
- `-urlProxyPrefix` string: URL 代理路径前缀，默认 `/proxy`，设为空字符串关闭
- `-direct-file` string: 直连域名名单文件路径（每行一个）
- `-main-proxy-file` string: 走主代理域名名单文件路径（每行一个）
- `-second-proxy-file` string: 走备用代理域名名单文件路径（每行一个）
- `-default` string: 默认策略，`direct`、`main-proxy` 或 `second-proxy`，默认 `main-proxy`

## 示例

基本用法 - 使用主代理：
```
MAIN_KEY=ss://ENCRYPTION_KEY@HOST:PORT/
./http2transport -main-proxy "$MAIN_KEY" -localAddr 0.0.0.0:1080
```

使用主代理 + 备用代理：
```
MAIN_KEY=ss://ENCRYPTION_KEY@HOST:PORT/
SECOND_KEY=ss://ENCRYPTION_KEY2@HOST2:PORT/
./http2transport -main-proxy "$MAIN_KEY" -second-proxy "$SECOND_KEY" -localAddr 0.0.0.0:1080
```

同时启动 HTTP 和 SOCKS5 代理：
```
./http2transport -main-proxy "$MAIN_KEY" -localAddr 0.0.0.0:1080 -socket-port 1082
```

使用直连/代理名单：
```
echo -e "265.com\n.zzxworld.com" > direct.txt
echo -e "api.example.com\n.openai.com" > main-proxy.txt
echo -e "backup.example.com\n.fallback.com" > second-proxy.txt

./http2transport -main-proxy "$MAIN_KEY" -second-proxy "$SECOND_KEY" \
  -direct-file direct.txt -main-proxy-file main-proxy.txt \
  -second-proxy-file second-proxy.txt -default main-proxy
```

完整示例：
```
./http2transport -main-proxy "$MAIN_KEY" -second-proxy "$SECOND_KEY" \
  -localAddr 0.0.0.0:1080 -socket-port 1082 \
  -direct-file direct.txt \
  -main-proxy-file main-proxy.txt \
  -second-proxy-file second-proxy.txt \
  -default main-proxy
```

## 名单文件格式

```
265.com         # 完全匹配该域名
.zzxworld.com   # 匹配该域名及其所有子域名
# 以 # 开头为注释行
```

## 默认策略

- `direct`       - 域名不在任何名单时直连
- `main-proxy`   - 域名不在任何名单时走主代理（默认）
- `second-proxy` - 域名不在任何名单时走备用代理

## 服务端点

- HTTP 代理: `http://localhost:1080`
- SOCKS5 代理: `socks5://localhost:1082`（若启用）
- 统计接口: `http://localhost:1080/stats`

更多监控细节与移动端集成，可参见 `README_MONITORING.md`。

## 部署示例(pm2)


config.js
```javascript
const KEY_OUT = 'ss://host:port/?outline=1&prefix=%16%03%01%00%C2%A8%01%01';//便宜的VPS
const KEY_GGG = 'ss://xxxx:port/?outline=1&prefix=%16%03%01%00%C2%A8%01%01';//流量费用贵的VPS

module.exports = {
  apps: [
    {
      name: 'outline-gg',
      script: '/bin/bash',
      args: [
        '-lc',
        '/Users/sam/.bin/http2transport -main-proxy "$KEY_GGG" -second-proxy "$KEY_OUT"  -localAddr 127.0.0.1:1081 -socket-port 1082  -main-proxy-file ./proxy.txt -direct-file ./direct.txt -default second-proxy'
      ],
      env: {
        KEY_GGG: KEY_GGG,
        KEY_OUT: KEY_OUT
      },
      autorestart: true,
      restart_delay: 2000
    }
  ]
};
```

执行: pm2 start config.js