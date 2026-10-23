/*
1. count 开销
2. 缺少文章组合索引
3. created_at 无索引
4. %key% 模糊搜索
5. 深分页
6. 置顶排序表达式
7. 4 次 Redis 往返

TODO:加一个 has_more 的功能
*/

package common

import (
	"errors"
	"fmt"
	"myblogx/models/ctype"

	"gorm.io/gorm"
)

// ErrInvalidOrder 表示前端传入了未授权的排序字段。
var ErrInvalidOrder = errors.New("排序字段错误")
var ErrListQueryDBRequired = errors.New("缺少查询数据库依赖")

// TODO: 添加时间范围筛选支持。
type PageInfo struct {
	Limit int    `form:"limit"`
	Page  int    `form:"page"`
	Key   string `form:"key"`
	Order string `form:"order"` // 允许前端覆盖排序字段
}

func (p PageInfo) NormalizePage() int {
	page := p.Page
	if page <= 0 {
		return 1
	}
	return page
}

func (p PageInfo) GetPage(count int) int {
	page := p.NormalizePage()

	if count <= 0 {
		return 1
	}

	limit := p.GetLimit()
	max := (count + limit - 1) / limit
	if page > max {
		return max
	}
	return page
}

func (p PageInfo) GetLimit() int {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 10
	}
	return p.Limit
}

func (p PageInfo) GetOffset(count int) int {
	return (p.GetPage(count) - 1) * p.GetLimit()
}

func (p PageInfo) GetOffsetNoCount() int {
	return (p.NormalizePage() - 1) * p.GetLimit()
}

type Options struct {
	PageInfo
	DB            *gorm.DB
	Select        []string
	Likes         []string
	Preloads      []string
	ExactPreloads map[string][]string
	Where         *gorm.DB
	Debug         bool
	OrderMap      map[string]bool
	DefaultOrder  string
	Unscoped      bool
}

// IDPageOptions 用于“先分页取主键 ID，再回表查详情”的场景。
type IDPageOptions struct {
	PageInfo
	IDColumn     string
	OrderMap     map[string]string
	DefaultOrder string
	Unscoped     bool
}

// ListQuery 适合简单列表查询：
// 1. 单表或轻量条件过滤
// 2. 模糊搜索
// 3. 预加载关联
// 4. 直接返回当前页完整记录
//
// 如果查询需要先拿主键，再回表查详情，应改用 PageIDQuery。
func ListQuery[T any](model T, option Options) (list []T, count int, err error) {
	baseQuery := buildListQuery(model, option)
	if baseQuery == nil {
		return nil, 0, ErrListQueryDBRequired
	}

	count, err = CountQuery(baseQuery)
	if err != nil {
		return
	}

	listQuery := baseQuery.Session(&gorm.Session{})
	limit := option.PageInfo.GetLimit()
	offset := option.PageInfo.GetOffset(count)
	listQuery = listQuery.Limit(limit).Offset(offset)

	if option.PageInfo.Order != "" {
		if option.OrderMap == nil || !option.OrderMap[option.PageInfo.Order] {
			err = fmt.Errorf("排序字段错误")
			return
		}
		listQuery = listQuery.Order(option.PageInfo.Order)
	} else if option.DefaultOrder != "" {
		listQuery = listQuery.Order(option.DefaultOrder)
	}

	// Select 只影响列表查询，不影响 count。
	if len(option.Select) > 0 {
		listQuery = listQuery.Select(option.Select)
	}

	for _, preload := range option.Preloads {
		listQuery = listQuery.Preload(preload)
	}

	for preload, fields := range option.ExactPreloads {
		listQuery = listQuery.Preload(preload, func(db *gorm.DB) *gorm.DB {
			return db.Select(fields)
		})
	}

	err = listQuery.Find(&list).Error
	return
}

// ListQueryHasMore 适合“不需要精确总数，只关心还有没有下一页”的前台列表。
// 查询时会多取一条记录，用来判断 has_more，从而避免额外 count 开销。
func ListQueryHasMore[T any](model T, option Options) (list []T, hasMore bool, err error) {
	baseQuery := buildListQuery(model, option)
	if baseQuery == nil {
		return nil, false, ErrListQueryDBRequired
	}

	listQuery := baseQuery.Session(&gorm.Session{})
	limit := option.PageInfo.GetLimit()
	offset := option.PageInfo.GetOffsetNoCount()
	listQuery = listQuery.Limit(limit + 1).Offset(offset)

	if option.PageInfo.Order != "" {
		if option.OrderMap == nil || !option.OrderMap[option.PageInfo.Order] {
			err = ErrInvalidOrder
			return
		}
		listQuery = listQuery.Order(option.PageInfo.Order)
	} else if option.DefaultOrder != "" {
		listQuery = listQuery.Order(option.DefaultOrder)
	}

	if len(option.Select) > 0 {
		listQuery = listQuery.Select(option.Select)
	}

	for _, preload := range option.Preloads {
		listQuery = listQuery.Preload(preload)
	}

	for preload, fields := range option.ExactPreloads {
		listQuery = listQuery.Preload(preload, func(db *gorm.DB) *gorm.DB {
			return db.Select(fields)
		})
	}

	err = listQuery.Find(&list).Error
	if err != nil {
		return
	}
	if len(list) > limit {
		hasMore = true
		list = list[:limit]
	}
	return
}

