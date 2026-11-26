// Package qiniu 的图片信息探测：先走 fop（快），失败再按文件头识别（稳）。
//
// 为什么需要这一层：`?imageInfo` 是七牛「图片处理」能力，依赖空间/域名配置；
// 私有空间还要求 fop 一起进签名。任一处不满足时七牛不会报错，而是把**原图**
// 直接回给客户端（HTTP 200 + PNG/JPEG 字节）。把图片字节当 JSON 解，就会得到
// `invalid character '\u0089' looking for beginning of value`（0x89 是 PNG 文件头）
// —— 上传因此被判失败，而提示与真实原因毫无关系（2026-11-26 头像上传即此）。
//
// 所以这里不再把"能不能探到图片信息"押在七牛那侧的一个可选能力上。
package qiniu

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"

	"myblogx/internal/apperr"

	// 注册文件头解码器：兜底探测要能在不依赖七牛侧能力的前提下读出格式与尺寸。
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/qiniu/go-sdk/v7/storage"
)

// maxProbeBytes 是兜底探测时最多读取的字节数。
//
// `image.DecodeConfig` 只读到能判定尺寸为止（PNG 十几字节、JPEG 要到 SOF 段），
// 这个上限只是防止极端文件把整张图读进内存；白名单内的图片都远小于它。
const maxProbeBytes = 8 << 20

// ImageInfo 读取图片的格式与像素尺寸。
//
// 顺序：fop（一小段 JSON）→ 兜底（取对象字节按文件头识别）。
// 两条都失败才报错，且报的是给人看的中文，不把编码器的内部错误抛给用户。
func (s *Storage) ImageInfo(key string) (ImageInfo, error) {
	domain := s.imageInfoBase
	if domain == "" {
		domain = s.mediaBase()
	}
	if domain == "" {
		return ImageInfo{}, apperr.New(apperr.CodeStorageDomainMissing)
	}

	info, fopErr := s.imageInfoByFop(domain, key)
	if fopErr == nil {
		return info, nil
	}
	info, sniffErr := s.sniffImageInfo(domain, key)
	if sniffErr == nil {
		return info, nil
	}
	return ImageInfo{}, apperr.Wrap(
		apperr.CodeStorageImageInfoInvalid,
		fmt.Errorf("qiniu: 读取图片信息失败（fop: %v；字节: %v）", fopErr, sniffErr),
	)
}

// imageInfoByFop 走七牛的 `?imageInfo`，返回它给的格式与尺寸。
//
// fop 必须参与签名：`MakePrivateURLv2WithQueryString` 把 query 一起签进 token，
// 私有空间才认这个 fop；旧写法"先签再追加 `&imageInfo`"在私有空间下会被忽略，
// 拿回的是原图字节。
func (s *Storage) imageInfoByFop(domain, key string) (ImageInfo, error) {
	runtime := getRuntime(s.cfg)
	deadline := time.Now().Add(3 * time.Minute).Unix()
	url := storage.MakePrivateURLv2WithQueryString(runtime.mac, domain, key, "imageInfo", deadline)

	resp, err := runtime.httpClient.Get(url)
	if err != nil {
		return ImageInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ImageInfo{}, fmt.Errorf("imageInfo status=%d", resp.StatusCode)
	}
	var result ImageInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ImageInfo{}, fmt.Errorf("解析 imageInfo 响应: %w", err)
	}
	if result.Format == "" || result.Width <= 0 || result.Height <= 0 {
		return ImageInfo{}, fmt.Errorf("imageInfo 字段不完整: %+v", result)
	}
	return result, nil
}

// sniffImageInfo 取回对象字节，按文件头识别格式与尺寸。
//
// 这是"不依赖七牛图片处理"的那条通道：只要对象能被读到，就能得到格式与尺寸，
// 白名单与比例校验照旧在真实字节上做。
func (s *Storage) sniffImageInfo(domain, key string) (ImageInfo, error) {
	runtime := getRuntime(s.cfg)
	deadline := time.Now().Add(3 * time.Minute).Unix()
	url := storage.MakePrivateURLv2(runtime.mac, domain, key, deadline)

	resp, err := runtime.httpClient.Get(url)
	if err != nil {
		return ImageInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ImageInfo{}, fmt.Errorf("下载对象 status=%d", resp.StatusCode)
	}
	cfg, format, err := image.DecodeConfig(io.LimitReader(resp.Body, maxProbeBytes))
	if err != nil {
		return ImageInfo{}, fmt.Errorf("按文件头识别图片: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ImageInfo{}, fmt.Errorf("图片尺寸无效: %d×%d", cfg.Width, cfg.Height)
	}
	return ImageInfo{Format: format, Width: cfg.Width, Height: cfg.Height, Size: resp.ContentLength}, nil
}
