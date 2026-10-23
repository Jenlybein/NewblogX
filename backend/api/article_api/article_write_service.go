package article_api

import (
	"errors"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/image_service"
	"myblogx/service/site_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"myblogx/utils/markdown"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var (
	errArticleUserNotFound     = errors.New("用户不存在")
	errArticleNotFound         = errors.New("文章不存在")
	errArticleCategoryNotFound = errors.New("分类不存在")
	errArticleTagInvalid       = errors.New("标签不存在或已停用")
)

type articleWriteService struct {
	DB          *gorm.DB
	Logger      *logrus.Logger
	RuntimeSite *site_service.RuntimeConfigService
}

type articleUpdateResult struct {
	UpdateMap  map[string]any
	OldTagIDs  []ctype.ID
	NewTagIDs  []ctype.ID
	TagChanged bool
	ContentSet bool
	Noop       bool
}

func newArticleWriteService(db *gorm.DB, logger *logrus.Logger, runtimeSite *site_service.RuntimeConfigService) *articleWriteService {
	return &articleWriteService{
		DB:          db,
		Logger:      logger,
		RuntimeSite: runtimeSite,
	}
}

func (h *articleWriteService) CreateArticle(claims *jwts.MyClaims, cr ArticleCreateRequest) (*models.ArticleModel, []ctype.ID, error) {
	if h.RuntimeSite == nil {
		return nil, nil, errors.New("运行时配置服务未初始化")
	}
	runtimeSite := h.RuntimeSite.GetRuntimeSite()

	if err := h.DB.Take(&models.UserModel{}, claims.UserID).Error; err != nil {
		return nil, nil, errArticleUserNotFound
	}

	if err := validateArticleCategory(h.DB, claims.UserID, cr.CategoryID); err != nil {
		return nil, nil, errArticleCategoryNotFound
	}
	coverURL, err := resolveArticleCoverURL(h.DB, cr.CoverImageID)
	if err != nil {
		return nil, nil, err
	}

	tagList, err := loadEnabledTagsByIDs(h.DB, cr.TagIDs)
	if err != nil {
		return nil, nil, err
	}

	safeContent := markdown.MdToSafe(cr.Content)
	if cr.Abstract == "" {
		textContent := markdown.MdToText(safeContent)
		cr.Abstract = markdown.ExtractText(textContent, 200)
	}

	publishStatus := resolvePublishStatus(cr.Status, runtimeSite.Article.SkipExamining)
	visibilityStatus := normalizeArticleVisibilityStatus(cr.VisibilityStatus)
	var submittedAt *time.Time
	if publishStatus == enum.ArticleStatusExamining || publishStatus == enum.ArticleStatusPublished && cr.Status == enum.ArticleStatusExamining {
		now := time.Now()
		submittedAt = &now
	}

	article := &models.ArticleModel{
		AuthorID:         claims.UserID,
		Title:            cr.Title,
		Abstract:         cr.Abstract,
		Content:          safeContent,
		CategoryID:       cr.CategoryID,
		Cover:            coverURL,
		CommentsToggle:   cr.CommentsToggle,
		PublishStatus:    publishStatus,
		VisibilityStatus: visibilityStatus,
		SubmittedAt:      submittedAt,
	}

	tagIDs := extractTagIDs(tagList)
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(article).Error; err != nil {
			return err
		}
		if err := syncArticleTags(tx, article.ID, tagIDs); err != nil {
			return err
		}
		if article.PublishStatus == enum.ArticleStatusExamining {
			if _, err := createReviewTask(tx, *article, models.ArticleReviewTaskSourceCreate, claims.UserID); err != nil {
				return err
			}
		}
		return user_service.StatApplyArticleDelta(tx, article.AuthorID, 1, 0)
	}); err != nil {
		return nil, nil, err
	}

	return article, tagIDs, nil
}

