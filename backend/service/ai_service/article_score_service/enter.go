package article_score_service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/ai_service/ai_scoring"
	"myblogx/service/site_service"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const articleAIScoringPromptVersion = "article-scoring-v1"

func EnsureArticleScoreIfMissingAsync(db *gorm.DB, logger *logrus.Logger, runtimeSite *site_service.RuntimeConfigService, articleID ctype.ID) {
	if db == nil || runtimeSite == nil || articleID == 0 {
		return
	}

	go func() {
		if err := EnsureArticleScoreIfMissing(db, runtimeSite, articleID); err != nil && logger != nil {
			logger.Errorf("自动补充文章质量评分失败: article_id=%s err=%v", articleID.String(), err)
		}
	}()
}

func EnsureArticleScoreIfMissing(db *gorm.DB, runtimeSite *site_service.RuntimeConfigService, articleID ctype.ID) error {
	if db == nil || runtimeSite == nil || articleID == 0 {
		return nil
	}

	aiConf := runtimeSite.GetRuntimeAI()
	if !aiConf.Enable {
		return nil
	}

	article, err := LoadArticleForScoring(db, articleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if article.EffectivePublishStatus() != enum.ArticleStatusPublished {
		return nil
	}

	record, err := LoadLatestArticleAIScoreRecord(db, articleID)
	if err != nil {
		return err
	}
	if record != nil {
		return nil
	}

	title := strings.TrimSpace(article.Title)
	content := strings.TrimSpace(article.Content)
	scoreResp, err := ai_scoring.ScoreArticleQuality(aiConf, ai_scoring.ArticleScoreRequest{
		Title:   title,
		Content: content,
	})
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		existing, err := LoadLatestArticleAIScoreRecord(tx, articleID)
		if err != nil {
			return err
		}
		if existing != nil {
			return nil
		}
		_, err = PersistArticleAIScoreRecord(tx, article, article.AuthorID, title, content, scoreResp, aiConf)
		return err
	})
}

func LoadArticleForScoring(db *gorm.DB, articleID ctype.ID) (*models.ArticleModel, error) {
	var article models.ArticleModel
	if err := db.Select("id", "author_id", "title", "content", "updated_at", "publish_status").Take(&article, articleID).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

func LoadLatestArticleAIScoreRecord(db *gorm.DB, articleID ctype.ID) (*models.ArticleAIScoreRecordModel, error) {
	var record models.ArticleAIScoreRecordModel
	if err := db.Where("article_id = ?", articleID).Order("created_at desc").Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func PersistArticleAIScoreRecord(
	db *gorm.DB,
	article *models.ArticleModel,
	userID ctype.ID,
	title string,
	content string,
	scoreResp *ai_scoring.ArticleScoreResponse,
	aiConf conf.AI,
) (*models.ArticleAIScoreRecordModel, error) {
	if article == nil || scoreResp == nil {
		return nil, gorm.ErrInvalidData
	}

	dimensionsJSON, err := json.Marshal(scoreResp.Dimensions)
	if err != nil {
		return nil, err
	}
	mainIssuesJSON, err := json.Marshal(scoreResp.MainIssues)
	if err != nil {
		return nil, err
	}

	hashBytes := sha256.Sum256([]byte(content))
	record := &models.ArticleAIScoreRecordModel{
		ArticleID:                article.ID,
		UserID:                   userID,
		TitleSnapshot:            title,
		ContentHash:              hex.EncodeToString(hashBytes[:]),
		ContentLength:            len([]rune(content)),
		ArticleUpdatedAtSnapshot: &article.UpdatedAt,
		AITotalScore:             scoreResp.AITotalScore,
		TotalScore:               scoreResp.TotalScore,
		ScoreLevel:               scoreResp.ScoreLevel,
		ArticleType:              scoreResp.ArticleType,
		DimensionsJSON:           string(dimensionsJSON),
		MainIssuesJSON:           string(mainIssuesJSON),
		OverallComment:           scoreResp.OverallComment,
		Provider:                 "llm",
		ModelName:                resolveArticleAIScoreModelName(aiConf),
		PromptVersion:            articleAIScoringPromptVersion,
	}
	if err := db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func resolveArticleAIScoreModelName(aiConf conf.AI) string {
	return strings.TrimSpace(aiConf.ChatModel)
}
