# HTTP2Transport - HTTP 和 SOCKS5 代理服务器

> **基于 [Outline SDK](https://github.com/Jigsaw-Code/outline-sdk) 开发**

本工具提供一个本地 HTTP CONNECT 代理，并可选开启 SOCKS5 代理。可基于 Outline/Shadowsocks 等传输配置进行拨号，并支持直连/主代理的域名名单与统计接口。

## 功能特性

- 同时提供 HTTP CONNECT 和 SOCKS5 代理服务
- 基于域名名单的智能路由（直连/代理）
- 支持 Shadowsocks 和其他 Outline 传输协议
- 实时流量统计接口
- 支持多平台（Linux、macOS、Windows）

## 快速开始

### 构建

```bash
cd x/examples/http2transport
go build
```

### 运行

基本用法：
```bash
MAIN_KEY=ss://ENCRYPTION_KEY@HOST:PORT/
./http2transport -main-proxy "$MAIN_KEY" -localAddr 0.0.0.0:1080
```

同时启动 HTTP 和 SOCKS5 代理：
```bash
./http2transport -main-proxy "$MAIN_KEY" -localAddr 0.0.0.0:1080 -socket-port 1079
```

使用域名路由：
```bash
./http2transport -main-proxy "$MAIN_KEY" \
  -localAddr 0.0.0.0:1080 -socket-port 1079 \
  -direct-file config/direct.txt \
  -default main-proxy
```

## 服务端点

- **HTTP 代理**: `http://localhost:1080`
- **SOCKS5 代理**: `socks5://localhost:1079`（若启用）
- **统计接口**: `http://localhost:1080/stats`

## 命令行选项

使用 `-h` 查看所有可用选项：

```
./http2transport -h
```

常用选项：
- `-main-proxy` string: 主代理传输配置（必填，例：`ss://...`）
- `-localAddr` string: 本地 HTTP 代理监听地址，默认 `localhost:1080`
- `-socket-port` string: 启动 SOCKS5 代理的端口（例如 `1082`，留空则不启用）
- `-urlProxyPrefix` string: URL 代理路径前缀，默认 `/proxy`，设为空字符串关闭
- `-direct-file` string: 直连域名名单文件路径（每行一个）
- `-main-proxy-file` string: 走主代理域名名单文件路径（每行一个）
- `-default` string: 默认策略，`direct` 或 `main-proxy`，默认 `main-proxy`

## 域名路由配置

### 域名名单文件格式

域名名单文件每行一个域名，支持：
- 精确匹配：`example.com`
- 子域名匹配：`.example.com` (匹配所有子域名)
- 注释：以 `#` 开头的行

示例 `config/direct.txt`：
```
# 国内常用网站直连
.baidu.com
.qq.com
.taobao.com
```

### 路由策略

使用 `-default` 参数设置默认策略：
- `main-proxy`: 默认走代理，仅 `-direct-file` 中的域名直连
- `direct`: 默认直连，仅 `-main-proxy-file` 中的域名走代理

更多监控细节与移动端集成，可参见 `README_MONITORING.md`。

## 部署示例(pm2)

使用提供的配置文件：[`x/examples/http2transport/config/pm2.config.js`](x/examples/http2transport/config/pm2.config.js)

**步骤：**

1. 编辑配置文件，修改 `KEY_OUT` 为你的 Shadowsocks 密钥
2. 修改 http2transport 二进制文件路径（默认：`/Users/sam/.bin/http2transport`）
3. 执行部署：

```bash
pm2 start x/examples/http2transport/config/pm2.config.js
```

## 配置系统 Proxy

### 使用 xbar 快速切换代理

[xbar](https://xbarapp.com/) 是一个 macOS 菜单栏工具，可以通过脚本快速切换系统代理设置。

**安装步骤：**

1. 安装 xbar 应用
2. 打开 xbar，选择 "Open Plugins Folder"
3. 将 [`x/examples/http2transport/config/xbar-proxy-switcher.sh`](x/examples/http2transport/config/xbar-proxy-switcher.sh) 复制到 plugins 目录
4. 根据需要修改脚本中的端口配置（默认 HTTP: 1080, SOCKS: 1079）
5. xbar 会自动加载脚本，在菜单栏显示代理状态图标

**使用说明：**

- 点击菜单栏图标可以快速开启/关闭系统代理
- 🇬🇧 表示代理已开启
- 🇪🇸 表示代理已关闭
- 脚本会同时配置 HTTP 和 SOCKS5 代理
