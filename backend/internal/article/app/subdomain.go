package app

import (
	"context"
	"sort"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/risk"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
)

// ── 分类 ──

// ListCategories 列出某用户的分类。
//
// 真实语义是"公开可读、写入私有"：读不校验归属，匿名也能按 user_id 读他人分类
// （个人主页要展示分类）；写入才要求登录并校验归属（见 SaveCategory /
// DeleteCategories）。未指定用户时退回到当前登录者，两者都没有才拒绝，
// 免得匿名请求落成"列出全站分类"。
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
//
// **删过的名字可以再用**：新建时若同用户下存在同名软删行，仓储恢复它
// （保留原 ID）而不是插入一条新的（ADR-0026 决策 1）。所以这里返回的分类
// 可能带着"本次生成的 ID"，而库里那条仍是旧 ID —— 调用方不要拿新建时的 ID
// 做后续写入，建完重拉列表即可（写入接口本来也不返回它）。
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
//
// 删除不是"删掉一个词条"：它把挂在该分类下的 N 篇文章改成"未分类"，文章本身还在。
// 所以它是**两步、同一事务**：先解除文章归属、再软删分类（见
// CategoryRepository.Delete 的契约；ADR-0026 约束 3 规定顺序不得颠倒）。
// 中途失败整单回滚，不会留下"文章已解绑、分类还在"的半截状态。
//
// 别人的分类是**拒绝**而不是静默跳过：批量删除是"一次动作"，删掉一半再返回
// 成功会让调用方以为全部成功。不存在的 ID 仍然跳过（重复提交同一批不该报错）。
// 两人用同一个分类名互不影响：归属按 user_id 判，删不到对方的行。
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
				// 文案用「分类不存在」而不是「无权限」：不向调用方确认别人的分类存在。
				return taxonomy.ErrCategoryNotFound
			}
			if err := r.Categories().Delete(ctx, id); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})
	if err != nil {
		// 事务已回滚，一条都没删掉：不能把中间累计的条数报给调用方。
		return 0, err
	}
	return deleted, nil
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
//
// includeDisabled 只对管理员生效：标签词库本身公开可读（发布页要选标签），
// 但停用项属于管理视图，匿名与普通用户即使显式传了也降级成只返回启用项
// （不报错——前端统一带这个参数，报错只会让公开读白屏）。
func (s *Service) ListTags(ctx context.Context, viewer domain.Viewer, includeDisabled bool) ([]*taxonomy.Tag, error) {
	if !viewer.IsAdmin() {
		includeDisabled = false
	}
	var out []*taxonomy.Tag
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		out, err = r.Tags().List(ctx, includeDisabled)
		return err
	})
	return out, err
}

