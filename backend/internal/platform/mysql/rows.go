package mysql

import (
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
)

// BaseRow 是所有行结构的公共列（主键 + 三个时间列）。
//
// 为什么放在平台层而不是各包各写一份：主键是雪花值、由代码生成，
// 数据库没有默认值。漏了 BeforeCreate 钩子，Create 会报
// "Field 'id' doesn't have a default value" —— 这是个纯粹的技术约定，
// 不含任何业务语义，所以集中一次比复制十次好。
//
// 必须导出：GORM 用反射给字段赋值，未导出的内嵌结构体它写不进去，
// 结果是主键与时间列静默变成零值。
type BaseRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// BeforeCreate 生成雪花主键。
//
// 只在主键为空时生成，让调用方能显式指定 ID（测试要可复现的 ID）。
func (b *BaseRow) BeforeCreate(*gorm.DB) error {
	if !b.ID.IsZero() {
		return nil
	}
	next, err := idgen.Next()
	if err != nil {
		return err
	}
	b.ID = next
	return nil
}
