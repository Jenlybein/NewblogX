package domain_test

import (
	"errors"
	"myblogx/internal/kernel/ctype"
	"strings"
	"testing"

	"myblogx/internal/article/domain"
)

const (
	authorID   = ctype.ID("1001")
	strangerID = ctype.ID("2002")
	adminID    = ctype.ID("9001")
)

// newArticle 造一个测试用文章（走 Restore，绕开创建校验以便构造任意状态）。
func newArticle(t *testing.T, status domain.Status, visibility domain.Visibility, deleted bool) *domain.Article {
	t.Helper()
	return domain.Restore(domain.RestoreParams{
		ID:         ctype.NewID(1),
		AuthorID:   authorID,
		Title:      "测试文章",
		Content:    "正文",
		Status:     status,
		Visibility: visibility,
		Deleted:    deleted,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	})
}

// articleInStatus 是 newArticle 的"不需要 *testing.T"版本，
// 供权限矩阵在表格里预置各状态的文章。
func articleInStatus(status domain.Status) *domain.Article {
	return domain.Restore(domain.RestoreParams{
		ID:         ctype.NewID(1),
		AuthorID:   authorID,
		Title:      "测试文章",
		Content:    "正文",
		Status:     status,
		Visibility: domain.VisibilityVisible,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	})
}

// TestArticleVisibility 是可见性不变量矩阵：状态 × 角色穷举。
//
// 期望值全部来自改造前的 if 分支（article_detail.go 第 70–78 行），
// 不是"看起来合理"的猜测。
func TestArticleVisibility(t *testing.T) {
	t.Parallel()

	author := domain.NormalUser(authorID)
	stranger := domain.NormalUser(strangerID)
	admin := domain.AdminUser(adminID)
	anon := domain.Anonymous()

	cases := []struct {
		name       string
		status     domain.Status
		visibility domain.Visibility
		deleted    bool
		viewer     domain.Viewer
		want       bool
	}{
		// 草稿：只有作者与管理员
		{"draft/author", domain.StatusDraft, domain.VisibilityVisible, false, author, true},
		{"draft/stranger", domain.StatusDraft, domain.VisibilityVisible, false, stranger, false},
		{"draft/admin", domain.StatusDraft, domain.VisibilityVisible, false, admin, true},
		{"draft/anonymous", domain.StatusDraft, domain.VisibilityVisible, false, anon, false},

		// 审核中：同上
		{"examining/author", domain.StatusExamining, domain.VisibilityVisible, false, author, true},
		{"examining/stranger", domain.StatusExamining, domain.VisibilityVisible, false, stranger, false},
		{"examining/admin", domain.StatusExamining, domain.VisibilityVisible, false, admin, true},
		{"examining/anonymous", domain.StatusExamining, domain.VisibilityVisible, false, anon, false},

		// 已发布 + visible：所有人都能看
		{"published/author", domain.StatusPublished, domain.VisibilityVisible, false, author, true},
		{"published/stranger", domain.StatusPublished, domain.VisibilityVisible, false, stranger, true},
		{"published/admin", domain.StatusPublished, domain.VisibilityVisible, false, admin, true},
		{"published/anonymous", domain.StatusPublished, domain.VisibilityVisible, false, anon, true},

		// 已拒绝：只有作者与管理员
		{"rejected/author", domain.StatusRejected, domain.VisibilityVisible, false, author, true},
		{"rejected/stranger", domain.StatusRejected, domain.VisibilityVisible, false, stranger, false},
		{"rejected/admin", domain.StatusRejected, domain.VisibilityVisible, false, admin, true},
		{"rejected/anonymous", domain.StatusRejected, domain.VisibilityVisible, false, anon, false},

		// 已删除：谁都看不到（软删后旧实现连 Take 都查不到）
		{"deleted/author", domain.StatusPublished, domain.VisibilityVisible, true, author, false},
		{"deleted/stranger", domain.StatusPublished, domain.VisibilityVisible, true, stranger, false},
		{"deleted/admin", domain.StatusPublished, domain.VisibilityVisible, true, admin, false},
		{"deleted/anonymous", domain.StatusPublished, domain.VisibilityVisible, true, anon, false},

		// 已发布但被作者隐藏：路人不可见，作者与管理员可见
		{"user_hidden/author", domain.StatusPublished, domain.VisibilityUserHidden, false, author, true},
		{"user_hidden/stranger", domain.StatusPublished, domain.VisibilityUserHidden, false, stranger, false},
		{"user_hidden/admin", domain.StatusPublished, domain.VisibilityUserHidden, false, admin, true},
		{"user_hidden/anonymous", domain.StatusPublished, domain.VisibilityUserHidden, false, anon, false},

		// 被管理员隐藏：同上，但这是管理员操作的结果
		{"admin_hidden/author", domain.StatusPublished, domain.VisibilityAdminHidden, false, author, true},
		{"admin_hidden/stranger", domain.StatusPublished, domain.VisibilityAdminHidden, false, stranger, false},
		{"admin_hidden/admin", domain.StatusPublished, domain.VisibilityAdminHidden, false, admin, true},
		{"admin_hidden/anonymous", domain.StatusPublished, domain.VisibilityAdminHidden, false, anon, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := newArticle(t, c.status, c.visibility, c.deleted)
			if got := domain.CanBeViewedBy(a, c.viewer); got != c.want {
				t.Fatalf("CanBeViewedBy = %v，期望 %v（状态=%s 可见性=%s 已删=%v）",
					got, c.want, c.status, c.visibility, c.deleted)
			}
		})
	}
}

