package sitemsg_api_test

import (
	"encoding/json"
	"myblogx/api/sitemsg_api"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/models/enum/global_notif_enum"
	"myblogx/models/enum/message_enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newSitemsgCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func readSitemsgBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
	return body
}

func readSitemsgCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	return int(readSitemsgBody(t, w)["code"].(float64))
}

func newSitemsgAPI() sitemsg_api.SitemsgApi {
	return sitemsg_api.New(sitemsg_api.Deps{
		DB: testutil.DB(),
	})
}

func setupSitemsgEnv(t *testing.T) *models.UserModel {
	t.Helper()
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.ArticleMessageModel{},
		&models.ChatSessionModel{},
		&models.GlobalNotifModel{},
		&models.UserGlobalNotifModel{},
	)

	user := &models.UserModel{
		Username: "msg_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	return user
}

func TestSitemsgUserViewCountsUnreadGlobalNotif(t *testing.T) {
	user := setupSitemsgEnv(t)
	db := testutil.DB()
	api := newSitemsgAPI()

	registerAt := time.Now().Add(-24 * time.Hour).Round(time.Second)
	if err := db.Model(user).Update("created_at", registerAt).Error; err != nil {
		t.Fatalf("更新用户注册时间失败: %v", err)
	}
	if err := db.Take(user, user.ID).Error; err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}

	msgs := []models.ArticleMessageModel{
		{ReceiverID: user.ID, Type: message_enum.CommentArticleType, Content: "comment-unread"},
		{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "digg-unread"},
		{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "system-unread"},
		{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "system-read", IsRead: true},
	}
	if err := db.Create(&msgs).Error; err != nil {
		t.Fatalf("创建站内消息失败: %v", err)
	}

	chatSessions := []models.ChatSessionModel{
		{SessionID: "chat:1:2", UserID: user.ID, ReceiverID: 2, UnreadCount: 2},
		{SessionID: "chat:1:3", UserID: user.ID, ReceiverID: 3, UnreadCount: 1},
		{SessionID: "chat:4:5", UserID: user.ID, ReceiverID: 4, UnreadCount: 0},
	}
	if err := db.Create(&chatSessions).Error; err != nil {
		t.Fatalf("创建聊天会话失败: %v", err)
	}

	notifs := []models.GlobalNotifModel{
		{
			Title:           "global-unread",
			Content:         "global-unread",
			UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
			ExpireTime:      time.Now().Add(24 * time.Hour),
		},
		{
			Title:           "global-read",
			Content:         "global-read",
			UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
			ExpireTime:      time.Now().Add(24 * time.Hour),
		},
		{
			Title:           "global-deleted",
			Content:         "global-deleted",
			UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
			ExpireTime:      time.Now().Add(24 * time.Hour),
		},
	}
	if err := db.Create(&notifs).Error; err != nil {
		t.Fatalf("创建全局通知失败: %v", err)
	}

	now := time.Now()
	userStates := []models.UserGlobalNotifModel{
		{MsgID: notifs[1].ID, UserID: user.ID, IsRead: true, ReadAt: &now},
		{
			Model: models.Model{
				DeletedAt: gorm.DeletedAt{Time: now, Valid: true},
			},
			MsgID:  notifs[2].ID,
			UserID: user.ID,
		},
	}
	if err := db.Create(&userStates).Error; err != nil {
		t.Fatalf("创建全局通知用户态失败: %v", err)
	}

	c, w := newSitemsgCtx()
	setClaims(c, user)
	api.SitemsgUserView(c)
	if code := readSitemsgCode(t, w); code != 0 {
		t.Fatalf("查询用户消息统计失败, body=%s", w.Body.String())
	}

	data := readSitemsgBody(t, w)["data"].(map[string]any)
	if int(data["comment_msg_count"].(float64)) != 1 {
		t.Fatalf("评论未读数异常, body=%s", w.Body.String())
	}
	if int(data["digg_favor_msg_count"].(float64)) != 1 {
		t.Fatalf("点赞/收藏未读数异常, body=%s", w.Body.String())
	}
	if int(data["private_msg_count"].(float64)) != 3 {
		t.Fatalf("私信未读数异常, body=%s", w.Body.String())
	}
	if int(data["system_msg_count"].(float64)) != 1 {
		t.Fatalf("系统消息未读数应只包含站内系统消息, body=%s", w.Body.String())
	}
	if int(data["global_msg_count"].(float64)) != 1 {
		t.Fatalf("全局消息未读数异常, body=%s", w.Body.String())
	}
}

