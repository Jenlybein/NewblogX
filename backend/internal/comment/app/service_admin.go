package app

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 管理端评论管理的用例（第一期：列表 / 批量删除 / 禁言 / 封禁）。
//
// 三条写动作都在这里再判一次 viewer.Admin：HTTP 层的 AdminMiddleware 是第一道
// 闸，但用例层不能把"不会有人绕过路由直接调服务"当成前提 —— 关键词风险的处置
// 链路就是绕过 HTTP 直接调 CommentService.Remove 的（票 398），同一份服务被两条
// 入口复用时，权限判定必须落在离数据最近的地方。
//
// 删除复用 Remove，而不是另写一条批量 UPDATE：级联同一根下的回复、清点赞记录、
// 按已发布条数扣文章计数、清回复缓存这些副作用全在 Remove 里。后台删除必须是
// 同一份语义，否则"读者删一条评论"和"管理员删一条评论"会让计数走向两个方向。

// AdminCommentItem 是管理端评论列表的一行。
//
// 放在用例层、带 json 标签，与既有 RootItem / ReplyItem 是同一约定：swag 注解
// 直接引用 app 的具名类型，契约里就能派生出字段级类型（不内联 object{}）。
type AdminCommentItem struct {
	ID        ctype.ID `json:"id"`
	ArticleID ctype.ID `json:"article_id"`
	// ArticleTitle 由 domain.ArticleInfo 端口批量读；读不到的保持空串。
	ArticleTitle string   `json:"article_title"`
	UserID       ctype.ID `json:"user_id"`
	UserNickname string   `json:"user_nickname"`
	UserAvatar   string   `json:"user_avatar"`
	Content      string   `json:"content"`
	// Status 是对外状态：2 已发布 / 3 已删除。软删过的行（deleted_at 非空但
	// status 列仍是 2）也一律报 3，见 EffectiveStatus。
	Status int8 `json:"status"`
	// Deleted 表示这条评论是否已被删除（软删标记或状态为已删除）。
	Deleted    bool      `json:"deleted"`
	ReplyCount int       `json:"reply_count"`
	DiggCount  int       `json:"digg_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// AdminRemoveFailure 是批量删除里失败的一条。
type AdminRemoveFailure struct {
	ID ctype.ID `json:"id"`
	// Message 是失败原因的中文文案（来自错误码文案表；没有错误码的内部错误
	// 折成通用服务异常文案，不把 SQL 报错原文透给后台）。
	Message string `json:"message"`
}

// AdminRemoveManyResult 是批量删除的结果：成功条数 + 逐条失败。
//
// 不是"全成功 / 全失败"：一批评论可能分属不同文章、不同根评论家族，逐条删除
// 各自成败，跨家族回滚做不到也不该做（把已删掉的那几条再"恢复"只会留下更多
// 半截状态）。所以请求级非法在动手之前整体拒绝，通过之后逐条如实报告。
type AdminRemoveManyResult struct {
	Succeeded int                  `json:"succeeded"`
	Failed    []AdminRemoveFailure `json:"failed"`
}

// WithAccountDisposer 注入账号处置端口（禁言 / 封禁）。
//
// 用可选项而不是往 New 上加参数：账号处置端口要等 user 模块装配完才拿得到
// 那一份用例服务（见 app/comment_wiring.go），而评论服务在更早的 WireComment
// 里就建好了。没接入的调用点（单测、不服务管理端的进程角色）行为不变 ——
// 只是 AdminMute / AdminBan 会返回 comment.admin_unavailable。
func (s *Service) WithAccountDisposer(disposer domain.AccountDisposer) *Service {
	s.accounts = disposer
	return s
}

// AdminList 是管理端评论列表（含已删除行，created_at 倒序）。
//
// 只读 comment_models：用户昵称 / 头像走评论行上的快照列，文章标题走 ArticleInfo
// 端口的批量方法 —— 评论模块的适配器不与 article_models 做 JOIN，两个模块的
// 表结构因此互不牵连。
func (s *Service) AdminList(
	ctx context.Context,
	viewer domain.Viewer,
	filter domain.AdminFilter,
	page domain.AdminPage,
) ([]AdminCommentItem, bool, error) {
	if !viewer.Admin {
		return nil, false, apperr.New(apperr.CodeCommentForbidden)
	}
	filter = filter.Normalize()
	// 状态在这里再校验一次：adapter 层可能被别的入口直接调用（例如后台脚本），
	// 不能把"handler 已经 parse 过"当成类型上的保证。
	if !filter.Status.IsValid() {
		return nil, false, apperr.New(apperr.CodeCommentAdminStatusInvalid)
	}
	// 分页在用例层就收敛：默认 20 条、上限 100 是管理端口径，不该由每个仓储
	// 各自实现一遍（仓储只负责 Offset/Limit，错误值只在这里产生）。
	page = page.Normalize()

	rows, hasMore, err := s.repo.ListAdmin(ctx, filter, page)
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return []AdminCommentItem{}, hasMore, nil
	}

	// 点赞 / 回复的"当前值"有一部分还在 Redis 增量里（尚未刷回表），读者侧列表
	// 已经按同一口径合并过；管理端看到的数字必须与读者侧一致，否则运营会按
	// 一个偏小的计数判断"这评论没人看"。
	rawIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		rawIDs = append(rawIDs, row.ID.Uint64())
	}
	diggDelta, replyDelta := s.counterBatch(ctx, rawIDs)
	titles := s.articleTitles(ctx, rows)

	items := make([]AdminCommentItem, 0, len(rows))
	for _, row := range rows {
		// 对外状态用 EffectiveStatus：删除路径只写软删标记、不改 status，直接
		// 回 row.Status 会让"已删除"行显示成"已发布"（deleted=true + status=2
		// 这种自相矛盾的组合前端没法判断该信哪个）。
		state := row.EffectiveStatus()
		item := AdminCommentItem{
			ID:           row.ID,
			ArticleID:    row.ArticleID,
			ArticleTitle: titles[row.ArticleID],
			UserID:       row.UserID,
			UserNickname: row.UserNickname,
			UserAvatar:   row.UserAvatar,
			Content:      row.Content,
			Status:       int8(state),
			Deleted:      row.IsDeleted(),
			ReplyCount:   row.ReplyCount,
			DiggCount:    row.DiggCount,
			CreatedAt:    row.CreatedAt,
		}
		// 已删除行不再叠加增量：删除时缓存已经被清掉（见 Remove），再去合并
		// 一份可能迟到的旧增量就会让删除后的计数看起来还是正数。
		if !row.IsDeleted() {
			item.ReplyCount += replyDelta[row.ID.Uint64()]
			item.DiggCount += diggDelta[row.ID.Uint64()]
		}
		items = append(items, item)
	}
	return items, hasMore, nil
}

// AdminRemoveMany 批量删除评论；逐条报告结果，部分失败不影响其余条目。
//
// 每一条都走 Remove（管理员身份由调用方保证），因此级联、点赞清理、计数扣减
// 与读者侧删除完全一致。已经被删掉的 ID 记成功而不是失败：同一批里可能同时
// 选了一级评论和它下面的回复，一级评论的级联已经把回复删掉了；重复点击、
// 两个人同时操作同理。目标状态（这条评论不在了）已经达成，把它报成"失败"
// 会让运营以为要再点一次。
func (s *Service) AdminRemoveMany(
	ctx context.Context,
	viewer domain.Viewer,
	ids []ctype.ID,
) (AdminRemoveManyResult, error) {
	result := AdminRemoveManyResult{Failed: []AdminRemoveFailure{}}
	if !viewer.Admin {
		return result, apperr.New(apperr.CodeCommentForbidden)
	}
	// 请求级非法在动手之前整体拒绝：条数超限或混了零值 ID 时，一条都不删，
	// 避免"删了一半才发现参数不对"。
	if len(ids) == 0 || len(ids) > domain.MaxAdminBatchIDs {
		return result, apperr.New(apperr.CodeCommentAdminBatchInvalid)
	}
	for _, id := range ids {
		if id.IsZero() {
			return result, apperr.New(apperr.CodeCommentAdminBatchInvalid)
		}
	}

	for _, id := range ids {
		err := s.Remove(ctx, viewer, id.Uint64())
		if err == nil {
			result.Succeeded++
			continue
		}
		if apperr.IsCode(err, apperr.CodeCommentNotFound) {
			result.Succeeded++
			continue
		}
		result.Failed = append(result.Failed, AdminRemoveFailure{
			ID:      id,
			Message: removeFailureMessage(err),
		})
	}
	return result, nil
}

// AdminMute 禁言评论发布人，返回到期时间。
//
// days = 0 表示永久，交给账号处置端口换算成"没有到期日的远期时间"：禁止在
// 这一层把 0 折成 3650 天之类的数字，那会把"永久"变成一个会到期的谎话。
func (s *Service) AdminMute(
	ctx context.Context,
	viewer domain.Viewer,
	target ctype.ID,
	days int,
	reason string,
) (time.Time, error) {
	if !viewer.Admin {
		return time.Time{}, apperr.New(apperr.CodeCommentForbidden)
	}
	if target.IsZero() {
		return time.Time{}, apperr.New(apperr.CodeCommentAdminUserInvalid)
	}
	// 理由先校验再动手：这条路径不写任何数据，但顺序保持"参数先于副作用"，
	// 以后若加上"把理由发给用户"也不会有"先禁言后报参数错误"的窗口。
	if _, err := domain.NormalizeAdminReason(reason); err != nil {
		return time.Time{}, err
	}
	normalizedDays, err := domain.NormalizeAdminMuteDays(days)
	if err != nil {
		return time.Time{}, err
	}
	if s.accounts == nil {
		return time.Time{}, apperr.New(apperr.CodeCommentAdminUnavailable)
	}
	return s.accounts.Mute(ctx, viewer.ID, target, normalizedDays)
}

// AdminBan 封禁评论发布人（账号状态置"封禁"）。
//
// 理由可留空（封禁没有到期日要向用户解释），但填了就不能超过 200 字。
func (s *Service) AdminBan(ctx context.Context, viewer domain.Viewer, target ctype.ID, reason string) error {
	if !viewer.Admin {
		return apperr.New(apperr.CodeCommentForbidden)
	}
	if target.IsZero() {
		return apperr.New(apperr.CodeCommentAdminUserInvalid)
	}
	if _, err := domain.NormalizeAdminOptionalReason(reason); err != nil {
		return err
	}
	if s.accounts == nil {
		return apperr.New(apperr.CodeCommentAdminUnavailable)
	}
	return s.accounts.Ban(ctx, viewer.ID, target)
}

// articleTitles 批量取列表所需的文章标题。
//
// 失败只记日志、列表照常返回：标题是展示字段（前端还要靠 article_id 跳文章页），
// 为了它让整个评论列表打不开是本末倒置。列表里 article_id 仍在，运营照样能处理。
func (s *Service) articleTitles(ctx context.Context, rows []domain.Comment) map[ctype.ID]string {
	if len(rows) == 0 {
		return map[ctype.ID]string{}
	}
	ids := make([]ctype.ID, 0, len(rows))
	seen := make(map[ctype.ID]bool, len(rows))
	for _, row := range rows {
		if row.ArticleID.IsZero() || seen[row.ArticleID] {
			continue
		}
		seen[row.ArticleID] = true
		ids = append(ids, row.ArticleID)
	}
	got, err := s.articles.TitlesByIDs(ctx, ids)
	if err != nil {
		s.logError("批量读取评论所属文章标题失败: %v", err)
		return map[ctype.ID]string{}
	}
	return got
}

// removeFailureMessage 把一条失败翻译成给后台看的中文文案。
//
// 有错误码的用码的文案；没有码的是内部错误（SQL 失败等），折成通用服务异常
// 文案 —— 审计日志与后台提示都不该出现 SQL 原文。
func removeFailureMessage(err error) string {
	if code, ok := apperr.CodeOf(err); ok {
		if msg := apperr.Message(code); msg != "" && msg != string(code) {
			return msg
		}
	}
	return apperr.Message(apperr.CodeCommonServiceFailed)
}
