package conf_test

import (
	"myblogx/conf"
	"strings"
	"testing"
)

func TestSystemAddr(t *testing.T) {
	s := conf.System{
		IP:   "127.0.0.1",
		Port: 8080,
	}
	if got := s.Addr(); got != "127.0.0.1:8080" {
		t.Fatalf("Addr() 错误: %s", got)
	}
}

func TestRedisGetAddr(t *testing.T) {
	r := conf.Redis{
		Addr: "localhost",
		Port: 6379,
	}
	if got := r.GetAddr(); got != "localhost:6379" {
		t.Fatalf("GetAddr() 错误: %s", got)
	}
}

func TestDBHelpers(t *testing.T) {
	db := conf.DB{
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Password: "pwd",
		DBName:   "blogx",
	}

	dsn := db.DSN()
	if !strings.Contains(dsn, "root:pwd@tcp(127.0.0.1:3306)/blogx") {
		t.Fatalf("DSN() 内容异常: %s", dsn)
	}

	if db.Addr() != "127.0.0.1:3306" {
		t.Fatalf("Addr() 错误: %s", db.Addr())
	}

	if db.Empty() {
		t.Fatal("Empty() 不应为 true")
	}

	if !(conf.DB{}).Empty() {
		t.Fatal("空 DB 配置应为 true")
	}
}

func TestQQUrl(t *testing.T) {
	q := conf.QQ{
		AppID:    "123",
		Redirect: "https://a.example/callback",
	}
	url := q.Url()
	if !strings.Contains(url, "client_id=123") {
		t.Fatalf("URL 未包含 app_id: %s", url)
	}
	if !strings.Contains(url, "redirect_uri=https://a.example/callback") {
		t.Fatalf("URL 未包含 redirect_uri: %s", url)
	}
}