func setClaims(c *gin.Context, user *models.UserModel) {
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     user.Role,
			Username: user.Username,
		},
	})
}

func TestUserMsgConfViewAndUpdate(t *testing.T) {
	user := setupSitemsgEnv(t)
	api := newSitemsgAPI()

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		api.UserMsgConfView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("查询消息配置失败, body=%s", w.Body.String())
		}

		data := readSitemsgBody(t, w)["data"].(map[string]any)
		if data["digg_notice_enabled"] != true || data["private_chat_notice_enabled"] != true {
			t.Fatalf("默认消息配置异常, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.UserMsgConfResponseAndRequest{
			DiggNoticeEnabled:        false,
			CommentNoticeEnabled:     false,
			FavorNoticeEnabled:       false,
			PrivateChatNoticeEnabled: false,
		})
		api.UserMsgConfUpdateView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("更新消息配置失败, body=%s", w.Body.String())
		}
	}

	var conf models.UserConfModel
	if err := testutil.DB().Take(&conf, user.ID).Error; err != nil {
		t.Fatalf("查询配置失败: %v", err)
	}
	if conf.DiggNoticeEnabled || conf.CommentNoticeEnabled || conf.FavorNoticeEnabled || conf.PrivateChatNoticeEnabled {
		t.Fatalf("消息配置未更新: %+v", conf)
	}
}

func TestUserMsgConfViewAndUpdateFailBranches(t *testing.T) {
	user := setupSitemsgEnv(t)
	api := newSitemsgAPI()

	if err := testutil.DB().Unscoped().Delete(&models.UserConfModel{}, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("删除用户配置失败: %v", err)
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		api.UserMsgConfView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("配置不存在时查询应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.UserMsgConfResponseAndRequest{
			DiggNoticeEnabled:        false,
			CommentNoticeEnabled:     false,
			FavorNoticeEnabled:       false,
			PrivateChatNoticeEnabled: false,
		})
		api.UserMsgConfUpdateView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("配置不存在时更新应失败, body=%s", w.Body.String())
		}
	}
}

func TestSitemsgListViewFiltersByType(t *testing.T) {
	user := setupSitemsgEnv(t)
	db := testutil.DB()
	api := newSitemsgAPI()

	other := &models.UserModel{
		Username: "other_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	msgs := []models.ArticleMessageModel{
		{ReceiverID: user.ID, Type: message_enum.CommentArticleType, Content: "c1"},
		{ReceiverID: user.ID, Type: message_enum.CommentReplyType, Content: "c2"},
		{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "d1"},
		{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "s1"},
		{ReceiverID: other.ID, Type: message_enum.CommentArticleType, Content: "other"},
	}
	if err := db.Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestQuery", sitemsg_api.SitemsgListRequest{T: 1})
		api.SitemsgListView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("查询评论消息失败, body=%s", w.Body.String())
		}
		data := readSitemsgBody(t, w)["data"].(map[string]any)
		if data["has_more"].(bool) {
			t.Fatalf("评论消息 has_more 应为 false, body=%s", w.Body.String())
		}
		if len(data["list"].([]any)) != 2 {
			t.Fatalf("评论消息数量异常, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestQuery", sitemsg_api.SitemsgListRequest{T: 2})
		api.SitemsgListView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("查询点赞/收藏消息失败, body=%s", w.Body.String())
		}
		data := readSitemsgBody(t, w)["data"].(map[string]any)
		if data["has_more"].(bool) {
			t.Fatalf("点赞/收藏消息 has_more 应为 false, body=%s", w.Body.String())
		}
		if len(data["list"].([]any)) != 1 {
			t.Fatalf("点赞/收藏消息数量异常, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestQuery", sitemsg_api.SitemsgListRequest{T: 3})
		api.SitemsgListView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("查询系统消息失败, body=%s", w.Body.String())
		}
		data := readSitemsgBody(t, w)["data"].(map[string]any)
		if data["has_more"].(bool) {
			t.Fatalf("系统消息 has_more 应为 false, body=%s", w.Body.String())
		}
		if len(data["list"].([]any)) != 1 {
			t.Fatalf("系统消息数量异常, body=%s", w.Body.String())
		}
	}
}

