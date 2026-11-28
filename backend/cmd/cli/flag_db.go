package cli

import (
	"fmt"

	"myblogx/conf"
	articlemysql "myblogx/internal/article/adapter/mysql"
	"myblogx/internal/platform/config"
	mysqlddl "myblogx/internal/platform/mysql/ddl"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// FlagDB 建出全部表，并补齐既有表缺失的列。
//
// 表结构执行的是 internal/platform/mysql/ddl 里的 schema.sql，不用 AutoMigrate：
//
//   - 表结构是冻结的契约，该是可 review 的文本，
//     而不是"当前 Go 结构长什么样"的推导结果；
//   - AutoMigrate 会为了对齐结构主动改既有表（改名 / 改类型 / 删列），
//     这些会让 CDC 按列名解析静默错位，仍然明确禁止；
//   - 它还会建出建模副产物，例如一条物理上无法满足的外键
//     （v1 基线留档）。
//
// 需要给既有表**补列**时不再靠人工敲 ALTER：在 ddl 包的 columnAdditions
// 登记一条，本命令会幂等地补齐（列已存在则跳过）。仍只做加法。
func FlagDB(db *gorm.DB, logger *logrus.Logger) error {
	if db == nil {
		return fmt.Errorf("数据库迁移失败: 数据库未初始化")
	}

	if logger != nil {
		logger.Info("数据库开始迁移")
	}

	if err := mysqlddl.Apply(db); err != nil {
		if logger != nil {
			logger.Error("数据库迁移失败: ", err)
		}
		return err
	}

	// 建表之后补一次冗余计数回填（幂等、只写数据不改结构）。
	// 与建表同批执行的原因：`article_count` 曾是"从不更新的列"，
	// 不做这一遍历史夹子的「N 篇」会一直是假值（#344）。
	if err := articlemysql.BackfillFavoriteCounts(db); err != nil {
		if logger != nil {
			logger.Error("收藏夹计数回填失败: ", err)
		}
		return err
	}

	// 「文章审核设置」的数据迁移（2026-11-28 / ADR-0037）：删 quality_auto_release、
	// 补 score_enabled / ai_compliance_enabled / ai_failure_action、旧值 mask → release。
	//
	// 放在 -db 里而不是只给一条手动命令：这是**新代码配旧数据**最容易出事的一批键 ——
	// 结构体零值恰好都是"关"，缺键会静默把线上行为翻成"评分关闭 + 落点未知"。
	// 三个改动全部幂等，重复执行结果一致（有单测逐字验证）。
	//
	// 用不带默认值文件的服务实例：这条迁移只看库里已有的原始 payload，
	// 不该因为默认值文件缺失或改过而决定"要搬什么"。
	if db.Migrator().HasTable("runtime_config_models") {
		svc := config.NewRuntimeConfigService(conf.RuntimeDefaults{}, logger, db)
		result, err := svc.MigrateArticleReview(true)
		if err != nil {
			if logger != nil {
				logger.Error("文章审核设置迁移失败: ", err)
			}
			return err
		}
		if logger != nil && result.Changed() {
			logger.Infof("文章审核设置迁移完成: site=%v content_filter=%v",
				result.SiteChanged, result.ContentFilterChanged)
		}
	}

	if logger != nil {
		logger.Info("数据库迁移成功")
	}
	return nil
}
