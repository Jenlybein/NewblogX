package secretbox

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	return key
}

func TestSealOpenRoundTrip(t *testing.T) {
	box, err := New(newKey(t))
	if err != nil {
		t.Fatalf("构造失败: %v", err)
	}

	sealed, err := box.Seal("sk-live-abcdef")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if !IsSealed(sealed) {
		t.Fatalf("密文应带前缀，实际 %q", sealed)
	}
	if strings.Contains(sealed, "sk-live-abcdef") {
		t.Fatal("密文里不该出现明文")
	}

	plain, err := box.Open(sealed)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if plain != "sk-live-abcdef" {
		t.Fatalf("解密结果不符: %q", plain)
	}
}

func TestSealIsIdempotentOnSealedValue(t *testing.T) {
	box, _ := New(newKey(t))
	sealed, _ := box.Seal("v1")

	again, err := box.Seal(sealed)
	if err != nil {
		t.Fatalf("二次加密不该失败: %v", err)
	}
	if again != sealed {
		t.Fatal("已经是密文的值不该被再加密一次（迁移命令要幂等）")
	}
}

func TestSealEmptyStaysEmpty(t *testing.T) {
	box, _ := New(newKey(t))
	sealed, err := box.Seal("")
	if err != nil || sealed != "" {
		t.Fatalf("空值应原样返回，实际 %q err=%v", sealed, err)
	}
}

func TestOpenPassesThroughLegacyPlaintext(t *testing.T) {
	box, _ := New(newKey(t))
	plain, err := box.Open("legacy-plain-secret")
	if err != nil {
		t.Fatalf("历史明文必须仍可读: %v", err)
	}
	if plain != "legacy-plain-secret" {
		t.Fatalf("不该改动历史明文，实际 %q", plain)
	}
}

func TestWrongKeyFailsLoudly(t *testing.T) {
	box, _ := New(newKey(t))
	sealed, _ := box.Seal("secret")

	other, _ := New(newKey(t))
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("换一把密钥必须解密失败，而不是悄悄返回空值")
	}
}

func TestNilBoxRefusesWork(t *testing.T) {
	var box *Box
	if _, err := box.Seal("x"); err == nil {
		t.Fatal("没有主密钥时加密必须报错（不能悄悄存明文）")
	}
	if _, err := box.Open("enc:v1:AAAA"); err == nil {
		t.Fatal("没有主密钥时解密密文必须报错")
	}
	// 历史明文在没有主密钥时仍可读：否则升级前部署会直接停摆。
	if got, err := box.Open("plain"); err != nil || got != "plain" {
		t.Fatalf("历史明文应可读，实际 %q err=%v", got, err)
	}
}

func TestFromEnvAcceptsThreeFormats(t *testing.T) {
	key := newKey(t)
	cases := map[string]string{
		"base64": base64.StdEncoding.EncodeToString(key),
		"hex":    strings.ToUpper(strings.TrimSpace(hexOf(key))),
		// 原文形态用可打印字符：环境变量在 Windows 上是 UTF-16，随机字节
		// 含 NUL 或非法序列会被改写，那种情况只可能出现在 Linux 上。
		"raw": strings.Repeat("k", keySize),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvKey, value)
			box, err := FromEnv()
			if err != nil {
				t.Fatalf("应接受这种写法: %v", err)
			}
			if box == nil {
				t.Fatal("应构造出 Box")
			}
			sealed, err := box.Seal("v")
			if err != nil {
				t.Fatalf("加密失败: %v", err)
			}
			if got, _ := box.Open(sealed); got != "v" {
				t.Fatalf("往返失败: %q", got)
			}
		})
	}
}

func TestFromEnvEmptyMeansNoKey(t *testing.T) {
	t.Setenv(EnvKey, "")
	box, err := FromEnv()
	if err != nil {
		t.Fatalf("没配主密钥不算错误: %v", err)
	}
	if box != nil {
		t.Fatal("没配主密钥应返回 nil")
	}
}

func TestFromEnvRejectsShortKey(t *testing.T) {
	t.Setenv(EnvKey, "too-short")
	if _, err := FromEnv(); err == nil {
		t.Fatal("长度不足的密钥必须报错，不能凑合着用")
	}
}

func hexOf(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexDigits[c>>4], hexDigits[c&0x0f])
	}
	return string(out)
}

func TestEnvKeyNameIsStable(t *testing.T) {
	// 部署脚本、文档与代码共用这一个名字；改名等于让线上静默退回明文。
	if EnvKey != "BLOGX_CONFIG_SECRET_KEY" {
		t.Fatalf("环境变量名变了: %s", EnvKey)
	}
	if os.Getenv(EnvKey) != "" && EnvKey == "" {
		t.Fatal("unreachable")
	}
}
