package app

import (
	"context"
	"strings"

	"myblogx/internal/apperr"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/imagedisplay"
)

// StyleProbeItem 是一条探活结果：某个槽位的地址与它的实际响应。
type StyleProbeItem struct {
	// Tier 是槽位名（original / cover / avatar / content / chat）。
	Tier string
	// StyleName 是该槽位配置的样式名（original 为空）。
	StyleName string
	// URL 是这次实际请求的地址。
	URL string
	// Status 是 HTTP 状态码（0 表示请求本身失败）。
	Status int
	// Size 是响应声明的字节数（-1 表示服务端没给 Content-Length）。
	Size int64
	// Error 非空表示这次探测失败的原因。
	Error string
}

// StyleProbeReport 是一次完整探活的结果。
type StyleProbeReport struct {
	// ObjectKey 是探活用的对象名。
	ObjectKey string
	// Items 依次是原图与四档样式。
	Items []StyleProbeItem
}

// ProbeImageStyles 对同一张图分别请求原图与四档样式地址，返回对照结果。
//
// 为什么值得专门做一个端点：七牛对"样式名不存在"的响应是 404，**不会回退原图**。
// 也就是说后台把样式名填错一个字，站内那一档的图片会全部裂掉，而七牛不会报任何错。
// 有了这个对照表，运营在保存后点一下就能看到"原图 200 / 2.3MB、cover 200 / 310KB"
// 或者"cover 404"，一眼能判断配置对不对。
func (s *Service) ProbeImageStyles(ctx context.Context, objectKey string) (*StyleProbeReport, error) {
	if s.storage == nil {
		return nil, apperr.New(apperr.CodeImageUnavailable)
	}
	cfg := s.cfg()
	key := strings.Trim(strings.TrimSpace(objectKey), "/")
	if key == "" {
		// 没指定对象：拿库里最新一张图，省得运营还要去翻 object key。
		images, _, err := s.repo.List(ctx, domain.Query{Page: 1, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(images) == 0 {
			return nil, apperr.NewWithMessage(apperr.CodeImageUnavailable,
				"图库里没有图片可用来探活，请手动填一个对象 key（形如 myblogx/images/20261102/xxxx）")
		}
		key = images[0].ObjectKey
	}

	display := displayConfigOf(cfg)
	report := &StyleProbeReport{ObjectKey: key}

	// 原图先探一次：它是"样式到底有没有生效"的参照系。
	original := display.PublicURL(key)
	report.Items = append(report.Items, probeOne(ctx, s, "original", "", original))

	tiers := []struct {
		tier    string
		purpose imagedisplay.Purpose
	}{
		{"cover", imagedisplay.PurposeCover},
		{"avatar", imagedisplay.PurposeAvatar},
		{"content", imagedisplay.PurposeContent},
		{"chat", imagedisplay.PurposeChat},
	}
	for _, tier := range tiers {
		style := display.StyleOf(tier.purpose)
		url := display.PublicURL(display.WithStyle(key, style))
		report.Items = append(report.Items, probeOne(ctx, s, tier.tier, style, url))
	}
	return report, nil
}

// probeOne 探测一条地址并组装成一行结果。
func probeOne(ctx context.Context, s *Service, tier, style, rawURL string) StyleProbeItem {
	item := StyleProbeItem{Tier: tier, StyleName: style, URL: rawURL}
	if rawURL == "" {
		item.Error = "地址为空：请检查对象存储的访问域名"
		return item
	}
	result := s.storage.Probe(ctx, rawURL)
	item.Status = result.Status
	item.Size = result.Size
	item.Error = result.Error
	return item
}

// displayConfigOf 把媒体运行配置翻译成展示派生配置（仅供探活使用）。
//
// 正常展示走的是装配层构造的那一个派生器；这里单独构造一份是因为探活需要
// "不带闸门、不带签名"的裸地址 —— 它要验的是存储侧有没有这个样式，
// 而不是"这张图能不能给读者看"。
func displayConfigOf(cfg domain.Config) imagedisplay.Config {
	return imagedisplay.Config{
		MediaBase: cfg.Uri,
		Prefix:    cfg.Prefix,
		Delimiter: cfg.StyleDelimiter,
		Styles: map[imagedisplay.Purpose]string{
			imagedisplay.PurposeCover:   cfg.StyleCover,
			imagedisplay.PurposeAvatar:  cfg.StyleAvatar,
			imagedisplay.PurposeContent: cfg.StyleContent,
			imagedisplay.PurposeChat:    cfg.StyleChat,
		},
	}
}
