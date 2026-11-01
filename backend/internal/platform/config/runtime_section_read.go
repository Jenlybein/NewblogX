package config

import (
	"context"
	"strings"

	"myblogx/internal/apperr"

	"gorm.io/gorm"
)

// ReadSection 只读地读出一段运行时配置：不落库、也不补种缺失的段。
//
// 为什么不能直接用 RuntimeConfigService：
//   - 它的缓存只在启动时加载、后台更新时刷新。一次性运维命令（CLI 子命令）
//     没有"启动加载"这一步，拿不到当前值；
//   - 它初始化时会给库里没有的段补一行默认值（见 seedMissing），那是写操作。
//     一条"只读"命令顺带往 runtime_config_models 写东西，是说不清的行为。
//
// 段不存在、整段被软删、或 payload 为空时，整段回落到 defaults；
// JSON 里缺哪个字段就用 defaults 补哪个 —— 与 decodeSection 的合并语义一致，
// 所以新增配置字段不需要数据迁移。
func ReadSection[T any](ctx context.Context, db *gorm.DB, name string, defaults T) (T, error) {
	if db == nil {
		return defaults, apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	var rows []runtimeConfigRow
	if err := db.WithContext(ctx).Where("name = ?", name).Limit(1).Find(&rows).Error; err != nil {
		return defaults, err
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].Payload) == "" {
		return defaults, nil
	}
	// 默认值先占位，payload 里出现的字段覆盖上去。
	out, err := decodeSection(name, defaults, map[string]string{name: rows[0].Payload})
	if err != nil {
		return defaults, err
	}
	return out, nil
}
