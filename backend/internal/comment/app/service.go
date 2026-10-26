// Package app 是评论模块的用例层：发评论、点赞、删除、三个列表。
package app

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// CreateInput 是发表评论的输入。
type CreateInput struct {
	Content   string
	ArticleID ctype.ID
	ReplyID   *ctype.ID
}

// CreateResult 是发表评论的结果（字段名与旧契约一致）。
type CreateResult struct {
	ID        ctype.ID `json:"id"`
	ArticleID ctype.ID `json:"article_id"`
	ReplyID   ctype.ID `json:"reply_id"`
	RootID    ctype.ID `json:"root_id"`
	Status    int8     `json:"status"`
	Content   string   `json:"content"`
}

// RootItem 是一级评论列表项（字段名与旧契约逐字一致）。
type RootItem struct {
	ID           ctype.ID  `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Content      string    `json:"content"`
	UserID       ctype.ID  `json:"user_id"`
	ReplyID      ctype.ID  `json:"reply_id"`
	RootID       ctype.ID  `json:"root_id"`
	DiggCount    int       `json:"digg_count"`
	ReplyCount   int       `json:"reply_count"`
	IsDigg       bool      `json:"is_digg"`
	Relation     int8      `json:"relation"`
	Status       int8      `json:"status"`
	UserNickname string    `json:"user_nickname"`
	UserAvatar   string    `json:"user_avatar"`
}

// ReplyItem 是二级评论列表项。
type ReplyItem struct {
	ID                ctype.ID  `json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	Content           string    `json:"content"`
	UserID            ctype.ID  `json:"user_id"`
	ReplyID           ctype.ID  `json:"reply_id"`
	DiggCount         int       `json:"digg_count"`
	ReplyCount        int       `json:"reply_count"`
	IsDigg            bool      `json:"is_digg"`
	Relation          int8      `json:"relation"`
	Status            int8      `json:"status"`
	UserNickname      string    `json:"user_nickname"`
	UserAvatar        string    `json:"user_avatar"`
	ReplyUserNickname string    `json:"reply_user_nickname"`
}

// ManageItem 是管理端列表项。
type ManageItem struct {
	ID           ctype.ID `json:"id"`
	CreatedAt    string   `json:"created_at"`
	Content      string   `json:"content"`
	DiggCount    int      `json:"digg_count"`
	ReplyCount   int      `json:"reply_count"`
	UserID       ctype.ID `json:"user_id"`
	UserNickname string   `json:"user_nickname"`
	UserAvatar   string   `json:"user_avatar"`
	Relation     int8     `json:"relation,omitempty"`
	ArticleID    ctype.ID `json:"article_id"`
	ArticleTitle string   `json:"article_title"`
	ArticleCover string   `json:"article_cover"`
}

// ManagedInput 是管理端查询输入。
type ManagedInput struct {
	Type      int8
	Status    int8
	ArticleID ctype.ID
	UserID    ctype.ID
	Page      domain.Page
}

// Service 是评论应用服务。
type Service struct {
	repo      domain.Repository
	diggs     domain.DiggRepository
	counters  domain.Counters
	relations domain.RelationReader
	users     domain.UserDirectory
	articles  domain.ArticleInfo
	cache     domain.CacheWriter
	notifier  domain.Notifier
	ids       domain.IDGenerator
	logger    domain.Logger
	skipExam  func() bool
}

// New 构造评论服务。
func New(
	repo domain.Repository,
	diggs domain.DiggRepository,
	counters domain.Counters,
	relations domain.RelationReader,
	users domain.UserDirectory,
	articles domain.ArticleInfo,
	cache domain.CacheWriter,
	notifier domain.Notifier,
	ids domain.IDGenerator,
	logger domain.Logger,
	skipExam func() bool,
) *Service {
	if repo == nil {
		panic("comment/app: 评论仓储未注入")
	}
	if users == nil {
		panic("comment/app: 用户目录未注入")
	}
	if articles == nil {
		panic("comment/app: 文章信息未注入")
	}
	if ids == nil {
		panic("comment/app: ID 生成器未注入")
	}
	return &Service{
		repo:      repo,
		diggs:     diggs,
		counters:  counters,
		relations: relations,
		users:     users,
		articles:  articles,
		cache:     cache,
		notifier:  notifier,
		ids:       ids,
		logger:    logger,
		skipExam:  skipExam,
	}
}

