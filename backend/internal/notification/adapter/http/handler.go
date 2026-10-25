// Package http 是通知模块的 HTTP 适配器。
package http

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"myblogx/common/res"
	"myblogx/internal/apperr"
	"myblogx/internal/notification/app"
	"myblogx/internal/notification/domain"
	"myblogx/models/ctype"
	"myblogx/utils/jwts"
)

// Handler 聚合通知模块的所有 HTTP 入口。
type Handler struct {
	svc *app.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service) *Handler {
	if svc == nil {
		panic("notification/http: 用例服务未注入")
	}
	return &Handler{svc: svc}
}

// userID 从 claims 取当前登录用户。
func userID(c *gin.Context) uint64 {
	claims := jwts.GetClaimsByGin(c)
	if claims == nil {
		return 0
	}
	return uint64(claims.UserID)
}

// isAdmin 判断当前用户是否管理员。
func isAdmin(c *gin.Context) bool {
	claims := jwts.GetClaimsByGin(c)
	return claims != nil && claims.IsAdmin()
}

// fail 交给统一响应层：apperr 自带中文文案。
func fail(c *gin.Context, err error) {
	res.FailWithError(err, c)
}

// ── 全局公告 ────────────────────────────────────────────────────

// GlobalList 获取全局公告列表。
// @Summary 全局通知列表
// @Tags global_notif
// @Produce json
// @Security BearerAuth
// @Param type query int true "查询类型：1 普通用户可见通知 2 管理员查全部通知"
// @Success 200 {object} object{code=int,data=object{list=[]GlobalListResponse,has_more=bool},msg=string}
// @Router /global_notif [get]
func (h *Handler) GlobalList(c *gin.Context) {
	cr, ok := bindQuery[GlobalListRequest](c)
	if !ok {
		return
	}
	if cr.Order != "" {
		// 旧实现没有开放排序白名单，传 order 会直接报错。
		res.FailWithMsg("排序字段错误", c)
		return
	}
	admin := false
	switch cr.Type {
	case 2:
		if !isAdmin(c) {
			res.FailWithMsg("权限不足", c)
			return
		}
		admin = true
	case 1:
	default:
		res.FailWithMsg("参数错误", c)
		return
	}
	items, hasMore, err := h.svc.ListGlobal(c.Request.Context(), userID(c), admin, domain.Page{
		Page:  cr.Page,
		Limit: cr.Limit,
		Key:   cr.Key,
	})
	if err != nil {
		if !admin && apperr.IsCode(err, apperr.CodeNotifUserMissing) {
			res.FailWithMsg("用户不存在", c)
			return
		}
		fail(c, err)
		return
	}
	list := make([]GlobalListResponse, 0, len(items))
	for _, item := range items {
		list = append(list, GlobalListResponse{
			ID:       ctype.ID(item.ID),
			CreateAt: item.CreatedAt,
			Title:    item.Title,
			Icon:     item.Icon,
			Content:  item.Content,
			Href:     item.Href,
			IsRead:   item.IsRead,
		})
	}
	res.OkWithHasMoreList(list, hasMore, c)
}

