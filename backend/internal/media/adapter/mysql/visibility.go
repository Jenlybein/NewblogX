package mysql

import (
	"context"
	"strings"

	"myblogx/internal/media/domain"

	"gorm.io/gorm"
)

// VisibilityStore 提供"这批图片地址现在能不能对外展示"的批量查询。
//
// 为什么需要单独一个 Store 而不是塞进 Repo：它是**展示策略**的只读依赖，
// 调用方（imagedisplay 的 Policy）只需要这一个能力。方法少、语义窄，
// 便于替换与断言；塞进 Repo 会让它成为第二个大接口。
type VisibilityStore struct {
	db *gorm.DB
}

// NewVisibilityStore 构造可见性查询。
func NewVisibilityStore(db *gorm.DB) *VisibilityStore { return &VisibilityStore{db: db} }

// VisibilityByURL 批量判定地址是否可展示。
//
// 口径：**只有 pass 可展示**。其余状态（待审核 / 疑似 / 违规 / 孤儿 / 已删除）
// 一律不可见。这条判定就是 ADR-0034 的"闸门"本体 —— 它只回答"能不能给地址"，
// 不关心给哪个地址（样式派生在外层）。
//
// 查不到的地址按**不可展示**处理：一条 URL 不在图片表里，说明它要么是外链
// （外层已经放过，不会走到这里），要么是图已从库里删掉（对象可能还在，
// 但既然没有记录就没法证明它过审了）。
func (s *VisibilityStore) VisibilityByURL(ctx context.Context, urls []string) (map[string]bool, error) {
	out := make(map[string]bool, len(urls))
	if len(urls) == 0 {
		return out, nil
	}

	// 去重后再查：一篇文章里同一个封面地址可能出现在多个字段上。
	unique := make([]string, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, raw := range urls {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		unique = append(unique, url)
	}
	if len(unique) == 0 {
		return out, nil
	}

	var rows []imageRow
	if err := s.db.WithContext(ctx).
		Select("url", "status").
		Where("url IN ?", unique).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		out[strings.TrimSpace(row.URL)] = domain.Status(row.Status) == domain.StatusPass
	}
	return out, nil
}
