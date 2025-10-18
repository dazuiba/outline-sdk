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
    mainDialer := &mockStreamDialer{}
    secondDialer := &mockStreamDialer{}

    // 测试规则：基于你提供的示例
    directDomains := []string{"a.com", ".a.com", "265.com", ".zzxworld.com"}

    dialer := NewWhitelistDialer(mainDialer, secondDialer, directDomains, []string{}, []string{}, "main-proxy", nil)

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

func TestProxyHostLists(t *testing.T) {
    mainDialer := &mockStreamDialer{}
    secondDialer := &mockStreamDialer{}

    directDomains := []string{}
    mainProxyDomains := []string{"proxy.example.com", ".openai.com"}
    secondProxyDomains := []string{"second.example.com"}

    dialer := NewWhitelistDialer(mainDialer, secondDialer, directDomains, mainProxyDomains, secondProxyDomains, "main-proxy", nil)

    // 主代理匹配
    if !dialer.isMainProxyHost("proxy.example.com") {
        t.Errorf("expected proxy.example.com to be main proxy host")
    }
    if !dialer.isMainProxyHost("api.openai.com") {
        t.Errorf("expected api.openai.com to match .openai.com as main proxy host")
    }
    if dialer.isMainProxyHost("second.example.com") {
        t.Errorf("did not expect second.example.com to be main proxy host")
    }

    // 备用代理匹配
    if !dialer.isSecondProxyHost("second.example.com") {
        t.Errorf("expected second.example.com to be second proxy host")
    }
    if dialer.isSecondProxyHost("api.openai.com") {
        t.Errorf("did not expect api.openai.com to be second proxy host")
    }
}
