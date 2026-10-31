package domain

import (
	"context"

	"myblogx/conf"
)

// BannerRepository 是轮播图的持久化端口。
type BannerRepository interface {
	// List 按展示状态分页查询；hasMore 由多取一条判断。
	List(ctx context.Context, q BannerQuery) ([]Banner, bool, error)
	// Find 查一条轮播图。
	Find(ctx context.Context, id uint64) (Banner, error)
	// Create 新建轮播图。
	Create(ctx context.Context, b *Banner) error
	// Update 覆盖封面/链接/展示状态。
	Update(ctx context.Context, id uint64, cover, href string, show bool) error
	// Delete 批量删除，返回实际删除条数。
	Delete(ctx context.Context, ids []uint64) (int, error)
}

// ImageResolver 负责图片 ID 与 URL 的双向换算。
//
// 轮播图存的是 URL，但管理端编辑时要回填图片 ID；
// 图片表属于 media 模块，这里只声明需要的能力。
type ImageResolver interface {
	// URLByID 由图片 ID 取访问地址。
	URLByID(ctx context.Context, id uint64) (string, error)
	// IDsByURLs 批量由 URL 反查图片 ID。
	IDsByURLs(ctx context.Context, urls []string) (map[string]uint64, error)
}

// RuntimeConfig 是运行时站点配置的读写端口。
//
// 具体实现是 platform/config 的 RuntimeConfigService。
// 八个配置段都从这里走：站点模块是"配置"这件事的唯一归属，
// 其它模块只消费、不写。
type RuntimeConfig interface {
	// Site 返回当前生效的站点配置。
	Site() conf.Site
	// AI 返回当前生效的 AI 配置。
	AI() conf.AI
	// Mail 返回当前邮件（腾讯云 SES）配置。
	Mail() conf.Mail
	// Storage 返回当前对象存储配置。
	Storage() conf.QiNiu
	// OAuth 返回当前第三方登录配置。
	OAuth() conf.QQ
	// Log 返回当前日志配置。
	Log() conf.Logrus
	// Upload 返回当前上传策略。
	Upload() conf.Upload
	// Deco 返回当前装饰配置。
	Deco() conf.Deco

	// UpdateSite 覆盖站点配置。
	UpdateSite(site conf.Site) error
	// UpdateAI 覆盖 AI 配置。
	UpdateAI(ai conf.AI) error
	// UpdateMail 覆盖邮件配置。
	UpdateMail(mail conf.Mail) error
	// UpdateStorage 覆盖对象存储配置。
	UpdateStorage(storage conf.QiNiu) error
	// UpdateOAuth 覆盖第三方登录配置。
	UpdateOAuth(oauth conf.QQ) error
	// UpdateLog 覆盖日志配置。
	UpdateLog(log conf.Logrus) error
	// UpdateUpload 覆盖上传策略。
	UpdateUpload(upload conf.Upload) error
	// UpdateDeco 覆盖装饰配置。
	UpdateDeco(deco conf.Deco) error
}
