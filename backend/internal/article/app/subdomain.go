package app

import (
	"context"
	"myblogx/internal/kernel/ctype"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
)

// ── 分类 ──

// ListCategories 列出某用户的分类。
//
// 未指定用户时用当前登录者：分类是"个人分类"，不允许匿名枚举他人分类列表。
func (s *Service) ListCategories(ctx context.Context, viewer domain.Viewer, target domain.UserID) ([]*taxonomy.Category, error) {
	if target.IsZero() {
		target = viewer.UserID
	}
	if target.IsZero() {
		return nil, domain.ErrForbidden
	}
	var out []*taxonomy.Category
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		out, err = r.Categories().ListByUser(ctx, target)
		return err
	})
	return out, err
}

// SaveCategory 创建或更新分类。
//
// 名称唯一性显式检查：库里虽然有唯一索引，但那会返回 1062 错误码，
// 前端拿不到"名称重复"这个可读提示。
func (s *Service) SaveCategory(ctx context.Context, viewer domain.Viewer, id domain.CategoryID, title string) (*taxonomy.Category, error) {
	if !viewer.IsLoggedIn() {
		return nil, domain.ErrForbidden
	}

	var saved *taxonomy.Category
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if id.IsZero() {
			id = s.nextCategoryID()
		} else {
			// 更新前确认归属，避免改到他人分类。
			existing, err := r.Categories().Find(ctx, id)
			if err != nil {
				return err
			}
			if !existing.BelongsTo(viewer.UserID) {
				return taxonomy.ErrCategoryNotFound
			}
		}

		c, err := taxonomy.NewCategory(id, viewer.UserID, title)
		if err != nil {
			return err
		}
		dup, err := r.Categories().ExistsTitle(ctx, viewer.UserID, c.Title(), id)
		if err != nil {
			return err
		}
		if dup {
			return taxonomy.ErrTitleDuplicated
		}
		if err := r.Categories().Save(ctx, c); err != nil {
			return err
		}
		saved = c
		return nil
	})
	return saved, err
}

// DeleteCategories 删除分类（只删自己的）。
func (s *Service) DeleteCategories(ctx context.Context, viewer domain.Viewer, ids []domain.CategoryID) (int, error) {
	if !viewer.IsLoggedIn() {
		return 0, domain.ErrForbidden
	}
	deleted := 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		for _, id := range ids {
			c, err := r.Categories().Find(ctx, id)
			if err != nil {
				continue // 不存在就跳过：批量删除不应因一个脏 ID 整单失败
			}
			if !c.BelongsTo(viewer.UserID) {
				continue
			}
			if err := r.Categories().Delete(ctx, id); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})
	return deleted, err
}

// nextCategoryID 从文章 ID 生成器借号。
//
// 分类没有独立的 ID 服务：旧实现依赖雪花在 BeforeCreate 时生成。
// 这里复用同一个序列，保证全局唯一即可。
func (s *Service) nextCategoryID() domain.CategoryID {
	return ctype.NewID(s.ids.NextArticleID().Uint64())
}

// ── 标签 ──

// ListTags 列出标签。
func (s *Service) ListTags(ctx context.Context, includeDisabled bool) ([]*taxonomy.Tag, error) {
	var out []*taxonomy.Tag
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		out, err = r.Tags().List(ctx, includeDisabled)
		return err
	})
	return out, err
}

// SaveTag 创建或更新标签（仅管理员）。
func (s *Service) SaveTag(ctx context.Context, viewer domain.Viewer, in TagInput) (*taxonomy.Tag, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	var saved *taxonomy.Tag
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		id := in.ID
		if id.IsZero() {
			id = ctype.NewID(s.ids.NextArticleID().Uint64())
		}

		dup, err := r.Tags().ExistsTitle(ctx, in.Title, id)
		if err != nil {
			return err
		}
		if dup {
			return taxonomy.ErrTitleDuplicated
		}

		tag, err := taxonomy.NewTag(id, in.Title, viewer.UserID)
		if err != nil {
			return err
		}
		if err := tag.Rename(in.Title, in.Sort, in.Description); err != nil {
			return err
		}
		if in.Enabled != nil {
			if *in.Enabled {
				tag.Enable()
			} else {
				tag.Disable()
			}
		}
		if err := r.Tags().Save(ctx, tag); err != nil {
			return err
		}
		saved = tag
		return nil
	})
	return saved, err
}

