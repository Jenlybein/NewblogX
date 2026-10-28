package http

import (
	"myblogx/internal/ai/app"
	"myblogx/internal/ai/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"

	// 供 swag 注解里的 kernel.Envelope 解析用。
	// swag 是按**文件**解析 import 的：注解写在本文件，就必须本文件有这条 import，
	// 否则报 "cannot find type definition" 并整份跳过写入（exit code 仍是 0）。
	_ "myblogx/internal/kernel"

	"github.com/gin-gonic/gin"
)

// ProviderItem 是一条模型配置（出站：密钥已遮蔽）。
type ProviderItem struct {
	ID      ctype.ID          `json:"id"`
	Name    string            `json:"name"`
	Vendor  string            `json:"vendor"`
	Model   string            `json:"model"`
	Enabled bool              `json:"enabled"`
	Fields  map[string]string `json:"fields"`
}

// ProviderPayload 是新增 / 修改一条配置的请求体。
//
// 修改时密钥字段回传 ****** 表示"没改"，服务层会沿用旧值。
type ProviderPayload struct {
	Name    string            `json:"name" binding:"omitempty,max=64"`
	Vendor  string            `json:"vendor" binding:"required,max=32"`
	Model   string            `json:"model" binding:"required,max=128"`
	Fields  map[string]string `json:"fields"`
	Enabled bool              `json:"enabled"`
}

// VendorFieldItem 是某个供应商需要的单个配置项（出站）。
//
// 这里刻意用本包的 DTO 而不是直接返回 app.VendorSpec：
// swag 解析 `kernel.Envelope[kernel.ListData[app.VendorSpec]]` 时会报
// "cannot find type definition" —— 跨包类型作为泛型实参它认不出来。
type VendorFieldItem struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Placeholder string `json:"placeholder"`
	Default     string `json:"default"`
}

// VendorItem 是一个供应商的元数据（出站）。
type VendorItem struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Note   string            `json:"note"`
	Fields []VendorFieldItem `json:"fields"`
}

// ProviderIDRequest 是路径里的配置 ID。
type ProviderIDRequest struct {
	ID ctype.ID `uri:"id" binding:"required"`
}

// ProviderVendors 返回供应商元数据（字段定义），前端据此渲染动态表单。
// @Summary 供应商元数据
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[VendorItem]]
// @Router /ai/providers/vendors [get]
func (h *Handler) ProviderVendors(c *gin.Context) {
	if h.providers == nil {
		res.FailWithMsg("模型配置未初始化", c)
		return
	}
	specs := h.providers.Vendors()
	out := make([]VendorItem, 0, len(specs))
	for _, spec := range specs {
		fields := make([]VendorFieldItem, 0, len(spec.Fields))
		for _, field := range spec.Fields {
			fields = append(fields, VendorFieldItem{
				Key:         field.Key,
				Label:       field.Label,
				Required:    field.Required,
				Secret:      field.Secret,
				Placeholder: field.Placeholder,
				Default:     field.Default,
			})
		}
		out = append(out, VendorItem{ID: spec.ID, Name: spec.Name, Note: spec.Note, Fields: fields})
	}
	res.OkWithData(out, c)
}

// ProviderList 返回全部模型配置（密钥遮蔽）。
// @Summary 模型配置列表
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[ProviderItem]]
// @Router /ai/providers [get]
func (h *Handler) ProviderList(c *gin.Context) {
	if h.providers == nil {
		res.FailWithMsg("模型配置未初始化", c)
		return
	}
	items := h.providers.List()
	out := make([]ProviderItem, 0, len(items))
	for _, item := range items {
		out = append(out, toProviderItem(item))
	}
	res.OkWithData(out, c)
}

// ProviderCreate 新增一条模型配置。
// @Summary 新增模型配置
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ProviderPayload true "配置内容"
// @Success 200 {object} kernel.Envelope[ProviderItem]
// @Router /ai/providers [post]
func (h *Handler) ProviderCreate(c *gin.Context) {
	if h.providers == nil {
		res.FailWithMsg("模型配置未初始化", c)
		return
	}
	var payload ProviderPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	created, err := h.providers.Create(c.Request.Context(), toProviderInput(payload))
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(toProviderItem(created), c)
}

// ProviderUpdate 修改一条模型配置。
// @Summary 修改模型配置
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "配置 ID"
// @Param body body ProviderPayload true "配置内容"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /ai/providers/{id} [put]
func (h *Handler) ProviderUpdate(c *gin.Context) {
	if h.providers == nil {
		res.FailWithMsg("模型配置未初始化", c)
		return
	}
	cr, ok := bindURI[ProviderIDRequest](c)
	if !ok {
		return
	}
	var payload ProviderPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	if err := h.providers.Update(c.Request.Context(), cr.ID, toProviderInput(payload)); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("配置已更新", c)
}

// ProviderEnable 启用一条模型配置（其余自动停用）。
// @Summary 启用模型配置
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Param id path string true "配置 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /ai/providers/{id}/enable [post]
func (h *Handler) ProviderEnable(c *gin.Context) {
	if h.providers == nil {
		res.FailWithMsg("模型配置未初始化", c)
		return
	}
	cr, ok := bindURI[ProviderIDRequest](c)
	if !ok {
		return
	}
	if err := h.providers.Enable(c.Request.Context(), cr.ID); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("已启用该配置", c)
}

// ProviderRemove 删除一条模型配置。
// @Summary 删除模型配置
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Param id path string true "配置 ID"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /ai/providers/{id} [delete]
func (h *Handler) ProviderRemove(c *gin.Context) {
	if h.providers == nil {
		res.FailWithMsg("模型配置未初始化", c)
		return
	}
	cr, ok := bindURI[ProviderIDRequest](c)
	if !ok {
		return
	}
	if err := h.providers.Delete(c.Request.Context(), cr.ID); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("配置已删除", c)
}

// toProviderItem 把领域实体转成出站结构。
func toProviderItem(item domain.AIProvider) ProviderItem {
	return ProviderItem{
		ID:      item.ID,
		Name:    item.Name,
		Vendor:  item.Vendor,
		Model:   item.Model,
		Enabled: item.Enabled,
		Fields:  item.Fields,
	}
}

// toProviderInput 把请求体转成服务层入参。
func toProviderInput(payload ProviderPayload) app.ProviderInput {
	return app.ProviderInput{
		Name:    payload.Name,
		Vendor:  payload.Vendor,
		Model:   payload.Model,
		Fields:  payload.Fields,
		Enabled: payload.Enabled,
	}
}

// bindURI 是 mw.BindUri 的读取端：把中间件存下的结构取出来。
//
// 与 site 模块的做法一致：绑定发生在中间件里，handler 只取值并做类型断言。
func bindURI[T any](c *gin.Context) (T, bool) {
	var out T
	value, exists := c.Get("requestUri")
	if !exists {
		res.FailWithMsg("参数错误", c)
		return out, false
	}
	typed, ok := value.(T)
	if !ok {
		res.FailWithMsg("参数错误", c)
		return out, false
	}
	return typed, true
}
