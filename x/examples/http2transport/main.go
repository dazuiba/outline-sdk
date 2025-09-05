// Copyright 2023 The Outline Authors
// https://claude.ai/chat/db78cb07-7844-4eeb-a467-8b27392d6dc5
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/x/configurl"
	"github.com/Jigsaw-Code/outline-sdk/x/httpproxy"
)

// formatBytes 将字节数转换为人类可读的格式
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration 将毫秒数转换为人类可读的时间格式
func formatDuration(milliseconds int64) string {
	duration := time.Duration(milliseconds) * time.Millisecond

	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%d天%d小时", days, hours)
		}
		return fmt.Sprintf("%d天", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%d小时%d分钟", hours, minutes)
		}
		return fmt.Sprintf("%d小时", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d分钟", minutes)
	}
	return fmt.Sprintf("%d秒", seconds)
}

// DomainStats 存储单个域名的流量统计
type DomainStats struct {
	uploadBytes   int64
	downloadBytes int64
}

// DomainTrafficStats 管理按域名分组的流量统计
type DomainTrafficStats struct {
	mu          sync.RWMutex
	domainStats map[string]*DomainStats
	globalStats *httpproxy.TrafficStats
}

// NewDomainTrafficStats 创建新的域名流量统计实例
func NewDomainTrafficStats() *DomainTrafficStats {
	return &DomainTrafficStats{
		domainStats: make(map[string]*DomainStats),
		globalStats: httpproxy.NewTrafficStats(),
	}
}

// AddTraffic 为指定域名添加流量统计
func (dts *DomainTrafficStats) AddTraffic(domain string, uploadBytes, downloadBytes int64) {
	if domain == "" {
		return
	}

	dts.mu.Lock()
	defer dts.mu.Unlock()

	if _, exists := dts.domainStats[domain]; !exists {
		dts.domainStats[domain] = &DomainStats{}
	}

	atomic.AddInt64(&dts.domainStats[domain].uploadBytes, uploadBytes)
	atomic.AddInt64(&dts.domainStats[domain].downloadBytes, downloadBytes)
}

// GetDomainStats 获取指定域名的流量统计
func (dts *DomainTrafficStats) GetDomainStats(domain string) (uploadBytes, downloadBytes int64) {
	dts.mu.RLock()
	defer dts.mu.RUnlock()

	if stats, exists := dts.domainStats[domain]; exists {
		return atomic.LoadInt64(&stats.uploadBytes), atomic.LoadInt64(&stats.downloadBytes)
	}
	return 0, 0
}

// GetAllDomainStats 获取所有域名的流量统计（人类可读格式）
func (dts *DomainTrafficStats) GetAllDomainStats() map[string]map[string]string {
	dts.mu.RLock()
	defer dts.mu.RUnlock()

	result := make(map[string]map[string]string)
	for domain, stats := range dts.domainStats {
		uploadBytes := atomic.LoadInt64(&stats.uploadBytes)
		downloadBytes := atomic.LoadInt64(&stats.downloadBytes)
		result[domain] = map[string]string{
			"uploadBytes":   formatBytes(uploadBytes),
			"downloadBytes": formatBytes(downloadBytes),
		}
	}
	return result
}

// GetGlobalStats 获取全局流量统计
func (dts *DomainTrafficStats) GetGlobalStats() *httpproxy.TrafficStats {
	return dts.globalStats
}

// DomainMonitoredConn 带域名跟踪的连接包装器
type DomainMonitoredConn struct {
	transport.StreamConn
	domain      string
	domainStats *DomainTrafficStats
	globalConn  transport.StreamConn // 用于全局统计的连接
}

// NewDomainMonitoredConn 创建带域名跟踪的连接
func NewDomainMonitoredConn(conn transport.StreamConn, domain string, domainStats *DomainTrafficStats) *DomainMonitoredConn {
	// 为全局统计创建监控连接
	globalConn := httpproxy.NewMonitoredConn(conn, domainStats.GetGlobalStats())

	return &DomainMonitoredConn{
		StreamConn:  conn,
		domain:      domain,
		domainStats: domainStats,
		globalConn:  globalConn,
	}
}

