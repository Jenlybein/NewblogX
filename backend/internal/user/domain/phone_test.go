package domain_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"
)

// 手机号是登录主标识：形状校验挡在发码之前，非法号连短信配额都不该占。

func TestValidatePhone_合法号(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"13800000000", "19912345678", "  13800000000  "} {
		if err := domain.ValidatePhone(s); err != nil {
			t.Fatalf("%q 应是合法手机号，实际报错: %v", s, err)
		}
	}
}

func TestValidatePhone_非法号(t *testing.T) {
	t.Parallel()

	// 空串、位数不足/超出、第二位越界、首位不是 1、带国际区号、纯字母。
	for _, s := range []string{"", "1380000000", "138000000000", "12800000000", "23800000000", "+8613800000000", "abcdefghijk"} {
		err := domain.ValidatePhone(s)
		if err == nil {
			t.Fatalf("%q 应被判为非法手机号", s)
		}
		code, ok := apperr.CodeOf(err)
		if !ok || code != apperr.CodeUserPhoneInvalid {
			t.Fatalf("%q 的错误码应是 %s，实际: %v", s, apperr.CodeUserPhoneInvalid, err)
		}
	}
}

func TestMaskPhone_保留前三后四(t *testing.T) {
	t.Parallel()

	if got := domain.MaskPhone("13800001234"); got != "138****1234" {
		t.Fatalf("打码结果 got=%s want=138****1234", got)
	}
	// 前后空格不该影响打码结果（历史数据里带空格的情况真实存在）。
	if got := domain.MaskPhone(" 13800001234 "); got != "138****1234" {
		t.Fatalf("带空格的号码打码结果 got=%s want=138****1234", got)
	}
}

func TestMaskPhone_非11位整体打码(t *testing.T) {
	t.Parallel()

	// 长度不对的输入不猜结构，也不原样返回：原样返回等于把脏数据当号码展示。
	for _, s := range []string{"", "123", "13800001234567"} {
		if got := domain.MaskPhone(s); got != "***" {
			t.Fatalf("%q 应整体打码成 ***，实际 got=%s", s, got)
		}
	}
}
