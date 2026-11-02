package config

import (
	"context"
	"testing"

	"myblogx/conf"

	"gorm.io/gorm"
)

// 本文件钉住 ReadSection 的两条承诺：缺段时回落到默认值，且**不写库**。
// 一次性运维命令靠它读运行时配置；它顺带写一行配置就等于"只读命令也不干净"。

func TestReadSectionFallsBackToDefaultsWhenSectionMissing(t *testing.T) {
	_, db := newTestService(t, conf.RuntimeDefaults{})

	defaults := conf.ImageGC{GraceDays: 14, BatchLimit: 50}
	got, err := ReadSection(context.Background(), db, SectionImageGC, defaults)
	if err != nil {
		t.Fatalf("读配置段失败: %v", err)
	}
	if got != defaults {
		t.Fatalf("缺段时应整段回落默认值: want=%+v got=%+v", defaults, got)
	}

	// 关键：读一次不能凭空补一行配置出来。
	var count int64
	if err := db.Model(&runtimeConfigRow{}).Where("name = ?", SectionImageGC).Count(&count).Error; err != nil {
		t.Fatalf("统计配置行失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("ReadSection 不该补种配置段，实际写了 %d 行", count)
	}
}

func TestReadSectionMergesPayloadOverDefaults(t *testing.T) {
	_, db := newTestService(t, conf.RuntimeDefaults{})

	if err := db.Create(&runtimeConfigRow{
		Name:    SectionImageGC,
		Payload: `{"grace_days":3}`,
	}).Error; err != nil {
		t.Fatalf("写入配置段失败: %v", err)
	}

	got, err := ReadSection(context.Background(), db, SectionImageGC, conf.ImageGC{GraceDays: 14, BatchLimit: 50})
	if err != nil {
		t.Fatalf("读配置段失败: %v", err)
	}
	if got.GraceDays != 3 {
		t.Fatalf("库里的值应覆盖默认值: grace_days=%d", got.GraceDays)
	}
	if got.BatchLimit != 50 {
		t.Fatalf("库里没有的字段应保留默认值: batch_limit=%d", got.BatchLimit)
	}
}

func TestReadSectionIgnoresSoftDeletedSection(t *testing.T) {
	_, db := newTestService(t, conf.RuntimeDefaults{})

	row := runtimeConfigRow{Name: SectionImageGC, Payload: `{"grace_days":1}`}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("写入配置段失败: %v", err)
	}
	if err := db.Delete(&row).Error; err != nil {
		t.Fatalf("软删配置段失败: %v", err)
	}

	got, err := ReadSection(context.Background(), db, SectionImageGC, conf.ImageGC{GraceDays: 14, BatchLimit: 50})
	if err != nil {
		t.Fatalf("读配置段失败: %v", err)
	}
	if got.GraceDays != 14 {
		t.Fatalf("软删的段应视作不存在: grace_days=%d", got.GraceDays)
	}
}

func TestReadSectionRequiresDB(t *testing.T) {
	var db *gorm.DB
	if _, err := ReadSection(context.Background(), db, SectionImageGC, conf.ImageGC{}); err == nil {
		t.Fatal("数据库未初始化时应报错，而不是悄悄给默认值")
	}
}
