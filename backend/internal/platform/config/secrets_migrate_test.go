package config

import (
	"encoding/json"
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/platform/secretbox"
)

// 迁移命令（-encrypt-config）的用例：它要能在**线上已有明文**的库上跑一次，
// 把历史数据加密，并且重复跑不产生任何改动。

func TestEncryptStoredSecretsSealsLegacyPlaintext(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 造出升级前的库状态：mail / storage / oauth 三段都是明文。
	writeLegacy := func(name SectionName, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("构造历史数据失败: %v", err)
		}
		if err := upsertPayload(db, string(name), string(raw)); err != nil {
			t.Fatalf("写历史数据失败: %v", err)
		}
	}
	writeLegacy(SectionMail, conf.Mail{SecretID: "id", SecretKey: "mail-plain", FromAddress: "a@example.com"})
	writeLegacy(SectionStorage, conf.QiNiu{AccessKey: "ak", SecretKey: "storage-plain"})
	writeLegacy(SectionOAuth, conf.QQ{AppID: "app", AppKey: "oauth-plain"})

	changed, err := svc.EncryptStoredSecrets(testSealer(t))
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if changed != 3 {
		t.Fatalf("应更新 3 段，实际 %d", changed)
	}

	// 库里不该再有明文，且三个字段都是密文。
	payloads, err := loadPayloads(db)
	if err != nil {
		t.Fatalf("读库失败: %v", err)
	}
	for _, plain := range []string{"mail-plain", "storage-plain", "oauth-plain"} {
		if strings.Contains(payloads[string(SectionMail)]+payloads[string(SectionStorage)]+payloads[string(SectionOAuth)], plain) {
			t.Fatalf("迁移后库里仍有明文: %s", plain)
		}
	}
	for name, field := range map[SectionName]func(string) bool{
		SectionMail:    func(v string) bool { return secretbox.IsSealed(v) },
		SectionStorage: func(v string) bool { return secretbox.IsSealed(v) },
		SectionOAuth:   func(v string) bool { return secretbox.IsSealed(v) },
	} {
		var probe struct {
			SecretKey string `json:"secret_key"`
			AppKey    string `json:"app_key"`
		}
		if err := json.Unmarshal([]byte(payloads[string(name)]), &probe); err != nil {
			t.Fatalf("解析 %s 失败: %v", name, err)
		}
		value := probe.SecretKey
		if name == SectionOAuth {
			value = probe.AppKey
		}
		if !field(value) {
			t.Fatalf("%s 的密钥仍是明文: %q", name, value)
		}
	}

	// 迁移后仍能解回明文（否则等于把配置弄丢）。
	reloaded := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)
	reloaded.SetSealer(testSealer(t))
	if err := reloaded.InitRuntimeConfig(); err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if got := reloaded.GetRuntimeMail().SecretKey; got != "mail-plain" {
		t.Fatalf("迁移后应能解回明文，实际 %q", got)
	}
}

func TestEncryptStoredSecretsIsIdempotent(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 走正常写入路径：已经加密过了。
	if err := svc.UpdateMail(conf.Mail{SecretKey: "already-sealed", FromAddress: "a@example.com"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	before, err := loadPayloads(db)
	if err != nil {
		t.Fatalf("读库失败: %v", err)
	}

	changed, err := svc.EncryptStoredSecrets(testSealer(t))
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if changed != 0 {
		t.Fatalf("已经是密文时不该再改，实际改了 %d 段", changed)
	}

	after, err := loadPayloads(db)
	if err != nil {
		t.Fatalf("读库失败: %v", err)
	}
	if before[string(SectionMail)] != after[string(SectionMail)] {
		t.Fatal("幂等要求：重复迁移不能改写已经加密的行")
	}
}

func TestEncryptStoredSecretsRequiresMasterKey(t *testing.T) {
	svc, _ := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	if _, err := svc.EncryptStoredSecrets(nil); err == nil {
		t.Fatal("没有主密钥时必须报错，不能什么都不做就退出")
	}
}