func TestEditAndDeletePermission(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)

	cases := []struct {
		name      string
		viewer    domain.Viewer
		canEdit   bool
		canDelete bool
	}{
		{"author", domain.NormalUser(authorID), true, true},
		{"stranger", domain.NormalUser(strangerID), false, false},
		{"admin", domain.AdminUser(adminID), false, true},
		{"anonymous", domain.Anonymous(), false, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.CanBeEditedBy(a, c.viewer); got != c.canEdit {
				t.Fatalf("CanBeEditedBy = %v，期望 %v", got, c.canEdit)
			}
			if got := domain.CanBeDeletedBy(a, c.viewer); got != c.canDelete {
				t.Fatalf("CanBeDeletedBy = %v，期望 %v", got, c.canDelete)
			}
		})
	}
}

func TestExaminePermission(t *testing.T) {
	t.Parallel()

	examining := newArticle(t, domain.StatusExamining, domain.VisibilityVisible, false)
	draft := newArticle(t, domain.StatusDraft, domain.VisibilityVisible, false)

	if !domain.CanBeExaminedBy(examining, domain.AdminUser(adminID)) {
		t.Fatal("管理员应能审核审核中的文章")
	}
	if domain.CanBeExaminedBy(draft, domain.AdminUser(adminID)) {
		t.Fatal("草稿不该进入审核动作")
	}
	if domain.CanBeExaminedBy(examining, domain.NormalUser(authorID)) {
		t.Fatal("普通用户不能审核")
	}
}

func TestSetVisibilityPermission(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)

	if !domain.CanSetVisibility(a, domain.NormalUser(authorID), domain.VisibilityUserHidden) {
		t.Fatal("作者应能隐藏自己的文章")
	}
	if domain.CanSetVisibility(a, domain.NormalUser(authorID), domain.VisibilityAdminHidden) {
		t.Fatal("作者不得自行设置管理员隐藏（否则能撤销管理员的处置）")
	}
	if !domain.CanSetVisibility(a, domain.AdminUser(adminID), domain.VisibilityAdminHidden) {
		t.Fatal("管理员应能设置管理员隐藏")
	}
	if domain.CanSetVisibility(a, domain.NormalUser(strangerID), domain.VisibilityUserHidden) {
		t.Fatal("他人不能改可见性")
	}
}

func TestFavoriteAndTopPermission(t *testing.T) {
	t.Parallel()

	published := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)
	draft := newArticle(t, domain.StatusDraft, domain.VisibilityVisible, false)

	if domain.CanBeFavorited(published, domain.NormalUser(authorID)) {
		t.Fatal("不能收藏自己的文章")
	}
	if !domain.CanBeFavorited(published, domain.NormalUser(strangerID)) {
		t.Fatal("他人应能收藏可见文章")
	}
	if domain.CanBeFavorited(draft, domain.NormalUser(strangerID)) {
		t.Fatal("不可见文章不能被收藏")
	}
	if domain.CanBeFavorited(published, domain.Anonymous()) {
		t.Fatal("匿名不能收藏")
	}

	if !domain.CanBeTopped(published, domain.NormalUser(authorID)) {
		t.Fatal("作者应能置顶自己的文章")
	}
	if domain.CanBeTopped(published, domain.NormalUser(strangerID)) {
		t.Fatal("不能置顶他人文章")
	}
}

func TestToggleCommentsPermission(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)
	if !domain.CanToggleComments(a, domain.NormalUser(authorID)) {
		t.Fatal("作者应能开关评论")
	}
	if domain.CanToggleComments(a, domain.NormalUser(strangerID)) {
		t.Fatal("他人不能开关评论")
	}
}

