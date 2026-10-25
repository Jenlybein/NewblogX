package app

import (
	"context"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	commenthttp "myblogx/internal/comment/adapter/http"
	commentmysql "myblogx/internal/comment/adapter/mysql"
	commentapp "myblogx/internal/comment/app"
	"myblogx/internal/comment/domain"
	"myblogx/internal/platform/idgen"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/platform/redis/redis_comment"
	socialdomain "myblogx/internal/social/domain"
	"myblogx/models/ctype"
	"myblogx/platform/cachex"
	"myblogx/repository/read_repo"
	"myblogx/router"
	"myblogx/service/message_service"
)

// WireComment 装配评论模块并注入路由处理器。
func WireComment(infra *Infra) {
	if infra == nil {
		return
	}
	repo := commentmysql.New(infra.DB)
	redisDeps := redis_service.NewDeps(infra.Redis, infra.Logger)

	svc := commentapp.New(
		repo,
		commentmysql.NewDigg(infra.DB),
		&counterBridge{db: infra.DB, redisDeps: redisDeps, logger: infra.Logger},
		&commentRelationBridge{},
		&userDirBridge{db: infra.DB},
		&articleBridge{db: infra.DB},
		&cacheBridge{deps: redisDeps},
		&notifierBridge{db: infra.DB, logger: infra.Logger},
		commentIDs{},
		infra.Logger,
		func() bool { return infra.RuntimeSite.GetRuntimeComment().SkipExamining },
	)
	router.SetCommentHandler(commenthttp.NewHandler(svc))
}

// commentIDs 用雪花生成评论 ID。
type commentIDs struct{}

// NextCommentID 返回下一个评论 ID。
func (commentIDs) NextCommentID() domain.CommentID {
	next, err := idgen.Next()
	if err != nil {
		panic("comment: 雪花 ID 生成失败: " + err.Error())
	}
	return domain.CommentID(next.Uint64())
}

// counterBridge 读评论计数的 Redis 增量。
type counterBridge struct {
	db        *gorm.DB
	redisDeps redis_service.Deps
	logger    *logrus.Logger
}

// Batch 返回点赞 / 回复增量。
func (b *counterBridge) Batch(_ context.Context, ids []uint64) (map[uint64]int, map[uint64]int) {
	reader := read_repo.NewCommentCounterReader(cachex.NewDeps(b.redisDeps.Client, b.logger))
	got := reader.Batch(toCtypeIDs(ids))
	digg := make(map[uint64]int, len(got.DiggMap))
	reply := make(map[uint64]int, len(got.ReplyMap))
	for id, v := range got.DiggMap {
		digg[uint64(id)] = v
	}
	for id, v := range got.ReplyMap {
		reply[uint64(id)] = v
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
	raw, err := read_repo.LoadUserDisplayMap(b.db, toCtypeIDs(ids))
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]domain.Display, len(raw))
	for id, item := range raw {
		out[uint64(id)] = domain.Display{Nickname: item.Nickname, Avatar: item.Avatar}
	}
	return out, nil
}

// articleRow 是评论模块读取文章所需的最小列。
type articleRow struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement:false"`
	AuthorID       uint64
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

// BaseMap 批量查文章。
func (b *articleBridge) BaseMap(_ context.Context, ids []uint64) (map[uint64]domain.Article, error) {
	out := make(map[uint64]domain.Article, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []articleRow
	if err := b.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = toArticle(row)
	}
	return out, nil
}

// IDsByAuthor 查某作者的文章 ID 列表。
func (b *articleBridge) IDsByAuthor(_ context.Context, authorID, articleID uint64) ([]uint64, error) {
	if authorID == 0 {
		return nil, nil
	}
	tx := b.db.Model(&articleRow{}).Select("id").Where("author_id = ?", authorID)
	if articleID != 0 {
		tx = tx.Where("id = ?", articleID)
	}
	var ids []uint64
	if err := tx.Order("id asc").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// cacheBridge 回写计数缓存。
type cacheBridge struct {
	deps redis_service.Deps
}

// AddArticleComments 文章评论数增量。
func (b *cacheBridge) AddArticleComments(_ context.Context, articleID uint64, delta int) error {
	return redis_article.SetCacheComment(b.deps, ctype.ID(articleID), delta)
}

// AddReply 根评论回复数增量。
func (b *cacheBridge) AddReply(_ context.Context, rootID uint64, delta int) error {
	return redis_comment.SetCacheReply(b.deps, ctype.ID(rootID), delta)
}

// DelReply 删除根评论回复缓存。
func (b *cacheBridge) DelReply(_ context.Context, rootID uint64) error {
	return redis_comment.DelCacheReply(b.deps, ctype.ID(rootID))
}

// AddDigg 评论点赞数增量。
func (b *cacheBridge) AddDigg(_ context.Context, commentID uint64, delta int) error {
	return redis_comment.SetCacheDigg(b.deps, ctype.ID(commentID), delta)
}

// DelDigg 删除评论点赞缓存。
func (b *cacheBridge) DelDigg(_ context.Context, commentID uint64) error {
	return redis_comment.DelCacheDigg(b.deps, ctype.ID(commentID))
}

// notifierBridge 发送评论通知。
//
// 旧实现是裸 goroutine；这里改成同步调用，失败只记日志，
// 之后 S4.6 会替换成 outbox 事件。
type notifierBridge struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// CommentCreated 通知文章作者。
func (b *notifierBridge) CommentCreated(_ context.Context, in domain.CommentNotice) error {
	message_service.InsertCommentMessage(b.db, b.logger, message_service.ArticleCommentMessage{
		CommentID:    ctype.ID(in.CommentID),
		Content:      in.Content,
		ReceiverID:   ctype.ID(in.ReceiverID),
		ActionUserID: ctype.ID(in.ActionUserID),
		ArticleID:    ctype.ID(in.ArticleID),
		ArticleTitle: in.ArticleTitle,
	})
	return nil
}

// ReplyCreated 通知被回复的人。
func (b *notifierBridge) ReplyCreated(_ context.Context, in domain.ReplyNotice) error {
	message_service.InsertReplyMessage(b.db, b.logger, message_service.ArticleReplyMessage{
		CommentID:    ctype.ID(in.CommentID),
		Content:      in.Content,
		ReceiverID:   ctype.ID(in.ReceiverID),
		ActionUserID: ctype.ID(in.ActionUserID),
		ArticleID:    ctype.ID(in.ArticleID),
		ArticleTitle: in.ArticleTitle,
	})
	return nil
}

// DiggCreated 通知被点赞的评论作者。
func (b *notifierBridge) DiggCreated(_ context.Context, in domain.DiggNotice) error {
	message_service.InsertCommentDiggMessage(b.db, b.logger, message_service.CommentDiggMessage{
		CommentID:    ctype.ID(in.CommentID),
		Content:      in.Content,
		ReceiverID:   ctype.ID(in.ReceiverID),
		ActionUserID: ctype.ID(in.ActionUserID),
		ArticleID:    ctype.ID(in.ArticleID),
		ArticleTitle: in.ArticleTitle,
	})
	return nil
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
		out = append(out, ctype.ID(id))
	}
	return out
}
