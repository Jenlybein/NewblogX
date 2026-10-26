package readmodel

import (
	"sort"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"

	"gorm.io/gorm"
)

type UserDisplay struct {
	ID       ctype.ID
	Nickname string
	Avatar   string
	Abstract string
	Role     enum.RoleType
}

func NormalizeIDs(ids []ctype.ID) []ctype.ID {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[ctype.ID]struct{}, len(ids))
	result := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func LoadUserDisplayMap(db *gorm.DB, userIDs []ctype.ID) (map[ctype.ID]UserDisplay, error) {
	result := make(map[ctype.ID]UserDisplay)
	userIDs = NormalizeIDs(userIDs)
	if len(userIDs) == 0 {
		return result, nil
	}

	var rows []userDisplayRow
	if err := db.Select("id", "nickname", "avatar", "abstract", "role").
		Where("id IN ?", userIDs).
		Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		result[row.ID] = UserDisplay{
			ID:       row.ID,
			Nickname: row.Nickname,
			Avatar:   row.Avatar,
			Abstract: row.Abstract,
			Role:     row.Role,
		}
	}
	return result, nil
}

// userDisplayRow 是 LoadUserDisplayMap 的查询投影。
//
// 只有 SELECT 里那几列 —— 这里刻意不用整行结构：列名写在这里，
// 写错了查询会直接报 Unknown column，不会静默读空值。
type userDisplayRow struct {
	ID       ctype.ID
	Nickname string
	Avatar   string
	Abstract string
	Role     enum.RoleType
}

// TableName 固定表名。
func (userDisplayRow) TableName() string { return "user_models" }
