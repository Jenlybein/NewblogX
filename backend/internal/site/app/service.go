// Package app 是站点模块的用例层。
package app

import (
	"context"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/domain"
)

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}

// SEO 是站点 SEO 摘要（给前端做 SSR 用）。
type SEO struct {
	SiteTitle    string
	ProjectTitle string
	Logo         string
	Icon         string
	Keywords     string
	Description  string
}

// AIInfo 是站点公开的 AI 信息（不包含密钥等敏感项）。
type AIInfo struct {
	Enable   bool
	Nickname string
	Avatar   string
	Abstract string
}

// Service 是站点服务。
type Service struct {
	banners domain.BannerRepository
	images  domain.ImageResolver
	runtime domain.RuntimeConfig
	version string
	qqURL   func() string
	logger  Logger
}

// New 构造站点服务。
func New(
	banners domain.BannerRepository,
	images domain.ImageResolver,
	runtime domain.RuntimeConfig,
	version string,
	qqURL func() string,
	logger Logger,
) *Service {
	return &Service{
		banners: banners,
		images:  images,
		runtime: runtime,
		version: version,
		qqURL:   qqURL,
		logger:  logger,
	}
}

// QQLoginURL 返回 QQ 登录跳转地址。
//
// 每次现算：AppID 与回调地址都可能在后台被改，缓存住会让跳转指向旧地址。
func (s *Service) QQLoginURL() string {
	if s.qqURL == nil {
		return ""
	}
	return s.qqURL()
}

// ── 公开配置 ────────────────────────────────────────────────────

// PublicSite 返回公开的站点配置，并把构建版本号塞进 About.Version。
func (s *Service) PublicSite() conf.Site {
	site := s.runtime.Site()
	site.About.Version = s.version
	return site
}

// PublicSEO 返回 SEO 摘要。
func (s *Service) PublicSEO() SEO {
	site := s.runtime.Site()
	return SEO{
		SiteTitle:    site.SiteInfo.Title,
		ProjectTitle: site.Project.Title,
		Logo:         site.SiteInfo.Logo,
		Icon:         site.Project.Icon,
		Keywords:     site.Seo.Keywords,
		Description:  site.Seo.Description,
	}
}

// PublicAI 返回站点 AI 信息。
func (s *Service) PublicAI() AIInfo {
	ai := s.runtime.AI()
	return AIInfo{Enable: ai.Enable, Nickname: ai.Nickname, Avatar: ai.Avatar, Abstract: ai.Abstract}
}

// ── 管理端配置 ──────────────────────────────────────────────────

// AdminSite 返回管理端的站点配置。
func (s *Service) AdminSite() conf.Site { return s.PublicSite() }

// AdminAI 返回管理端的 AI 配置，密钥用占位符遮蔽。
func (s *Service) AdminAI() conf.AI {
	ai := s.runtime.AI()
	ai.SecretKey = domain.SensitivePlaceholder
	return ai
}

// UpdateSite 覆盖站点配置。
func (s *Service) UpdateSite(site conf.Site) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	return s.runtime.UpdateSite(site)
}

// UpdateAI 覆盖 AI 配置。
//
// 密钥传回占位符表示"没改"，此时沿用当前值——否则前端每次保存都会把密钥写成 ******。
func (s *Service) UpdateAI(ai conf.AI) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	if ai.SecretKey == domain.SensitivePlaceholder {
		ai.SecretKey = s.runtime.AI().SecretKey
	}
	return s.runtime.UpdateAI(ai)
}

// ── 轮播图 ──────────────────────────────────────────────────────

// BannerInput 是创建/更新轮播图的入参。
type BannerInput struct {
	CoverImageID ctype.ID
	Href         string
	Show         bool
}

// ListBanners 分页查轮播图，并回填封面图片 ID。
func (s *Service) ListBanners(ctx context.Context, q domain.BannerQuery) ([]domain.Banner, bool, error) {
	rows, hasMore, err := s.banners.List(ctx, q)
	if err != nil {
		return nil, false, err
	}
	s.attachCoverImageIDs(ctx, rows)
	return rows, hasMore, nil
}

// CreateBanner 新建轮播图。
func (s *Service) CreateBanner(ctx context.Context, in BannerInput) (domain.Banner, error) {
	cover, err := s.resolveCover(ctx, in.CoverImageID.Uint64())
	if err != nil {
		return domain.Banner{}, err
	}
	banner := domain.Banner{
		Show:         in.Show,
		Cover:        cover,
		CoverImageID: in.CoverImageID,
		Href:         in.Href,
	}
	if err := s.banners.Create(ctx, &banner); err != nil {
		return domain.Banner{}, err
	}
	return banner, nil
}

// UpdateBanner 更新轮播图。
//
// 先解析封面再查存在性，与旧实现顺序一致：封面 ID 非法时优先报图片错误。
func (s *Service) UpdateBanner(ctx context.Context, id uint64, in BannerInput) error {
	cover, err := s.resolveCover(ctx, in.CoverImageID.Uint64())
	if err != nil {
		return err
	}
	if _, err := s.banners.Find(ctx, id); err != nil {
		return err
	}
	return s.banners.Update(ctx, id, cover, in.Href, in.Show)
}

// RemoveBanners 批量删除轮播图。
func (s *Service) RemoveBanners(ctx context.Context, ids []uint64) (int, error) {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	return s.banners.Delete(ctx, ids)
}

// resolveCover 由图片 ID 取封面地址。
func (s *Service) resolveCover(ctx context.Context, imageID uint64) (string, error) {
	if imageID == 0 {
		return "", apperr.New(apperr.CodeImageUnavailable)
	}
	if s.images == nil {
		return "", apperr.New(apperr.CodeImageUnavailable)
	}
	return s.images.URLByID(ctx, imageID)
}

// attachCoverImageIDs 批量回填封面图片 ID。
//
// 反查失败不影响列表本身：前端只会拿不到编辑回填的 ID。
func (s *Service) attachCoverImageIDs(ctx context.Context, rows []domain.Banner) {
	if len(rows) == 0 || s.images == nil {
		return
	}
	urls := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.Cover == "" {
			continue
		}
		if _, ok := seen[row.Cover]; ok {
			continue
		}
		seen[row.Cover] = struct{}{}
		urls = append(urls, row.Cover)
	}
	if len(urls) == 0 {
		return
	}
	idByURL, err := s.images.IDsByURLs(ctx, urls)
	if err != nil {
		s.logError("反查轮播图封面图片 ID 失败: %v", err)
		return
	}
	for i := range rows {
		if id, ok := idByURL[rows[i].Cover]; ok {
			rows[i].CoverImageID = ctype.NewID(id)
		}
	}
}

// normalizeIDs 去零去重。
func normalizeIDs(ids []uint64) []uint64 {
	out := make([]uint64, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// logError 写一条错误日志。
func (s *Service) logError(format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Errorf(format, args...)
}
