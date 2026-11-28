package app

import (
	"context"

	"myblogx/conf"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"myblogx/internal/apperr"
	commenthttp "myblogx/internal/comment/adapter/http"
	commentmysql "myblogx/internal/comment/adapter/mysql"
	commentapp "myblogx/internal/comment/app"
	"myblogx/internal/comment/domain"
	filterapp "myblogx/internal/filter/app"
	filterdomain "myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	notificationapp "myblogx/internal/notification/app"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/idgen"
	"myblogx/internal/platform/readmodel"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/platform/redis/redis_comment"
	socialdomain "myblogx/internal/social/domain"
	userapp "myblogx/internal/user/app"
	userdomain "myblogx/internal/user/domain"
)

// WireComment 装配评论模块并注入路由处理器。
func WireComment(infra *Infra) {
	if infra == nil {
		return
	}
	repo := commentmysql.New(infra.DB)
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)

	svc := commentapp.New(
		repo,
		commentmysql.NewDigg(infra.DB),
		&counterBridge{db: infra.DB, redisOptions: redisOptions, logger: infra.Logger},
		&commentRelationBridge{},
		&userDirBridge{db: infra.DB},
		&articleBridge{db: infra.DB},
		&cacheBridge{deps: redisOptions},
		notifierBridge{},
		commentIDs{},
		infra.Logger,
	).WithFilterDisposition(commentHitDisposition(infra)).
		WithContentFilter(commentContentFilterBridge{svc: infra.Filter}).
		// 管理端「评论管理」的禁言 / 封禁：接 user 模块那一份用例服务。
		// WireUser 在 WireComment 之前跑（见 app/wiring_http.go），所以这里拿得到。
		WithAccountDisposer(commentAccountDisposerBridge{users: infra.UserService})
	// 挂到 Infra：关键词风险页的处置动作要复用**这一份**装配（票 398）。
	infra.CommentService = svc
	router.SetCommentHandler(commenthttp.NewHandler(svc))
}

// RetireCommentExamining 把存量「审核中」评论一次性转成已删除（票 400）。
//
// 放在启动路径上而不是做一个 CLI：退场是**代码与数据同时生效**的一件事 ——
// 代码上线后已经没有任何地方会把评论置为「审核中」，如果存量还留着 status=1，
// 它们就会永远停在一个没有任何入口能处理的状态。迁移本身幂等（只命中 status=1），
// 所以每个进程角色启动时都可以跑一遍，跑第二遍命中 0 行、不写库。
func RetireCommentExamining(infra *Infra) error {
	if infra == nil || infra.DB == nil {
		return nil
	}
	retired, err := commentmysql.New(infra.DB).RetireExamining(context.Background())
	if err != nil {
		return err
	}
	if infra.Logger != nil {
		// 条数一定要记：这是唯一能回答"上线后到底有多少条评论被这条迁移删掉"的线索。
		infra.Logger.Infof("评论「审核中」状态退场：%d 条存量评论转为已删除", retired)
	}
	return nil
}

// commentContentFilterBridge 把内容过滤服务接成评论模块的端口（票 396）。
//
// 文本面在装配层钉死成 comment：评论模块的端口刻意不带 surface（它只知道
// "这是我的评论文本"），面枚举属于内容过滤模块。
type commentContentFilterBridge struct {
	svc *filterapp.Service
}

// Filter 过滤一段评论文本并记一条关键词风险。
func (b commentContentFilterBridge) Filter(ctx context.Context, in domain.FilterRequest) (domain.FilterResult, error) {
	// 过滤器没装配时原样返回：评论照常发布，只是没有过滤与命中记录。
	// 这与"引擎没构建好就不启动"不冲突 —— 真到了线上，那条路径不存在。
	if b.svc == nil {
		return domain.FilterResult{Masked: in.Text}, nil
	}
	got, err := b.svc.Filter(ctx, filterapp.FilterInput{
		Surface:   filterdomain.SurfaceComment,
		UserID:    in.UserID,
		ContentID: in.ContentID,
		Text:      in.Text,
	})
	if err != nil {
		return domain.FilterResult{}, err
	}
	// 命中事实照报，处置由用例层按站点档位判（与文章面同一条分工）。
	return domain.FilterResult{
		Masked:          got.Masked,
		FilteredCount:   got.FilteredCount,
		BlockTierMasked: got.BlockTierMasked,
	}, nil
}