func TestSitemsgReadViewSingleAndBatch(t *testing.T) {
	user := setupSitemsgEnv(t)
	db := testutil.DB()
	api := newSitemsgAPI()

	other := &models.UserModel{
		Username: "msg_other",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	single := models.ArticleMessageModel{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "single"}
	batchA := models.ArticleMessageModel{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "batch-a"}
	batchB := models.ArticleMessageModel{ReceiverID: user.ID, Type: message_enum.FavorArticleType, Content: "batch-b"}
	otherUserMsg := models.ArticleMessageModel{ReceiverID: other.ID, Type: message_enum.DiggArticleType, Content: "other"}
	if err := db.Create(&[]models.ArticleMessageModel{single, batchA, batchB, otherUserMsg}).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	var list []models.ArticleMessageModel
	if err := db.Order("id asc").Find(&list).Error; err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	single = list[0]
	batchA = list[1]
	batchB = list[2]
	otherUserMsg = list[3]

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgReadRequest{})
		api.SitemsgReadView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("id 和 t 同时为空时应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgReadRequest{ID: single.ID})
		api.SitemsgReadView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("单条已读失败, body=%s", w.Body.String())
		}
	}

	var singleCheck models.ArticleMessageModel
	if err := db.Take(&singleCheck, single.ID).Error; err != nil {
		t.Fatalf("查询单条消息失败: %v", err)
	}
	if !singleCheck.IsRead {
		t.Fatalf("单条消息未标记已读: %+v", singleCheck)
	}
	if singleCheck.ReadAt == nil {
		t.Fatalf("单条消息未写入 read_at: %+v", singleCheck)
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgReadRequest{ID: single.ID})
		api.SitemsgReadView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("重复标记已读应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgReadRequest{T: 2})
		api.SitemsgReadView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("批量已读失败, body=%s", w.Body.String())
		}
	}

	var batchCheck []models.ArticleMessageModel
	if err := db.Where("id in ?", []ctype.ID{batchA.ID, batchB.ID, otherUserMsg.ID}).Order("id asc").Find(&batchCheck).Error; err != nil {
		t.Fatalf("查询批量消息失败: %v", err)
	}
	if !batchCheck[0].IsRead || !batchCheck[1].IsRead {
		t.Fatalf("目标消息未全部标记已读: %+v", batchCheck)
	}
	if batchCheck[0].ReadAt == nil || batchCheck[1].ReadAt == nil {
		t.Fatalf("批量消息未写入 read_at: %+v", batchCheck)
	}
	if batchCheck[2].IsRead {
		t.Fatalf("其他用户消息不应被标记已读: %+v", batchCheck[2])
	}
	if batchCheck[2].ReadAt != nil {
		t.Fatalf("其他用户消息不应更新 read_at: %+v", batchCheck[2])
	}
}

func TestSitemsgReadViewFailBranches(t *testing.T) {
	user := setupSitemsgEnv(t)
	db := testutil.DB()
	api := newSitemsgAPI()

	other := &models.UserModel{
		Username: "read_other",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	otherMsg := models.ArticleMessageModel{ReceiverID: other.ID, Type: message_enum.SystemType, Content: "other"}
	if err := db.Create(&otherMsg).Error; err != nil {
		t.Fatalf("创建其他用户消息失败: %v", err)
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgReadRequest{ID: otherMsg.ID})
		api.SitemsgReadView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("读取他人消息应失败, body=%s", w.Body.String())
		}
	}

	var check models.ArticleMessageModel
	if err := db.Take(&check, otherMsg.ID).Error; err != nil {
		t.Fatalf("查询其他用户消息失败: %v", err)
	}
	if check.IsRead || check.ReadAt != nil {
		t.Fatalf("他人消息不应被修改: %+v", check)
	}
}

