// Package apptest 提供站点模块的内存替身。
package apptest

import (
	"context"
	"sort"
	"sync"
	"time"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/app"
	"myblogx/internal/site/domain"
)

// Banners 是轮播图的内存实现。
//
// 取景参数单独存一份（crops），模拟"一对一附加表"：banner 行里没有取景，
// 读的时候按 id 去取景表里找，找不到就是缺省（居中、缩放 1）。
type Banners struct {
	mu    sync.Mutex
	rows  []domain.Banner
	crops map[uint64]domain.Crop
	next  uint64
}

var _ domain.BannerRepository = (*Banners)(nil)

// NewBanners 构造空仓库。
func NewBanners() *Banners {
	return &Banners{next: 1, crops: make(map[uint64]domain.Crop)}
}

// List 实现 domain.BannerRepository。
func (b *Banners) List(_ context.Context, q domain.BannerQuery) ([]domain.Banner, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	q = q.Normalize()
	var hit []domain.Banner
	for _, row := range b.rows {
		if row.Show != q.Show {
			continue
		}
		hit = append(hit, b.withCrop(row))
	}
	sort.SliceStable(hit, func(i, j int) bool { return hit[i].CreatedAt.After(hit[j].CreatedAt) })
	offset := q.Offset()
	if offset >= len(hit) {
		return nil, false, nil
	}
	hit = hit[offset:]
	hasMore := len(hit) > q.Limit
	if hasMore {
		hit = hit[:q.Limit]
	}
	return hit, hasMore, nil
}

// Find 实现 domain.BannerRepository。
func (b *Banners) Find(_ context.Context, id uint64) (domain.Banner, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, row := range b.rows {
		if row.ID.Uint64() == id {
			return b.withCrop(row), nil
		}
	}
	return domain.Banner{}, apperr.New(apperr.CodeBannerNotFound)
}

// Create 实现 domain.BannerRepository。
func (b *Banners) Create(_ context.Context, banner *domain.Banner) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if banner.ID.IsZero() {
		banner.ID = ctype.NewID(b.next)
		b.next++
	}
	now := time.Now()
	banner.CreatedAt = now
	banner.UpdatedAt = now
	b.crops[banner.ID.Uint64()] = banner.Crop
	stored := *banner
	b.rows = append(b.rows, stored)
	return nil
}

// Update 实现 domain.BannerRepository。crop 为 nil 表示没传取景：保留既有记录。
func (b *Banners) Update(_ context.Context, id uint64, cover, href string, show bool, crop *domain.Crop) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.rows {
		if b.rows[i].ID.Uint64() == id {
			b.rows[i].Cover = cover
			b.rows[i].Href = href
			b.rows[i].Show = show
			b.rows[i].UpdatedAt = time.Now()
			if crop != nil {
				b.crops[id] = *crop
			}
			return nil
		}
	}
	return apperr.New(apperr.CodeBannerNotFound)
}

// withCrop 按 id 补上取景参数；没有记录时用缺省值。
//
// 调用方必须已持有锁。
func (b *Banners) withCrop(row domain.Banner) domain.Banner {
	if crop, ok := b.crops[row.ID.Uint64()]; ok {
		row.Crop = crop
		return row
	}
	row.Crop = domain.DefaultCrop
	return row
}

// DropCrop 删掉一张图的取景记录，模拟"取景表里没有这一行的历史图"。
func (b *Banners) DropCrop(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.crops, id)
}

// Delete 实现 domain.BannerRepository。
func (b *Banners) Delete(_ context.Context, ids []uint64) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	want := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	kept := make([]domain.Banner, 0, len(b.rows))
	count := 0
	for _, row := range b.rows {
		if _, ok := want[row.ID.Uint64()]; ok {
			count++
			continue
		}
		kept = append(kept, row)
	}
	b.rows = kept
	return count, nil
}

// All 返回全部轮播图（测试断言用）。
func (b *Banners) All() []domain.Banner {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]domain.Banner(nil), b.rows...)
}

// Images 是图片换算的内存实现。
type Images struct {
	mu       sync.Mutex
	urlByID  map[uint64]string
	idByURL  map[string]uint64
	unusable map[uint64]struct{}
}

var _ domain.ImageResolver = (*Images)(nil)

