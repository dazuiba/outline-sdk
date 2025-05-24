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
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/x/configurl"
	"github.com/Jigsaw-Code/outline-sdk/x/httpproxy"
)

// WhitelistDialer 实现了一个带有域名白名单的拨号器
type WhitelistDialer struct {
	// 原始拨号器，用于通过shadowsocks代理连接
	proxyDialer transport.StreamDialer
	// 域名白名单
	whitelist map[string]bool
}

// NewWhitelistDialer 创建一个新的WhitelistDialer
func NewWhitelistDialer(proxyDialer transport.StreamDialer, whitelistDomains []string) *WhitelistDialer {
	whitelist := make(map[string]bool)
	for _, domain := range whitelistDomains {
		whitelist[domain] = true
	}

	return &WhitelistDialer{
		proxyDialer: proxyDialer,
		whitelist:   whitelist,
	}
}

// DialStream 实现了transport.StreamDialer接口
func (d *WhitelistDialer) DialStream(ctx context.Context, address string) (transport.StreamConn, error) {
	// 解析地址，获取域名部分
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	// 检查域名是否在白名单中
	if d.isWhitelisted(host) {
		log.Printf("使用直连连接 %s", address)
		netDialer := &net.Dialer{}
		conn, err := netDialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, err
		}
		// 将net.Conn转换为transport.StreamConn
		return &tcpStreamConn{Conn: conn}, nil
	}

	// 不在白名单中，使用代理连接
	log.Printf("使用代理连接 %s", address)
	return d.proxyDialer.DialStream(ctx, address)
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

func main() {
	transportFlag := flag.String("transport", "", "Transport config")
	addrFlag := flag.String("localAddr", "localhost:1080", "Local proxy address")
	urlProxyPrefixFlag := flag.String("urlProxyPrefix", "/proxy", "Path where to run the URL proxy. Set to empty (\"\") to disable it.")
	whitelistFlag := flag.String("whitelist", "", "Comma-separated list of domains to bypass proxy")
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

	// 创建带白名单的拨号器
	dialer := NewWhitelistDialer(baseDialer, whitelistDomains)

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
	server := http.Server{Handler: proxyHandler}
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
