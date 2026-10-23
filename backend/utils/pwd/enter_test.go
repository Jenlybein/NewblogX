package pwd_test

import (
	"myblogx/utils/pwd"
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	raw := "S3cr3t!"
	hashed, err := pwd.GenerateFromPassword(raw)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	if hashed == raw {
		t.Fatal("哈希结果不应等于原文")
	}
	if !pwd.CompareHashAndPassword(hashed, raw) {
		t.Fatal("正确密码校验失败")
	}
	if pwd.CompareHashAndPassword(hashed, "wrong") {
		t.Fatal("错误密码不应通过校验")
	}
}
