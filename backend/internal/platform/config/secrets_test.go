package config

import (
	"encoding/json"
	"strings"
	"testing"

	"myblogx/conf"
)

// 本文件的用例锁住"密钥不以明文落库"这条不变量本身 ——
// 不是测加密算法（那在 secretbox 包里），而是测**配置服务有没有真的用上它**：
// 三条路径（写入、读取、无主密钥时的行为）任意一条退化成明文，这些用例就该红。

func TestUpdateMailStoresSecretSealed(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	plainKey := "AKID-plaintext-should-never-appear"
	if err := svc.UpdateMail(conf.Mail{
		SecretID:    "id-1",
		SecretKey:   plainKey,
		Region:      "ap-guangzhou",
		FromAddress: "a@example.com",
	}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	// 1) 库里必须是密文，且整行 JSON 不含明文
	var row runtimeConfigRow
	if err := db.Where("name = ?", string(SectionMail)).Take(&row).Error; err != nil {
		t.Fatalf("读行失败: %v", err)
	}
	if strings.Contains(row.Payload, plainKey) {
		t.Fatalf("库里出现了明文密钥: %s", row.Payload)
	}
	var stored conf.Mail
	if err := json.Unmarshal([]byte(row.Payload), &stored); err != nil {
		t.Fatalf("解析落库 JSON 失败: %v", err)
	}
	if !strings.HasPrefix(stored.SecretKey, "enc:v1:") {
		t.Fatalf("secret_key 应是密文，实际 %q", stored.SecretKey)
	}

	// 2) 内存里必须是明文：发信/上传拿到的是能直接用的值
	if got := svc.GetRuntimeMail().SecretKey; got != plainKey {
		t.Fatalf("缓存里应是明文，实际 %q", got)
	}
}

func TestUpdateStorageStoresSecretSealed(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	const plain = "qiniu-secret-plain"
	if err := svc.UpdateStorage(conf.QiNiu{
		Enable: true, AccessKey: "ak", SecretKey: plain, Bucket: "b", Uri: "u",
	}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	var row runtimeConfigRow
	if err := db.Where("name = ?", string(SectionStorage)).Take(&row).Error; err != nil {
		t.Fatalf("读行失败: %v", err)
	}
	if strings.Contains(row.Payload, plain) {
		t.Fatalf("库里出现了明文存储密钥: %s", row.Payload)
	}
	if got := svc.GetRuntimeStorage().SecretKey; got != plain {
		t.Fatalf("缓存里应是明文，实际 %q", got)
	}
}

func TestUpdateOAuthStoresAppKeySealed(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	const plain = "qq-app-key-plain"
	if err := svc.UpdateOAuth(conf.QQ{AppID: "app", AppKey: plain, Redirect: "https://x"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	var row runtimeConfigRow
	if err := db.Where("name = ?", string(SectionOAuth)).Take(&row).Error; err != nil {
		t.Fatalf("读行失败: %v", err)
	}
	if strings.Contains(row.Payload, plain) {
		t.Fatalf("库里出现了明文 app_key: %s", row.Payload)
	}
	if got := svc.GetRuntimeOAuth().AppKey; got != plain {
		t.Fatalf("缓存里应是明文，实际 %q", got)
	}
}

func TestUpdateMailWithoutMasterKeyFails(t *testing.T) {
	svc, _ := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	// 显式去掉主密钥：模拟"部署时忘了设环境变量"。
	svc.SetSealer(nil)

	err := svc.UpdateMail(conf.Mail{SecretKey: "would-be-plaintext", FromAddress: "a@example.com"})
	if err == nil {
		t.Fatal("没有主密钥时写密钥必须失败，不能悄悄存明文")
	}
	if !strings.Contains(err.Error(), "BLOGX_CONFIG_SECRET_KEY") {
		t.Fatalf("错误信息要指出缺哪个环境变量，实际: %v", err)
	}
}

func TestReloadDecryptsSealedSecret(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	const plain = "round-trip-secret"
	if err := svc.UpdateMail(conf.Mail{SecretKey: plain, FromAddress: "a@example.com"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	// 模拟进程重启：新服务实例从同一个库加载，必须能解回明文。
	reloaded := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)
	reloaded.SetSealer(testSealer(t))
	if err := reloaded.InitRuntimeConfig(); err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if got := reloaded.GetRuntimeMail().SecretKey; got != plain {
		t.Fatalf("重启后应解回明文，实际 %q", got)
	}
}

func TestLegacyPlaintextStillReadableAfterUpgrade(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 直接写一行历史形态的明文（升级前的数据），模拟线上现状。
	legacy := conf.Mail{SecretKey: "legacy-plain", FromAddress: "old@example.com"}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("构造历史数据失败: %v", err)
	}
	if err := upsertPayload(db, string(SectionMail), string(raw)); err != nil {
		t.Fatalf("写历史数据失败: %v", err)
	}

	reloaded := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)
	reloaded.SetSealer(testSealer(t))
	if err := reloaded.InitRuntimeConfig(); err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	// 升级当天必须还能发信：历史明文原样可读。
	if got := reloaded.GetRuntimeMail().SecretKey; got != "legacy-plain" {
		t.Fatalf("历史明文应可读，实际 %q", got)
	}
}