// TestCanBeSeenByReader 是「显示非管理员发布文章」开关的读面规则矩阵。
//
// ADR-0017 决策 4：开关关闭（onlyAdminAuthors 为真）时，作者不是管理员的文章
// 在公开读面一律读不出来（详情按"不存在"处理）。两处豁免：管理员作者的文章，
// 以及作者本人（studio 用同一个详情接口取正文）。
// 开关状态由 adapter 从站点运行时配置现读后传入，domain 不读配置，所以这里
// 只穷举 (开关, 作者是否管理员, 读者是否作者本人) 三个维度。
func TestCanBeSeenByReader(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		onlyAdminAuthors bool
		authorIsAdmin    bool
		isAuthor         bool
		want             bool
	}{
		{name: "开关打开×管理员作者×路人", onlyAdminAuthors: false, authorIsAdmin: true, want: true},
		{name: "开关打开×普通作者×路人", onlyAdminAuthors: false, want: true},
		{name: "开关关闭×管理员作者×路人", onlyAdminAuthors: true, authorIsAdmin: true, want: true},
		{name: "开关关闭×普通作者×路人", onlyAdminAuthors: true, want: false},
		{name: "开关关闭×普通作者×作者本人", onlyAdminAuthors: true, isAuthor: true, want: true},
		{name: "开关打开×普通作者×作者本人", onlyAdminAuthors: false, isAuthor: true, want: true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.CanBeSeenByReader(c.onlyAdminAuthors, c.authorIsAdmin, c.isAuthor); got != c.want {
				t.Fatalf("CanBeSeenByReader(%v, %v, %v) = %v, want %v",
					c.onlyAdminAuthors, c.authorIsAdmin, c.isAuthor, got, c.want)
			}
		})
	}
}

// ── 写作开关（#64）──

// writeAction 是权限矩阵里的一行"写动作"，needs 说明它归哪个开关管。
type writeAction struct {
	name string
	// needs 是判定该动作所需的开关："publish" 或 "draft"。
	needs string
	check func(domain.WritePolicy) error
}

const (
	needsPublish = "publish"
	needsDraft   = "draft"
)

// writeActionsFor 列出「发布与展示」两个开关作用到的全部写动作。
//
// 七条动作对应 ADR-0017 决策 2 与 #64 验收标准的原文：
//   - 创建即发布 / 提交审核 / 发布自己的草稿 / 更新已发布文章 → 发布开关；
//   - 新建草稿 / 保存草稿（显式退草稿也走同一判定）→ 草稿开关；
//   - 审核中的文章被编辑会重新送审，所以它同样归发布开关。
func writeActionsFor(v domain.Viewer) []writeAction {
	draft := articleInStatus(domain.StatusDraft)
	published := articleInStatus(domain.StatusPublished)
	examining := articleInStatus(domain.StatusExamining)
	draftTarget := domain.StatusDraft
	submitTarget := domain.StatusExamining

	return []writeAction{
		{"create_draft", needsDraft, func(p domain.WritePolicy) error {
			return domain.CheckWriteOnCreate(v, domain.StatusDraft, p)
		}},
		{"create_submit", needsPublish, func(p domain.WritePolicy) error {
			return domain.CheckWriteOnCreate(v, domain.StatusExamining, p)
		}},
		{"edit_draft_content", needsDraft, func(p domain.WritePolicy) error {
			return domain.CheckWriteOnUpdate(draft, v, nil, p)
		}},
		{"save_draft_explicit", needsDraft, func(p domain.WritePolicy) error {
			return domain.CheckWriteOnUpdate(draft, v, &draftTarget, p)
		}},
		{"publish_draft", needsPublish, func(p domain.WritePolicy) error {
			return domain.CheckWriteOnUpdate(draft, v, &submitTarget, p)
		}},
		{"edit_published_content", needsPublish, func(p domain.WritePolicy) error {
			return domain.CheckWriteOnUpdate(published, v, nil, p)
		}},
		{"edit_examining_content", needsPublish, func(p domain.WritePolicy) error {
			return domain.CheckWriteOnUpdate(examining, v, nil, p)
		}},
	}
}