// commentIDs 用雪花生成评论 ID。
type commentIDs struct{}

// NextCommentID 返回下一个评论 ID。
func (commentIDs) NextCommentID() domain.CommentID {
	next, err := idgen.Next()
	if err != nil {
		panic("comment: 雪花 ID 生成失败: " + err.Error())
	}
	return ctype.NewID(next.Uint64())
}

// counterBridge 读评论计数的 Redis 增量。
type counterBridge struct {
	db           *gorm.DB
	redisOptions redis_service.Options
	logger       *logrus.Logger
}

// Batch 返回点赞 / 回复增量。
func (b *counterBridge) Batch(_ context.Context, ids []uint64) (map[uint64]int, map[uint64]int) {
	reader := readmodel.NewCommentCounterReader(b.redisOptions)
	got := reader.Batch(toCtypeIDs(ids))
	digg := make(map[uint64]int, len(got.DiggMap))
	reply := make(map[uint64]int, len(got.ReplyMap))
	for id, v := range got.DiggMap {
		digg[id.Uint64()] = v
	}
	for id, v := range got.ReplyMap {
		reply[id.Uint64()] = v
	}
	return digg, reply
}

// commentRelationBridge 读关注关系（数据来自 social 模块）。
type commentRelationBridge struct{}

// Batch 返回 viewer 与一批用户的关系。
func (b *commentRelationBridge) Batch(ctx context.Context, viewerID uint64, userIDs []uint64) (map[uint64]int8, error) {
	out := make(map[uint64]int8, len(userIDs))
	if socialService == nil || viewerID == 0 {
		for _, id := range userIDs {
			out[id] = socialdomain.RelationStranger.Int8()
		}
		return out, nil
	}
	got, err := socialService.RelationBatch(ctx, viewerID, userIDs)
	if err != nil {
		for _, id := range userIDs {
			out[id] = socialdomain.RelationStranger.Int8()
		}
		return out, nil
	}
	for id, rel := range got {
		out[id] = rel.Int8()
	}
	return out, nil
}

// userDirBridge 读用户展示快照。
type userDirBridge struct {
	db *gorm.DB
}

// Displays 批量取昵称头像。
func (b *userDirBridge) Displays(_ context.Context, ids []uint64) (map[uint64]domain.Display, error) {
	raw, err := readmodel.LoadUserDisplayMap(b.db, toCtypeIDs(ids))
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]domain.Display, len(raw))
	for id, item := range raw {
		out[id.Uint64()] = domain.Display{Nickname: item.Nickname, Avatar: item.Avatar}
	}
	return out, nil
}

// articleRow 是评论模块读取文章所需的最小列。
type articleRow struct {
	ID             ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	AuthorID       ctype.ID
	Title          string
	Cover          string
	CommentsToggle bool
}

// TableName 固定表名。
func (articleRow) TableName() string { return "article_models" }

// articleBridge 读文章信息。
type articleBridge struct {
	db *gorm.DB
}

// ByID 查一篇文章。
func (b *articleBridge) ByID(_ context.Context, id uint64) (domain.Article, error) {
	var row articleRow
	if err := b.db.Take(&row, "id = ?", id).Error; err != nil {
		return domain.Article{}, err
	}
	return toArticle(row), nil
}

// articleTitleRow 是批量读标题时的最小形状（只 select 两列，不建整行）。
type articleTitleRow struct {
	ID    ctype.ID
	Title string
}

// TitlesByIDs 批量查文章标题（管理端评论列表用）。
//
// 用 Table(...) 而不是过 gorm 的模型作用域：管理端要显示"这条评论挂在哪篇文章下"，
// 文章被软删之后标题仍然该显示 —— 否则列表里会莫名其妙地少一列，而评论本身
// 明明还在（评论不会因为文章下架就被删除）。
func (b *articleBridge) TitlesByIDs(_ context.Context, ids []ctype.ID) (map[ctype.ID]string, error) {
	raw := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if !id.IsZero() {
			raw = append(raw, id.Uint64())
		}
	}
	if len(raw) == 0 {
		return map[ctype.ID]string{}, nil
	}
	var rows []articleTitleRow
	if err := b.db.Table(articleRow{}.TableName()).
		Select("id, title").
		Where("id IN ?", raw).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[ctype.ID]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.Title
	}
	return out, nil
}