// Create 发表评论或回复。
func (s *Service) Create(ctx context.Context, viewer domain.Viewer, in CreateInput) (*CreateResult, error) {
	if viewer.IsZero() {
		return nil, apperr.New(apperr.CodeAuthRequired)
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, apperr.New(apperr.CodeCommentContentEmpty)
	}
	article, err := s.articles.ByID(ctx, in.ArticleID.Uint64())
	if err != nil {
		return nil, apperr.New(apperr.CodeArticleNotFound)
	}
	if !article.CommentsToggle {
		return nil, apperr.New(apperr.CodeCommentClosed)
	}

	display := s.display(ctx, viewer.ID.Uint64())
	c := domain.Comment{
		Content:      in.Content,
		UserID:       viewer.ID,
		UserNickname: display.Nickname,
		UserAvatar:   display.Avatar,
		ArticleID:    in.ArticleID,
		Status:       domain.StatusExamining,
	}

	var replyUserID uint64
	if in.ReplyID != nil && !(*in.ReplyID).IsZero() {
		reply, err := s.repo.FindPublished(ctx, *in.ReplyID, in.ArticleID.Uint64())
		if err != nil {
			return nil, apperr.New(apperr.CodeCommentReplyInvalid)
		}
		c.ReplyID = *in.ReplyID
		c.ReplyUserID = reply.UserID
		c.ReplyUserNickname = reply.UserNickname
		if c.ReplyUserNickname == "" {
			c.ReplyUserNickname = s.display(ctx, reply.UserID.Uint64()).Nickname
		}
		if !reply.RootID.IsZero() {
			c.RootID = reply.RootID
		} else {
			c.RootID = reply.ID
		}
		replyUserID = reply.UserID.Uint64()
	}

	if err := s.repo.Create(ctx, &c); err != nil {
		return nil, err
	}

	published := viewer.Admin || (s.skipExam != nil && s.skipExam())
	if published {
		if err := s.repo.UpdateStatus(ctx, c.ID, domain.StatusPublished); err != nil {
			return nil, err
		}
		c.Status = domain.StatusPublished
		s.addArticleComments(ctx, c.ArticleID.Uint64(), 1)
		if !c.RootID.IsZero() {
			s.addReply(ctx, c.RootID.Uint64(), 1)
		}
		s.notifyComment(ctx, c, article)
		if !c.RootID.IsZero() && replyUserID != 0 {
			s.notifyReply(ctx, c, article, replyUserID)
		}
	}

	return &CreateResult{
		ID:        c.ID,
		ArticleID: c.ArticleID,
		ReplyID:   c.ReplyID,
		RootID:    c.RootID,
		Status:    int8(c.Status),
		Content:   c.Content,
	}, nil
}

// ToggleDigg 点赞 / 取消点赞；返回操作后是否处于"已点赞"。
func (s *Service) ToggleDigg(ctx context.Context, viewer domain.Viewer, commentID uint64) (bool, error) {
	if viewer.IsZero() {
		return false, apperr.New(apperr.CodeAuthRequired)
	}
	comment, err := s.repo.FindPublishedByID(ctx, ctype.NewID(commentID))
	if err != nil {
		return false, err
	}

	exists, err := s.diggs.Exists(ctx, commentID, viewer.ID.Uint64())
	if err != nil {
		return false, err
	}
	if exists {
		removed, err := s.diggs.Delete(ctx, commentID, viewer.ID.Uint64())
		if err != nil {
			return false, err
		}
		if !removed {
			return false, apperr.New(apperr.CodeCommentDiggChanged)
		}
		s.addDigg(ctx, commentID, -1)
		return false, nil
	}

	created, err := s.diggs.Create(ctx, commentID, viewer.ID.Uint64())
	if err != nil {
		return false, err
	}
	if !created {
		return false, apperr.New(apperr.CodeCommentDiggDuplicated)
	}
	s.addDigg(ctx, commentID, 1)
	s.notifyDigg(ctx, comment, viewer.ID.Uint64())
	return true, nil
}

