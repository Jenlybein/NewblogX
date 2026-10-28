//go:build integration

package schema_test

import (
	"testing"

	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// projectionRows 是所有**平台投影**读写的行结构。
//
// 这份清单就是 M6 的搬迁清单：row 从 models 搬到各包自持之后，
// 把这里的类型换成新类型即可，断言逻辑不用动。
//
// 为什么只列投影相关的表而不是"全部 30 个模型"：投影是唯一
// "读写列名但编译期看不出来"的地方 —— 业务模块的 row 走自己 adapter 里的
// 显式 SELECT，漏列会立刻报错；投影走 map[string]any 与 binlog 列名，
// 漏列只会静默出错。
func projectionRows() []struct {
	name string
	row  any
} {
	return []struct {
		name string
		row  any
	}{
		// cdc/imageref：图片引用重建
		{"article", schema.ArticleRow{}},
		{"image", schema.ImageRow{}},
		{"image_ref", schema.ImageRefRow{}},
		{"banner", schema.BannerRow{}},
		{"favorite", schema.FavoriteRow{}},

		// readmodel：用户展示信息快照同步（更新的全是冗余列）
		{"comment", schema.CommentRow{}},
		{"user_follow", schema.UserFollowRow{}},
		{"user_article_favor", schema.UserArticleFavorRow{}},
		{"chat_session", schema.ChatSessionRow{}},

		// es/articlesync：构建 ES 文档
		{"tag", schema.TagRow{}},
		{"article_tag", schema.ArticleTagRow{}},
		{"user_top_article", schema.UserTopArticleRow{}},

		// cdc/deadletter 与 outbox 中继
		{"cdc_dead_letter", schema.DeadLetterRow{}},
		{"outbox_event", schema.OutboxRow{}},

		// 站点运行时配置
		{"runtime_config", schema.RuntimeConfigRow{}},
	}
}

// TestProjectionRowsMatchDatabase 锁定"投影行结构 == 真实表列"。
//
// 这是 M6 搬迁 row 的安全网：把结构体搬到别的包、改名、拆文件都不会触发它，
// 但只要**少一个字段或改一个字段名**，这里立刻红。
//
// 之所以值得一条独立测试：M5 收尾在 chat 里连踩三个"测试全绿但线上必坏"的坑
// （PostgreSQL 的 excluded 语法、主键恒为 0、未读清零依赖 map 顺序），
// 全是编译通过 + 逻辑测试通过的静默错误。列名漂移是同一类。
func TestProjectionRowsMatchDatabase(t *testing.T) {
	db := mysqlcontainer.DB(t)

	schema.Migrate(t, db,
		schema.UserCore, schema.UserSession, schema.ArticleCore,
		schema.ArticleReview, schema.Favorite, schema.UserReading,
		schema.Comment, schema.Chat, schema.Outbox,
		schema.ImageAssets, schema.Site, schema.Social, schema.DeadLetter,
	)

	for _, c := range projectionRows() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			schema.AssertColumns(t, db, c.row)
		})
	}
}

// TestColumnsDiffCatchesDrift 证明列名契约真的会红。
//
// 门禁的教训：一个永远通过的断言等于没有。
// 这里造两种漂移，分别对应"结构体多列"与"结构体漏字段"。
// 直接测 ColumnsDiff（纯函数）而不是 AssertColumns，
// 是因为后者的失败会把父测试一起标红，没法在同一测试里既造错又断言。
func TestColumnsDiffCatchesDrift(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.ImageAssets)

	// 正常结构体：必须先确认它对得上，否则下面的"能报错"毫无意义。
	t.Run("字段对得上时不报差异", func(t *testing.T) {
		missing, extra, err := schema.ColumnsDiff(db, schema.ImageRow{})
		if err != nil {
			t.Fatalf("比对失败: %v", err)
		}
		if len(missing) != 0 || len(extra) != 0 {
			t.Fatalf("image_models 应当一致，实际 missing=%v extra=%v", missing, extra)
		}
	})

	t.Run("结构体多一列会被报出来", func(t *testing.T) {
		missing, _, err := schema.ColumnsDiff(db, rowWithGhost{})
		if err != nil {
			t.Fatalf("比对失败: %v", err)
		}
		if len(missing) == 0 {
			t.Fatal("结构体多出来的列应当被报成 missing，但没报 —— 断言失效")
		}
	})

	t.Run("结构体漏字段会被报出来", func(t *testing.T) {
		_, extra, err := schema.ColumnsDiff(db, rowTooNarrow{})
		if err != nil {
			t.Fatalf("比对失败: %v", err)
		}
		if len(extra) == 0 {
			t.Fatal("表里有、结构体没映射的列应当被报成 extra，但没报 —— 断言失效")
		}
	})
}

// 下面两个结构体故意与 image_models 对不上，用于验证契约本身有效。
//
// 必须定义在包级并显式指定 TableName：局部类型会被推导成
// row_with_ghosts 这种表名，比对直接报"表不存在"，
// 测出来的就不是"能不能发现漂移"了。

// rowWithGhost 比真实表多一个字段（模拟改错字段名 / 凭空捏造列）。
type rowWithGhost struct {
	ID          uint64
	ObjectKey   string
	GhostColumn string
}

// TableName 指向真实表。
func (rowWithGhost) TableName() string { return "image_models" }

// rowTooNarrow 只有一列（模拟搬迁时漏了一大片字段）。
type rowTooNarrow struct {
	ID uint64
}

// TableName 指向真实表。
func (rowTooNarrow) TableName() string { return "image_models" }