// GlobalCreate 创建全局公告（管理员）。
// @Summary 创建全局通知
// @Tags global_notif
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body GlobalCreateRequest true "请求体"
// @Success 200 {object} object{code=int,data=GlobalCreateResponse,msg=string}
// @Router /global_notif [post]
func (h *Handler) GlobalCreate(c *gin.Context) {
	if !isAdmin(c) {
		res.FailWithMsg("权限错误", c)
		return
	}
	cr, ok := bindJSON[GlobalCreateRequest](c)
	if !ok {
		return
	}
	notif, err := h.svc.CreateGlobal(c.Request.Context(), userID(c), app.CreateGlobalInput{
		ExpireAt:    cr.ExpireTime,
		VisibleRule: domain.VisibleRule(cr.UserVisibleRule),
		Title:       cr.Title,
		Content:     cr.Content,
		Icon:        cr.Icon,
		Href:        cr.Href,
	})
	if err != nil {
		switch {
		case apperr.IsCode(err, apperr.CodeNotifExpireTooSoon):
			res.FailWithMsg("过期时间不能小于一天", c)
		case apperr.IsCode(err, apperr.CodeNotifTitleDuplicated):
			res.FailWithMsg("全局通知标题重复", c)
		default:
			res.FailWithMsg(fmt.Sprintf("全局通知创建失败 %v", err), c)
		}
		return
	}
	res.OkWithData(GlobalCreateResponse{
		ID:              ctype.ID(notif.ID),
		ExpireTime:      notif.ExpireAt,
		UserVisibleRule: int8(notif.VisibleRule),
		Title:           notif.Title,
		Content:         notif.Content,
		Icon:            notif.Icon,
		Href:            notif.Href,
	}, c)
}

// GlobalRead 批量标记公告已读。
// @Summary 标记全局通知已读
// @Tags global_notif
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body IDListRequest true "通知 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /global_notif/read [post]
func (h *Handler) GlobalRead(c *gin.Context) {
	cr, ok := bindJSON[IDListRequest](c)
	if !ok {
		return
	}
	if len(cr.IDList) == 0 {
		res.FailWithMsg("请输入要读取的消息 id 列表", c)
		return
	}
	count, err := h.svc.MarkGlobalRead(c.Request.Context(), userID(c), toIDs(cr.IDList))
	if err != nil {
		switch {
		case apperr.IsCode(err, apperr.CodeNotifNotFound):
			res.FailWithMsg("消息不存在", c)
		case apperr.IsCode(err, apperr.CodeMessageNothingRead):
			res.FailWithMsg("没有可标记已读的消息", c)
		case apperr.IsCode(err, apperr.CodeNotifUserMissing):
			res.FailWithMsg("用户不存在", c)
		default:
			fail(c, err)
		}
		return
	}
	res.OkWithMsg(fmt.Sprintf("标记已读%d条消息", count), c)
}

// GlobalUserRemove 用户删除自己可见的公告。
// @Summary 用户删除全局通知
// @Tags global_notif
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body IDListRequest true "通知 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /global_notif/user [delete]
func (h *Handler) GlobalUserRemove(c *gin.Context) {
	cr, ok := bindJSON[IDListRequest](c)
	if !ok {
		return
	}
	if len(cr.IDList) == 0 {
		// 旧接口在空列表时也算成功（数据库层自然筛不到任何行）。
		res.OkWithMsg("请求删除公告0个，成功0条", c)
		return
	}
	count, err := h.svc.DeleteGlobalForUser(c.Request.Context(), userID(c), toIDs(cr.IDList))
	if err != nil {
		if apperr.IsCode(err, apperr.CodeNotifUserMissing) {
			res.FailWithMsg("用户不存在", c)
			return
		}
		fail(c, err)
		return
	}
	res.OkWithMsg(fmt.Sprintf("请求删除公告%d个，成功%d条", len(cr.IDList), count), c)
}

// GlobalAdminRemove 管理员删除公告。
// @Summary 删除全局通知（管理员）
// @Tags global_notif
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body IDListRequest true "通知 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /global_notif [delete]
func (h *Handler) GlobalAdminRemove(c *gin.Context) {
	cr, ok := bindJSON[IDListRequest](c)
	if !ok {
		return
	}
	if len(cr.IDList) == 0 {
		res.FailWithMsg("请输入要删除的公告 id 列表", c)
		return
	}
	count, err := h.svc.DeleteGlobal(c.Request.Context(), toIDs(cr.IDList))
	if err != nil {
		if apperr.IsCode(err, apperr.CodeNotifNotFound) {
			res.FailWithMsg("未找到需要删除的公告", c)
			return
		}
		fail(c, err)
		return
	}
	res.OkWithMsg(fmt.Sprintf("请求删除公告%d个，成功%d条", len(cr.IDList), count), c)
}

