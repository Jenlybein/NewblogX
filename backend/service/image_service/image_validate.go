package image_service

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"gorm.io/gorm/utils"

	"github.com/gabriel-vasile/mimetype"
)

// GetImageSuffix 获取图片后缀（不包含前缀点）。
func GetImageSuffix(str string) string {
	suffix := strings.ToLower(filepath.Ext(str))
	if strings.HasPrefix(suffix, ".") {
		return suffix[1:]
	}
	return ""
}

// VerifyImageFormat 校验 multipart 上传文件是否为真实图片。
func VerifyImageFormat(whitelist []string, fileHeader *multipart.FileHeader) error {
	suffix := GetImageSuffix(fileHeader.Filename)
	if suffix == "" {
		return errors.New("图片名中的格式后缀错误")
	}
	if !utils.Contains(whitelist, suffix) {
		return fmt.Errorf("图片后缀 %s 不在服务器允许上传的图片格式白名单中", suffix)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("图片格式验证时，创建文件读取器失败：%w", err)
	}
	defer file.Close()

	rs, ok := file.(io.ReadSeeker)
	if !ok {
		return errors.New("上传的文件无法进行完整校验")
	}

	mt, err := mimetype.DetectReader(rs)
	if err != nil {
		return err
	}

	if suffix == "jpg" {
		suffix = "jpeg"
	}
	mime := fmt.Sprintf("image/%s", suffix)
	if !mt.Is(mime) {
		return errors.New("非服务器允许上传的图片格式")
	}

	if _, err = rs.Seek(0, io.SeekStart); err != nil {
		return err
	}

	cfg, _, err := image.DecodeConfig(rs)
	if err != nil {
		return fmt.Errorf("图片结构校验失败：%w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 10000 || cfg.Height > 10000 {
		return errors.New("图片尺寸不合规或图片尺寸过大")
	}

	return nil
}

// VerifyImageBytes 用于校验已经拿到内存中的图片内容，适合对象存储回调后的二次验收。
func VerifyImageBytes(whitelist []string, filename string, data []byte) (mime string, width, height int, err error) {
	suffix := GetImageSuffix(filename)
	if suffix == "" {
		return "", 0, 0, errors.New("图片名中的格式后缀错误")
	}
	if !utils.Contains(whitelist, suffix) {
		return "", 0, 0, fmt.Errorf("图片后缀 %s 不在服务器允许上传的图片格式白名单中", suffix)
	}

	mt := mimetype.Detect(data)
	if suffix == "jpg" {
		suffix = "jpeg"
	}
	expectMime := fmt.Sprintf("image/%s", suffix)
	if !mt.Is(expectMime) {
		return "", 0, 0, errors.New("非服务器允许上传的图片格式")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, fmt.Errorf("图片结构校验失败：%w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 10000 || cfg.Height > 10000 {
		return "", 0, 0, errors.New("图片尺寸不合规或图片尺寸过大")
	}

	return mt.String(), cfg.Width, cfg.Height, nil
}