// Remove 删除评论（管理员 / 评论作者 / 文章作者）。
func (s *Service) Remove(ctx context.Context, viewer domain.Viewer, commentID uint64) error {
	if viewer.IsZero() {
		return apperr.New(apperr.CodeAuthRequired)
	}
	target, err := s.repo.FindByID(ctx, ctype.NewID(commentID))
	if err != nil {
		return err
	}
	article, err := s.articles.ByID(ctx, target.ArticleID.Uint64())
	if err != nil {
		return apperr.New(apperr.CodeArticleNotFound)
	}
	if !target.CanDelete(viewer.ID.Uint64(), viewer.Admin, article.AuthorID.Uint64()) {
		return apperr.New(apperr.CodeCommentForbidden)
	}

	var family []domain.Comment
	if target.IsRoot() {
		family, err = s.repo.ListFamily(ctx, target.ID)
		if err != nil {
			return err
		}
	} else {
		family = []domain.Comment{target}
	}

	ids := make([]domain.CommentID, 0, len(family))
	rawIDs := make([]uint64, 0, len(family))
	articleDelta := 0
	for _, item := range family {
		ids = append(ids, item.ID)
		rawIDs = append(rawIDs, item.ID.Uint64())
		if item.Status.IsPublished() {
			articleDelta--
		}
	}
	if err := s.repo.SoftDeleteMany(ctx, ids); err != nil {
		return err
	}
	if err := s.diggs.DeleteByComments(ctx, rawIDs); err != nil {
		s.logError("删除评论点赞记录失败: %v", err)
	}
	for _, id := range rawIDs {
		s.delDigg(ctx, id)
	}
	if articleDelta != 0 {
		s.addArticleComments(ctx, target.ArticleID.Uint64(), articleDelta)
	}
	if target.IsRoot() {
		s.delReply(ctx, target.ID.Uint64())
	} else if target.Status.IsPublished() && !target.RootID.IsZero() {
		s.addReply(ctx, target.RootID.Uint64(), -1)
	}
	return nil
}

// ListRoot 一级评论列表。
func (s *Service) ListRoot(ctx context.Context, articleID ctype.ID, page domain.Page, viewerID ctype.ID) ([]RootItem, bool, error) {
	if _, err := s.articles.ByID(ctx, articleID.Uint64()); err != nil {
		return nil, false, apperr.New(apperr.CodeArticleNotFound)
	}
	rows, hasMore, err := s.repo.ListRoot(ctx, articleID.Uint64(), page)
	if err != nil {
		return nil, false, err
	}
	if err := s.hydrate(ctx, rows); err != nil {
		return nil, false, err
	}

	ids, userIDs := splitIDs(rows)
	diggDelta, replyDelta := s.counterBatch(ctx, ids)
	digged := s.diggedSet(ctx, viewerID.Uint64(), ids)
	relations := s.relationBatch(ctx, viewerID.Uint64(), userIDs)

	items := make([]RootItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RootItem{
			ID:           row.ID,
			CreatedAt:    row.CreatedAt,
			Content:      row.Content,
			UserID:       row.UserID,
			ReplyID:      row.ReplyID,
			RootID:       row.RootID,
			DiggCount:    row.DiggCount + diggDelta[row.ID.Uint64()],
			ReplyCount:   row.ReplyCount + replyDelta[row.ID.Uint64()],
			IsDigg:       digged[row.ID.Uint64()],
			Relation:     relations[row.UserID.Uint64()],
			Status:       int8(row.Status),
			UserNickname: row.UserNickname,
			UserAvatar:   row.UserAvatar,
		})
	}
	return items, hasMore, nil
}