// ── 站内信 ─────────────────────────────────────────────────────

// Messages 获取当前用户的消息列表。
// @Summary 获取消息列表
// @Tags sitemsg
// @Produce json
// @Security BearerAuth
// @Param t query int true "消息类型：1 评论和回复 2 点赞和收藏 3 系统通知"
// @Success 200 {object} object{code=int,data=object{list=[]MessageItem,has_more=bool},msg=string}
// @Router /sitemsg [get]
func (h *Handler) Messages(c *gin.Context) {
	cr, ok := bindQuery[MessageListRequest](c)
	if !ok {
		return
	}
	if cr.Order != "" {
		res.FailWithMsg("排序字段错误", c)
		return
	}
	rows, hasMore, err := h.svc.ListMessages(c.Request.Context(), userID(c), domain.Group(cr.T), domain.Page{
		Page:  cr.Page,
		Limit: cr.Limit,
	})
	if err != nil {
		fail(c, err)
		return
	}
	list := make([]MessageItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, toMessageItem(row))
	}
	res.OkWithHasMoreList(list, hasMore, c)
}

// MessageRead 按 ID 或分组标记消息已读。
// @Summary 标记消息已读
// @Tags sitemsg
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body MessageSelectRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /sitemsg [post]
func (h *Handler) MessageRead(c *gin.Context) {
	cr, ok := bindJSON[MessageSelectRequest](c)
	if !ok {
		return
	}
	if cr.ID == 0 && cr.T == 0 {
		res.FailWithMsg("id 和 t 不能同时为空", c)
		return
	}
	ctx := c.Request.Context()
	uid := userID(c)
	if cr.ID != 0 {
		err := h.svc.MarkMessageRead(ctx, uid, uint64(cr.ID))
		if err != nil {
			switch {
			case apperr.IsCode(err, apperr.CodeMessageNotFound):
				res.FailWithMsg("消息不存在", c)
			case apperr.IsCode(err, apperr.CodeMessageAlreadyRead):
				res.FailWithMsg("消息已被读取", c)
			default:
				fail(c, err)
			}
			return
		}
		res.OkWithMsg("标记已读成功", c)
		return
	}
	count, err := h.svc.MarkGroupRead(ctx, uid, domain.Group(cr.T))
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg(fmt.Sprintf("批量标记已读%d条消息", count), c)
}

// MessageRemove 按 ID 或分组删除消息。
// @Summary 删除消息
// @Tags sitemsg
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body MessageSelectRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /sitemsg [delete]
func (h *Handler) MessageRemove(c *gin.Context) {
	cr, ok := bindJSON[MessageSelectRequest](c)
	if !ok {
		return
	}
	if cr.ID == 0 && cr.T == 0 {
		res.FailWithMsg("id 和 t 不能同时为空", c)
		return
	}
	ctx := c.Request.Context()
	uid := userID(c)
	if cr.ID != 0 {
		err := h.svc.DeleteMessage(ctx, uid, uint64(cr.ID))
		if err != nil {
			if apperr.IsCode(err, apperr.CodeMessageNotFound) {
				res.FailWithMsg("消息不存在", c)
				return
			}
			fail(c, err)
			return
		}
		res.OkWithMsg("消息删除成功", c)
		return
	}
	count, err := h.svc.DeleteGroup(ctx, uid, domain.Group(cr.T))
	if err != nil {
		fail(c, err)
		return
	}
	res.OkWithMsg(fmt.Sprintf("批量删除%d条消息", count), c)
}

