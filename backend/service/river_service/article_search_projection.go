package river_service

import (
	"reflect"
	"sort"
	"strings"

	"myblogx/models/ctype"
	"myblogx/service/es_service"

	"github.com/go-mysql-org/go-mysql/canal"
)

func (r *River) handleArticleSearchProjectionEvent(e *canal.RowsEvent) (bool, error) {
	table := strings.ToLower(strings.TrimSpace(e.Table.Name))
	switch table {
	case "article_models":
		return true, r.syncArticleDocsByArticleRows(e)
	case "user_models":
		return true, r.syncArticleDocsByUserRows(e)
	case "category_models":
		return true, r.syncArticleDocsByCategoryRows(e)
	case "tag_models":
		return true, r.syncArticleDocsByTagRows(e)
	case "article_tag_models":
		return true, r.syncArticleDocsByArticleTagRows(e)
	case "user_top_article_models":
		return true, r.syncArticleDocsByTopRows(e)
	default:
		return false, nil
	}
}

func (r *River) syncArticleDocsByArticleRows(e *canal.RowsEvent) error {
	switch e.Action {
	case canal.DeleteAction:
		articleIDs := collectRowsIDs(e, "id")
		if len(articleIDs) == 0 {
			return nil
		}
		return es_service.SyncArticleSearchProjection(r.db, r.es, es_service.ArticleSearchProjectionEvent{
			Type: es_service.ArticleSearchProjectionArticleDelete,
			IDs:  articleIDs,
		})
	case canal.InsertAction:
		snapshots, err := collectArticleRowSnapshots(e)
		if err != nil {
			return err
		}
		if len(snapshots) == 0 {
			return nil
		}
		return es_service.SyncESDocsByArticleSnapshots(r.db, r.es, snapshots)
	case canal.UpdateAction:
		deltas := collectArticleModelDeltas(e)
		if len(deltas) == 0 {
			return nil
		}
		return es_service.UpdateESDocsByArticleDeltas(r.db, r.es, deltas)
	default:
		return nil
	}
}

func (r *River) syncArticleDocsByUserRows(e *canal.RowsEvent) error {
	userIDs := collectRowsIDs(e, "id")
	if len(userIDs) == 0 {
		return nil
	}
	if err := es_service.SyncArticleSearchProjection(r.db, r.es, es_service.ArticleSearchProjectionEvent{
		Type: es_service.ArticleSearchProjectionAuthorSnapshot,
		IDs:  userIDs,
	}); err != nil {
		return err
	}
	return es_service.SyncArticleSearchProjection(r.db, r.es, es_service.ArticleSearchProjectionEvent{
		Type: es_service.ArticleSearchProjectionTopUserChanged,
		IDs:  userIDs,
	})
}

func (r *River) syncArticleDocsByCategoryRows(e *canal.RowsEvent) error {
	categoryIDs := collectRowsIDs(e, "id")
	if len(categoryIDs) == 0 {
		return nil
	}
	return es_service.SyncArticleSearchProjection(r.db, r.es, es_service.ArticleSearchProjectionEvent{
		Type: es_service.ArticleSearchProjectionCategorySnapshot,
		IDs:  categoryIDs,
	})
}

func (r *River) syncArticleDocsByTagRows(e *canal.RowsEvent) error {
	tagIDs := collectRowsIDs(e, "id")
	if len(tagIDs) == 0 {
		return nil
	}
	return es_service.SyncArticleSearchProjection(r.db, r.es, es_service.ArticleSearchProjectionEvent{
		Type: es_service.ArticleSearchProjectionTagSnapshot,
		IDs:  tagIDs,
	})
}

func (r *River) syncArticleDocsByArticleTagRows(e *canal.RowsEvent) error {
	articleIDs := collectRowsIDs(e, "article_id")
	if len(articleIDs) == 0 {
		return nil
	}
	return es_service.SyncArticleSearchProjection(r.db, r.es, es_service.ArticleSearchProjectionEvent{
		Type: es_service.ArticleSearchProjectionArticleTagsChanged,
		IDs:  articleIDs,
	})
}

func (r *River) syncArticleDocsByTopRows(e *canal.RowsEvent) error {
	articleIDs := collectRowsIDs(e, "article_id")
	if len(articleIDs) == 0 {
		return nil
	}
	return es_service.SyncArticleSearchProjection(r.db, r.es, es_service.ArticleSearchProjectionEvent{
		Type: es_service.ArticleSearchProjectionArticleTopChanged,
		IDs:  articleIDs,
	})
}

func collectRowsIDs(e *canal.RowsEvent, columnName string) []ctype.ID {
	if e == nil || e.Table == nil {
		return nil
	}
	columnIndex := -1
	for index, column := range e.Table.Columns {
		if strings.EqualFold(column.Name, columnName) {
			columnIndex = index
			break
		}
	}
	if columnIndex < 0 {
		return nil
	}

	idSet := make(map[ctype.ID]struct{}, len(e.Rows))
	for _, row := range e.Rows {
		if columnIndex >= len(row) {
			continue
		}
		var id ctype.ID
		if err := id.Scan(row[columnIndex]); err != nil || id == 0 {
			continue
		}
		idSet[id] = struct{}{}
	}

	result := make([]ctype.ID, 0, len(idSet))
	for id := range idSet {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func collectArticleModelDeltas(e *canal.RowsEvent) []es_service.ArticleModelDelta {
	if e == nil || e.Table == nil || len(e.Rows) == 0 || len(e.Rows)%2 != 0 {
		return nil
	}

	idColumnIndex := -1
	for index, column := range e.Table.Columns {
		if strings.EqualFold(strings.TrimSpace(column.Name), "id") {
			idColumnIndex = index
			break
		}
	}
	if idColumnIndex < 0 {
		return nil
	}

	deltas := make([]es_service.ArticleModelDelta, 0, len(e.Rows)/2)
	for i := 0; i < len(e.Rows); i += 2 {
		before := e.Rows[i]
		after := e.Rows[i+1]
		if idColumnIndex >= len(after) || idColumnIndex >= len(before) {
			continue
		}

		var articleID ctype.ID
		if err := articleID.Scan(after[idColumnIndex]); err != nil || articleID == 0 {
			if err = articleID.Scan(before[idColumnIndex]); err != nil || articleID == 0 {
				continue
			}
		}

		changed := make(map[string]any)
		for colIndex, column := range e.Table.Columns {
			if colIndex >= len(before) || colIndex >= len(after) {
				continue
			}
			if reflect.DeepEqual(before[colIndex], after[colIndex]) {
				continue
			}
			columnName := strings.ToLower(strings.TrimSpace(column.Name))
			if columnName == "" || columnName == "id" {
				continue
			}
			changed[columnName] = after[colIndex]
		}
		if len(changed) == 0 {
			continue
		}
		deltas = append(deltas, es_service.ArticleModelDelta{
			ArticleID: articleID,
			Changed:   changed,
		})
	}
	return deltas
}