// Read 重写读取方法以跟踪下载流量
func (dmc *DomainMonitoredConn) Read(b []byte) (int, error) {
	n, err := dmc.globalConn.Read(b)
	if n > 0 {
		dmc.domainStats.AddTraffic(dmc.domain, 0, int64(n))
	}
	return n, err
}

// Write 重写写入方法以跟踪上传流量
func (dmc *DomainMonitoredConn) Write(b []byte) (int, error) {
	n, err := dmc.globalConn.Write(b)
	if n > 0 {
		dmc.domainStats.AddTraffic(dmc.domain, int64(n), 0)
	}
	return n, err
}

// Close 重写关闭方法
func (dmc *DomainMonitoredConn) Close() error {
	return dmc.globalConn.Close()
}

// WhitelistDialer 实现了一个带有域名白名单的拨号器
type WhitelistDialer struct {
	// 原始拨号器，用于通过shadowsocks代理连接
	proxyDialer transport.StreamDialer
	// 域名白名单
	whitelist map[string]bool
	// 域名流量统计
	domainStats *DomainTrafficStats
	// 监控域名列表
	monitorDomains map[string]bool
}

// NewWhitelistDialer 创建一个新的WhitelistDialer
func NewWhitelistDialer(proxyDialer transport.StreamDialer, whitelistDomains []string, monitorDomains []string, domainStats *DomainTrafficStats) *WhitelistDialer {
	whitelist := make(map[string]bool)
	for _, domain := range whitelistDomains {
		whitelist[domain] = true
	}

	monitorDomainsMap := make(map[string]bool)
	for _, domain := range monitorDomains {
		monitorDomainsMap[domain] = true
	}

	return &WhitelistDialer{
		proxyDialer:    proxyDialer,
		whitelist:      whitelist,
		domainStats:    domainStats,
		monitorDomains: monitorDomainsMap,
	}
}

// DialStream 实现了transport.StreamDialer接口
func (d *WhitelistDialer) DialStream(ctx context.Context, address string) (transport.StreamConn, error) {
	// 解析地址，获取域名部分
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	// 检查是否应该监控这个域名
	shouldMonitorDomain := d.shouldMonitor(host)

	// 检查域名是否在白名单中
	if d.isWhitelisted(host) {
		log.Printf("使用直连连接 %s", address)
		netDialer := &net.Dialer{}
		conn, err := netDialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, err
		}
		// 将net.Conn转换为transport.StreamConn并根据域名监控设置包装监控
		wrappedConn := &tcpStreamConn{Conn: conn}
		if d.domainStats != nil && shouldMonitorDomain {
			return NewDomainMonitoredConn(wrappedConn, host, d.domainStats), nil
		}
		return wrappedConn, nil
	}

	// 不在白名单中，使用代理连接
	log.Printf("使用代理连接 %s", address)
	conn, err := d.proxyDialer.DialStream(ctx, address)
	if err != nil {
		return nil, err
	}
	// 为代理连接也根据域名监控设置添加监控
	if d.domainStats != nil && shouldMonitorDomain {
		return NewDomainMonitoredConn(conn, host, d.domainStats), nil
	}
	return conn, nil
}

// tcpStreamConn 实现了transport.StreamConn接口，包装了标准库的net.Conn
type tcpStreamConn struct {
	net.Conn
}

// CloseRead 实现了transport.StreamConn接口的CloseRead方法
func (c *tcpStreamConn) CloseRead() error {
	if tc, ok := c.Conn.(*net.TCPConn); ok {
		return tc.CloseRead()
	}
	return nil
}

// CloseWrite 实现了transport.StreamConn接口的CloseWrite方法
func (c *tcpStreamConn) CloseWrite() error {
	if tc, ok := c.Conn.(*net.TCPConn); ok {
		return tc.CloseWrite()
	}
	return nil
}

// matchDomain 检查目标域名是否匹配给定的域名规则
func matchDomain(rule, host string) bool {

	// 统一使用以 . 开头的匹配规则
	// 如果规则不是以 . 开头，则添加 .
	var domain string
	var dotRule string

	if strings.HasPrefix(rule, ".") {
		domain = rule[1:] // 去掉开头的 .
		dotRule = rule
	} else {
		domain = rule
		dotRule = "." + rule
	}

	// 匹配该域名及其所有子域名
	return host == domain || strings.HasSuffix(host, dotRule)
}