func TestSitemsgRemoveViewSingleAndBatch(t *testing.T) {
	user := setupSitemsgEnv(t)
	db := testutil.DB()
	api := newSitemsgAPI()

	other := &models.UserModel{
		Username: "remove_other",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	single := models.ArticleMessageModel{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "single"}
	batchA := models.ArticleMessageModel{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "batch-a"}
	batchB := models.ArticleMessageModel{ReceiverID: user.ID, Type: message_enum.FavorArticleType, Content: "batch-b"}
	readMsg := models.ArticleMessageModel{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "read-one", IsRead: true}
	otherUserMsg := models.ArticleMessageModel{ReceiverID: other.ID, Type: message_enum.DiggArticleType, Content: "other"}
	if err := db.Create(&[]models.ArticleMessageModel{single, batchA, batchB, readMsg, otherUserMsg}).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	var list []models.ArticleMessageModel
	if err := db.Order("id asc").Find(&list).Error; err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	single = list[0]
	batchA = list[1]
	batchB = list[2]
	readMsg = list[3]
	otherUserMsg = list[4]

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgRemoveRequest{})
		api.SitemsgRemoveView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("id 和 t 同时为空时应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgRemoveRequest{ID: single.ID})
		api.SitemsgRemoveView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("单条删除失败, body=%s", w.Body.String())
		}
	}

	var singleCount int64
	if err := db.Model(&models.ArticleMessageModel{}).Where("id = ?", single.ID).Count(&singleCount).Error; err != nil {
		t.Fatalf("查询单条消息失败: %v", err)
	}
	if singleCount != 0 {
		t.Fatalf("单条消息未被删除, count=%d", singleCount)
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgRemoveRequest{ID: single.ID})
		api.SitemsgRemoveView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("重复删除不存在消息应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgRemoveRequest{T: 2})
		api.SitemsgRemoveView(c)
		if code := readSitemsgCode(t, w); code != 0 {
			t.Fatalf("批量删除失败, body=%s", w.Body.String())
		}
	}

	var remain []models.ArticleMessageModel
	if err := db.Order("id asc").Find(&remain).Error; err != nil {
		t.Fatalf("查询剩余消息失败: %v", err)
	}
	if len(remain) != 1 {
		t.Fatalf("剩余消息数量异常: %+v", remain)
	}
	if remain[0].ID != otherUserMsg.ID {
		t.Fatalf("批量删除范围异常: %+v", remain)
	}
}

func TestSitemsgRemoveViewFailBranches(t *testing.T) {
	user := setupSitemsgEnv(t)
	db := testutil.DB()
	api := newSitemsgAPI()

	other := &models.UserModel{
		Username: "delete_other",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	otherMsg := models.ArticleMessageModel{ReceiverID: other.ID, Type: message_enum.SystemType, Content: "other"}
	if err := db.Create(&otherMsg).Error; err != nil {
		t.Fatalf("创建其他用户消息失败: %v", err)
	}

	{
		c, w := newSitemsgCtx()
		setClaims(c, user)
		c.Set("requestJson", sitemsg_api.SitemsgRemoveRequest{ID: otherMsg.ID})
		api.SitemsgRemoveView(c)
		if code := readSitemsgCode(t, w); code == 0 {
			t.Fatalf("删除他人消息应失败, body=%s", w.Body.String())
		}
	}

	var count int64
	if err := db.Model(&models.ArticleMessageModel{}).Where("id = ?", otherMsg.ID).Count(&count).Error; err != nil {
		t.Fatalf("查询其他用户消息失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("他人消息不应被删除, count=%d", count)
	}
}
