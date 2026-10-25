package cli

import (
	"fmt"

	mysqlddl "myblogx/internal/platform/mysql/ddl"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// FlagDB 建出全部表。
//
// 执行的是 internal/platform/mysql/ddl 里的 schema.sql，不再用 AutoMigrate：
//
//   - 表结构是冻结的契约（零列变更口径），该是可 review 的文本，
//     而不是"当前 Go 结构长什么样"的推导结果；
//   - AutoMigrate 会为了对齐结构主动 ALTER 既有表，
//     这在零列变更口径下是明确禁止的；
//   - 它还会建出建模副产物，例如一条物理上无法满足的外键
//     （见 docs/architecture/baseline.md §35）。
//
// 语句都是 CREATE TABLE IF NOT EXISTS，可重复执行。
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

	if logger != nil {
		logger.Info("数据库迁移成功")
	}
	return nil
}
