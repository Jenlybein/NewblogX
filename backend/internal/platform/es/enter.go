package es_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
)

// 通用响应结构体
type ESResponse struct {
	Success bool   // 操作是否成功
	Msg     string // 提示信息
	Data    any    // 响应数据（文档ID/列表/版本号等）
}

// --- 通用工具函数 ---

// 安全关闭响应体，避免资源泄漏
func closeResponse(res *esapi.Response) {
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
}

// 错误处理，提取 ES 错误信息
func handleError(res *esapi.Response) error {
	var errResp map[string]any
	if decodeErr := json.NewDecoder(res.Body).Decode(&errResp); decodeErr != nil {
		return fmt.Errorf("解析错误响应失败: %v，状态: %s", decodeErr, res.Status())
	}

	errorReason := ""

	// 尝试从 ES 标准错误对象中提取 reason
	if errorObj, ok := errResp["error"].(map[string]any); ok {
		if reason, ok := errorObj["reason"].(string); ok {
			errorReason = reason
		}
		// 进一步检查是否有更深层的错误原因（如 causado_by）
		if causedBy, ok := errorObj["caused_by"].(map[string]any); ok {
			if cbReason, ok := causedBy["reason"].(string); ok {
				errorReason = fmt.Sprintf("%s (原因: %s)", errorReason, cbReason)
			}
		}
	}

	// 如果没能提取到有效 Reason，根据状态码提供常见解释
	if errorReason == "" {
		switch res.StatusCode {
		case 400:
			errorReason = "请求参数错误"
		case 401:
			errorReason = "未授权或凭证失效"
		case 403:
			errorReason = "权限不足，拒绝访问"
		case 404:
			errorReason = "资源未找到"
		case 409:
			errorReason = "版本冲突或文档已存在"
		default:
			errorReason = "未知错误"
		}
	}

	return fmt.Errorf("%s (状态码: %d)", errorReason, res.StatusCode)
}

// 执行ES请求，处理错误响应
func doRequest(client *elasticsearch.Client, req esapi.Request) (res *esapi.Response, err error) {
	res, err = req.Do(context.Background(), client)
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		defer closeResponse(res)
		return nil, handleError(res)
	}
	return res, nil
}

// 泛型解析响应体
func decodeResponse(body io.ReadCloser) (map[string]any, error) {
	var target map[string]any
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	err := decoder.Decode(&target)
	return target, err
}

// --- 批量操作相关常量 ---

const (
	ActionCreate = "create" // 创建动作
	ActionUpdate = "update" // 更新动作
	ActionDelete = "delete" // 删除动作
	ActionIndex  = "index"  // 索引动作
)

// --- 批量操作相关结构体 ---

// BulkRequest 用于批量发送多个请求
type BulkRequest struct {
	Action   string                 // 批量操作的动作类型
	Index    string                 // 索引名称
	Type     string                 // 文档类型
	ID       string                 // 文档ID
	Parent   string                 // 父文档ID
	Pipeline string                 // 处理管道
	Data     map[string]interface{} // 请求数据
}

// BulkResponse 是批量请求的响应
type BulkResponse struct {
	Took   int                            `json:"took"`   // 执行耗时（毫秒）
	Errors bool                           `json:"errors"` // 是否有错误
	Items  []map[string]*BulkResponseItem `json:"items"`  // 批量操作结果项
}

// BulkResponseItem 是批量响应中的项目
type BulkResponseItem struct {
	Index   string          `json:"_index"`   // 索引名称
	Type    string          `json:"_type"`    // 文档类型
	ID      string          `json:"_id"`      // 文档ID
	Version int             `json:"_version"` // 版本号
	Status  int             `json:"status"`   // 状态码
	Error   json.RawMessage `json:"error"`    // 错误信息
	Found   bool            `json:"found"`    // 是否找到
}

// Mapping 表示ES映射
type Mapping map[string]struct {
	Mappings map[string]struct {
		Properties map[string]struct {
			Type   string      `json:"type"`   // 字段类型
			Fields interface{} `json:"fields"` // 字段定义
		} `json:"properties"` // 属性映射
	} `json:"mappings"` // 映射定义
}