// isWhitelisted 检查域名是否在白名单中，支持通配符匹配
func (d *WhitelistDialer) isWhitelisted(host string) bool {
	// 检查完整域名是否在白名单中
	if d.whitelist[host] {
		return true
	}

	// 检查是否匹配白名单中的规则
	for rule := range d.whitelist {
		if matchDomain(rule, host) {
			return true
		}
	}

	return false
}

// shouldMonitor 检查域名是否应该被监控
func (d *WhitelistDialer) shouldMonitor(host string) bool {
	// 如果没有设置监控域名列表，则监控所有域名
	if len(d.monitorDomains) == 0 {
		return true
	}

	// 检查完整域名是否在监控列表中
	if d.monitorDomains[host] {
		return true
	}

	// 检查是否匹配监控列表中的规则
	for rule := range d.monitorDomains {
		if matchDomain(rule, host) {
			return true
		}
	}

	return false
}

// SOCKS5ProxyServer 实现一个简单的SOCKS5代理服务器
type SOCKS5ProxyServer struct {
	dialer transport.StreamDialer
}

// NewSOCKS5ProxyServer 创建一个新的SOCKS5代理服务器
func NewSOCKS5ProxyServer(dialer transport.StreamDialer) *SOCKS5ProxyServer {
	return &SOCKS5ProxyServer{dialer: dialer}
}

// handleConnection 处理单个SOCKS5连接
func (s *SOCKS5ProxyServer) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// SOCKS5握手
	if err := s.performHandshake(clientConn); err != nil {
		log.Printf("SOCKS5 握手失败: %v", err)
		return
	}

	// 处理连接请求
	targetAddr, err := s.handleConnectRequest(clientConn)
	if err != nil {
		log.Printf("SOCKS5 连接请求处理失败: %v", err)
		return
	}

	// 建立到目标地址的连接
	targetConn, err := s.dialer.DialStream(context.Background(), targetAddr)
	if err != nil {
		log.Printf("连接到目标地址失败 %s: %v", targetAddr, err)
		// 发送连接失败响应
		s.sendConnectResponse(clientConn, 0x05) // Connection refused
		return
	}
	defer targetConn.Close()

	// 发送连接成功响应
	if err := s.sendConnectResponse(clientConn, 0x00); err != nil {
		log.Printf("发送连接响应失败: %v", err)
		return
	}

	log.Printf("SOCKS5 代理连接建立: %s", targetAddr)

	// 开始双向数据转发
	s.relayData(clientConn, targetConn)
}

// performHandshake 执行SOCKS5握手
func (s *SOCKS5ProxyServer) performHandshake(conn net.Conn) error {
	// 读取客户端hello消息
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("读取握手消息失败: %w", err)
	}

	if n < 3 || buf[0] != 0x05 {
		return fmt.Errorf("无效的SOCKS版本")
	}

	// 响应：选择无认证方法
	_, err = conn.Write([]byte{0x05, 0x00}) // VER=5, METHOD=0 (no auth)
	return err
}

// handleConnectRequest 处理连接请求
func (s *SOCKS5ProxyServer) handleConnectRequest(conn net.Conn) (string, error) {
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("读取连接请求失败: %w", err)
	}

	if n < 7 || buf[0] != 0x05 || buf[1] != 0x01 {
		return "", fmt.Errorf("无效的连接请求")
	}

	// 解析目标地址
	var targetAddr string
	addrType := buf[3]

	switch addrType {
	case 0x01: // IPv4
		if n < 10 {
			return "", fmt.Errorf("IPv4地址数据不完整")
		}
		ip := net.IP(buf[4:8])
		port := binary.BigEndian.Uint16(buf[8:10])
		targetAddr = fmt.Sprintf("%s:%d", ip.String(), port)

	case 0x03: // Domain name
		if n < 7 {
			return "", fmt.Errorf("域名地址数据不完整")
		}
		domainLen := int(buf[4])
		if n < 7+domainLen {
			return "", fmt.Errorf("域名数据不完整")
		}
		domain := string(buf[5 : 5+domainLen])
		port := binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])
		targetAddr = fmt.Sprintf("%s:%d", domain, port)

	case 0x04: // IPv6
		if n < 22 {
			return "", fmt.Errorf("IPv6地址数据不完整")
		}
		ip := net.IP(buf[4:20])
		port := binary.BigEndian.Uint16(buf[20:22])
		targetAddr = fmt.Sprintf("[%s]:%d", ip.String(), port)

	default:
		return "", fmt.Errorf("不支持的地址类型: %d", addrType)
	}

	return targetAddr, nil
}

