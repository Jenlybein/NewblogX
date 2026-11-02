package app

import (
	"context"
	"sort"
	"strings"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/domain"
)

// SidebarService 是右栏卡片的用例层。
//
// 与站点配置服务分开：右栏卡片有独立的表与独立的读写入口，
// 塞进 Service 会让那个已经装了八段配置的服务再长一层。
type SidebarService struct {
	cards  domain.SidebarCardRepository
	data   domain.SidebarDataSource
	logger Logger
}

// NewSidebar 构造右栏卡片服务。data 可为 nil（数据源未装配时卡片照样下发，只是没有数据）。
func NewSidebar(cards domain.SidebarCardRepository, data domain.SidebarDataSource, logger Logger) *SidebarService {
	return &SidebarService{cards: cards, data: data, logger: logger}
}

// SidebarCardView 是读者侧一张卡片的完整视图：配置 + 该种类自己的数据。
//
// 三种数据字段互不重叠：前端按 Kind 只读对应的那一个数组，
// 空数组即"这张卡没有数据"（读者站据此整块隐藏）。
type SidebarCardView struct {
	ID             ctype.ID
	Kind           domain.SidebarCardKind
	Title          string
	SortOrder      int
	HotArticles    []domain.SidebarArticle
	LatestComments []domain.SidebarComment
	Tags           []domain.SidebarTag
}

// PublicCards 返回读者侧要渲染的卡片：只含启用中的，按运营顺序，各自带数据。
//
// 取数失败只清空那张卡的数据，不影响其它卡、也不让接口报错 ——
// 与首页其它数据源同一条口径（ADR-0017 决策 5）。
func (s *SidebarService) PublicCards(ctx context.Context) ([]SidebarCardView, error) {
	rows, err := s.cards.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	sortSidebarCards(rows)

	out := make([]SidebarCardView, 0, len(rows))
	for _, row := range rows {
		view := SidebarCardView{
			ID:        row.ID,
			Kind:      row.Kind,
			Title:     row.Title,
			SortOrder: row.SortOrder,
		}
		s.fillCardData(ctx, &view)
		out = append(out, view)
	}
	return out, nil
}

// AdminCards 返回管理端要编辑的卡片：包含停用卡，顺序与读者侧一致。
func (s *SidebarService) AdminCards(ctx context.Context) ([]domain.SidebarCard, error) {
	rows, err := s.cards.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	sortSidebarCards(rows)
	return rows, nil
}

// SidebarCardInput 是管理端保存单张卡片的入参。
//
// 没有 SortOrder：排序由提交顺序决定（后台拖完就是新顺序），
// 让调用方同时传顺序号与数组顺序会造出两种互相矛盾的权威。
type SidebarCardInput struct {
	Kind    domain.SidebarCardKind
	Title   string
	Enabled bool
}

// ReplaceCards 全量保存卡片配置。
//
// 先校验再写库：脏类型 / 超量提交必须在落库前拒绝，
// 否则读者侧会拿到一张永远渲染不出来的卡。
func (s *SidebarService) ReplaceCards(ctx context.Context, inputs []SidebarCardInput) error {
	if len(inputs) > domain.SidebarCardMaxCount {
		return apperr.New(apperr.CodeSidebarCardLimitExceeded)
	}
	rows := make([]domain.SidebarCard, 0, len(inputs))
	for _, in := range inputs {
		if !in.Kind.Valid() {
			return apperr.New(apperr.CodeSidebarCardKindInvalid)
		}
		title := strings.TrimSpace(in.Title)
		if title == "" {
			title = domain.DefaultSidebarCardTitle(in.Kind)
		}
		rows = append(rows, domain.SidebarCard{
			Kind:    in.Kind,
			Title:   title,
			Enabled: in.Enabled,
		})
	}
	// 排序号按提交顺序重排成 0..n-1：库里永远是一段连续、无重复的顺序，
	// 读者侧不必再处理并列或空洞。
	for i := range rows {
		rows[i].SortOrder = i
	}
	return s.cards.Replace(ctx, rows)
}

// fillCardData 按种类取数据；未知种类不带数据，取数失败只记日志。
func (s *SidebarService) fillCardData(ctx context.Context, view *SidebarCardView) {
	if s.data == nil {
		return
	}
	var err error
	switch view.Kind {
	case domain.CardKindHotArticles:
		view.HotArticles, err = s.data.HotArticles(ctx, domain.SidebarCardDataLimit)
	case domain.CardKindLatestComments:
		view.LatestComments, err = s.data.LatestComments(ctx, domain.SidebarCardDataLimit)
	case domain.CardKindTagCloud:
		view.Tags, err = s.data.TagCloud(ctx, domain.SidebarCardDataLimit)
	}
	if err != nil {
		s.logError("读取右栏卡片数据失败（kind=%s）: %v", view.Kind, err)
	}
}

// logError 写一条错误日志。
func (s *SidebarService) logError(format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Errorf(format, args...)
}

// sortSidebarCards 按 SortOrder 升序、同序按 ID 升序稳定排序。
func sortSidebarCards(rows []domain.SidebarCard) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SortOrder != rows[j].SortOrder {
			return rows[i].SortOrder < rows[j].SortOrder
		}
		return rows[i].ID.Uint64() < rows[j].ID.Uint64()
	})
}