// ListReplies 二级评论列表；第二个返回值是根评论的总回复数。
func (s *Service) ListReplies(ctx context.Context, articleID, rootID ctype.ID, page domain.Page, viewerID ctype.ID) ([]ReplyItem, int, bool, error) {
	root, err := s.repo.FindPublished(ctx, rootID, articleID.Uint64())
	if err != nil {
		return nil, 0, false, apperr.New(apperr.CodeCommentNotFound)
	}
	if !root.IsRoot() {
		return nil, 0, false, apperr.New(apperr.CodeCommentReplyInvalid)
	}

	rows, hasMore, err := s.repo.ListReplies(ctx, articleID.Uint64(), rootID.Uint64(), page)
	if err != nil {
		return nil, 0, false, err
	}
	if err := s.hydrate(ctx, rows); err != nil {
		return nil, 0, false, err
	}

	ids, userIDs := splitIDs(rows)
	diggDelta, replyDelta := s.counterBatch(ctx, ids)
	digged := s.diggedSet(ctx, viewerID.Uint64(), ids)
	relations := s.relationBatch(ctx, viewerID.Uint64(), userIDs)

	items := make([]ReplyItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ReplyItem{
			ID:                row.ID,
			CreatedAt:         row.CreatedAt,
			Content:           row.Content,
			UserID:            row.UserID,
			ReplyID:           row.ReplyID,
			DiggCount:         row.DiggCount + diggDelta[row.ID.Uint64()],
			ReplyCount:        row.ReplyCount + replyDelta[row.ID.Uint64()],
			IsDigg:            digged[row.ID.Uint64()],
			Relation:          relations[row.UserID.Uint64()],
			Status:            int8(row.Status),
			UserNickname:      row.UserNickname,
			UserAvatar:        row.UserAvatar,
			ReplyUserNickname: row.ReplyUserNickname,
		})
	}

	_, rootReplyDelta := s.counterBatch(ctx, []uint64{rootID.Uint64()})
	if rootReplyDelta == nil {
		rootReplyDelta = map[uint64]int{}
	}
	return items, root.ReplyCount + rootReplyDelta[rootID.Uint64()], hasMore, nil
}

// ListManaged 管理端评论列表。
func (s *Service) ListManaged(ctx context.Context, viewer domain.Viewer, in ManagedInput) ([]ManageItem, int64, error) {
	if in.Type == 3 && !viewer.Admin {
		return nil, 0, apperr.New(apperr.CodeUserForbidden)
	}
	q := domain.ManagedQuery{
		Type:      in.Type,
		Status:    domain.Status(in.Status),
		ViewerID:  viewer.ID,
		ArticleID: in.ArticleID,
		UserID:    in.UserID,
		Page:      in.Page,
	}
	switch in.Type {
	case 1:
		ids, err := s.articles.IDsByAuthor(ctx, viewer.ID, in.ArticleID)
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			return []ManageItem{}, 0, nil
		}
		q.ArticleIDs = ids
	case 2:
		q.UserID = viewer.ID
	}

	rows, total, err := s.repo.ListManaged(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	if err := s.hydrate(ctx, rows); err != nil {
		return nil, 0, err
	}

	ids, userIDs := splitIDs(rows)
	diggDelta, replyDelta := s.counterBatch(ctx, ids)
	articleIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		articleIDs = append(articleIDs, row.ArticleID.Uint64())
	}
	var articleMap map[uint64]domain.Article
	if s.articles != nil && len(articleIDs) > 0 {
		articleMap, _ = s.articles.BaseMap(ctx, articleIDs)
	}
	var relations map[uint64]int8
	if in.Type == 1 {
		relations = s.relationBatch(ctx, viewer.ID.Uint64(), userIDs)
	}

	items := make([]ManageItem, 0, len(rows))
	for _, row := range rows {
		article := articleMap[row.ArticleID.Uint64()]
		items = append(items, ManageItem{
			ID:           row.ID,
			CreatedAt:    row.CreatedAt.Format("2006-01-02 15:04:05"),
			Content:      row.Content,
			DiggCount:    row.DiggCount + diggDelta[row.ID.Uint64()],
			ReplyCount:   row.ReplyCount + replyDelta[row.ID.Uint64()],
			UserID:       row.UserID,
			UserNickname: row.UserNickname,
			UserAvatar:   row.UserAvatar,
			Relation:     relations[row.UserID.Uint64()],
			ArticleID:    row.ArticleID,
			ArticleTitle: article.Title,
			ArticleCover: article.Cover,
		})
	}
	return items, total, nil
}

