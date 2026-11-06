package http

import (
	"myblogx/conf"

	"github.com/gin-gonic/gin"

	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// 九段运行时配置的具名 HTTP 出入口（#283 / ADR-0029）。
//
// 参数化 /site/{name} 与 /site/admin/{name} 已整体退场：每段各有自己的
// GET /site/admin/<name> 与 PUT /site/<name>，注解直接引用该段的具名形状。
// 于是契约（开放接口契约）能派生出九套字段级类型，两端不再手写视图模型。

// updateNamedSection 把请求体绑定到具名形状后交给该段的更新用例。
func updateNamedSection[T any](c *gin.Context, update func(T) error) error {
	var body T
	if err := c.ShouldBindJSON(&body); err != nil {
		return err
	}
	return update(body)
}

// AdminSiteSection 返回 site 段管理端配置。
// @Summary 获取 site 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionSiteResponse]
// @Router /site/admin/site [get]
func (h *Handler) AdminSiteSection(c *gin.Context) {
	res.OkWithData(SiteSectionSiteResponse(h.svc.AdminSite()), c)
}

// UpdateSiteSection 覆盖 site 段运行时配置。
// @Summary 更新 site 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionSiteRequest true "site 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/site [put]
func (h *Handler) UpdateSiteSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionSiteRequest) error {
		return h.svc.UpdateSite(conf.Site(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "site",
		Success:    true,
		Message:    "更新运行时配置成功: site",
	})
}

// AdminAiSection 返回 ai 段管理端配置。
// @Summary 获取 ai 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionAiResponse]
// @Router /site/admin/ai [get]
func (h *Handler) AdminAiSection(c *gin.Context) {
	res.OkWithData(SiteSectionAiResponse(h.svc.AdminAI()), c)
}

// UpdateAiSection 覆盖 ai 段运行时配置。
// @Summary 更新 ai 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionAiRequest true "ai 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/ai [put]
func (h *Handler) UpdateAiSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionAiRequest) error {
		return h.svc.UpdateAI(conf.AI(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "ai",
		Success:    true,
		Message:    "更新运行时配置成功: ai",
	})
}

// AdminMailSection 返回 mail 段管理端配置。
// @Summary 获取 mail 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionMailResponse]
// @Router /site/admin/mail [get]
func (h *Handler) AdminMailSection(c *gin.Context) {
	res.OkWithData(SiteSectionMailResponse(h.svc.AdminMail()), c)
}

// UpdateMailSection 覆盖 mail 段运行时配置。
// @Summary 更新 mail 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionMailRequest true "mail 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/mail [put]
func (h *Handler) UpdateMailSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionMailRequest) error {
		return h.svc.UpdateMail(conf.Mail(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "mail",
		Success:    true,
		Message:    "更新运行时配置成功: mail",
	})
}

// AdminStorageSection 返回 storage 段管理端配置。
// @Summary 获取 storage 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionStorageResponse]
// @Router /site/admin/storage [get]
func (h *Handler) AdminStorageSection(c *gin.Context) {
	res.OkWithData(SiteSectionStorageResponse(h.svc.AdminStorage()), c)
}

// UpdateStorageSection 覆盖 storage 段运行时配置。
// @Summary 更新 storage 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionStorageRequest true "storage 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/storage [put]
func (h *Handler) UpdateStorageSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionStorageRequest) error {
		return h.svc.UpdateStorage(conf.QiNiu(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "storage",
		Success:    true,
		Message:    "更新运行时配置成功: storage",
	})
}

// AdminOauthSection 返回 oauth 段管理端配置。
// @Summary 获取 oauth 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionOauthResponse]
// @Router /site/admin/oauth [get]
func (h *Handler) AdminOauthSection(c *gin.Context) {
	res.OkWithData(SiteSectionOauthResponse(h.svc.AdminOAuth()), c)
}

// UpdateOauthSection 覆盖 oauth 段运行时配置。
// @Summary 更新 oauth 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionOauthRequest true "oauth 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/oauth [put]
func (h *Handler) UpdateOauthSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionOauthRequest) error {
		return h.svc.UpdateOAuth(conf.QQ(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "oauth",
		Success:    true,
		Message:    "更新运行时配置成功: oauth",
	})
}

