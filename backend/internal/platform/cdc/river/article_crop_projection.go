package river

import (
	"strings"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/es/articlesync"

	"github.com/go-mysql-org/go-mysql/canal"
)

// articleCropTargetType 是文章封面在通用取景表里的主体类型。
const articleCropTargetType = "article"

// syncArticleDocsByCropRows 处理通用取景表（picture_crop_models）的变更。
//
// 为什么需要这条：封面取景不是 article_models 的列，它单独存在通用取景表里
// （ADR-0020）。作者在创作页调整取景时，文章行可能没有任何字段变化，于是
// 只监听 article_models 的话，ES 文档里的 crop 会一直停在旧值 —— 列表封面
// 便与新构图不一致。这里按 `target_type = 'article'` 取出受影响的文章 ID，
// 直接重建它们的文档。
//
// 轮播（`target_type = 'banner'`）不在这里处理：它不进文章索引。
func (r *River) syncArticleDocsByCropRows(e *canal.RowsEvent) error {
	articleIDs := collectArticleCropTargetIDs(e)
	if len(articleIDs) == 0 {
		return nil
	}
	return articlesync.SyncESDocs(r.db, r.es, articleIDs)
}

// collectArticleCropTargetIDs 从取景表行事件里挑出文章主体 ID。
//
// DELETE 事件的 Rows 里只有被删的旧行，UPDATE 是"旧行 + 新行"成对出现，
// 两种情况都按同一个口径扫：只要行的 target_type 是 article，就取它的 target_id。
func collectArticleCropTargetIDs(e *canal.RowsEvent) []ctype.ID {
	if e == nil || e.Table == nil || len(e.Rows) == 0 {
		return nil
	}

	typeIndex, targetIndex := columnIndex(e, "target_type"), columnIndex(e, "target_id")
	if typeIndex < 0 || targetIndex < 0 {
		return nil
	}

	seen := make(map[ctype.ID]struct{}, len(e.Rows))
	out := make([]ctype.ID, 0, len(e.Rows))
	for _, row := range e.Rows {
		if typeIndex >= len(row) || targetIndex >= len(row) {
			continue
		}
		if !strings.EqualFold(scanCellString(row[typeIndex]), articleCropTargetType) {
			continue
		}
		var id ctype.ID
		if err := id.Scan(row[targetIndex]); err != nil || id.IsZero() {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// columnIndex 返回列名在行里的下标；找不到返回 -1。
func columnIndex(e *canal.RowsEvent, name string) int {
	for index, column := range e.Table.Columns {
		if strings.EqualFold(column.Name, name) {
			return index
		}
	}
	return -1
}

// scanCellString 把 binlog 单元格值读成字符串（兼容 []byte 与 string）。
func scanCellString(value any) string {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		return ""
	}
}