// SaveTag 创建或更新标签（仅管理员）。
//
// 更新走**读—改—写**：先读出库里的标签，再只覆盖请求里给出的字段。
// 早先的实现每次都从零构造一个 NewTag（默认启用、排序 0、描述空），于是
// "编辑一个停用标签"会顺手把它放回全站可选，只改标题也会把排序与描述清空。
// 没传就是没改：Sort / Description / Enabled 都是可选值，nil 表示保持原值。
func (s *Service) SaveTag(ctx context.Context, viewer domain.Viewer, in TagInput) (*taxonomy.Tag, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	var saved *taxonomy.Tag
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		tag, err := s.loadOrNewTag(ctx, r, viewer, in)
		if err != nil {
			return err
		}
		if err := applyTagInput(tag, in); err != nil {
			return err
		}
		// 查重放在改名之后：标题已去空白，撞库前就给可读文案。
		dup, err := r.Tags().ExistsTitle(ctx, tag.Title(), tag.ID())
		if err != nil {
			return err
		}
		if dup {
			return taxonomy.ErrTitleDuplicated
		}
		if err := r.Tags().Save(ctx, tag); err != nil {
			return err
		}
		saved = tag
		return nil
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// loadOrNewTag 取出要写的标签：有 ID 就按 ID 读（读不到就是不存在，
// 不再拿客户端给的 ID 新建一条），没有 ID 才新建。
func (s *Service) loadOrNewTag(ctx context.Context, r Repos, viewer domain.Viewer, in TagInput) (*taxonomy.Tag, error) {
	if !in.ID.IsZero() {
		return r.Tags().Find(ctx, in.ID)
	}
	id := ctype.NewID(s.ids.NextArticleID().Uint64())
	return taxonomy.NewTag(id, in.Title, viewer.UserID)
}

// applyTagInput 把请求里"给了的"字段写到标签上。
//
// 标题总是重写（写入接口的必填项）；其余三项为 nil 时保持原值。
func applyTagInput(tag *taxonomy.Tag, in TagInput) error {
	sort := tag.Sort()
	if in.Sort != nil {
		sort = *in.Sort
	}
	desc := tag.Description()
	if in.Description != nil {
		desc = *in.Description
	}
	if err := tag.Rename(in.Title, sort, desc); err != nil {
		return err
	}
	switch {
	case in.Enabled == nil:
		// 没传启用状态：不动。编辑停用标签不能把它悄悄放回全站可选。
	case *in.Enabled:
		tag.Enable()
	default:
		tag.Disable()
	}
	return nil
}

// TagInput 是标签写入的输入。
//
// Sort / Description / Enabled 用指针：nil = "这次没动它"。
// 更新是读—改—写，所以"没传"与"显式传零值"必须能分开。
type TagInput struct {
	ID          domain.TagID
	Title       string
	Sort        *int
	Description *string
	Enabled     *bool
}

// DeleteTags 删除标签（仅管理员）。
//
// 只对**没有被任何文章引用**的标签开放：被引用时整单拒绝，并给出可读提示
// （几篇在用 + 建议改用停用）。引用数按关联表实时统计——tag_models 上的
// article_count 没有维护者，按它放行会把真有文章在用的标签删掉。
//
// 一条被引用就整单回滚：批量删除是"一次动作"，删掉一半再报错会让管理员
// 以为全部失败，回头再点一次又删掉一批。
func (s *Service) DeleteTags(ctx context.Context, viewer domain.Viewer, ids []domain.TagID) (int, error) {
	if !viewer.IsAdmin() {
		return 0, domain.ErrForbidden
	}
	deleted := 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		for _, id := range ids {
			refs, err := r.Tags().CountArticleRefs(ctx, id)
			if err != nil {
				return err
			}
			if refs > 0 {
				return taxonomy.NewTagInUseError(refs)
			}
			if err := r.Tags().Delete(ctx, id); err != nil {
				continue // 不存在就跳过：批量删除不应因一个脏 ID 整单失败
			}
			deleted++
		}
		return nil
	})
	if err != nil {
		// 事务已回滚，一条都没删掉：不能把中间累计的条数报给调用方。
		return 0, err
	}
	return deleted, nil
}

// ReplaceTagSortOrder 用完整有序列表原子重排标签之间的展示顺序（仅管理员）。
//
// 输入必须是当前全部未软删标签（含停用）的一个排列（taxonomy.CheckSortOrder）：
// 集合不一致整单拒绝，后台据此刷新重试，而不是把并发新建 / 删除的标签静默排丢。
// 落库的是既有 tag_models.sort 列（不是 sort_order）：列表读面按 `sort desc`，
// 所以第 0 个（网格最靠前）拿到最大的 sort 值。读者站首页标签云不读这个字段。
func (s *Service) ReplaceTagSortOrder(ctx context.Context, viewer domain.Viewer, ids []domain.TagID) error {
	if !viewer.IsAdmin() {
		return domain.ErrForbidden
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		current, err := r.Tags().ListIDs(ctx)
		if err != nil {
			return err
		}
		if err := taxonomy.CheckSortOrder(current, ids); err != nil {
			return err
		}
		return r.Tags().UpdateSortOrders(ctx, ids)
	})
}

// ── 收藏夹 ──

