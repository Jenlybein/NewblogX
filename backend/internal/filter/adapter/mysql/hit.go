// Package mysql 是内容过滤模块的 MySQL 适配器。
//
// 只在这一层出现表名与列名（content_filter_hit_models），上层拿到的是领域记录。
// 命中明细（词 / 档位 / 类目 / 上下文片段）以 JSON 落在 hits_json 一列里：
// 它是一份不可变的快照，没有任何查询按"词"过筛（词云走的是配置段，不是这张表），
// 拆成子表只会把一条记录的读放大成一次 join。
package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/apperr"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
	"myblogx/internal/platform/idgen"
	"myblogx/internal/platform/readmodel"
)

// contentFilterHitRow 对应表 content_filter_hit_models。
type contentFilterHitRow struct {
	ID        ctype.ID       `gorm:"primaryKey;autoIncrement:false;column:id"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
	// Surface 是文本面（article / comment / nickname / abstract / chat / collection）。
	Surface string `gorm:"column:surface"`
	// ContentID 是命中内容在其所属实体表里的 ID。
	ContentID ctype.ID `gorm:"column:content_id"`
	// UserID 是内容作者；顶部命中用户 Top 按它聚合。
	UserID ctype.ID `gorm:"column:user_id"`
	// HitsJSON 是命中明细的 JSON 快照（词 / 档位 / 类目 / 上下文片段）。
	HitsJSON string `gorm:"column:hits_json"`
	// HitCount 是命中处数（白名单命中计入，重叠命中按并集算一处）。
	HitCount int `gorm:"column:hit_count"`
	// Status 是处理状态：pending / ignored / deleted。
	Status string `gorm:"column:status"`
	// HandlerID / HandledAt / Note 是处置痕迹（处置动作是后续票，这里先留列）。
	HandlerID ctype.ID   `gorm:"column:handler_id"`
	HandledAt *time.Time `gorm:"column:handled_at"`
	Note      string     `gorm:"column:note"`
}

// TableName 固定表名。
func (contentFilterHitRow) TableName() string { return "content_filter_hit_models" }

// hitWordJSON 是 hits_json 里的一项（存的是原文写法，不做归一化）。
//
// 刻意不直接序列化领域结构：存储格式是这张表的契约，领域结构改了不该顺手改库里的
// 历史 JSON。两个方向的转换都收在这个文件里。
type hitWordJSON struct {
	Word     string        `json:"word"`
	Tier     string        `json:"tier"`
	Category string        `json:"category"`
	Snippets []snippetJSON `json:"snippets"`
}

// snippetJSON 是一条上下文片段；Start / End 是命中词在 text 内的 rune 下标。
type snippetJSON struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// Repo 实现 domain.Repository。
type Repo struct {
	db *gorm.DB
}

var _ domain.Repository = (*Repo)(nil)

// New 构造命中记录仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// Create 写入一条命中记录。
func (r *Repo) Create(ctx context.Context, hit *domain.Hit) error {
	if hit == nil {
		return nil
	}
	now := time.Now()
	id := hit.ID
	if id.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		id = next
	}
	createdAt := hit.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	status := hit.Status
	if !status.Valid() {
		status = domain.StatusPending
	}
	payload, err := encodeWords(hit.Words)
	if err != nil {
		return err
	}
	row := contentFilterHitRow{
		ID:        id,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		Surface:   string(hit.Surface),
		ContentID: hit.ContentID,
		UserID:    hit.UserID,
		HitsJSON:  payload,
		HitCount:  hit.HitCount,
		Status:    string(status),
		HandlerID: hit.HandlerID,
		Note:      hit.Note,
	}
	if !hit.HandledAt.IsZero() {
		handledAt := hit.HandledAt
		row.HandledAt = &handledAt
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	hit.ID = row.ID
	hit.CreatedAt = row.CreatedAt
	hit.Status = status
	return nil
}

// List 分页查风险记录（created_at 倒序），并返回满足条件的总条数。
func (r *Repo) List(ctx context.Context, q domain.Query) ([]domain.Hit, int64, error) {
	var total int64
	if err := applyFilters(r.db.WithContext(ctx).Model(&contentFilterHitRow{}), q).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	page := q.Page.Normalize()
	var rows []contentFilterHitRow
	if err := applyFilters(r.db.WithContext(ctx).Model(&contentFilterHitRow{}), q).
		Order("created_at DESC").Order("id DESC").
		Offset(page.Offset()).Limit(page.Limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	hits := make([]domain.Hit, 0, len(rows))
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		hit, err := toHit(row)
		if err != nil {
			return nil, 0, err
		}
		hits = append(hits, hit)
		userIDs = append(userIDs, row.UserID)
	}
	r.hydrateNicknames(hits, userIDs)
	return hits, total, nil
}

// TopUsers 返回 since 之后命中处数最多的前 limit 个用户。
func (r *Repo) TopUsers(ctx context.Context, since time.Time, limit int) ([]domain.TopUser, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []struct {
		UserID   ctype.ID `gorm:"column:user_id"`
		HitCount int      `gorm:"column:hit_count"`
	}
	err := r.db.WithContext(ctx).Model(&contentFilterHitRow{}).
		Select("user_id, SUM(hit_count) AS hit_count").
		Where("created_at >= ?", since).
		Group("user_id").
		Order("hit_count DESC").Order("user_id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]domain.TopUser, 0, len(rows))
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.TopUser{UserID: row.UserID, HitCount: row.HitCount})
		userIDs = append(userIDs, row.UserID)
	}
	displays, err := readmodel.LoadUserDisplayMap(r.db, userIDs)
	if err != nil {
		// 昵称只是展示字段：查不到就退回空串，不把"Top 列表"整条弄失败。
		return out, nil
	}
	for i := range out {
		out[i].Nickname = displays[out[i].UserID].Nickname
	}
	return out, nil
}

// FindByID 按 ID 查一条命中记录（软删的行也会被 gorm 默认排除）。
func (r *Repo) FindByID(ctx context.Context, id ctype.ID) (domain.Hit, error) {
	if id.IsZero() {
		return domain.Hit{}, apperr.New(apperr.CodeFilterHitNotFound)
	}
	var row contentFilterHitRow
	err := r.db.WithContext(ctx).Take(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Hit{}, apperr.New(apperr.CodeFilterHitNotFound)
	}
	if err != nil {
		return domain.Hit{}, err
	}
	hit, err := toHit(row)
	if err != nil {
		return domain.Hit{}, err
	}
	hit.UserNickname = r.nicknameOf(hit.UserID)
	return hit, nil
}

// MarkHandled 把一条**待处理**记录改写成已处置，返回是否真的改写了。
//
// 条件更新（WHERE status = 'pending'）是幂等与并发的落点：两个管理员同时点
// 处置，只有一个拿到 true，另一个拿到 false 后由用例层重读当前状态返回。
// 这样后到者不会覆盖先到者的处理人 / 时间 / 备注。
func (r *Repo) MarkHandled(ctx context.Context, id ctype.ID, h domain.Handle) (bool, error) {
	if id.IsZero() {
		return false, apperr.New(apperr.CodeFilterHitNotFound)
	}
	status := h.Status
	if !status.Valid() {
		return false, apperr.New(apperr.CodeFilterActionInvalid)
	}
	updates := map[string]any{
		"status":     string(status),
		"handler_id": h.HandlerID,
		"note":       h.Note,
		"updated_at": time.Now(),
	}
	if !h.HandledAt.IsZero() {
		updates["handled_at"] = h.HandledAt
	}
	res := r.db.WithContext(ctx).Model(&contentFilterHitRow{}).
		Where("id = ? AND status = ?", id, string(domain.StatusPending)).
		Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// PurgeBefore 物理删除 cutoff 之前的记录（保留期清理，ADR-0032 决策 10）。
//
// 不走软删：命中记录本身就是审计副本，软删等于在同一张表里继续留着那些
// ±20 字上下文片段，与"保留 90 天"的承诺相反。分批删除由调度侧的调用频率兜住
// （每天一次，单次删掉的量就是一天新增的量）。
func (r *Repo) PurgeBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		// 零值 cutoff 会删掉全部记录：宁可什么都不做，也不接受一个"没算出来"的时间。
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&contentFilterHitRow{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// WordStats 统计 since 之后每个命中词的出现处数（词云字号的数据源）。
//
// 为什么在 Go 里聚合而不是用 SQL 的 JSON 函数：命中明细是一段 words_json
// （词 / 档位 / 类目 / 上下文片段），按词拆开需要读 JSON 数组；MySQL 的
// JSON_TABLE 在 5.7 上没有，仓库也没引 8.0 专有语法。窗口内的记录数
// （默认 90 天、个人博客量级）足够小，一次读出来在 Go 里数一遍就够。
func (r *Repo) WordStats(ctx context.Context, since time.Time) ([]domain.WordStat, error) {
	var rows []contentFilterHitRow
	err := r.db.WithContext(ctx).Model(&contentFilterHitRow{}).
		Select("id, created_at, hits_json").
		Where("created_at >= ?", since).
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	stats := make(map[string]*domain.WordStat, len(rows))
	for _, row := range rows {
		words, decodeErr := decodeWords(row.HitsJSON)
		if decodeErr != nil {
			// 一条坏记录不该让整张词云空掉：跳过它，其余照常统计。
			continue
		}
		for _, word := range words {
			existing, ok := stats[word.Word]
			if !ok {
				stats[word.Word] = &domain.WordStat{
					Word:     word.Word,
					Tier:     word.Tier,
					Category: word.Category,
					// 记录按 created_at 倒序读，所以第一次见到的就是"最近一次"的档位。
					Count: countWordOccurrences(word),
				}
				continue
			}
			existing.Count += countWordOccurrences(word)
		}
	}

	out := make([]domain.WordStat, 0, len(stats))
	for _, stat := range stats {
		out = append(out, *stat)
	}
	// 处数倒序、同处数按词排序：词云要稳定（两次请求的顺序一致，运营才不会
	// 以为自己看错了）。
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Word < out[j].Word
	})
	return out, nil
}

// countWordOccurrences 是一条记录里某个词的命中处数（上下文片段数；没有片段时算一处）。
func countWordOccurrences(word domain.Word) int {
	if len(word.Snippets) > 0 {
		return len(word.Snippets)
	}
	return 1
}

// nicknameOf 现查一个用户的昵称（读面展示字段，不落库）。
func (r *Repo) nicknameOf(userID ctype.ID) string {
	displays, err := readmodel.LoadUserDisplayMap(r.db, []ctype.ID{userID})
	if err != nil {
		return ""
	}
	return displays[userID].Nickname
}

// applyFilters 把筛选条件挂到查询上（列表与计数共用一份口径）。
func applyFilters(tx *gorm.DB, q domain.Query) *gorm.DB {
	if q.Surface != "" {
		tx = tx.Where("surface = ?", string(q.Surface))
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", string(q.Status))
	}
	if !q.UserID.IsZero() {
		tx = tx.Where("user_id = ?", q.UserID)
	}
	return tx
}

// hydrateNicknames 批量补昵称（不落库：昵称会变，命中记录不该存快照）。
func (r *Repo) hydrateNicknames(hits []domain.Hit, userIDs []ctype.ID) {
	if len(hits) == 0 || len(userIDs) == 0 {
		return
	}
	displays, err := readmodel.LoadUserDisplayMap(r.db, userIDs)
	if err != nil {
		return
	}
	for i := range hits {
		hits[i].UserNickname = displays[hits[i].UserID].Nickname
	}
}

// toHit 把行转成领域记录。
func toHit(row contentFilterHitRow) (domain.Hit, error) {
	words, err := decodeWords(row.HitsJSON)
	if err != nil {
		return domain.Hit{}, err
	}
	status, ok := domain.ParseStatus(row.Status)
	if !ok {
		status = domain.StatusPending
	}
	hit := domain.Hit{
		ID:        row.ID,
		CreatedAt: row.CreatedAt,
		Surface:   domain.Surface(row.Surface),
		ContentID: row.ContentID,
		UserID:    row.UserID,
		Words:     words,
		HitCount:  row.HitCount,
		Status:    status,
		HandlerID: row.HandlerID,
		Note:      row.Note,
	}
	if row.HandledAt != nil {
		hit.HandledAt = *row.HandledAt
	}
	return hit, nil
}

// encodeWords 把命中明细序列化成 hits_json。
func encodeWords(words []domain.Word) (string, error) {
	payload := make([]hitWordJSON, 0, len(words))
	for _, word := range words {
		snippets := make([]snippetJSON, 0, len(word.Snippets))
		for _, snippet := range word.Snippets {
			snippets = append(snippets, snippetJSON{
				Start: snippet.Start,
				End:   snippet.End,
				Text:  snippet.Text,
			})
		}
		payload = append(payload, hitWordJSON{
			Word:     word.Word,
			Tier:     string(word.Tier),
			Category: string(word.Category),
			Snippets: snippets,
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// decodeWords 反序列化 hits_json；空串按"没有明细"处理（历史脏数据不阻断整页）。
func decodeWords(payload string) ([]domain.Word, error) {
	if payload == "" {
		return nil, nil
	}
	var rows []hitWordJSON
	if err := json.Unmarshal([]byte(payload), &rows); err != nil {
		return nil, err
	}
	words := make([]domain.Word, 0, len(rows))
	for _, row := range rows {
		snippets := make([]domain.Snippet, 0, len(row.Snippets))
		for _, snippet := range row.Snippets {
			snippets = append(snippets, domain.Snippet{
				Start: snippet.Start,
				End:   snippet.End,
				Text:  snippet.Text,
			})
		}
		words = append(words, domain.Word{
			Word:     row.Word,
			Tier:     contentfilter.Tier(row.Tier),
			Category: contentfilter.Category(row.Category),
			Snippets: snippets,
		})
	}
	return words, nil
}

