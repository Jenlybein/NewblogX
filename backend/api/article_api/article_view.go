// 该接口与文章内容返回接口分开，加快文章内容返回速度

package article_api

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"myblogx/utils/user_info"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h ArticleApi) ArticleVisitView(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleViewCountRequest](c)
	redisDeps := redis_service.NewDeps(h.App.Redis, h.App.Logger)

	var articleMeta struct {
		ID               ctype.ID
		AuthorID         ctype.ID
		PublishStatus    enum.ArticleStatus
		VisibilityStatus enum.ArticleVisibilityStatus
	}
	if err := h.App.DB.Model(&models.ArticleModel{}).
		Select("id", "author_id", "publish_status", "visibility_status").
		Take(&articleMeta, "id = ?", cr.ArticleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res.FailWithMsg("文章不存在", c)
			return
		}
		h.App.Logger.Errorf("查询文章访问元信息失败: 错误=%v 文章ID=%d", err, cr.ArticleID)
		res.FailWithMsg("服务器内部错误", c)
		return
	}
	articleVisible := models.ArticleModel{
		PublishStatus:    articleMeta.PublishStatus,
		VisibilityStatus: articleMeta.VisibilityStatus,
	}
	if !articleVisible.IsPublicVisible() {
		res.FailWithMsg("文章不存在或未发布", c)
		return
	}

	// 获取用户登录信息
	var authResult *user_service.AuthResult
	token := jwts.GetTokenByGin(c)
	if token != "" {
		authenticator := user_service.NewAuthenticator(
			h.App.DB,
			h.App.Logger,
			h.App.JWT,
			redis_service.Deps{Client: h.App.Redis, Logger: h.App.Logger},
		)
		if result, err := authenticator.AuthenticateAccessToken(token); err == nil {
			authResult = result
		}
	}

	if authResult == nil {
		// TODO：获取更真实可靠的ip和设备id防爬虫？
		// 未登录用户，靠 ip 和 设备id 进行确认
		ip := user_info.GetClientIP(c)
		ua := c.GetHeader("User-Agent")
		if ip == "" || ua == "" {
			res.OkWithMsg("无法获取有效访问标识，跳过统计", c)
			return
		}

		// 先生成 ip:ua 字符串，再转为字节切片计算 MD5
		hash := md5.Sum([]byte(fmt.Sprintf("%s:%s", ip, ua)))
		key := fmt.Sprintf("g:%s", hex.EncodeToString(hash[:]))

		if redis_article.GetGuestArticleHistoryCache(redisDeps, int(cr.ArticleID), key) {
			fmt.Printf("访客已经阅读过该文章, %d", cr.ArticleID)
			res.OkWithMsg("访客已访问过该文章", c)
			return
		}

		redis_article.SetGuestArticleHistoryCache(redisDeps, int(cr.ArticleID), key)
	} else {
		claims := authResult.Claims
		// 已登录用户，靠用户 id 进行确认
		if redis_article.GetUserArticleHistoryCache(redisDeps, int(cr.ArticleID), int(claims.UserID)) {
			// TODO：加消息队列通知数据库更新访问历史
			res.OkWithMsg("用户已访问过该文章", c)
			return
		}

		// 同时更新数据库浏览历史(TODO：可选改消息队列异步)
		articleHistory := models.UserArticleViewHistoryModel{
			ArticleID: cr.ArticleID,
			UserID:    claims.UserID,
		}

		if err := h.App.DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "article_id"},
				{Name: "user_id"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"updated_at": time.Now(),
				"deleted_at": nil,
			}),
		}).Create(&articleHistory).Error; err != nil {
			h.App.Logger.Errorf("数据库更新浏览历史失败: 错误=%v 文章ID=%d", err, cr.ArticleID)
			res.FailWithMsg("服务器内部错误", c)
			return
		}

		redis_article.SetUserArticleHistoryCache(redisDeps, int(cr.ArticleID), int(claims.UserID))
	}

	if err := user_service.StatApplyArticleDelta(h.App.DB, articleMeta.AuthorID, 0, 1); err != nil {
		h.App.Logger.Errorf("更新作者文章访问统计失败: 错误=%v 文章ID=%d 作者ID=%d", err, cr.ArticleID, articleMeta.AuthorID)
		res.FailWithMsg("服务器内部错误", c)
		return
	}
	redis_article.SetCacheView(redisDeps, cr.ArticleID, 1)
	res.OkWithMsg("文章访问量增加成功", c)
}
