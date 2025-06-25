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
	"encoding/json"
	"flag"
	"fmt"
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

// matchWildcard 检查目标字符串是否匹配给定的通配符模式
func matchWildcard(pattern, target string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:]
		return strings.HasSuffix(target, suffix) || target == suffix
	}
	return pattern == target
}

// isWhitelisted 检查域名是否在白名单中，支持通配符匹配
func (d *WhitelistDialer) isWhitelisted(host string) bool {
	// 检查完整域名是否在白名单中
	if d.whitelist[host] {
		return true
	}

	// 检查是否匹配白名单中的通配符规则
	for pattern := range d.whitelist {
		if matchWildcard(pattern, host) {
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

	// 检查是否匹配监控列表中的通配符规则
	for pattern := range d.monitorDomains {
		if matchWildcard(pattern, host) {
			return true
		}
	}

	return false
}

func main() {
	transportFlag := flag.String("transport", "", "Transport config")
	addrFlag := flag.String("localAddr", "localhost:1080", "Local proxy address")
	urlProxyPrefixFlag := flag.String("urlProxyPrefix", "/proxy", "Path where to run the URL proxy. Set to empty (\"\") to disable it.")
	whitelistFlag := flag.String("whitelist", "", "Comma-separated list of domains to bypass proxy")
	monitorDomainsFlag := flag.String("monitorDomains", "", "Comma-separated list of domains to monitor traffic for (empty = monitor all)")
	flag.Parse()

	// 创建原始拨号器
	baseDialer, err := configurl.NewDefaultProviders().NewStreamDialer(context.Background(), *transportFlag)
	if err != nil {
		log.Fatalf("Could not create dialer: %v", err)
	}

	// 解析白名单域名
	var whitelistDomains []string
	if *whitelistFlag != "" {
		whitelistDomains = strings.Split(*whitelistFlag, ",")
		log.Printf("域名白名单: %v", whitelistDomains)
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
			response := map[string]interface{}{
				"uploadBytes":             formatBytes(globalStats.GetUploadBytes()),
				"downloadBytes":           formatBytes(globalStats.GetDownloadBytes()),
				"outlineConnectionTime":   formatDuration(globalStats.GetCurrentSessionDuration()),
				"currentSessionDuration":  formatDuration(globalStats.GetCurrentSessionDuration()),
				"activeConnections":       globalStats.GetActiveConnections(),
				"totalConnections":        globalStats.GetTotalConnections(),
				"monitoredDomains":        allDomainStats,
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