// sendConnectResponse 发送连接响应
func (s *SOCKS5ProxyServer) sendConnectResponse(conn net.Conn, replyCode byte) error {
	// 简单的响应：VER=5, REP=replyCode, RSV=0, ATYP=1, BND.ADDR=0.0.0.0, BND.PORT=0
	response := []byte{0x05, replyCode, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, err := conn.Write(response)
	return err
}

// relayData 在客户端和目标服务器之间转发数据
func (s *SOCKS5ProxyServer) relayData(clientConn net.Conn, targetConn transport.StreamConn) {
	done := make(chan struct{}, 2)

	// 客户端 -> 目标服务器
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(targetConn, clientConn)
		targetConn.CloseWrite()
	}()

	// 目标服务器 -> 客户端
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(clientConn, targetConn)
		if tcpConn, ok := clientConn.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// 等待任一方向的传输结束
	<-done
}

// startSOCKS5Server 启动SOCKS5代理服务器
func startSOCKS5Server(address string, dialer transport.StreamDialer) {
	server := NewSOCKS5ProxyServer(dialer)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("SOCKS5服务器监听失败 %s: %v", address, err)
	}
	defer listener.Close()

	log.Printf("SOCKS5代理服务器监听在 %s", listener.Addr().String())

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("接受SOCKS5连接失败: %v", err)
			continue
		}

		go server.handleConnection(clientConn)
	}
}