// cacheBridge 回写计数缓存。
type cacheBridge struct {
	deps redis_service.Options
}

// AddArticleComments 文章评论数增量。
func (b *cacheBridge) AddArticleComments(_ context.Context, articleID uint64, delta int) error {
	return redis_article.SetCacheComment(b.deps, ctype.NewID(articleID), delta)
}

// AddReply 根评论回复数增量。
func (b *cacheBridge) AddReply(_ context.Context, rootID uint64, delta int) error {
	return redis_comment.SetCacheReply(b.deps, ctype.NewID(rootID), delta)
}

// DelReply 删除根评论回复缓存。
func (b *cacheBridge) DelReply(_ context.Context, rootID uint64) error {
	return redis_comment.DelCacheReply(b.deps, ctype.NewID(rootID))
}

// AddDigg 评论点赞数增量。
func (b *cacheBridge) AddDigg(_ context.Context, commentID uint64, delta int) error {
	return redis_comment.SetCacheDigg(b.deps, ctype.NewID(commentID), delta)
}

// DelDigg 删除评论点赞缓存。
func (b *cacheBridge) DelDigg(_ context.Context, commentID uint64) error {
	return redis_comment.DelCacheDigg(b.deps, ctype.NewID(commentID))
}

// notifierBridge 把评论事件翻译成站内信。
//
// 数据只经 internal/notification 落库，评论模块不再直接写消息表。
// 通知服务在 WireNotification 里赋值，这里做 nil 兜底：
// 通知没装配时评论照常成功，只是不发通知。
type notifierBridge struct{}

// CommentCreated 通知文章作者。
func (notifierBridge) CommentCreated(ctx context.Context, in domain.CommentNotice) error {
	if notificationService == nil {
		return nil
	}
	return notificationService.NotifyComment(ctx, notificationapp.CommentNotice{
		CommentID:    in.CommentID,
		Content:      in.Content,
		Receiver:     in.ReceiverID.Uint64(),
		ActionUser:   in.ActionUserID.Uint64(),
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	})
}

// ReplyCreated 通知被回复的人。
func (notifierBridge) ReplyCreated(ctx context.Context, in domain.ReplyNotice) error {
	if notificationService == nil {
		return nil
	}
	return notificationService.NotifyReply(ctx, notificationapp.ReplyNotice{
		CommentID:    in.CommentID,
		Content:      in.Content,
		Receiver:     in.ReceiverID.Uint64(),
		ActionUser:   in.ActionUserID.Uint64(),
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	})
}

// DiggCreated 通知被点赞的评论作者。
func (notifierBridge) DiggCreated(ctx context.Context, in domain.DiggNotice) error {
	if notificationService == nil {
		return nil
	}
	return notificationService.NotifyCommentDigg(ctx, notificationapp.CommentDiggNotice{
		CommentID:    in.CommentID,
		Content:      in.Content,
		Receiver:     in.ReceiverID.Uint64(),
		ActionUser:   in.ActionUserID.Uint64(),
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	})
}

// commentAccountDisposerBridge 把账号处置（禁言 / 封禁）翻成 user 模块的用例。
//
// 后台「评论管理」的禁言 / 封禁按钮走这里（票 399 的账号处置在评论侧的入口）。
// 两个动作都走 user 模块已有权限判定与会话处理的用例，而不是直接写 SQL：
//   - 禁言 = Mute(days)：只写 mute_until，不动会话（禁言不拦登录）；
//     永久档（days=0）走 MuteForever —— user 侧的天数归一化上限是 365 天，
//     把"永久"塞成 3650 天会被直接拒绝，压成 365 天又会对管理员撒谎。
//   - 封禁 = AdminUpdate(status=3)：与后台账号页的「封禁」同一口径，
//     会把令牌版本 +1 并吊销全部会话（被封的人手里那张未过期令牌立即失效）。
//
// 管理员身份在这里用 RoleAdmin 钉死：评论模块只交出"动作发起人 ID"，
// 不负责证明他是不是管理员 —— 评论用例层已经用 `viewer.Admin` 判过一次。
// **user 侧拿到的是"被钉成管理员"的 Viewer，所以那次权限判定恒为真**：
// 这里的防线只有一道（评论用例层），不是两道；之后再加账号处置入口时，
// 必须在调用方自己判权限，别指望这层兜底。
type commentAccountDisposerBridge struct {
	users *userapp.Service
}