// CountQuery 只负责统计总数。
// 这里单独复制 session，避免与后续列表查询的 limit/order/preload 相互污染。
func CountQuery(query *gorm.DB) (count int, err error) {
	var total int64
	if err = query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return 0, err
	}
	return int(total), nil
}

// ResolveOrder 把前端排序参数映射成白名单内的 SQL 排序表达式。
func ResolveOrder(order string, orderMap map[string]string, defaultOrder string) (string, error) {
	if order == "" {
		return defaultOrder, nil
	}
	if orderMap == nil {
		return "", ErrInvalidOrder
	}
	sqlOrder, ok := orderMap[order]
	if !ok {
		return "", ErrInvalidOrder
	}
	return sqlOrder, nil
}

// PageIDQuery 用于复杂列表的分页第一阶段：
// 1. 先统计总数
// 2. 再按排序规则取出当前页主键 ID
//
// 适合文章列表这类需要 join/filter 后，再回表查询完整详情的场景。
func PageIDQuery(query *gorm.DB, option IDPageOptions) (ids []ctype.ID, count int, err error) {
	if option.Unscoped {
		query = query.Unscoped()
	}

	count, err = CountQuery(query)
	if err != nil {
		return
	}

	order, err := ResolveOrder(option.PageInfo.Order, option.OrderMap, option.DefaultOrder)
	if err != nil {
		return
	}

	idColumn := option.IDColumn
	if idColumn == "" {
		idColumn = "id"
	}

	listQuery := query.Session(&gorm.Session{})
	if order != "" {
		listQuery = listQuery.Order(order)
	}

	limit := option.PageInfo.GetLimit()
	offset := option.PageInfo.GetOffset(count)
	err = listQuery.
		Select(idColumn).
		Limit(limit).
		Offset(offset).
		Pluck(idColumn, &ids).Error
	return
}

// PageIDHasMoreQuery 用于复杂列表的分页第一阶段，但不再统计 count。
// 通过多取一条主键记录判断是否还有下一页，适合前台滚动加载列表。
func PageIDHasMoreQuery(query *gorm.DB, option IDPageOptions) (ids []ctype.ID, hasMore bool, err error) {
	if option.Unscoped {
		query = query.Unscoped()
	}

	order, err := ResolveOrder(option.PageInfo.Order, option.OrderMap, option.DefaultOrder)
	if err != nil {
		return nil, false, err
	}

	idColumn := option.IDColumn
	if idColumn == "" {
		idColumn = "id"
	}

	listQuery := query.Session(&gorm.Session{})
	if order != "" {
		listQuery = listQuery.Order(order)
	}

	limit := option.PageInfo.GetLimit()
	offset := option.PageInfo.GetOffsetNoCount()

	err = listQuery.
		Select(idColumn).
		Limit(limit+1).
		Offset(offset).
		Pluck(idColumn, &ids).Error
	if err != nil {
		return nil, false, err
	}
	if len(ids) > limit {
		hasMore = true
		ids = ids[:limit]
	}
	return
}

// buildListQuery 只负责拼接公共过滤条件，不执行查询。
// Where 预期传入附加过滤条件，而不是完整查询链。
func buildListQuery[T any](model T, option Options) *gorm.DB {
	queryDB := option.DB
	if queryDB == nil {
		return nil
	}

	query := queryDB.Model(model)

	if option.Unscoped {
		query = query.Unscoped()
	}

	query = query.Where(model)

	if option.Debug {
		query = query.Debug()
	}

	if len(option.Likes) > 0 && option.PageInfo.Key != "" {
		query = query.Where(buildLikeCondition(queryDB, option.Likes, option.PageInfo.Key))
	}

	if option.Where != nil {
		query = query.Where(option.Where)
	}

	return query
}

// buildLikeCondition 构造多个列的 OR LIKE 条件。
func buildLikeCondition(db *gorm.DB, columns []string, key string) *gorm.DB {
	if db == nil {
		return nil
	}
	pattern := "%" + key + "%"
	likeQuery := db.Where(fmt.Sprintf("%s LIKE ?", columns[0]), pattern)
	for _, column := range columns[1:] {
		likeQuery = likeQuery.Or(fmt.Sprintf("%s LIKE ?", column), pattern)
	}
	return likeQuery
}
