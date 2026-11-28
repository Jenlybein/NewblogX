package app

import (
	"testing"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"github.com/sirupsen/logrus"
)

// 2026-12-01：评论有**自己的**一档处置（`content_filter.auto_dispose.comment_action`）。
//
// 之前评论跟着 `block_action` 走：运营想让文章"命中即拒"，评论区就被连坐 ——
// 可评论没有草稿态、也没有申诉通道（票 400 之后更没有待审队列），一条推广话术
// 整条不发，代价远大于打星。现在两者解耦，装配层交给评论服务的就是 comment_action。
//
// 这一组用例从**装配函数**入手（不是直接塞字符串）：它验的是"读的是哪一段配置、
// 缺键/旧值怎么映射"，正是解耦最容易漂的一步。
func newCommentDispositionInfra(t *testing.T, dispose string) *Infra {
	t.Helper()

	db := testutil.SetupSQLite(t, &schema.RuntimeConfigRow{})
	// 运行时配置按段落库：这里直接写 content_filter 段的 payload，
	// 等价于"运营在后台保存过一次命中处理"。
	payload := `{"auto_dispose":` + dispose + `}`
	if err := db.Create(&schema.RuntimeConfigRow{
		BaseRow: schema.BaseRow{ID: ctype.NewID(1)},
		Name:    "content_filter",
		Payload: payload,
	}).Error; err != nil {
		t.Fatalf("写入 content_filter 段失败: %v", err)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	runtimeSite := config.NewRuntimeConfigService(conf.RuntimeDefaults{}, logger, db)
	// 运行时配置是"启动时把各段读进内存、之后每次现读快照"，
	// 所以这里要先跑一遍加载，否则读到的永远是零值默认。
	if err := runtimeSite.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化运行时配置失败: %v", err)
	}

	return &Infra{RuntimeSite: runtimeSite}
}

// TestCommentDispositionDoesNotFollowBlockAction 钉住"两者互不影响"。
func TestCommentDispositionDoesNotFollowBlockAction(t *testing.T) {
	// 文章侧命中即拒、评论侧仍打星照发。
	infra := newCommentDispositionInfra(t, `{"block_action":"reject","watch_action":"release","comment_action":"release"}`)

	if got := commentHitDisposition(infra)(); got != conf.ContentFilterActionRelease {
		t.Fatalf("block_action=reject 不该影响评论（评论读 comment_action），实际 %q", got)
	}
}

// TestCommentDispositionUsesOwnReject 评论侧单独配 reject 时生效。
func TestCommentDispositionUsesOwnReject(t *testing.T) {
	infra := newCommentDispositionInfra(t, `{"block_action":"release","watch_action":"release","comment_action":"reject"}`)

	if got := commentHitDisposition(infra)(); got != conf.ContentFilterActionReject {
		t.Fatalf("comment_action=reject 应被读到，实际 %q", got)
	}
}

// TestCommentDispositionFallsBackToReleaseWhenMissing 缺键必须是放行。
//
// 这是发版安全阀：这一栏是本次才加的，缺键那一刻若按"其余一律拒收"处理，
// 一次上线就能把评论区整片关掉。
func TestCommentDispositionFallsBackToReleaseWhenMissing(t *testing.T) {
	infra := newCommentDispositionInfra(t, `{"block_action":"reject","watch_action":"reject"}`)

	if got := commentHitDisposition(infra)(); got != conf.ContentFilterActionRelease {
		t.Fatalf("comment_action 缺键时应读成 release（改造前的行为），实际 %q", got)
	}
}