// ListFolders 列出某用户的收藏夹。
//
// viewer 用来判定"这是不是公开读面"：读别人的夹子（含匿名读）只允许看见
// 已发布文章的计数（与收藏内容同一口径，见 ListFavoriteItems）。
// articleID 非零时逐夹计算 has_article，供收藏弹窗显示勾/加号（#344）。
func (s *Service) ListFolders(ctx context.Context, viewer domain.Viewer, target domain.UserID, articleID domain.ArticleID) ([]*favorite.Folder, error) {
	if target.IsZero() {
		return nil, domain.ErrForbidden
	}
	var out []*favorite.Folder
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		out, err = r.Favorites().ListFolders(ctx, favorite.ListFoldersQuery{
			UserID:        target,
			ArticleID:     articleID,
			PublishedOnly: viewer.UserID != target,
		})
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
//
// onlyAdminAuthors 是「显示非管理员发布文章」开关的反面：收藏内容属于公开
// 读面，开关关闭时作者不是管理员的收藏条目不再出现（ADR-0015 决策 4）。
// 过滤下推到查询层（favorite.Repository.ListItems），否则当页条数与 total
// 会与可见条目对不上。
//
// 计数与详情 / 列表同一口径：查询层给的是库内列，这里再批量叠上 Redis 增量
// （ADR-0016 决策 3）。少了这一步，同一篇文章会出现"详情页 6、收藏列表 5"。
func (s *Service) ListFavoriteItems(ctx context.Context, viewer domain.Viewer, folderID favorite.ID, page domain.Page, onlyAdminAuthors bool) ([]*favorite.Item, int64, error) {
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
		// 公开读面（他人夹）只列已发布文章：收藏时的文章可能在之后被撤回，
		// 读者站不该因此把草稿/审核中的内容放出来（#344）。
		publishedOnly := !f.BelongsTo(viewer.UserID)
		p := page.Normalize()
		items, total, err = r.Favorites().ListItems(ctx, folderID, p.Offset, p.Limit, onlyAdminAuthors, publishedOnly)
		if err != nil {
			return err
		}
		return enrichItemCounters(ctx, r, items)
	})
	return items, total, err
}

// enrichItemCounters 通过账本把权威计数写回收藏条目。
//
// 与详情 / 列表走的是同一个入口（#279）：收藏内容列表与详情给出同一个数字，
// 因为"怎么合成权威值"只写在 CountLedger 一处。
func enrichItemCounters(ctx context.Context, r Repos, items []*favorite.Item) error {
	return ledgerOf(r).ApplyToItems(ctx, items)
}

// ── 审核任务 ──

// ListReviewTasks 分页列出审核任务（仅管理员）。
//
// 条目带**文章当前正文**，而不是任务行上的标题快照：审核员要在列表里直接
// 判断"值不值得发"，标题不够（#307）。任务表没有正文列（零 DDL 硬约束），
// 所以正文从 article_models 批量读一次；文章已被删除时该条正文为空，
// 但任务照常出现，队列不会因此整页失败。
//
// 每条条目同时带上机审前两层的结论（词表 + 评分派生标签）。两层都是纯函数：
// 词表每次从运行期配置现读、评分记录每次从 ai 模块现读，不落库也不缓存，
// 所以运营改完词表，下一次列表请求立即按新词表判（票 311 验收标准 2）。
//
// 默认排序由仓储负责：提交时间升序（老投稿在前）。按风险排序时风险不在库里，
// 需要先取整个筛选范围、在内存里排序后再按页码切片（见 ReviewSortRisk）。
func (s *Service) ListReviewTasks(ctx context.Context, viewer domain.Viewer, status *review.Status, sortBy ReviewListSort, page domain.Page) ([]ReviewTaskView, int64, error) {
	if !viewer.IsAdmin() {
		return nil, 0, domain.ErrForbidden
	}
	var (
		tasks       []*review.Task
		bodies      map[domain.ArticleID]string
		scores      map[domain.ArticleID]moderation.ScoreFacts
		compliances map[ctype.ID]*risk.Record
		total       int64
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		if sortBy == ReviewSortRisk {
			// 风险排序：取整个筛选范围；total 就是实际条目数。
			tasks, err = r.Reviews().ListAll(ctx, status)
			total = int64(len(tasks))
		} else {
			p := page.Normalize()
			tasks, total, err = r.Reviews().List(ctx, status, p.Offset, p.Limit)
		}
		if err != nil {
			return err
		}
		bodies, err = reviewBodies(ctx, r, tasks)
		if err != nil {
			return err
		}
		scores, err = s.latestModerationScores(ctx, tasks)
		if err != nil {
			return err
		}
		compliances, err = riskRecords(ctx, r, tasks)
		return err
	})
	if err != nil {
		return nil, 0, err
	}

	words := moderation.WordLists{}
	if s.words != nil {
		words = s.words()
	}

	out := make([]ReviewTaskView, 0, len(tasks))
	for _, task := range tasks {
		// 没有评分记录时 facts 保持 nil：第②层不产生信号，不报错（票 311 验收标准 5）。
		var facts *moderation.ScoreFacts
		if got, ok := scores[task.ArticleID()]; ok {
			copied := got
			facts = &copied
		}
		body := bodies[task.ArticleID()]
		risk := moderation.Evaluate(task.ArticleTitle()+"\n"+body, words, facts)
		out = append(out, ReviewTaskView{
			Task:       *task,
			Content:    body,
			Risk:       risk,
			Compliance: compliances[task.ArticleID()],
		})
	}

	if sortBy == ReviewSortRisk {
		// 高风险在前；同等级保持提交时间升序、id 作同毫秒兜底，与默认排序的内层一致。
		sort.SliceStable(out, func(i, j int) bool {
			left, right := out[i].Risk.Level.Rank(), out[j].Risk.Level.Rank()
			if left != right {
				return left > right
			}
			if !out[i].Task.CreatedAt().Equal(out[j].Task.CreatedAt()) {
				return out[i].Task.CreatedAt().Before(out[j].Task.CreatedAt())
			}
			return out[i].Task.ID() < out[j].Task.ID()
		})
		p := page.Normalize()
		start := min(p.Offset, len(out))
		end := min(start+p.Limit, len(out))
		out = out[start:end]
	}
	return out, total, nil
}

