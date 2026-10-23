package es_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

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
func CreateDocument(client *elasticsearch.Client, index string, data any) ESResponse {
	body, _ := json.Marshal(data)
	req := esapi.IndexRequest{
		Index: index,
		Body:  bytes.NewReader(body),
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}
	defer closeResponse(res)

	return ESResponse{Success: true, Msg: "创建成功", Data: res.String()}
}

// Search 分页查询封装
func Search[T any](client *elasticsearch.Client, index string, page, limit int, query map[string]any, extraBody ...map[string]any) ESResponse {
	from := (page - 1) * limit
	searchBody := map[string]any{"from": from, "size": limit, "query": query}
	if len(extraBody) > 0 {
		for key, value := range extraBody[0] {
			searchBody[key] = value
		}
	}
	body, _ := json.Marshal(searchBody)

	req := esapi.SearchRequest{
		Index: []string{index},
		Body:  bytes.NewReader(body),
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	// 这里的 map[string]any 可以根据 ES 返回结构定义更精确的 struct
	result, _ := decodeResponse(res.Body)
	hitsObj := result["hits"].(map[string]any)

	return ESResponse{
		Success: true,
		Msg:     "查询成功",
		Data: map[string]any{
			"total": hitsObj["total"].(map[string]any)["value"],
			"hits":  hitsObj["hits"], // 保持原始 hits 供上层解析，或在此处进一步泛型转换
		},
	}
}

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
func UpdateDocument(client *elasticsearch.Client, index, docID string, updateData map[string]any) ESResponse {
	body, _ := json.Marshal(map[string]any{"doc": updateData})
	req := esapi.UpdateRequest{
		Index:      index,
		DocumentID: docID,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	return ESResponse{Success: true, Msg: "更新成功", Data: result}
}

// DeleteDocument 删除通用文档
func DeleteDocument(client *elasticsearch.Client, index, docID string) ESResponse {
	req := esapi.DeleteRequest{
		Index:      index,
		DocumentID: docID,
		Refresh:    "true",
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	return ESResponse{Success: true, Msg: "删除成功", Data: result}
}

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

// Bulk 发送批量请求
func Bulk(client *elasticsearch.Client, items []*BulkRequest) ESResponse {
	body, err := buildBulkBody(items)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	req := esapi.BulkRequest{
		Body: bytes.NewReader(body),
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	if errorsValue, ok := result["errors"].(bool); ok && errorsValue {
		return ESResponse{Success: false, Msg: "bulk 响应包含失败项（errors=true）", Data: result}
	}
	return ESResponse{Success: true, Msg: "批量操作成功", Data: result}
}

// IndexBulk 发送针对索引的批量请求
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
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	return ESResponse{Success: true, Msg: "索引批量操作成功", Data: result}
}

// IndexTypeBulk 发送针对索引和文档类型的批量请求
func IndexTypeBulk(client *elasticsearch.Client, index, docType string, items []*BulkRequest) ESResponse {
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
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	return ESResponse{Success: true, Msg: "索引类型批量操作成功", Data: result}
}

// --- 映射管理函数 ---

// CreateMapping 创建ES映射
func CreateMapping(client *elasticsearch.Client, index, docType string, mapping map[string]interface{}) ESResponse {
	// 先检查索引是否存在
	existsReq := esapi.IndicesExistsRequest{
		Index: []string{index},
	}
	existsRes, err := existsReq.Do(context.Background(), client)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}
	defer closeResponse(existsRes)

	// 如果索引不存在，创建索引
	if existsRes.IsError() {
		createReq := esapi.IndicesCreateRequest{
			Index: index,
		}
		var createRes *esapi.Response
		var createErr error
		createRes, createErr = createReq.Do(context.Background(), client)
		if createErr != nil {
			return ESResponse{Success: false, Msg: createErr.Error()}
		}
		defer closeResponse(createRes)
		if createRes.IsError() {
			return ESResponse{Success: false, Msg: handleError(createRes).Error()}
		}
	}

	// 创建映射
	mappingJSON, _ := json.Marshal(mapping)
	mapReq := esapi.IndicesPutMappingRequest{
		Index: []string{index},
		Body:  bytes.NewReader([]byte(fmt.Sprintf(`{"properties":%s}`, string(mappingJSON)))),
	}
	mapRes, err := mapReq.Do(context.Background(), client)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}
	defer closeResponse(mapRes)

	if mapRes.IsError() {
		return ESResponse{Success: false, Msg: handleError(mapRes).Error()}
	}

	return ESResponse{Success: true, Msg: "映射创建成功"}
}

// GetMapping 获取映射
func GetMapping(client *elasticsearch.Client, index, docType string) ESResponse {
	req := esapi.IndicesGetMappingRequest{
		Index: []string{index},
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	return ESResponse{Success: true, Msg: "获取映射成功", Data: result}
}

// --- 索引管理函数 ---

// DeleteIndexWithResponse 删除索引并返回响应
func DeleteIndexWithResponse(client *elasticsearch.Client, index string) ESResponse {
	req := esapi.IndicesDeleteRequest{
		Index: []string{index},
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	return ESResponse{Success: true, Msg: "删除索引成功", Data: result}
}

// --- 文档操作函数 ---

// Get 根据ID获取文档
func Get(client *elasticsearch.Client, index, docType, id string) ESResponse {
	req := esapi.GetRequest{
		Index:      index,
		DocumentID: id,
	}

	res, err := doRequest(client, req)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}

	result, _ := decodeResponse(res.Body)
	return ESResponse{Success: true, Msg: "获取文档成功", Data: result}
}

// Exists 检查文档是否存在
func Exists(client *elasticsearch.Client, index, docType, id string) ESResponse {
	req := esapi.ExistsRequest{
		Index:      index,
		DocumentID: id,
	}

	res, err := req.Do(context.Background(), client)
	if err != nil {
		return ESResponse{Success: false, Msg: err.Error()}
	}
	defer closeResponse(res)

	return ESResponse{Success: true, Msg: "检查文档存在性成功", Data: !res.IsError()}
}
