package hash_test

import (
	"bytes"
	"mime/multipart"
	"myblogx/utils/hash"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMd5Helpers(t *testing.T) {
	data := []byte("hello")
	want := "5d41402abc4b2a76b9719d911017c592"

	if got := hash.Md5(data); got != want {
		t.Fatalf("Md5 结果错误: %s", got)
	}

	f, err := os.CreateTemp("", "hash-*.txt")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		t.Fatalf("写临时文件失败: %v", err)
	}
	_ = f.Close()

	gotFile, err := hash.FileMd5(f.Name())
	if err != nil {
		t.Fatalf("FileMd5 失败: %v", err)
	}
	if gotFile != want {
		t.Fatalf("FileMd5 结果错误: %s", gotFile)
	}
}

func TestFileHeaderMd5(t *testing.T) {
	content := []byte("multipart-content")
	fileHeader := makeMultipartFileHeader(t, "demo.txt", content)

	got, err := hash.FileHeaderMd5(fileHeader)
	if err != nil {
		t.Fatalf("FileHeaderMd5 失败: %v", err)
	}
	if got != hash.Md5(content) {
		t.Fatalf("FileHeaderMd5 结果错误: %s", got)
	}
}

func makeMultipartFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile 失败: %v", err)
	}
	if _, err = part.Write(content); err != nil {
		t.Fatalf("写 multipart 内容失败: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("关闭 multipart writer 失败: %v", err)
	}

	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err = req.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("ParseMultipartForm 失败: %v", err)
	}

	return req.MultipartForm.File["file"][0]
}