// hydrate 补齐历史评论缺失的用户快照。
//
// 新写入的评论自带昵称头像；只有老数据或换了快照的表才需要这一步。
func (s *Service) hydrate(ctx context.Context, rows []domain.Comment) error {
	if len(rows) == 0 {
		return nil
	}
	var userIDs []uint64
	var replyIDs []uint64
	var replyUserIDs []uint64
	for _, row := range rows {
		if row.UserNickname == "" || row.UserAvatar == "" {
			userIDs = append(userIDs, row.UserID.Uint64())
		}
		if !row.ReplyID.IsZero() && row.ReplyUserNickname == "" {
			if !row.ReplyUserID.IsZero() {
				replyUserIDs = append(replyUserIDs, row.ReplyUserID.Uint64())
			} else {
				replyIDs = append(replyIDs, row.ReplyID.Uint64())
			}
		}
	}

	replyMap := map[uint64]domain.Comment{}
	if len(replyIDs) > 0 {
		ids := make([]domain.CommentID, 0, len(replyIDs))
		for _, id := range replyIDs {
			ids = append(ids, ctype.NewID(id))
		}
		got, err := s.repo.FindByIDs(ctx, ids)
		if err != nil {
			return err
		}
		for _, item := range got {
			replyMap[item.ID.Uint64()] = item
			if item.UserNickname == "" {
				replyUserIDs = append(replyUserIDs, item.UserID.Uint64())
			}
		}
	}

	displayIDs := append(userIDs, replyUserIDs...)
	displays := map[uint64]domain.Display{}
	if len(displayIDs) > 0 {
		got, err := s.users.Displays(ctx, displayIDs)
		if err != nil {
			return err
		}
		displays = got
	}

	for i := range rows {
		if rows[i].UserNickname == "" || rows[i].UserAvatar == "" {
			if user, ok := displays[rows[i].UserID.Uint64()]; ok {
				if rows[i].UserNickname == "" {
					rows[i].UserNickname = user.Nickname
				}
				if rows[i].UserAvatar == "" {
					rows[i].UserAvatar = user.Avatar
				}
			}
		}
		if rows[i].ReplyID.IsZero() || rows[i].ReplyUserNickname != "" {
			continue
		}
		if !rows[i].ReplyUserID.IsZero() {
			if user, ok := displays[rows[i].ReplyUserID.Uint64()]; ok {
				rows[i].ReplyUserNickname = user.Nickname
			}
			continue
		}
		if reply, ok := replyMap[rows[i].ReplyID.Uint64()]; ok {
			rows[i].ReplyUserID = reply.UserID
			if reply.UserNickname != "" {
				rows[i].ReplyUserNickname = reply.UserNickname
				continue
			}
			if user, ok := displays[reply.UserID.Uint64()]; ok {
				rows[i].ReplyUserNickname = user.Nickname
			}
		}
	}
	return nil
}

func (s *Service) display(ctx context.Context, userID uint64) domain.Display {
	got, err := s.users.Displays(ctx, []uint64{userID})
	if err != nil {
		return domain.Display{}
	}
	return got[userID]
}

func (s *Service) counterBatch(ctx context.Context, ids []uint64) (map[uint64]int, map[uint64]int) {
	if s.counters == nil || len(ids) == 0 {
		return map[uint64]int{}, map[uint64]int{}
	}
	return s.counters.Batch(ctx, ids)
}

func (s *Service) diggedSet(ctx context.Context, viewerID uint64, ids []uint64) map[uint64]bool {
	if s.diggs == nil || len(ids) == 0 {
		return map[uint64]bool{}
	}
	got, err := s.diggs.DiggedSet(ctx, viewerID, ids)
	if err != nil {
		return map[uint64]bool{}
	}
	return got
}

