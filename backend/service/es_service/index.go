package es_service

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
)

const esRequestTimeout = 15 * time.Second

// 创建索引
func CreateIndex(client *elasticsearch.Client, index, mapping string) error {
	// 构建创建索引的请求体
	req := bytes.NewBufferString(mapping)
	ctx, cancel := context.WithTimeout(context.Background(), esRequestTimeout)
	defer cancel()

	// 调用ES的Create Index API
	res, err := client.Indices.Create(
		index,
		client.Indices.Create.WithBody(req),
		client.Indices.Create.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("创建索引 %s 失败: %v", index, err)
	}
	defer res.Body.Close() // 必须关闭响应体，避免资源泄漏

	// 检查响应状态码
	if res.IsError() {
		return fmt.Errorf("创建索引 %s 失败，响应体: %s", index, res.String())
	}

	return nil
}

// 判断索引是否存在
func ExistsIndex(client *elasticsearch.Client, index string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), esRequestTimeout)
	defer cancel()

	res, err := client.Indices.Exists(
		[]string{index},
		client.Indices.Exists.WithContext(ctx),
	)
	if err != nil {
		return false, fmt.Errorf("检查索引 %s 是否存在失败: %v", index, err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case 200:
		return true, nil
	case 404:
		return false, nil
	default:
		return false, fmt.Errorf("检查索引 %s 是否存在失败，响应状态码: %d", index, res.StatusCode)
	}
}

// 删除索引
func DeleteIndex(client *elasticsearch.Client, index string) error {
	ctx, cancel := context.WithTimeout(context.Background(), esRequestTimeout)
	defer cancel()

	res, err := client.Indices.Delete(
		[]string{index},
		client.Indices.Delete.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("删除索引 %s 失败: %v", index, err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("删除索引 %s 失败，响应错误: %s", index, res.Status())
	}

	return nil
}

// 强制创建索引
func CreateIndexForce(client *elasticsearch.Client, index, mapping string) error {
	if exists, err := ExistsIndex(client, index); err != nil {
		return err
	} else if exists {
		if err := DeleteIndex(client, index); err != nil {
			return err
		}
	}
	return CreateIndex(client, index, mapping)
}

// EnsureIndex 确保索引存在；已存在时不做破坏性操作。
func EnsureIndex(client *elasticsearch.Client, index, mapping string) error {
	exists, err := ExistsIndex(client, index)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return CreateIndex(client, index, mapping)
}