// --- 业务操作封装 ---

// 创建通用文档（泛型）

// Search 分页查询封装

// SearchBody 允许上层直接传入完整搜索请求体。
// 适合需要自定义 size、track_total_hits 或 has_more 语义的读服务。
func SearchBody(client *elasticsearch.Client, index string, body map[string]any) ESResponse {
	payload, _ := json.Marshal(body)

	req := esapi.SearchRequest{
		Index: []string{index},
		Body:  bytes.NewReader(payload),
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	hitsObj, ok := result["hits"].(map[string]any)
	if !ok {
		return ESResponse{Success: false, Msg: "搜索结果格式错误"}
	}

	totalValue := 0.0
	if totalObj, ok := hitsObj["total"].(map[string]any); ok {
		switch value := totalObj["value"].(type) {
		case float64:
			totalValue = value
		case json.Number:
			if parsed, err := value.Float64(); err == nil {
				totalValue = parsed
			}
		}
	}

	return ESResponse{
		Success: true,
		Msg:     "查询成功",
		Data: map[string]any{
			"total": hitsObj["total"],
			"hits":  hitsObj["hits"],
			"value": totalValue,
		},
	}
}

// 更新通用文档

// DeleteDocument 删除通用文档

// --- 批量操作函数 ---

// 构建批量请求体
func buildBulkBody(items []*BulkRequest) ([]byte, error) {
	var buf bytes.Buffer
	for _, item := range items {
		meta := make(map[string]map[string]string)
		metaData := make(map[string]string)
		if item.Index != "" {
			metaData["_index"] = item.Index
		}
		if item.Type != "" {
			metaData["_type"] = item.Type
		}
		if item.ID != "" {
			metaData["_id"] = item.ID
		}
		if item.Parent != "" {
			metaData["_parent"] = item.Parent
		}
		if item.Pipeline != "" {
			metaData["pipeline"] = item.Pipeline
		}
		meta[item.Action] = metaData

		if data, err := json.Marshal(meta); err != nil {
			return nil, err
		} else {
			buf.Write(data)
			buf.WriteByte('\n')
		}

		switch item.Action {
		case "delete":
			// 删除操作不需要额外数据
		case "update":
			doc := map[string]interface{}{
				"doc": item.Data,
			}
			if data, err := json.Marshal(doc); err != nil {
				return nil, err
			} else {
				buf.Write(data)
				buf.WriteByte('\n')
			}
		default:
			// 用于创建和索引操作
			if data, err := json.Marshal(item.Data); err != nil {
				return nil, err
			} else {
				buf.Write(data)
				buf.WriteByte('\n')
			}
		}
	}
	return buf.Bytes(), nil
}

// BulkFailure 是 bulk 响应里**单条文档**的失败。
//
// 为什么要有这个类型：ES 的 bulk 是"HTTP 200 + 逐条结果"。
// 一个请求里 127 条成功、1 条因 mapping 冲突失败，HTTP 状态码完全正常，
// 只有响应体的 `items` 能看出来。以前这个判断散在各个调用方，
// 有的只回一句 "bulk errors"（不知道是哪篇、为什么），有的根本没判断。
// 现在由 IndexBulk 统一解析成结构，领域规则（404 视为删除成功、
// 404 的更新要补建）仍然留在调用方 —— 那是业务判断，不该下沉到 ES 封装里。
type BulkFailure struct {
	// Index 是这条在本次请求里的位置（0 起），调用方可据此回查文档 id 与动作。
	Index  int
	Action string // ES 返回的动作名：index / update / delete
	ID     string // 文档 id（ES 一般都会给）
	Status int    // 400 / 409 / 429 / 503 ...
	Type   string // error.type，例如 mapper_parsing_exception
	Reason string // 人看的原因（已拼上 caused_by）
}

// BulkResult 是一次 bulk 的逐条结果，放在 ESResponse.Data 里。
type BulkResult struct {
	Total    int           // 本次请求的条数
	Failures []BulkFailure // 失败的条目（成功的不在里面）
}

// HasFailure 表示这次 bulk 有没有任何一条失败。
func (r BulkResult) HasFailure() bool { return len(r.Failures) > 0 }

// IndexBulk 发送针对索引的批量请求。
//
// 返回的 Success 表示**每一条都写成功**：HTTP 200 但响应体 errors=true 也算失败，
// 并把失败明细放进 Data（BulkResult）。调用方据此决定是重试、补建还是落死信。
func IndexBulk(client *elasticsearch.Client, index string, items []*BulkRequest) ESResponse {
	body, err := buildBulkBody(items)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	req := esapi.BulkRequest{
		Index: index,
		Body:  bytes.NewReader(body),
	}

	res, err := doRequest(client, req)
	if err != nil {
		// 传输层/整请求失败：没有逐条信息，调用方只能整批重试。
		return ESResponse{Success: false, Msg: err.Error()}
	}

	decoded, err := decodeResponse(res.Body)
	if err != nil {
		return ESResponse{Success: false, Msg: fmt.Sprintf("解析 bulk 响应失败: %v", err)}
	}

	result := parseBulkResult(decoded, len(items))
	if result.HasFailure() {
		return ESResponse{
			Success: false,
			Msg:     fmt.Sprintf("bulk 有 %d/%d 条写入失败: %s", len(result.Failures), result.Total, summarizeBulkFailures(result.Failures, 3)),
			Data:    result,
		}
	}
	return ESResponse{Success: true, Msg: "索引批量操作成功", Data: result}
}

// parseBulkResult 把 bulk 响应体解析成逐条结果。
func parseBulkResult(root map[string]any, requested int) BulkResult {
	result := BulkResult{Total: requested}
	items, _ := root["items"].([]any)
	if len(items) > 0 {
		result.Total = len(items)
	}
	for index, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for action, raw := range itemMap {
			detail, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			status := parseBulkStatus(detail["status"])
			if status >= 200 && status < 300 {
				continue
			}
			failure := BulkFailure{Index: index, Action: action, Status: status}
			failure.ID, _ = detail["_id"].(string)
			if errObj, ok := detail["error"].(map[string]any); ok {
				failure.Type, _ = errObj["type"].(string)
				failure.Reason, _ = errObj["reason"].(string)
				if causedBy, ok := errObj["caused_by"].(map[string]any); ok {
					if cbReason, ok := causedBy["reason"].(string); ok && cbReason != "" {
						failure.Reason += " (caused_by: " + cbReason + ")"
					}
				}
			}
			result.Failures = append(result.Failures, failure)
		}
	}
	return result
}

// parseBulkStatus 兼容 json.Number 与各种整型（decodeResponse 用了 UseNumber）。
func parseBulkStatus(raw any) int {
	switch value := raw.(type) {
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

// summarizeBulkFailures 取前几条拼成人话，用于错误信息与死信。
func summarizeBulkFailures(failures []BulkFailure, limit int) string {
	parts := make([]string, 0, limit)
	for index, failure := range failures {
		if index >= limit {
			parts = append(parts, fmt.Sprintf("…另有 %d 条", len(failures)-limit))
			break
		}
		reason := failure.Reason
		if failure.Type != "" && reason == "" {
			reason = failure.Type
		}
		parts = append(parts, fmt.Sprintf("%s id=%s status=%d reason=%s", failure.Action, failure.ID, failure.Status, reason))
	}
	return strings.Join(parts, " | ")
}

// IndexTypeBulk 发送针对索引和文档类型的批量请求

// --- 映射管理函数 ---

// CreateMapping 创建ES映射

// GetMapping 获取映射

// --- 索引管理函数 ---

// DeleteIndexWithResponse 删除索引并返回响应

// --- 文档操作函数 ---

// Get 根据ID获取文档

// Exists 检查文档是否存在