// Conf 获取当前用户的通知开关。
// @Summary 获取消息通知配置
// @Tags sitemsg
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=ConfRequest,msg=string}
// @Router /sitemsg/conf [get]
func (h *Handler) Conf(c *gin.Context) {
	conf, err := h.svc.GetConf(c.Request.Context(), userID(c))
	if err != nil {
		if apperr.IsCode(err, apperr.CodeMessageConfMissing) {
			res.FailWithMsg("用户配置信息不存在", c)
			return
		}
		fail(c, err)
		return
	}
	res.OkWithData(ConfRequest{
		DiggNoticeEnabled:        conf.DiggNoticeEnabled,
		CommentNoticeEnabled:     conf.CommentNoticeEnabled,
		FavorNoticeEnabled:       conf.FavorNoticeEnabled,
		PrivateChatNoticeEnabled: conf.PrivateChatNoticeEnabled,
	}, c)
}

// ConfUpdate 更新当前用户的通知开关。
// @Summary 更新消息通知配置
// @Tags sitemsg
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ConfRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /sitemsg/conf [put]
func (h *Handler) ConfUpdate(c *gin.Context) {
	cr, ok := bindJSON[ConfRequest](c)
	if !ok {
		return
	}
	err := h.svc.UpdateConf(c.Request.Context(), userID(c), domain.Conf{
		DiggNoticeEnabled:        cr.DiggNoticeEnabled,
		CommentNoticeEnabled:     cr.CommentNoticeEnabled,
		FavorNoticeEnabled:       cr.FavorNoticeEnabled,
		PrivateChatNoticeEnabled: cr.PrivateChatNoticeEnabled,
	})
	if err != nil {
		if apperr.IsCode(err, apperr.CodeMessageConfMissing) {
			res.FailWithMsg("用户配置信息不存在", c)
			return
		}
		res.FailWithMsg("用户配置信息更新失败", c)
		return
	}
	res.OkWithMsg("用户配置信息更新成功", c)
}

// Unread 获取当前用户的各类未读数。
// @Summary 获取未读消息统计
// @Tags sitemsg
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=UnreadResponse,msg=string}
// @Router /sitemsg/user [get]
func (h *Handler) Unread(c *gin.Context) {
	counts, err := h.svc.UnreadCounts(c.Request.Context(), userID(c))
	if err != nil {
		if apperr.IsCode(err, apperr.CodeNotifUserMissing) {
			res.FailWithMsg("用户不存在", c)
			return
		}
		fail(c, err)
		return
	}
	res.OkWithData(UnreadResponse{
		CommentMsgCount:   counts.Comment,
		DiggFavorMsgCount: counts.DiggFavor,
		PrivateMsgCount:   counts.Private,
		SystemMsgCount:    counts.System,
		GlobalMsgCount:    counts.Global,
	}, c)
}

// toMessageItem 把领域实体转成旧契约的 JSON 形状。
func toMessageItem(m domain.Message) MessageItem {
	item := MessageItem{
		ID:           ctype.ID(m.ID),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		Type:         int8(m.Type),
		ReceiverID:   ctype.ID(m.Receiver),
		Content:      m.Content,
		ArticleID:    ctype.ID(m.ArticleID),
		CommentID:    ctype.ID(m.CommentID),
		ArticleTitle: m.ArticleTitle,
		LinkTitle:    m.LinkTitle,
		LinkHerf:     m.LinkHref,
		IsRead:       m.IsRead,
		ReadAt:       m.ReadAt,
	}
	if m.ActionUser != 0 {
		id := ctype.ID(m.ActionUser)
		item.ActionUserID = &id
	}
	if m.ActionUserNickname != "" {
		nickname := m.ActionUserNickname
		item.ActionUserNickname = &nickname
	}
	if m.ActionUserAvatar != "" {
		avatar := m.ActionUserAvatar
		item.ActionUserAvatar = &avatar
	}
	return item
}

// toIDs 把 ctype.ID 切片转成 uint64 切片。
func toIDs(ids []ctype.ID) []uint64 {
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		out = append(out, uint64(id))
	}
	return out
}