func main() {
	transportFlag := flag.String("transport", "", "Transport config")
	addrFlag := flag.String("localAddr", "localhost:1080", "Local proxy address")
	urlProxyPrefixFlag := flag.String("urlProxyPrefix", "/proxy", "Path where to run the URL proxy. Set to empty (\"\") to disable it.")
	whitelistFileFlag := flag.String("whitelist-file", "", "Path to file containing domains to bypass proxy (one per line)")
	monitorDomainsFlag := flag.String("monitorDomains", "", "Comma-separated list of domains to monitor traffic for (empty = monitor all)")
	socketPortFlag := flag.String("socket-port", "", "Port to run SOCKS5 proxy on (e.g., '1082')")

	// 自定义 help 信息
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "HTTP2Transport - HTTP和SOCKS5代理服务器\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n")
		fmt.Fprintf(os.Stderr, "  %s [选项]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  # 基本用法 - 启动HTTP代理\n")
		fmt.Fprintf(os.Stderr, "  KEY=ss://ENCRYPTION_KEY@HOST:PORT/\n")
		fmt.Fprintf(os.Stderr, "  %s -transport \"$KEY\" -localAddr 0.0.0.0:1080\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 同时启动HTTP和SOCKS5代理\n")
		fmt.Fprintf(os.Stderr, "  %s -transport \"$KEY\" -localAddr 0.0.0.0:1080 -socket-port 1082\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 使用白名单文件\n")
		fmt.Fprintf(os.Stderr, "  echo -e \"265.com\\n.zzxworld.com\" > whitelist.txt\n")
		fmt.Fprintf(os.Stderr, "  %s -transport \"$KEY\" -whitelist-file whitelist.txt\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 监控特定域名流量\n")
		fmt.Fprintf(os.Stderr, "  %s -transport \"$KEY\" -monitorDomains \"api.anthropic.com,*.openai.com\"\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 完整示例\n")
		fmt.Fprintf(os.Stderr, "  %s -transport \"$KEY\" \\\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    -localAddr 0.0.0.0:1080 \\\n")
		fmt.Fprintf(os.Stderr, "    -socket-port 1082 \\\n")
		fmt.Fprintf(os.Stderr, "    -whitelist-file whitelist.txt \\\n")
		fmt.Fprintf(os.Stderr, "    -monitorDomains \"api.anthropic.com,*.openai.com\"\n\n")
		fmt.Fprintf(os.Stderr, "白名单文件格式:\n")
		fmt.Fprintf(os.Stderr, "  265.com          # 完全匹配\n")
		fmt.Fprintf(os.Stderr, "  .zzxworld.com    # 匹配该域名及其所有子域名\n")
		fmt.Fprintf(os.Stderr, "  # 注释行\n\n")
		fmt.Fprintf(os.Stderr, "服务端点:\n")
		fmt.Fprintf(os.Stderr, "  HTTP代理:   http://localhost:1080\n")
		fmt.Fprintf(os.Stderr, "  SOCKS5代理: socks5://localhost:1082 (如果启用)\n")
		fmt.Fprintf(os.Stderr, "  统计接口:   http://localhost:1080/stats\n")
	}

	flag.Parse()

	// 创建原始拨号器
	baseDialer, err := configurl.NewDefaultProviders().NewStreamDialer(context.Background(), *transportFlag)
	if err != nil {
		log.Fatalf("Could not create dialer: %v", err)
	}

	// 从文件读取白名单域名
	var whitelistDomains []string
	if *whitelistFileFlag != "" {
		content, err := os.ReadFile(*whitelistFileFlag)
		if err != nil {
			log.Fatalf("无法读取白名单文件 %s: %v", *whitelistFileFlag, err)
		}
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				whitelistDomains = append(whitelistDomains, line)
			}
		}
		log.Printf("从文件 %s 读取到 %d 个白名单域名", *whitelistFileFlag, len(whitelistDomains))
	}

	// 解析监控域名
	var monitorDomains []string
	if *monitorDomainsFlag != "" {
		monitorDomains = strings.Split(*monitorDomainsFlag, ",")
		log.Printf("监控域名: %v", monitorDomains)
	}

	// 创建域名流量统计
	domainStats := NewDomainTrafficStats()

	// 创建带白名单的拨号器
	dialer := NewWhitelistDialer(baseDialer, whitelistDomains, monitorDomains, domainStats)

	// 如果指定了SOCKS5端口，启动SOCKS5代理服务器
	if *socketPortFlag != "" {
		socketAddr := fmt.Sprintf("localhost:%s", *socketPortFlag)
		go startSOCKS5Server(socketAddr, dialer)
	}

	listener, err := net.Listen("tcp", *addrFlag)
	if err != nil {
		log.Fatalf("Could not listen on address %v: %v", *addrFlag, err)
	}
	defer listener.Close()
	log.Printf("代理服务器监听在 %v", listener.Addr().String())

	// 创建HTTP代理处理器
	proxyHandler := httpproxy.NewProxyHandler(dialer)
	if *urlProxyPrefixFlag != "" {
		proxyHandler.FallbackHandler = http.StripPrefix(*urlProxyPrefixFlag, httpproxy.NewPathHandler(dialer))
	}

	// 创建一个handler来分离代理请求和统计请求
	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 处理统计端点
		if r.Method == http.MethodGet && r.URL.Path == "/stats" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			globalStats := domainStats.GetGlobalStats()
			allDomainStats := domainStats.GetAllDomainStats()

			// 构建完整的统计响应
			response := map[string]any{
				"uploadBytes":            formatBytes(globalStats.GetUploadBytes()),
				"downloadBytes":          formatBytes(globalStats.GetDownloadBytes()),
				"outlineConnectionTime":  formatDuration(globalStats.GetCurrentSessionDuration()),
				"currentSessionDuration": formatDuration(globalStats.GetCurrentSessionDuration()),
				"activeConnections":      globalStats.GetActiveConnections(),
				"totalConnections":       globalStats.GetTotalConnections(),
				"monitoredDomains":       allDomainStats,
			}

			jsonData, err := json.MarshalIndent(response, "", "  ")
			if err != nil {
				http.Error(w, "Failed to marshal JSON", http.StatusInternalServerError)
				return
			}

			w.Write(jsonData)
			return
		}

		// 其他所有请求交给代理handler处理（包括CONNECT方法）
		proxyHandler.ServeHTTP(w, r)
	})

	server := http.Server{Handler: mainHandler}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error running web server: %v", err)
		}
	}()

	// 等待中断信号以优雅地关闭服务器
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	log.Print("关闭中...")

	// 优雅关闭服务器，超时设置为5秒
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("关闭失败: %v", err)
	}
}
