package io_util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gin-gonic/gin"
)

func GetBody(body *io.ReadCloser) ([]byte, error) {
	byteData, err := io.ReadAll(*body)
	if err != nil {
		return nil, err
	}

	// 内容阅后即焚，所以需要恢复请求体内容
	*body = io.NopCloser(bytes.NewBuffer(byteData))

	return byteData, nil
}

func ShouldBindJSONWithRecover(c *gin.Context, structPtr any) error {
	// 读取 body
	body, err := io.ReadAll(c.Request.Body)
	fmt.Println(string(body))
	if err != nil {
		return fmt.Errorf("请求体读取失败: %w", err)
	}

	// 绑定 JSON
	if err = json.Unmarshal(body, structPtr); err != nil {
		return fmt.Errorf("JSON绑定失败: %w", err)
	}

	// 恢复 body，供后续使用
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	return nil
}