// AdminLogSection 返回 log 段管理端配置。
// @Summary 获取 log 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionLogResponse]
// @Router /site/admin/log [get]
func (h *Handler) AdminLogSection(c *gin.Context) {
	res.OkWithData(SiteSectionLogResponse(h.svc.AdminLog()), c)
}

// UpdateLogSection 覆盖 log 段运行时配置。
// @Summary 更新 log 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionLogRequest true "log 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/log [put]
func (h *Handler) UpdateLogSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionLogRequest) error {
		return h.svc.UpdateLog(conf.Logrus(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "log",
		Success:    true,
		Message:    "更新运行时配置成功: log",
	})
}

// AdminUploadSection 返回 upload 段管理端配置。
// @Summary 获取 upload 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionUploadResponse]
// @Router /site/admin/upload [get]
func (h *Handler) AdminUploadSection(c *gin.Context) {
	res.OkWithData(SiteSectionUploadResponse(h.svc.AdminUpload()), c)
}

// UpdateUploadSection 覆盖 upload 段运行时配置。
// @Summary 更新 upload 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionUploadRequest true "upload 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/upload [put]
func (h *Handler) UpdateUploadSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionUploadRequest) error {
		return h.svc.UpdateUpload(conf.Upload(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "upload",
		Success:    true,
		Message:    "更新运行时配置成功: upload",
	})
}

// AdminDecoSection 返回 deco 段管理端配置。
// @Summary 获取 deco 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionDecoResponse]
// @Router /site/admin/deco [get]
func (h *Handler) AdminDecoSection(c *gin.Context) {
	res.OkWithData(SiteSectionDecoResponse(h.svc.AdminDeco()), c)
}

// UpdateDecoSection 覆盖 deco 段运行时配置。
// @Summary 更新 deco 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionDecoRequest true "deco 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/deco [put]
func (h *Handler) UpdateDecoSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionDecoRequest) error {
		return h.svc.UpdateDeco(conf.Deco(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "deco",
		Success:    true,
		Message:    "更新运行时配置成功: deco",
	})
}

// AdminImageGcSection 返回 image_gc 段管理端配置。
// @Summary 获取 image_gc 段运行时配置（管理员）
// @Tags site
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SiteSectionImageGcResponse]
// @Router /site/admin/image_gc [get]
func (h *Handler) AdminImageGcSection(c *gin.Context) {
	res.OkWithData(SiteSectionImageGcResponse(h.svc.AdminImageGC()), c)
}

// UpdateImageGcSection 覆盖 image_gc 段运行时配置。
// @Summary 更新 image_gc 段运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SiteSectionImageGcRequest true "image_gc 段配置"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /site/image_gc [put]
func (h *Handler) UpdateImageGcSection(c *gin.Context) {
	if err := updateNamedSection(c, func(v SiteSectionImageGcRequest) error {
		return h.svc.UpdateImageGC(conf.ImageGC(v))
	}); err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg("配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "runtime_config_update",
		TargetType: "runtime_config",
		TargetID:   "image_gc",
		Success:    true,
		Message:    "更新运行时配置成功: image_gc",
	})
}

// PublicSiteSection 返回公开站点配置，并累计一次站点访问量。
// @Summary 获取公开站点配置
// @Tags site
// @Produce json
// @Success 200 {object} kernel.Envelope[SiteSectionSiteResponse]
// @Router /site/site [get]
func (h *Handler) PublicSiteSection(c *gin.Context) {
	if h.onPublicSiteView != nil {
		h.onPublicSiteView()
	}
	res.OkWithData(SiteSectionSiteResponse(h.svc.PublicSite()), c)
}

// PublicDecoSection 返回读者站要的装饰配置（无敏感字段，与管理读同形）。
// @Summary 获取公开装饰配置
// @Tags site
// @Produce json
// @Success 200 {object} kernel.Envelope[SiteSectionDecoResponse]
// @Router /site/deco [get]
func (h *Handler) PublicDecoSection(c *gin.Context) {
	res.OkWithData(SiteSectionDecoResponse(h.svc.PublicDeco()), c)
}
