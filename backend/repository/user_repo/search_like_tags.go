package user_repo

import (
	"myblogx/models"
	"myblogx/models/ctype"

	"gorm.io/gorm"
)

func idKeywordValues(ids []ctype.ID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		result = append(result, id.String())
	}
	return result
}

// LoadLikeTagIDs 加载用户偏好标签 ID 列表。
func LoadLikeTagIDs(db *gorm.DB, userID ctype.ID) ([]ctype.ID, error) {
	if userID == 0 || db == nil {
		return nil, nil
	}

	var userConf models.UserConfModel
	if err := db.Select("user_id", "like_tags").Take(&userConf, userID).Error; err != nil {
		return nil, err
	}
	return userConf.LikeTags, nil
}

// BuildLikeTagsQuery 根据用户偏好标签，为搜索 DSL 增加加权条件。
func BuildLikeTagsQuery(query map[string]any, likeTagIDs []ctype.ID) map[string]any {
	if len(likeTagIDs) == 0 {
		return query
	}

	functionScore, ok := query["function_score"].(map[string]any)
	if !ok {
		return query
	}
	queryMap, ok := functionScore["query"].(map[string]any)
	if !ok {
		return query
	}
	boolQuery, ok := queryMap["bool"].(map[string]any)
	if !ok {
		return query
	}

	should, _ := boolQuery["should"].([]any)
	should = append(should, map[string]any{
		"terms": map[string]any{
			"tags.id": idKeywordValues(likeTagIDs),
			"boost":   2,
		},
	})
	boolQuery["should"] = should

	return query
}
