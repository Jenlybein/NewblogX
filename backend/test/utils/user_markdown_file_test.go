package utils_test

import (
	"myblogx/utils/markdown"
	"myblogx/utils/user_info"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUserInfoHelpers(t *testing.T) {
	if user_info.IpType("127.0.0.1") != "ipv4" {
		t.Fatal("ipv4 识别失败")
	}
	if user_info.IpType("::1") != "ipv6" {
		t.Fatal("ipv6 识别失败")
	}
	if user_info.IpType("not-ip") != "" {
		t.Fatal("非法 IP 应返回空")
	}

	if !user_info.IsLocalIP("127.0.0.1", "ipv4") {
		t.Fatal("127.0.0.1 应被识别为本地 IP")
	}
	if !user_info.IsLocalIP("192.168.1.2", "ipv4") {
		t.Fatal("192.168.x.x 应被识别为本地 IP")
	}
	if user_info.IsLocalIP("8.8.8.8", "ipv4") {
		t.Fatal("公网 IP 不应被识别为本地 IP")
	}
}

func TestGetClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 8.8.8.8")
	c.Request = req

	if ip := user_info.GetClientIP(c); ip != "10.0.0.2" {
		t.Fatalf("X-Forwarded-For 提取失败: %s", ip)
	}
}

func TestMarkdownHelpers(t *testing.T) {
	md := "# 标题\n\n<script>alert(1)</script>\n\n[跳转](#标题)"
	unsafe := markdown.MdToHTMLUnsafe(md)
	if !strings.Contains(unsafe, "<script>") {
		t.Fatal("MdToHTMLUnsafe 不应过滤 script")
	}
	if !strings.Contains(unsafe, `href="#`) {
		t.Fatal("Markdown 链接应被渲染为 HTML 链接")
	}

	safe := markdown.MdToHTMLSafe(md)
	if strings.Contains(safe, "<script>") {
		t.Fatal("MdToHTMLSafe 应过滤 script")
	}
	if !strings.Contains(safe, `href="#`) {
		t.Fatal("安全模式应保留普通锚点链接")
	}

	text := markdown.MdToText("**hello**")
	if strings.Contains(text, "<") {
		t.Fatalf("MdToText 应返回纯文本: %s", text)
	}

	if got := markdown.ExtractText("abcdef", 3); got != "abc" {
		t.Fatalf("ExtractText 截断错误: %s", got)
	}
}
