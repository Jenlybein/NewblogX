// 图片信息探测：先走七牛 fop（快），失败再按文件头识别（稳）。
//
// 为什么需要这一层：`?imageInfo` 是七牛「图片处理」能力，依赖空间/域名配置，
// 且**必须作为首个 query 参数**出现。七牛不认这个 fop 时不会报错，而是把**原图**
// 直接回给客户端（HTTP 200 + PNG/JPEG 字节）；把图片字节当 JSON 解就会得到
// `invalid character '\u0089' looking for beginning of value`（0x89 是 PNG 头），
// 上传因此被判失败，提示与真实原因毫无关系（2026-11-26 头像上传即此）。
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

	// 注册文件头解码器。png / jpeg / gif 在标准库里，bmp 只在 x/image 里，
	// 而 bmp 是上传白名单允许的格式，所以这条依赖是必要的。
	_ "golang.org/x/image/bmp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/qiniu/go-sdk/v7/storage"
)

const (
	// probeMaxBytes 是兜底探测时最多读取的字节数。
	//
	// `image.DecodeConfig` 只读到能判定尺寸为止（PNG 十几字节、JPEG 要到 SOF 段），
	// 这个上限只是防止极端文件把整张图读进内存；白名单内的图片都远小于它。
	probeMaxBytes = 8 << 20

	// probeURLExpire 是探测用签名地址的有效期。
	//
	// 探测紧跟在上传回调之后发生，几秒内就会用完；给 3 分钟是为了覆盖七牛回调
	// 与超时扫描同时补跑的情况，同时不把有效期写死在两处（两个探测共用）。
	probeURLExpire = 3 * time.Minute
)

// ImageInfo 读取图片的格式与像素尺寸。
//
// 顺序：fop（一小段 JSON）→ 兜底（取对象字节按文件头识别）。
// 两条都失败才报错，且报的是给人看的中文（文案在 apperr 表里，调用点只写码）。
func (s *Storage) ImageInfo(key string) (ImageInfo, error) {
	domain := s.mediaBase()
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
	// 两条通道的原因都留在错误链里：上层日志会打出来，用户只看到 apperr 的中文文案。
	return ImageInfo{}, apperr.Wrap(
		apperr.CodeStorageImageInfoInvalid,
		fmt.Errorf("读取图片信息失败（fop: %v；字节: %v）", fopErr, sniffErr),
	)
}

// imageInfoByFop 走七牛的 `?imageInfo`，返回它给的格式与尺寸。
//
// fop 必须同时满足两件事：作为**首个** query 参数出现，且被签名覆盖。
// `MakePrivateURLv2WithQueryString` 把 query 一起签进 token，两者一次满足；
// 旧写法"先签 `?e=` 再追加 `&imageInfo`"在七牛侧不会被当成 fop 解析。
func (s *Storage) imageInfoByFop(domain, key string) (ImageInfo, error) {
	url := s.signedProbeURL(domain, key, "imageInfo")
	resp, err := getRuntime(s.cfg).httpClient.Get(url)
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
	url := s.signedProbeURL(domain, key, "")
	resp, err := getRuntime(s.cfg).httpClient.Get(url)
	if err != nil {
		return ImageInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ImageInfo{}, fmt.Errorf("下载对象 status=%d", resp.StatusCode)
	}
	return sniffImage(io.LimitReader(resp.Body, probeMaxBytes))
}

// sniffImage 从字节流读出格式与尺寸（纯函数，便于直接喂各种文件头做用例）。
func sniffImage(body io.Reader) (ImageInfo, error) {
	cfg, format, err := image.DecodeConfig(body)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("按文件头识别图片: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ImageInfo{}, fmt.Errorf("图片尺寸无效: %d×%d", cfg.Width, cfg.Height)
	}
	return ImageInfo{Format: format, Width: cfg.Width, Height: cfg.Height}, nil
}

// signedProbeURL 生成带下载凭证的探测地址；query 为空时就是对象本身。
func (s *Storage) signedProbeURL(domain, key, query string) string {
	deadline := time.Now().Add(probeURLExpire).Unix()
	if query == "" {
		return storage.MakePrivateURLv2(getRuntime(s.cfg).mac, domain, key, deadline)
	}
	return storage.MakePrivateURLv2WithQueryString(getRuntime(s.cfg).mac, domain, key, query, deadline)
}
