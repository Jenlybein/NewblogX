package imageref

import (
	"strings"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"

	"gorm.io/gorm"
)

// RebuildArticleRefs 重建一篇文章的图片引用。
//
// 参数是"内容 + 封面"两个字符串，而不是一整个文章行：
// 本函数只关心这两列，收窄参数后调用方不必为了让签名成立去构造整行结构。
func RebuildArticleRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, articleID ctype.ID, content, cover string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeArticle, articleID, parseArticleRefCandidates(content, cover))
}

func RebuildArticleRefsByRow(tx *gorm.DB, qiNiuConfig conf.QiNiu, snapshot rowSnapshot) error {
	articleID, err := snapshot.ID()
	if err != nil {
		return err
	}
	if snapshot.IsDeleted() {
		return DeleteOwnerRefs(tx, image_ref_enum.RefTypeArticle, articleID)
	}
	content, err := snapshot.RequireString("content")
	if err != nil {
		return err
	}
	cover, err := snapshot.RequireString("cover")
	if err != nil {
		return err
	}
	return RebuildArticleRefs(tx, qiNiuConfig, articleID, content, cover)
}

func parseArticleRefCandidates(content, cover string) []refCandidate {
	candidates := make([]refCandidate, 0, 8)
	for index, imageURL := range ParseMarkdownImageURLs(content) {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldArticleContent,
			Position: index,
			URL:      imageURL,
		})
	}
	if trimmed := strings.TrimSpace(cover); trimmed != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldArticleCover,
			Position: 0,
			URL:      trimmed,
		})
	}
	return candidates
}
