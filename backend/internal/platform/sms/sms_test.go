package sms

import (
	"context"
	"testing"

	dypns "github.com/alibabacloud-go/dypnsapi-20170525/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"myblogx/conf"
	"myblogx/internal/apperr"
)

// 这个包的"真的发一条短信"只能靠真账号联调，所以单测守住的是**不发请求的那些分支**：
// 未配置时的失败语义、平台返回体怎么判定成功、以及场景号位不要漂。

func TestKind_号位固定(t *testing.T) {
	t.Parallel()

	// 号位是对外契约的一部分：库里、日志里、阿里云控制台的 OutId 都按它解释，
	// 改动等于让历史记录无法解读。
	if KindAuth != 1 {
		t.Fatalf("KindAuth 应为 1，实际 %d", KindAuth)
	}
	if KindPasswordProof != 3 {
		t.Fatalf("KindPasswordProof 应为 3（2 是已下线的绑定手机号，号位不复用），实际 %d", KindPasswordProof)
	}
}

func TestOutID_每个场景有可读标记(t *testing.T) {
	t.Parallel()

	if got := outID(KindAuth); got != "auth" {
		t.Fatalf("KindAuth 的 OutId got=%s want=auth", got)
	}
	if got := outID(KindPasswordProof); got != "password_proof" {
		t.Fatalf("KindPasswordProof 的 OutId got=%s want=password_proof", got)
	}
	// 已下线场景不该再有自己的标记，否则控制台里会看到一个查不到用处的来源。
	if got := outID(Kind(2)); got != "unknown" {
		t.Fatalf("已下线的号位应落在 unknown，实际 %s", got)
	}
}

func TestSend_未配置时失败而不是静默成功(t *testing.T) {
	t.Parallel()

	c := &Client{}
	err := c.Send(context.Background(), conf.SMS{}, "13800001234", KindAuth, 5, 60)
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserSmsSendFailed {
		t.Fatalf("未配置应返回 %s，实际 %v", apperr.CodeUserSmsSendFailed, err)
	}
}

func TestCheck_未配置时用核验错误码(t *testing.T) {
	t.Parallel()

	// 这里刻意不走 sms_send_failed：用户看到的是"核验没跑通"，
	// 报"发送失败"会把排查方向指错。
	c := &Client{}
	_, err := c.Check(context.Background(), conf.SMS{}, "13800001234", "123456")
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserSmsVerifyFailed {
		t.Fatalf("未配置应返回 %s，实际 %v", apperr.CodeUserSmsVerifyFailed, err)
	}
}

func TestAssertSendOK_两个失败面都要看(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body *dypns.SendSmsVerifyCodeResponseBody
		ok   bool
	}{
		{"空响应体", nil, false},
		{"只有 Code 没有 Success", &dypns.SendSmsVerifyCodeResponseBody{Code: tea.String("OK")}, false},
		{"Code 不是 OK", &dypns.SendSmsVerifyCodeResponseBody{
			Code: tea.String("isv.BUSINESS_LIMIT_CONTROL"), Success: tea.Bool(true),
		}, false},
		{"Success 为 false", &dypns.SendSmsVerifyCodeResponseBody{
			Code: tea.String("OK"), Success: tea.Bool(false),
		}, false},
		{"Code OK 且 Success", &dypns.SendSmsVerifyCodeResponseBody{
			Code: tea.String("OK"), Success: tea.Bool(true),
		}, true},
	}

	for _, tc := range cases {
		err := assertSendOK(tc.body)
		if tc.ok && err != nil {
			t.Fatalf("%s：应判成功，实际 %v", tc.name, err)
		}
		if !tc.ok {
			if err == nil {
				t.Fatalf("%s：应判失败", tc.name)
			}
			if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserSmsSendFailed {
				t.Fatalf("%s：错误码应是 %s，实际 %v", tc.name, apperr.CodeUserSmsSendFailed, err)
			}
		}
	}
}
