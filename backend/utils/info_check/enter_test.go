package info_check_test

import (
	"myblogx/utils/info_check"
	"testing"
)

func TestSensitiveWordAndUsernameCheck(t *testing.T) {
	if word, ok := info_check.IsSensitiveWord("A_d_m_i_n_001"); !ok || word != "admin" {
		t.Fatalf("敏感词检测失败: word=%s ok=%v", word, ok)
	}
	if _, ok := info_check.IsSensitiveWord("normal_user_123"); ok {
		t.Fatal("正常用户名不应被识别为敏感词")
	}

	valid := []string{"hello_12", "abc12345", "my_name_1"}
	for _, v := range valid {
		if err := info_check.CheckUsername(v); err != nil {
			t.Fatalf("合法用户名被拒绝: %s, err=%v", v, err)
		}
	}

	invalid := []string{"", "abc", "a-b-c-123", "_abcdef", "admin123", "a___bcdef"}
	for _, v := range invalid {
		if err := info_check.CheckUsername(v); err == nil {
			t.Fatalf("非法用户名未报错: %s", v)
		}
	}
}
