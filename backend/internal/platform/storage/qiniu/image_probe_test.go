package qiniu

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"myblogx/conf"

	_ "golang.org/x/image/bmp"

	"myblogx/internal/apperr"
)

// newProbeStorage 造一个只用来调探测逻辑的 Storage（AK/SK 是假的，不出网）。
func newProbeStorage(t *testing.T) *Storage {
	t.Helper()
	return New(conf.QiNiu{Enable: true, Bucket: "demo", AccessKey: "ak-for-test", SecretKey: "sk-for-test"})
}

// encodePNG 造一张 w×h 的 PNG；尺寸刻意不是 1×1，免得"碰巧对了"。
func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("造 PNG 失败: %v", err)
	}
	return buf.Bytes()
}

func TestSniffImageReadsPNGHeader(t *testing.T) {
	t.Parallel()
	info, err := sniffImage(bytes.NewReader(encodePNG(t, 8, 6)))
	if err != nil {
		t.Fatalf("按文件头识别 PNG 失败: %v", err)
	}
	if info.Format != "png" || info.Width != 8 || info.Height != 6 {
		t.Fatalf("识别结果不对: %+v", info)
	}
}

func TestSniffImageReadsJPEGHeader(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 120, 80)), nil); err != nil {
		t.Fatalf("造 JPEG 失败: %v", err)
	}
	info, err := sniffImage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("按文件头识别 JPEG 失败: %v", err)
	}
	if info.Format != "jpeg" || info.Width != 120 || info.Height != 80 {
		t.Fatalf("识别结果不对: %+v", info)
	}
}

func TestSniffImageReadsGIFHeader(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 40, 30), color.Palette{color.White}), nil); err != nil {
		t.Fatalf("造 GIF 失败: %v", err)
	}
	info, err := sniffImage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("按文件头识别 GIF 失败: %v", err)
	}
	if info.Format != "gif" || info.Width != 40 || info.Height != 30 {
		t.Fatalf("识别结果不对: %+v", info)
	}
}

// bmp 是上传白名单允许的格式，而标准库没有它的解码器 —— 这条用例盯住
// "兜底路径也必须认得 bmp"，否则 fop 一旦不可用，bmp 图片会整批传不上来。
func TestSniffImageReadsBMPHeader(t *testing.T) {
	t.Parallel()
	if _, err := sniffImage(bytes.NewReader(bmpHeaderBytes(16, 9))); err != nil {
		t.Fatalf("按文件头识别 BMP 失败: %v", err)
	}
}

// bmpHeaderBytes 只造 x/image/bmp 解码器真正会读的那一段头部（BITMAPINFOHEADER），
// 不造完整像素数据：DecodeConfig 不看像素。
func bmpHeaderBytes(w, h int) []byte {
	// 14 字节 FILEHEADER + 40 字节 BITMAPINFOHEADER
	head := make([]byte, 54)
	binary.LittleEndian.PutUint16(head[0:2], 0x4D42) // BM
	binary.LittleEndian.PutUint32(head[10:14], 54)   // 像素数据偏移
	binary.LittleEndian.PutUint32(head[14:18], 40)   // 头部大小
	binary.LittleEndian.PutUint32(head[18:22], uint32(w))
	binary.LittleEndian.PutUint32(head[22:26], uint32(h))
	binary.LittleEndian.PutUint16(head[26:28], 1)  // planes
	binary.LittleEndian.PutUint16(head[28:30], 24) // bpp
	return head
}

func TestSniffImageRejectsNonImageBytes(t *testing.T) {
	t.Parallel()
	_, err := sniffImage(strings.NewReader("not an image at all"))
	if err == nil {
		t.Fatal("不是图片却识别成功了")
	}
}

// 两条通道都失败时：用户看到的是 apperr 表里的中文，而内部原因留在错误链里。
//
// 这两件事必须同时成立：只保留中文等于把排查线索丢掉（旧实现正是把编码器内部
// 错误直接甩给用户，反过来倒错）；只保留内部错误则用户看不懂。
func TestImageInfoFailureKeepsFriendlyMessageAndCause(t *testing.T) {
	t.Parallel()

	err := apperr.Wrap(apperr.CodeStorageImageInfoInvalid, errors.New("invalid character '\\u0089'"))
	if msg := err.Error(); !strings.Contains(msg, "读取图片信息失败") {
		t.Fatalf("对外应是中文文案，实际: %q", msg)
	}
	if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), "invalid character") {
		t.Fatalf("内部原因必须留在错误链里供日志排查，实际: %v", cause)
	}
}

func TestImageInfoRequiresDomain(t *testing.T) {
	t.Parallel()

	storage := newProbeStorage(t)
	storage.cfg.Uri = ""
	if _, err := storage.ImageInfo("images/a.png"); !apperr.IsCode(err, apperr.CodeStorageDomainMissing) {
		t.Fatalf("没配域名时必须报 storage.domain_missing，实际: %v", err)
	}
}
