package qiniu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"myblogx/internal/apperr"
)

// censorEndpoint 是七牛内容审核的 API 入口。
//
// 为什么直接用 AI 审核接口而不是"增量审核任务"：增量审核的回调要求公网可达的
// 回调地址，域名被封禁期间根本进不来；而这条接口是**出站**请求，本机拿结果，
// 不受入站限制。两条通道并存时取最严（见 media 用例层）。
const censorEndpoint = "https://ai.qiniuapi.com/v3/image/censor"

// censorScenes 是审核场景。
//
// 六类全开是站长的决定：涉黄、暴恐、敏感人物、广告、不良场景、异常检测。
// 少开一类就意味着那一类的违规只能靠人眼发现。
var censorScenes = []string{"porn", "terror", "politician", "ad", "behaviour", "abnormal"}

// CensorResult 是一次内容审核的结论。
type CensorResult struct {
	// Suggestion 是管控建议：pass / review / block。
	Suggestion string
	// Raw 是原始响应体（排查用，落日志不落库）。
	Raw string
}

type censorRequest struct {
	Data   censorData   `json:"data"`
	Params censorParams `json:"params"`
}

type censorData struct {
	// URI 用 "bucket:key" 形式引用空间内对象，避免再走一次公网下载。
	URI string `json:"uri"`
}

type censorParams struct {
	Scenes []string `json:"scenes"`
}

type censorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Result  struct {
		Suggestion string `json:"suggestion"`
	} `json:"result"`
}

// CensorImage 主动调用七牛图片内容审核，返回管控建议。
//
// 这是"审核结论"的第二条来源：回调走不通（域名封禁、地址配错、重试耗尽）时，
// 靠它把图从"待审核"推向终态。返回 error 表示**审核本身没跑起来**，
// 调用方应保持待审核并进人工队列 —— 绝不能把"调用失败"当成"通过"。
func (s *Storage) CensorImage(ctx context.Context, bucket, objectKey string) (CensorResult, error) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(objectKey) == "" {
		return CensorResult{}, apperr.New(apperr.CodeStorageBucketRequired)
	}

	payload := censorRequest{
		Data:   censorData{URI: bucket + ":" + objectKey},
		Params: censorParams{Scenes: censorScenes},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return CensorResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, censorEndpoint, bytes.NewReader(body))
	if err != nil {
		return CensorResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	// 用同一套 AK/SK 签请求：七牛的签名算法与上传/管理接口一致。
	if _, err := getRuntime(s.cfg).mac.SignRequest(req); err != nil {
		return CensorResult{}, err
	}

	resp, err := getRuntime(s.cfg).httpClient.Do(req)
	if err != nil {
		return CensorResult{}, err
	}
	defer resp.Body.Close()

	var decoded censorResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return CensorResult{}, fmt.Errorf("解析审核响应失败: %w", err)
	}
	// 七牛把业务失败也放在 200 里（code != 200），所以两个都要判。
	if resp.StatusCode != http.StatusOK || decoded.Code != http.StatusOK {
		return CensorResult{Raw: decoded.Message}, fmt.Errorf("审核接口返回 status=%d code=%d msg=%s",
			resp.StatusCode, decoded.Code, decoded.Message)
	}
	if strings.TrimSpace(decoded.Result.Suggestion) == "" {
		return CensorResult{}, fmt.Errorf("审核接口未返回 suggestion")
	}
	return CensorResult{Suggestion: decoded.Result.Suggestion}, nil
}

// SetObjectEnabled 启用/禁用空间内的对象（七牛"禁用文件"）。
//
// 用途：疑似违规时在存储侧也关掉（应用层"不给地址"只是弱保证，桶是公开的），
// 人工放行时再解禁。重复操作七牛会返回 already enabled/disabled，
// 这里按"目标状态已达成"处理，不算失败。
func (s *Storage) SetObjectEnabled(_ context.Context, bucket, objectKey string, enabled bool) error {
	err := getRuntime(s.cfg).bucketMgr.UpdateObjectStatus(bucket, objectKey, enabled)
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "already enabled") || strings.Contains(msg, "already disabled") {
		return nil
	}
	return err
}