var _ domain.AccountDisposer = commentAccountDisposerBridge{}

// adminViewer 把动作发起人折成 user 模块的管理员 Viewer。
func (b commentAccountDisposerBridge) adminViewer(actor ctype.ID) userdomain.Viewer {
	return userdomain.Viewer{ID: userdomain.UserID(actor), Role: userdomain.RoleAdmin}
}

// Mute 禁言目标用户，返回到期时间（永久档返回远期时间点）。
func (b commentAccountDisposerBridge) Mute(ctx context.Context, actor, target ctype.ID, days int) (time.Time, error) {
	if b.users == nil {
		return time.Time{}, apperr.New(apperr.CodeCommentAdminUnavailable)
	}
	var (
		updated userdomain.User
		err     error
	)
	if days == 0 {
		updated, err = b.users.MuteForever(ctx, b.adminViewer(actor), userdomain.UserID(target))
	} else {
		updated, err = b.users.Mute(ctx, b.adminViewer(actor), userdomain.UserID(target), days)
	}
	if err != nil {
		return time.Time{}, err
	}
	if updated.MuteUntil == nil {
		// 设了禁言却没回填到期时间：说明 user 侧的写法变了，别静默返回零时间。
		return time.Time{}, apperr.New(apperr.CodeCommentAdminUnavailable)
	}
	return *updated.MuteUntil, nil
}

// Ban 封禁目标账号（账号禁用，不可自助恢复）。
func (b commentAccountDisposerBridge) Ban(ctx context.Context, actor, target ctype.ID) error {
	if b.users == nil {
		return apperr.New(apperr.CodeCommentAdminUnavailable)
	}
	banned := userdomain.StatusBanned
	return b.users.AdminUpdate(ctx, b.adminViewer(actor), userdomain.UserID(target),
		userapp.AdminPatchInput{Status: &banned})
}

func toArticle(row articleRow) domain.Article {
	return domain.Article{
		ID:             row.ID,
		AuthorID:       row.AuthorID,
		Title:          row.Title,
		Cover:          row.Cover,
		CommentsToggle: row.CommentsToggle,
	}
}

func toCtypeIDs(ids []uint64) []ctype.ID {
	out := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		out = append(out, ctype.NewID(id))
	}
	return out
}

// commentHitDisposition 现读"拦截词命中之后评论怎么处置"（2026-11-28）。
//
// 评论只有两档语义：release（打星照发）与其余（拒收）。所以这里把 block_action
// 原样给出去，由用例层判 `!= release` —— 不在装配层折叠成 bool，是为了让
// 「为什么 draft 也算拒收」这件事只有一个解释处（用例层）。
//
// 读不到运行时配置时返回 release：失败方向必须是"照常发布"，不是把所有人拒掉。
func commentHitDisposition(infra *Infra) func() string {
	return func() string {
		if infra == nil || infra.RuntimeSite == nil {
			return conf.ContentFilterActionRelease
		}
		action := infra.RuntimeSite.GetRuntimeContentFilter().AutoDispose.BlockAction
		// 旧值 mask 读成 release（2026-11-28 三档化）：老配置里的 mask 表达的正是
		// 「只打星照常发布」。若原样返回，它会被用例层当成「非 release ⇒ 拒收」，
		// 一次发版就把评论区从打星变成全部拒收。
		if action == conf.ContentFilterActionLegacyMask {
			return conf.ContentFilterActionRelease
		}
		return action
	}
}
