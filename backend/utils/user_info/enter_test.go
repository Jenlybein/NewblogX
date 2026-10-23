package user_info_test

import (
	"myblogx/utils/user_info"
	"net/http"
	"net/http/httptest"
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