func (s *Service) relationBatch(ctx context.Context, viewerID uint64, userIDs []uint64) map[uint64]int8 {
	if s.relations == nil || viewerID == 0 || len(userIDs) == 0 {
		return map[uint64]int8{}
	}
	got, err := s.relations.Batch(ctx, viewerID, userIDs)
	if err != nil {
		return map[uint64]int8{}
	}
	return got
}

func (s *Service) addArticleComments(ctx context.Context, articleID uint64, delta int) {
	if s.cache == nil {
		return
	}
	if err := s.cache.AddArticleComments(ctx, articleID, delta); err != nil {
		s.logError("回写文章评论计数失败: article=%d delta=%d err=%v", articleID, delta, err)
	}
}

func (s *Service) addReply(ctx context.Context, rootID uint64, delta int) {
	if s.cache == nil {
		return
	}
	if err := s.cache.AddReply(ctx, rootID, delta); err != nil {
		s.logError("回写评论回复计数失败: root=%d delta=%d err=%v", rootID, delta, err)
	}
}

func (s *Service) delReply(ctx context.Context, rootID uint64) {
	if s.cache == nil {
		return
	}
	if err := s.cache.DelReply(ctx, rootID); err != nil {
		s.logError("删除评论回复缓存失败: root=%d err=%v", rootID, err)
	}
}

func (s *Service) addDigg(ctx context.Context, commentID uint64, delta int) {
	if s.cache == nil {
		return
	}
	if err := s.cache.AddDigg(ctx, commentID, delta); err != nil {
		s.logError("回写评论点赞计数失败: comment=%d delta=%d err=%v", commentID, delta, err)
	}
}

func (s *Service) delDigg(ctx context.Context, commentID uint64) {
	if s.cache == nil {
		return
	}
	if err := s.cache.DelDigg(ctx, commentID); err != nil {
		s.logError("删除评论点赞缓存失败: comment=%d err=%v", commentID, err)
	}
}

func (s *Service) notifyComment(ctx context.Context, c domain.Comment, article domain.Article) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.CommentCreated(ctx, domain.CommentNotice{
		CommentID:    c.ID,
		Content:      c.Content,
		ReceiverID:   article.AuthorID,
		ActionUserID: c.UserID,
		ArticleID:    article.ID,
		ArticleTitle: article.Title,
	}); err != nil {
		s.logError("发送评论通知失败: comment=%d err=%v", c.ID.Uint64(), err)
	}
}

func (s *Service) notifyReply(ctx context.Context, c domain.Comment, article domain.Article, receiverID uint64) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.ReplyCreated(ctx, domain.ReplyNotice{
		CommentID:    c.ID,
		Content:      c.Content,
		ReceiverID:   ctype.NewID(receiverID),
		ActionUserID: c.UserID,
		ArticleID:    article.ID,
		ArticleTitle: article.Title,
	}); err != nil {
		s.logError("发送回复通知失败: comment=%d err=%v", c.ID.Uint64(), err)
	}
}

func (s *Service) notifyDigg(ctx context.Context, c domain.Comment, actionUserID uint64) {
	if s.notifier == nil {
		return
	}
	article, _ := s.articles.ByID(ctx, c.ArticleID.Uint64())
	if err := s.notifier.DiggCreated(ctx, domain.DiggNotice{
		CommentID:    c.ID,
		Content:      c.Content,
		ReceiverID:   c.UserID,
		ActionUserID: ctype.NewID(actionUserID),
		ArticleID:    c.ArticleID,
		ArticleTitle: article.Title,
	}); err != nil {
		s.logError("发送点赞通知失败: comment=%d err=%v", c.ID.Uint64(), err)
	}
}

func (s *Service) logError(format string, args ...any) {
	if s.logger != nil {
		s.logger.Errorf(format, args...)
	}
}

func splitIDs(rows []domain.Comment) ([]uint64, []uint64) {
	ids := make([]uint64, 0, len(rows))
	userIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.Uint64())
		userIDs = append(userIDs, row.UserID.Uint64())
	}
	return ids, userIDs
}