func (h *articleWriteService) UpdateArticle(articleID ctype.ID, claims *jwts.MyClaims, cr ArticleUpdateRequest) (*models.ArticleModel, articleUpdateResult, error) {
	if err := h.DB.Take(&models.UserModel{}, claims.UserID).Error; err != nil {
		return nil, articleUpdateResult{}, errArticleUserNotFound
	}

	var article models.ArticleModel
	if err := h.DB.Take(&article, "id = ? AND author_id = ?", articleID, claims.UserID).Error; err != nil {
		return nil, articleUpdateResult{}, errArticleNotFound
	}

	updateMap := map[string]any{}
	if cr.Title != nil {
		updateMap["title"] = *cr.Title
	}
	if cr.Content != nil {
		updateMap["content"] = markdown.MdToSafe(*cr.Content)
	}
	if cr.Abstract != nil {
		abstract := *cr.Abstract
		if abstract == "" {
			content := article.Content
			if cr.Content != nil {
				content = markdown.MdToSafe(*cr.Content)
			}
			textContent := markdown.MdToText(content)
			abstract = markdown.ExtractText(textContent, 200)
		}
		updateMap["abstract"] = abstract
	}

	if cr.CategoryID != nil {
		if *cr.CategoryID == 0 {
			updateMap["category_id"] = nil
		} else {
			if err := validateArticleCategory(h.DB, claims.UserID, cr.CategoryID); err != nil {
				return nil, articleUpdateResult{}, errArticleCategoryNotFound
			}
			updateMap["category_id"] = cr.CategoryID
		}
	}
	if cr.CoverImageID != nil {
		coverURL, err := resolveArticleCoverURL(h.DB, cr.CoverImageID)
		if err != nil {
			return nil, articleUpdateResult{}, err
		}
		updateMap["cover"] = coverURL
	}
	if cr.CommentsToggle != nil {
		updateMap["comments_toggle"] = *cr.CommentsToggle
	}
	if cr.VisibilityStatus != nil {
		updateMap["visibility_status"] = normalizeArticleVisibilityStatus(*cr.VisibilityStatus)
	}

	result := articleUpdateResult{
		UpdateMap:  updateMap,
		ContentSet: cr.Content != nil,
	}

	if cr.TagIDs != nil {
		oldTagIDs, err := loadArticleTagIDs(h.DB, article.ID)
		if err != nil {
			return nil, articleUpdateResult{}, err
		}

		tagList, err := loadEnabledTagsByIDs(h.DB, *cr.TagIDs)
		if err != nil {
			return nil, articleUpdateResult{}, err
		}
		result.OldTagIDs = oldTagIDs
		result.NewTagIDs = extractTagIDs(tagList)
		result.TagChanged = true
	}

	if len(updateMap) == 0 && !result.TagChanged {
		result.Noop = true
		return &article, result, nil
	}

	if h.RuntimeSite == nil {
		return nil, articleUpdateResult{}, errors.New("运行时配置服务未初始化")
	}
	nextPublishStatus := article.EffectivePublishStatus()
	submitRequested := cr.Status != nil && *cr.Status == enum.ArticleStatusExamining
	draftRequested := cr.Status != nil && *cr.Status == enum.ArticleStatusDraft
	shouldReSubmitAfterEdit := !h.RuntimeSite.GetRuntimeArticle().SkipExamining &&
		(cr.Content != nil || cr.Title != nil || cr.Abstract != nil || cr.CategoryID != nil || cr.TagIDs != nil || cr.CoverImageID != nil || cr.CommentsToggle != nil) &&
		(article.EffectivePublishStatus() == enum.ArticleStatusPublished || article.EffectivePublishStatus() == enum.ArticleStatusExamining)

	switch {
	case draftRequested:
		nextPublishStatus = enum.ArticleStatusDraft
		updateMap["publish_status"] = nextPublishStatus
		updateMap["submitted_at"] = nil
		updateMap["reviewed_at"] = nil
		updateMap["reviewed_by"] = nil
	case submitRequested:
		nextPublishStatus = resolvePublishStatus(enum.ArticleStatusExamining, h.RuntimeSite.GetRuntimeArticle().SkipExamining)
		updateMap["publish_status"] = nextPublishStatus
		now := time.Now()
		updateMap["submitted_at"] = &now
		updateMap["reviewed_at"] = nil
		updateMap["reviewed_by"] = nil
	case shouldReSubmitAfterEdit:
		nextPublishStatus = enum.ArticleStatusExamining
		updateMap["publish_status"] = nextPublishStatus
		now := time.Now()
		updateMap["submitted_at"] = &now
		updateMap["reviewed_at"] = nil
		updateMap["reviewed_by"] = nil
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if len(updateMap) > 0 {
			if err := tx.Model(&article).Updates(updateMap).Error; err != nil {
				return err
			}
		}
		if result.TagChanged {
			if err := syncArticleTags(tx, article.ID, result.NewTagIDs); err != nil {
				return err
			}
		}
		if nextPublishStatus == enum.ArticleStatusDraft {
			if err := cancelPendingReviewTasks(tx, article.ID, claims.UserID, nextPublishStatus, "作者改回草稿"); err != nil {
				return err
			}
		}
		if nextPublishStatus == enum.ArticleStatusExamining {
			reason := "作者重新提交审核"
			if submitRequested && article.EffectivePublishStatus() == enum.ArticleStatusDraft {
				reason = "作者提交审核"
			}
			if err := cancelPendingReviewTasks(tx, article.ID, claims.UserID, nextPublishStatus, reason); err != nil {
				return err
			}
			updatedArticle := article
			updatedArticle.PublishStatus = nextPublishStatus
			if title, ok := updateMap["title"].(string); ok {
				updatedArticle.Title = title
			}
			if _, err := createReviewTask(tx, updatedArticle, models.ArticleReviewTaskSourceEdit, claims.UserID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, articleUpdateResult{}, err
	}

	return &article, result, nil
}

func resolveArticleCoverURL(db *gorm.DB, imageID *ctype.ID) (string, error) {
	if imageID == nil {
		return "", nil
	}
	return image_service.ResolveImageURLByID(db, *imageID)
}
