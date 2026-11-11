package cli

import (
	"fmt"

	"myblogx/conf"
	"myblogx/internal/platform/config"
	"myblogx/internal/platform/secretbox"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// FlagEncryptConfig 把库里已存的明文密钥一次性加密（票 359）。
//
// 为什么需要一次性迁移：加密落库只对**新写入**生效，历史行还是明文。
// 只加解密逻辑而不迁移，等于把"库里没有明文"这件事永远推到"下次管理员改配置"，
// 而很多部署根本不会再改一次。
//
// 幂等：已经有 `enc:v1:` 前缀的值原样跳过，可以重复执行（部署脚本不必判断状态）。
//
// 只碰三个敏感字段，其余配置不动；任何一段解析失败都中止并报错，
// 不做"跳过坏数据继续跑"——半迁移的状态最难排查。
func FlagEncryptConfig(db *gorm.DB, logger *logrus.Logger) error {
	if db == nil {
		return fmt.Errorf("加密配置失败: 数据库未初始化")
	}
	box, err := secretbox.FromEnv()
	if err != nil {
		return fmt.Errorf("加密配置失败: %w", err)
	}
	if box == nil {
		return fmt.Errorf("加密配置失败: 未设置环境变量 %s（需要 32 字节密钥，可用 openssl rand -base64 32 生成）", secretbox.EnvKey)
	}

	svc := config.NewRuntimeConfigService(conf.RuntimeDefaults{}, logger, db)
	changed, err := svc.EncryptStoredSecrets(box)
	if err != nil {
		return err
	}
	if logger != nil {
		if changed == 0 {
			logger.Info("配置密钥已全部加密（无改动）")
		} else {
			logger.Infof("配置密钥加密完成，共更新 %d 段", changed)
		}
	}
	return nil
}
