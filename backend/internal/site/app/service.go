// Package app 是站点模块的用例层。
package app

import (
	"context"
	"fmt"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
	"myblogx/internal/site/domain"
)

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}

// SEO 是站点 SEO 摘要（给前端做 SSR 用）。
//
// 这里曾经还有 SiteTitle / Logo：它们的来源是 site_info.title / logo，而那两个
// 字段随 #192 一起删除，失去来源的它们也一并删掉。站点标题回到读者站的品牌常量，
// favicon 仍由 Icon（project.icon）供给。
type SEO struct {
	ProjectTitle string
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
	aiReady func() bool
}

// New 构造站点服务。
//
// aiReady 是「AI 现在能不能真的用」的现读判定（总闸开着且至少有一条启用的模型
// 配置）。它只用于保存前的校验：开「发布质量闸门」这个动作本身要求 AI 可用，
// 否则新站点上线第一天就是"所有投稿都被拒"（票 306 / ADR-0032）。
// 传 nil 表示不校验（只有测试会这样），生产装配必须给真实现。
func New(
	banners domain.BannerRepository,
	images domain.ImageResolver,
	runtime domain.RuntimeConfig,
	version string,
	qqURL func() string,
	logger Logger,
	aiReady func() bool,
) *Service {
	return &Service{
		banners: banners,
		images:  images,
		runtime: runtime,
		version: version,
		qqURL:   qqURL,
		logger:  logger,
		aiReady: aiReady,
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
		ProjectTitle: site.Project.Title,
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

// AdminAI 返回管理端的 AI 配置。
//
// 这一段只剩站点级设置（总闸、展示信息、输入限制）：模型密钥与接口地址已经
// 搬到 ai_provider_models 表，由 AI 模块自己的接口管理并做遮蔽。
// 所以这里不再需要"遮蔽 + 占位符回滚"那一套。
func (s *Service) AdminAI() conf.AI { return s.runtime.AI() }

// UpdateSite 覆盖站点配置。
//
// 「开质量闸门」这个动作本身要求 AI 可用（票 306 / ADR-0032）：闸门是失败关闭的，
// 而 AI 默认是关闭的，两者叠在一起就是"新站点上线第一天所有投稿都被拒"。
// 因此把失败面从运行期挪到配置期——保存不了，这个状态就不存在。
func (s *Service) UpdateSite(site conf.Site) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	if site.Article.QualityGate.Enabled && s.aiReady != nil && !s.aiReady() {
		return apperr.New(apperr.CodeArticleQualityGateRequiresAI)
	}
	return s.runtime.UpdateSite(site)
}

// UpdateAI 覆盖站点级 AI 设置。
//
// 每日上限在保存时校验（ADR-0038）：0 是合法值（不限），负数与超过上界的值
// 一律拒绝。这里的拒绝是**否决保存**而不是静默回落 —— 后台是唯一写入口，
// 悄悄改掉管理员填的数字只会让人以为改成功了。
func (s *Service) UpdateAI(ai conf.AI) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	if err := validateAIDailyLimits(ai.DailyLimits); err != nil {
		return err
	}
	return s.runtime.UpdateAI(ai)
}

// validateAIDailyLimits 校验七个场景的每日上限。
func validateAIDailyLimits(limits conf.DailyLimits) error {
	fields := []struct {
		name  string
		value int
	}{
		{"scoring", limits.Scoring},
		{"metainfo", limits.Metainfo},
		{"overwrite", limits.Overwrite},
		{"diagnose", limits.Diagnose},
		{"search_rewrite", limits.SearchRewrite},
		{"search_answer", limits.SearchAnswer},
		{"moderation", limits.Moderation},
	}
	for _, f := range fields {
		if f.value < 0 || f.value > config.MaxAIDailyLimit {
			return apperr.Wrap(apperr.CodeSiteAIDailyLimitInvalid,
				fmt.Errorf("daily_limits.%s=%d", f.name, f.value))
		}
	}
	return nil
}

// AdminMail 返回管理端的邮件配置，密钥用占位符遮蔽。
func (s *Service) AdminMail() conf.Mail {
	mail := s.runtime.Mail()
	mail.SecretKey = domain.SensitivePlaceholder
	return mail
}

// UpdateMail 覆盖邮件配置。
//
// 密钥的三种语义（占位符 = 没改 / 空串 = 真清除 / 其它 = 覆盖）由生效值模块的
// ResolveSensitive 统一判定，不在这里再写一份分支。
func (s *Service) UpdateMail(mail conf.Mail) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	mail.SecretKey = config.ResolveSensitive(mail.SecretKey, s.runtime.Mail().SecretKey)
	return s.runtime.UpdateMail(mail)
}