// TestCheckWriteOnCreateFailsClosedOnUnsupportedTarget 未知/不可建文的 target 一律拒绝。
//
// 这条钉的是"失败关闭"：早先 CheckWriteOnCreate 的 switch 没有 default，
// 既不是草稿也不是审核中的 target 会被静默放行——一旦上游多出第三种建文
// 目标（比如「直接发布」），站点开关就被绕过了。管理员也要拒绝：非法
// target 不属于"管理员豁免"的范围，它是一次非法迁移。
func TestCheckWriteOnCreateFailsClosedOnUnsupportedTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		viewer domain.Viewer
		target domain.Status
	}{
		{"管理员/已发布", domain.AdminUser(adminID), domain.StatusPublished},
		{"管理员/零值", domain.AdminUser(adminID), domain.Status(0)},
		{"普通用户/已发布", domain.NormalUser(authorID), domain.StatusPublished},
		{"普通用户/已拒绝", domain.NormalUser(authorID), domain.StatusRejected},
		{"普通用户/零值", domain.NormalUser(authorID), domain.Status(0)},
		{"普通用户/越界", domain.NormalUser(authorID), domain.Status(99)},
		{"受限访客/已发布", domain.Viewer{UserID: authorID, Role: domain.RoleGuest}, domain.StatusPublished},
	}

	policies := []domain.WritePolicy{
		domain.OpenWritePolicy(),
		{AllowNonAdminPublish: false, AllowNonAdminDraft: false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, p := range policies {
				// 开关怎么配都不该改变结论：target 本身非法。
				if err := domain.CheckWriteOnCreate(c.viewer, c.target, p); !errors.Is(err, domain.ErrInvalidTransition) {
					t.Fatalf("target=%v 应返回 ErrInvalidTransition，实际 %v（策略=%+v）", c.target, err, p)
				}
			}
		})
	}
}

// TestWritePolicyMatrix 是「发布与展示」两个写作开关的权限矩阵：
// 四种开关组合 × 七种写动作 × 四类操作者（管理员 / 普通用户 / 受限访客 / role=0）。
//
// 期望值来自 ADR-0017 决策 2（开关语义）与决策 3（guest 视同未登录），
// 不是"看起来合理"的猜测：
//   - 管理员（1）永远不受三个开关限制；
//   - 普通用户（2）按动作所属的开关判定，关了就返回带站点策略语义的错误码；
//   - 受限访客（3）与库里未入枚举的 role=0 一律拒绝：它们不是"非管理员作者"，
//     鉴权层已按匿名处理，这里再兜一层，避免"开关全开"时被借道写进去。
func TestWritePolicyMatrix(t *testing.T) {
	t.Parallel()

	combos := []struct {
		name         string
		allowPublish bool
		allowDraft   bool
	}{
		{"both_on", true, true},
		{"publish_off", false, true},
		{"draft_off", true, false},
		{"both_off", false, false},
	}

	viewers := []struct {
		name string
		// viewer 是本次矩阵里的操作者。
		viewer domain.Viewer
		// writable 为假表示无论开关怎么配都不允许写（受限访客 / 未知角色）。
		writable bool
	}{
		{"admin", domain.AdminUser(adminID), true},
		{"normal_user", domain.NormalUser(authorID), true},
		{"guest", domain.Viewer{UserID: authorID, Role: domain.RoleGuest}, false},
		{"role_zero", domain.Viewer{UserID: authorID, Role: domain.Role(0)}, false},
	}

	for _, combo := range combos {
		combo := combo
		policy := domain.WritePolicy{
			AllowNonAdminPublish: combo.allowPublish,
			AllowNonAdminDraft:   combo.allowDraft,
		}

		for _, actor := range viewers {
			actor := actor

			for _, action := range writeActionsFor(actor.viewer) {
				action := action
				name := combo.name + "/" + actor.name + "/" + action.name

				t.Run(name, func(t *testing.T) {
					t.Parallel()

					err := action.check(policy)

					switch {
					case !actor.writable:
						// 受限访客与未知角色：四种组合下一律拒绝。
						if !errors.Is(err, domain.ErrForbidden) {
							t.Fatalf("%s 不应能写，期望 ErrForbidden，实际 %v", actor.name, err)
						}
					case actor.viewer.IsAdmin():
						// 管理员不受开关影响。
						if err != nil {
							t.Fatalf("管理员不应被开关拦住，实际 %v", err)
						}
					default:
						// 普通用户：只看该动作所属的那个开关。
						allowed, wantErr := false, error(nil)
						switch action.needs {
						case needsPublish:
							allowed, wantErr = combo.allowPublish, domain.ErrPublishDisabledBySite
						case needsDraft:
							allowed, wantErr = combo.allowDraft, domain.ErrDraftDisabledBySite
						default:
							t.Fatalf("动作 %s 没声明所属开关", action.name)
						}
						if allowed {
							if err != nil {
								t.Fatalf("开关已开，%s 应被允许，实际 %v", action.name, err)
							}
							return
						}
						if !errors.Is(err, wantErr) {
							t.Fatalf("开关已关，%s 应返回 %v，实际 %v", action.name, wantErr, err)
						}
						// 错误文案必须说清是站点策略，不是账号问题。
						if !strings.Contains(err.Error(), "站点") {
							t.Fatalf("错误文案应说明是站点策略，实际 %q", err.Error())
						}
					}
				})
			}
		}
	}
}
