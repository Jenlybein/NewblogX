package hash

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"os"
)

// 计算传入 fileHeader 的 md5 值
func FileHeaderMd5(fileHeader *multipart.FileHeader) (string, error) {
	f, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// 计算传入 file 的 md5 值
func FileMd5(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// 计算传入 data 的 md5 值
func Md5(data []byte) string {
	h := md5.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// 做SHA256哈希
func SHA256Hash(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}