// NewImages 构造空换算表。
func NewImages() *Images {
	return &Images{
		urlByID:  make(map[uint64]string),
		idByURL:  make(map[string]uint64),
		unusable: make(map[uint64]struct{}),
	}
}

// Put 登记一张可用图片。
func (i *Images) Put(id uint64, url string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.urlByID[id] = url
	i.idByURL[url] = id
}

// MarkUnusable 把图片标成不可用（已删除/审核不通过）。
func (i *Images) MarkUnusable(id uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.unusable[id] = struct{}{}
	delete(i.urlByID, id)
}

// URLByID 实现 domain.ImageResolver。
func (i *Images) URLByID(_ context.Context, id uint64) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	url, ok := i.urlByID[id]
	if !ok {
		return "", apperr.New(apperr.CodeImageUnavailable)
	}
	return url, nil
}

// IDsByURLs 实现 domain.ImageResolver。
func (i *Images) IDsByURLs(_ context.Context, urls []string) (map[string]uint64, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make(map[string]uint64, len(urls))
	for _, url := range urls {
		if id, ok := i.idByURL[url]; ok {
			out[url] = id
		}
	}
	return out, nil
}

// Runtime 是运行时配置的内存实现。
type Runtime struct {
	mu      sync.Mutex
	site    conf.Site
	ai      conf.AI
	mail    conf.Mail
	storage conf.QiNiu
	oauth   conf.QQ
	log     conf.Logrus
	upload  conf.Upload
}

var _ domain.RuntimeConfig = (*Runtime)(nil)

// NewRuntime 构造初始配置。
func NewRuntime(site conf.Site, ai conf.AI) *Runtime { return &Runtime{site: site, ai: ai} }

// Site 实现 domain.RuntimeConfig。
func (r *Runtime) Site() conf.Site {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.site
}

// AI 实现 domain.RuntimeConfig。
func (r *Runtime) AI() conf.AI {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ai
}

// UpdateSite 实现 domain.RuntimeConfig。
func (r *Runtime) UpdateSite(site conf.Site) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.site = site
	return nil
}

// UpdateAI 实现 domain.RuntimeConfig。
func (r *Runtime) UpdateAI(ai conf.AI) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ai = ai
	return nil
}

// Mail 实现 domain.RuntimeConfig。
func (r *Runtime) Mail() conf.Mail {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mail
}

// Storage 实现 domain.RuntimeConfig。
func (r *Runtime) Storage() conf.QiNiu {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storage
}

// OAuth 实现 domain.RuntimeConfig。
func (r *Runtime) OAuth() conf.QQ {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.oauth
}

// Log 实现 domain.RuntimeConfig。
func (r *Runtime) Log() conf.Logrus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.log
}

// Upload 实现 domain.RuntimeConfig。
func (r *Runtime) Upload() conf.Upload {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.upload
}

// UpdateMail 实现 domain.RuntimeConfig。
func (r *Runtime) UpdateMail(mail conf.Mail) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mail = mail
	return nil
}

// UpdateStorage 实现 domain.RuntimeConfig。
func (r *Runtime) UpdateStorage(storage conf.QiNiu) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storage = storage
	return nil
}

// UpdateOAuth 实现 domain.RuntimeConfig。
func (r *Runtime) UpdateOAuth(oauth conf.QQ) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.oauth = oauth
	return nil
}

// UpdateLog 实现 domain.RuntimeConfig。
func (r *Runtime) UpdateLog(log conf.Logrus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = log
	return nil
}

// UpdateUpload 实现 domain.RuntimeConfig。
func (r *Runtime) UpdateUpload(upload conf.Upload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upload = upload
	return nil
}

// Env 打包一组替身。
type Env struct {
	Banners *Banners
	Images  *Images
	Runtime *Runtime
	Service *app.Service
}

// NewEnv 用给定配置构造一整套替身。
func NewEnv(site conf.Site, ai conf.AI, version, qqURL string) *Env {
	banners := NewBanners()
	images := NewImages()
	runtime := NewRuntime(site, ai)
	return &Env{
		Banners: banners,
		Images:  images,
		Runtime: runtime,
		Service: app.New(banners, images, runtime, version, func() string { return qqURL }, nil),
	}
}

// NewDefaultService 用空配置组装服务（路由表测试用）。
func NewDefaultService() *app.Service {
	return NewEnv(conf.Site{}, conf.AI{}, "test", "").Service
}
