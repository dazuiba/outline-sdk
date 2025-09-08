package main

import (
	"context"
	"testing"

	"github.com/Jigsaw-Code/outline-sdk/transport"
)

// mockStreamDialer 用于测试的模拟拨号器
type mockStreamDialer struct{}

func (m *mockStreamDialer) DialStream(ctx context.Context, addr string) (transport.StreamConn, error) {
	return nil, nil
}

func TestMatchDomain(t *testing.T) {
	// 基于你提供的JS测试用例：规则 ["a.com", ".a.com"]
	testCases := []struct {
		rule     string
		host     string
		expected bool
	}{
		// 严格按照 JS 测试数据
		{"a.com", "www.a.com", true},
		{"a.com", "aa.com", false},
		{"a.com", "a.com", true},

		{".a.com", "www.a.com", true},
		{".a.com", "aa.com", false},
		{".a.com", "a.com", true},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			result := matchDomain(tc.rule, tc.host)
			if result != tc.expected {
				t.Errorf("matchDomain(%q, %q) = %v, expected %v", tc.rule, tc.host, result, tc.expected)
			}
		})
	}
}

func TestIsDirectHost(t *testing.T) {
	// 创建测试拨号器
	mockDialer := &mockStreamDialer{}

	// 测试规则：基于你提供的示例
    directDomains := []string{"a.com", ".a.com", "265.com", ".zzxworld.com"}
    monitorDomains := []string{}

    dialer := NewWhitelistDialer(mockDialer, directDomains, nil, "proxy", monitorDomains, nil)

	testCases := []struct {
		host     string
		expected bool
		desc     string
	}{
		// 基于你的JS测试用例
        {"www.a.com", true, "www.a.com should be direct"},
        {"aa.com", false, "aa.com should not be direct"},
        {"a.com", true, "a.com should be direct"},

		// 测试 265.com
        {"265.com", true, "265.com should be direct"},
        {"sub.265.com", true, "sub.265.com should be direct"},

		// 测试 .zzxworld.com 格式
        {"zzxworld.com", true, "zzxworld.com should be direct"},
        {"www.zzxworld.com", true, "www.zzxworld.com should be direct"},
        {"api.zzxworld.com", true, "api.zzxworld.com should be direct"},
        {"sub.sub.zzxworld.com", true, "nested subdomain should be direct"},

		// 不应该匹配的域名
        {"notwhitelisted.com", false, "notwhitelisted.com should not be direct"},
        {"zzxworld.com.evil.com", false, "domain with suffix should not be direct"},
        {"265.com.evil.com", false, "265.com with suffix should not be direct"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
            result := dialer.isDirectHost(tc.host)
            if result != tc.expected {
                t.Errorf("isDirectHost(%q) = %v, expected %v", tc.host, result, tc.expected)
            }
        })
    }
}

func TestShouldMonitor(t *testing.T) {
	// 创建测试拨号器
	mockDialer := &mockStreamDialer{}

	// 测试监控规则
    directDomains := []string{}
    monitorDomains := []string{"api.anthropic.com", "*.openai.com"}

    dialer := NewWhitelistDialer(mockDialer, directDomains, nil, "proxy", monitorDomains, nil)

	testCases := []struct {
		host     string
		expected bool
		desc     string
	}{
		{"api.anthropic.com", true, "api.anthropic.com should be monitored"},
		{"claude.anthropic.com", false, "claude.anthropic.com should not be monitored"},
		{"chat.openai.com", true, "chat.openai.com should match *.openai.com"},
		{"api.openai.com", true, "api.openai.com should match *.openai.com"},
		{"openai.com", true, "openai.com should match *.openai.com"},
		{"google.com", false, "google.com should not be monitored"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := dialer.shouldMonitor(tc.host)
			if result != tc.expected {
				t.Errorf("shouldMonitor(%q) = %v, expected %v", tc.host, result, tc.expected)
			}
		})
	}
}

func TestShouldMonitorEmptyList(t *testing.T) {
	// 测试空监控列表的情况（应该监控所有域名）
	mockDialer := &mockStreamDialer{}
    directDomains := []string{}
    monitorDomains := []string{} // 空列表

    dialer := NewWhitelistDialer(mockDialer, directDomains, nil, "proxy", monitorDomains, nil)

	testCases := []string{"api.anthropic.com", "google.com", "example.com"}

	for _, host := range testCases {
		t.Run("empty monitor list should monitor "+host, func(t *testing.T) {
			result := dialer.shouldMonitor(host)
			if !result {
				t.Errorf("shouldMonitor(%q) = false, expected true (empty list should monitor all)", host)
			}
		})
	}
}