// TagInput 是标签写入的输入。
type TagInput struct {
	ID          domain.TagID
	Title       string
	Sort        int
	Description string
	Enabled     *bool
}

// DeleteTags 删除标签（仅管理员）。
func (s *Service) DeleteTags(ctx context.Context, viewer domain.Viewer, ids []domain.TagID) (int, error) {
	if !viewer.IsAdmin() {
		return 0, domain.ErrForbidden
	}
	deleted := 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		for _, id := range ids {
			if err := r.Tags().Delete(ctx, id); err != nil {
				continue
			}
			deleted++
		}
		return nil
	})
	return deleted, err
}

// ── 收藏夹 ──

// ListFolders 列出某用户的收藏夹。
func (s *Service) ListFolders(ctx context.Context, target domain.UserID) ([]*favorite.Folder, error) {
	if target.IsZero() {
		return nil, domain.ErrForbidden
	}
	var out []*favorite.Folder
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		out, err = r.Favorites().ListFolders(ctx, target)
		return err
	})
	return out, err
}

// SaveFolder 创建或更新收藏夹。
func (s *Service) SaveFolder(ctx context.Context, viewer domain.Viewer, in FolderInput) (*favorite.Folder, error) {
	if !viewer.IsLoggedIn() {
		return nil, domain.ErrForbidden
	}

	var saved *favorite.Folder
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		id := in.ID
		if id.IsZero() {
			id = s.ids.NextFolderID()
		} else {
			existing, err := r.Favorites().FindFolder(ctx, id)
			if err != nil {
				return err
			}
			if !existing.BelongsTo(viewer.UserID) {
				return favorite.ErrFolderForbidden
			}
		}

		f, err := favorite.NewFolder(id, viewer.UserID, in.Title)
		if err != nil {
			return err
		}
		dup, err := r.Favorites().ExistsFolderTitle(ctx, viewer.UserID, f.Title(), id)
		if err != nil {
			return err
		}
		if dup {
			return favorite.ErrFolderExists
		}
		if err := f.Rename(in.Title, in.Cover, in.Abstract); err != nil {
			return err
		}
		if err := r.Favorites().SaveFolder(ctx, f); err != nil {
			return err
		}
		saved = f
		return nil
	})
	return saved, err
}

// FolderInput 是收藏夹写入的输入。
type FolderInput struct {
	ID       favorite.ID
	Title    string
	Cover    string
	Abstract string
}

// DeleteFolders 删除收藏夹（默认夹不可删）。
func (s *Service) DeleteFolders(ctx context.Context, viewer domain.Viewer, ids []favorite.ID) (int, error) {
	if !viewer.IsLoggedIn() {
		return 0, domain.ErrForbidden
	}
	deleted := 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		for _, id := range ids {
			f, err := r.Favorites().FindFolder(ctx, id)
			if err != nil {
				continue
			}
			if !f.BelongsTo(viewer.UserID) {
				continue
			}
			if !f.CanDelete() {
				return favorite.ErrFolderForbidden
			}
			if err := r.Favorites().DeleteFolder(ctx, id); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})
	return deleted, err
}