// taskArticleIDs 把审核任务映射成文章 ID 列表（三处批量读共用）。
//
// 抽出来是为了让"取哪些文章"只有一个写法：新增批量读时若各写一遍 for-range，
// 迟早有一处漏掉某类任务或悄悄改了去重口径。
func taskArticleIDs(tasks []*review.Task) []domain.ArticleID {
	ids := make([]domain.ArticleID, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ArticleID())
	}
	return ids
}

// riskRecords 批量读取第③层模型合规结论（票 312 / ADR-0024）。
//
// 缺失的文章不出现在结果里，视图层据此展示"未判定"——这与"判定为干净"不同。
func riskRecords(ctx context.Context, r Repos, tasks []*review.Task) (map[ctype.ID]*risk.Record, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	ids := taskArticleIDs(tasks)
	return r.Risks().ListByArticles(ctx, ids)
}

// latestModerationScores 批量读取机审第②层需要的评分片段。
//
// 端口为 nil 或没有条目时直接返回 nil：没有评分数据的站点仍然能用第①层
// 与审核列表，不因为没接模型就整页失败（spec #302 用户故事 18）。
func (s *Service) latestModerationScores(ctx context.Context, tasks []*review.Task) (map[domain.ArticleID]moderation.ScoreFacts, error) {
	if s.scores == nil || len(tasks) == 0 {
		return nil, nil
	}
	ids := taskArticleIDs(tasks)
	return s.scores.LatestScores(ctx, ids)
}

// reviewBodies 取审核队列条目对应的文章当前正文。
//
// 批量读一次（FindMany 会跳过缺失 / 已软删的 ID），不逐条查询；缺失即空串，
// 因为"文章没了但任务还在"的情形要让人工把任务处理掉，而不是让整页 500。
func reviewBodies(ctx context.Context, r Repos, tasks []*review.Task) (map[domain.ArticleID]string, error) {
	ids := taskArticleIDs(tasks)
	articles, err := r.Articles().FindMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	bodies := make(map[domain.ArticleID]string, len(articles))
	for _, a := range articles {
		bodies[a.ID()] = a.Content()
	}
	return bodies, nil
}

// HandleReviewTask 处理某条审核任务（仅管理员）。
//
// 与 Examine 的区别：Examine 是按文章审核，这里是按任务审核（管理后台的队列视图）。
// 两者最终都改文章状态，只是入口不同；"驳回必须选一条固定原因"由同一条规则把关
// （reviewReason），所以两个入口不会一个能空着原因通过、另一个不行。
func (s *Service) HandleReviewTask(ctx context.Context, viewer domain.Viewer, taskID ctype.ID, approve bool, code review.ReasonCode, note string) (*domain.Article, error) {
	if !viewer.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	// 空串是"通过"的落库形态：通过的语义里没有驳回原因这一项。
	reasonCode, reasonNote, err := reviewReason(approve, code, note)
	if err != nil {
		return nil, err
	}

	var result *domain.Article
	err = s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
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
			if err := task.Reject(viewer.UserID, reasonCode, reasonNote, now); err != nil {
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
