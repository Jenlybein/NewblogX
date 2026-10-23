package river_service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/es_service"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/schema"
)

type articleRowLayout struct {
	columns map[string]int
}

func newArticleRowLayout(columns []schema.TableColumn) articleRowLayout {
	result := make(map[string]int, len(columns))
	for index, column := range columns {
		result[strings.ToLower(strings.TrimSpace(column.Name))] = index
	}
	return articleRowLayout{columns: result}
}

func collectArticleRowSnapshots(e *canal.RowsEvent) ([]es_service.ArticleRowSnapshot, error) {
	if e == nil || e.Table == nil || len(e.Rows) == 0 {
		return nil, nil
	}

	layout := newArticleRowLayout(e.Table.Columns)
	snapshots := make([]es_service.ArticleRowSnapshot, 0, len(e.Rows))
	for _, row := range e.Rows {
		snapshot, err := parseArticleRowSnapshot(layout, row)
		if err != nil {
			return nil, err
		}
		if snapshot.ID == 0 {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func parseArticleRowSnapshot(layout articleRowLayout, row []any) (es_service.ArticleRowSnapshot, error) {
	var snapshot es_service.ArticleRowSnapshot
	if isDeleted, err := scanArticleDeletedAt(layout, row); err != nil {
		return snapshot, err
	} else if isDeleted {
		return snapshot, nil
	}

	var err error
	if snapshot.ID, err = scanArticleID(layout, row, "id"); err != nil {
		return snapshot, err
	}
	if snapshot.CreatedAt, err = scanArticleTime(layout, row, "created_at"); err != nil {
		return snapshot, err
	}
	if snapshot.UpdatedAt, err = scanArticleTime(layout, row, "updated_at"); err != nil {
		return snapshot, err
	}
	if snapshot.Title, err = scanArticleString(layout, row, "title"); err != nil {
		return snapshot, err
	}
	if snapshot.Abstract, err = scanArticleString(layout, row, "abstract"); err != nil {
		return snapshot, err
	}
	if snapshot.Content, err = scanArticleString(layout, row, "content"); err != nil {
		return snapshot, err
	}
	if snapshot.CategoryID, err = scanArticleNullableID(layout, row, "category_id"); err != nil {
		return snapshot, err
	}
	if snapshot.Cover, err = scanArticleString(layout, row, "cover"); err != nil {
		return snapshot, err
	}
	if snapshot.AuthorID, err = scanArticleID(layout, row, "author_id"); err != nil {
		return snapshot, err
	}
	if snapshot.ViewCount, err = scanArticleInt(layout, row, "view_count"); err != nil {
		return snapshot, err
	}
	if snapshot.DiggCount, err = scanArticleInt(layout, row, "digg_count"); err != nil {
		return snapshot, err
	}
	if snapshot.CommentCount, err = scanArticleInt(layout, row, "comment_count"); err != nil {
		return snapshot, err
	}
	if snapshot.FavorCount, err = scanArticleInt(layout, row, "favor_count"); err != nil {
		return snapshot, err
	}
	if snapshot.CommentsToggle, err = scanArticleBool(layout, row, "comments_toggle"); err != nil {
		return snapshot, err
	}
	publishStatus, err := scanArticleInt(layout, row, "publish_status")
	if err != nil {
		return snapshot, err
	}
	snapshot.PublishStatus = enum.ArticleStatus(publishStatus)
	visibilityStatus, err := scanArticleString(layout, row, "visibility_status")
	if err != nil {
		return snapshot, err
	}
	snapshot.VisibilityStatus = enum.ArticleVisibilityStatus(visibilityStatus)
	return snapshot, nil
}

func scanArticleDeletedAt(layout articleRowLayout, row []any) (bool, error) {
	value, ok := layout.value(row, "deleted_at")
	if !ok || value == nil {
		return false, nil
	}
	switch typed := value.(type) {
	case time.Time:
		return !typed.IsZero(), nil
	case *time.Time:
		return typed != nil && !typed.IsZero(), nil
	case string:
		return strings.TrimSpace(typed) != "", nil
	case []byte:
		return strings.TrimSpace(string(typed)) != "", nil
	default:
		return fmt.Sprint(typed) != "", nil
	}
}

func scanArticleID(layout articleRowLayout, row []any, column string) (ctype.ID, error) {
	value, err := layout.requireValue(row, column)
	if err != nil {
		return 0, err
	}
	var id ctype.ID
	if err = id.Scan(value); err != nil || id == 0 {
		return 0, fmt.Errorf("canal 行数据 %s 解析失败", column)
	}
	return id, nil
}

func scanArticleNullableID(layout articleRowLayout, row []any, column string) (*ctype.ID, error) {
	value, err := layout.requireValue(row, column)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	id, err := scanArticleID(layout, row, column)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func scanArticleString(layout articleRowLayout, row []any, column string) (string, error) {
	value, err := layout.requireValue(row, column)
	if err != nil {
		return "", err
	}
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return fmt.Sprint(typed), nil
	}
}

func scanArticleTime(layout articleRowLayout, row []any, column string) (time.Time, error) {
	value, err := layout.requireValue(row, column)
	if err != nil {
		return time.Time{}, err
	}
	switch typed := value.(type) {
	case time.Time:
		return typed, nil
	case *time.Time:
		if typed == nil {
			return time.Time{}, fmt.Errorf("canal 行数据 %s 为空", column)
		}
		return *typed, nil
	case string:
		return parseArticleTimeText(column, typed)
	case []byte:
		return parseArticleTimeText(column, string(typed))
	default:
		return time.Time{}, fmt.Errorf("canal 行数据 %s 不是合法时间类型", column)
	}
}

func parseArticleTimeText(column, value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("canal 行数据 %s 为空", column)
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("canal 行数据 %s 解析时间失败", column)
}

func scanArticleInt(layout articleRowLayout, row []any, column string) (int, error) {
	value, err := layout.requireValue(row, column)
	if err != nil {
		return 0, err
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int8:
		return int(typed), nil
	case int16:
		return int(typed), nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case uint:
		return int(typed), nil
	case uint8:
		return int(typed), nil
	case uint16:
		return int(typed), nil
	case uint32:
		return int(typed), nil
	case uint64:
		return int(typed), nil
	case float32:
		return int(typed), nil
	case float64:
		return int(typed), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(typed))
	case []byte:
		return strconv.Atoi(strings.TrimSpace(string(typed)))
	default:
		return 0, fmt.Errorf("canal 行数据 %s 不是合法整数类型", column)
	}
}

func scanArticleBool(layout articleRowLayout, row []any, column string) (bool, error) {
	value, err := layout.requireValue(row, column)
	if err != nil {
		return false, err
	}
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case int:
		return typed != 0, nil
	case int8:
		return typed != 0, nil
	case int16:
		return typed != 0, nil
	case int32:
		return typed != 0, nil
	case int64:
		return typed != 0, nil
	case uint:
		return typed != 0, nil
	case uint8:
		return typed != 0, nil
	case uint16:
		return typed != 0, nil
	case uint32:
		return typed != 0, nil
	case uint64:
		return typed != 0, nil
	case string:
		return parseArticleBoolText(column, typed)
	case []byte:
		return parseArticleBoolText(column, string(typed))
	default:
		return false, fmt.Errorf("canal 行数据 %s 不是合法布尔类型", column)
	}
}

func parseArticleBoolText(column, value string) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1", "true", "t", "yes", "y", "on":
		return true, nil
	case "0", "false", "f", "no", "n", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("canal 行数据 %s 解析布尔值失败", column)
	}
}

func (l articleRowLayout) value(row []any, column string) (any, bool) {
	index, ok := l.columns[strings.ToLower(strings.TrimSpace(column))]
	if !ok || index >= len(row) {
		return nil, false
	}
	return row[index], true
}

func (l articleRowLayout) requireValue(row []any, column string) (any, error) {
	value, ok := l.value(row, column)
	if !ok {
		return nil, fmt.Errorf("canal 行数据缺少 %s 列，请确认 binlog_row_image=FULL", column)
	}
	return value, nil
}