// ListFavoriteItems 列出某收藏夹里的收藏（含文章快照，不 join 文章表）。
//
// 若收藏夹是默认夹（is_default），额外交互性：返回前校验所属用户是否公开。
// 旧实现有"收藏夹不公开"的报错，这里按同语义：非本人查看默认夹以外的情况
// 需要目标用户允许（当前只允许看本人或默认夹）。
func (s *Service) ListFavoriteItems(ctx context.Context, viewer domain.Viewer, folderID favorite.ID, page domain.Page) ([]*favorite.Item, int64, error) {
	var (
		items []*favorite.Item
		total int64
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		f, err := r.Favorites().FindFolder(ctx, folderID)
		if err != nil {
			return err
		}
		// 只有本人、或该夹子属于"默认公开夹"时才允许查看。
		if !f.BelongsTo(viewer.UserID) && !f.IsDefault() {
			return favorite.ErrFolderForbidden
		}
		p := page.Normalize()
		items, total, err = r.Favorites().ListItems(ctx, folderID, p.Offset, p.Limit)
		return err
	})
	return items, total, err
}

// ── 审核任务 ──

// ListReviewTasks 分页列出审核任务（仅管理员）。
func (s *Service) ListReviewTasks(ctx context.Context, viewer domain.Viewer, status *review.Status, page domain.Page) ([]ReviewTaskView, int64, error) {
	if !viewer.IsAdmin() {
		return nil, 0, domain.ErrForbidden
	}
	var (
		tasks []*review.Task
		total int64
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		p := page.Normalize()
		var err error
		tasks, total, err = r.Reviews().List(ctx, status, p.Offset, p.Limit)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]ReviewTaskView, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, ReviewTaskView{Task: *t})
	}
	return out, total, nil
}

// HandleReviewTask 处理某条审核任务（仅管理员）。
//
// 与 Examine 的区别：Examine 是按文章审核，这里是按任务审核（管理后台的队列视图）。
// 两者最终都改文章状态，只是入口不同。
func (s *Service) HandleReviewTask(ctx context.Context, viewer domain.Viewer, taskID ctype.ID, approve bool, reason string) (*domain.Article, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	var result *domain.Article
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		task, err := r.Reviews().Find(ctx, taskID.Uint64())
		if err != nil {
			return err
		}
		a, err := r.Articles().Find(ctx, task.ArticleID())
		if err != nil {
			return err
		}
		if !domain.CanBeExaminedBy(a, viewer) {
			return domain.ErrInvalidTransition
		}

		now := s.clock.Now()
		if approve {
			if err := a.Approve(viewer.UserID, now); err != nil {
				return err
			}
			if err := task.Approve(viewer.UserID, now); err != nil {
				return err
			}
		} else {
			if err := a.Reject(viewer.UserID, now); err != nil {
				return err
			}
			if err := task.Reject(viewer.UserID, reason, now); err != nil {
				return err
			}
		}

		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}
		if err := r.Reviews().Update(ctx, task); err != nil {
			return err
		}
		result = a
		if approve {
			return emit(ctx, r, EventArticlePublished, a.ID())
		}
		return emit(ctx, r, EventArticleRejected, a.ID())
	})
	return result, err
}

// ── 管理员改可见性 ──

// SetVisibility 修改文章可见性。
//
// 作者只能设 visible/user_hidden；admin_hidden 只有管理员能设——
// 否则作者能自行撤销管理员的处置（见 domain.CanSetVisibility）。
func (s *Service) SetVisibility(ctx context.Context, viewer domain.Viewer, articleID domain.ArticleID, next domain.Visibility) (*domain.Article, error) {
	var result *domain.Article
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		if !domain.CanSetVisibility(a, viewer, next) {
			return domain.ErrForbidden
		}
		now := s.clock.Now()
		if _, err := a.ApplyEdit(domain.EditPatch{Visibility: &next}, now); err != nil {
			return err
		}
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}
		result = a
		return emit(ctx, r, EventArticleUpdated, a.ID())
	})
	return result, err
}

// AuthorProfile 取作者展示信息。
//
// 返回值 ID 为零、其余字段为空表示用户不存在——与 UserPort 的语义一致，
// 由 HTTP 层决定是返回 404 还是展示"未知作者"。
func (s *Service) AuthorProfile(ctx context.Context, userID domain.UserID) (AuthorView, error) {
	var v AuthorView
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		v, err = r.Users().Profile(ctx, userID)
		return err
	})
	return v, err
}