// AdminStorage 返回管理端的对象存储配置，密钥用占位符遮蔽。
func (s *Service) AdminStorage() conf.QiNiu {
	storage := s.runtime.Storage()
	storage.SecretKey = domain.SensitivePlaceholder
	return storage
}

// UpdateStorage 覆盖对象存储配置。
func (s *Service) UpdateStorage(storage conf.QiNiu) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	storage.SecretKey = config.ResolveSensitive(storage.SecretKey, s.runtime.Storage().SecretKey)
	return s.runtime.UpdateStorage(storage)
}

// AdminOAuth 返回管理端的第三方登录配置，AppKey 用占位符遮蔽。
func (s *Service) AdminOAuth() conf.QQ {
	oauth := s.runtime.OAuth()
	oauth.AppKey = domain.SensitivePlaceholder
	return oauth
}

// UpdateOAuth 覆盖第三方登录配置。
func (s *Service) UpdateOAuth(oauth conf.QQ) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	oauth.AppKey = config.ResolveSensitive(oauth.AppKey, s.runtime.OAuth().AppKey)
	return s.runtime.UpdateOAuth(oauth)
}

// AdminLog 返回管理端的日志配置。
func (s *Service) AdminLog() conf.Logrus { return s.runtime.Log() }

// UpdateLog 覆盖日志配置，但**只接受可以在后台改的那部分**。
//
// App / Dir / WALDir / StdoutFormat 是进程级配置：日志写到哪个目录、
// 用什么格式、服务名是什么，都由部署时的 settings.yaml 决定。
// 让后台改它们会出两种事故：
//   - 改了 Dir 而清理任务仍按旧目录算保留窗口，等于日志无人清理；
//   - 改了 Dir 之后容器里的路径不存在，写入直接失败，而失败日志也一起消失。
//
// 所以这几个字段一律用当前值覆盖回去，前端即使提交了也不生效。
func (s *Service) UpdateLog(log conf.Logrus) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	current := s.runtime.Log()
	log.App = current.App
	log.Dir = current.Dir
	log.WALDir = current.WALDir
	log.StdoutFormat = current.StdoutFormat
	return s.runtime.UpdateLog(log)
}

// AdminUpload 返回管理端的上传策略。
func (s *Service) AdminUpload() conf.Upload { return s.runtime.Upload() }

// UpdateUpload 覆盖上传策略。
func (s *Service) UpdateUpload(upload conf.Upload) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	return s.runtime.UpdateUpload(upload)
}

// PublicDeco 返回读者站要的装饰配置。
//
// 这段没有任何敏感字段，所以公开读与管理读返回同一份。
func (s *Service) PublicDeco() conf.Deco { return s.runtime.Deco() }

// AdminImageGC 返回管理端的孤儿图回收运行配置。
func (s *Service) AdminImageGC() conf.ImageGC { return s.runtime.ImageGC() }

// UpdateImageGC 覆盖孤儿图回收运行配置。
func (s *Service) UpdateImageGC(imageGC conf.ImageGC) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	return s.runtime.UpdateImageGC(imageGC)
}

// AdminDeco 返回管理端的装饰配置。
func (s *Service) AdminDeco() conf.Deco { return s.PublicDeco() }

// UpdateDeco 覆盖装饰配置。
//
// 写入前先 Normalize：越界钳到边界、缺省补默认值、非法值回落，**不报错**。
// 钳位放在用例层而不是 HTTP 层，是为了让"从哪条路径写进来"都受同一道防线
// 保护 —— 将来若多了导入配置的入口，也不会绕过它。
func (s *Service) UpdateDeco(deco conf.Deco) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	return s.runtime.UpdateDeco(deco.Normalize())
}

// ── 轮播图 ──────────────────────────────────────────────────────

// BannerInput 是创建/更新轮播图的入参。
type BannerInput struct {
	CoverImageID ctype.ID
	Title        string
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
		Title:        domain.NormalizeTitle(in.Title),
	}
	if err := s.banners.Create(ctx, &banner); err != nil {
		return domain.Banner{}, err
	}
	return banner, nil
}

// UpdateBanner 更新轮播图。
//
// 先解析封面再查存在性，与旧实现顺序一致：封面 ID 非法时优先报图片错误。
// 封面在上传前已按 16:9 定形，更新只有"整体替换"一种语义。
func (s *Service) UpdateBanner(ctx context.Context, id uint64, in BannerInput) error {
	cover, err := s.resolveCover(ctx, in.CoverImageID.Uint64())
	if err != nil {
		return err
	}
	if _, err := s.banners.Find(ctx, id); err != nil {
		return err
	}
	return s.banners.Update(ctx, id, domain.NormalizeTitle(in.Title), cover, in.Href, in.Show)
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
