package image_ref_river_service

import (
	"fmt"
	"strings"
	"time"

	"myblogx/models/ctype"
)

type rowSnapshot struct {
	layout rowLayout
	row    []any
}

type rowLayout struct {
	columns map[string]int
}

func newRowLayout(columnNames []string) rowLayout {
	columns := make(map[string]int, len(columnNames))
	for index, name := range columnNames {
		columns[name] = index
	}
	return rowLayout{columns: columns}
}

func newRowSnapshot(layout rowLayout, row []any) rowSnapshot {
	return rowSnapshot{
		layout: layout,
		row:    row,
	}
}

func (r rowSnapshot) ID() (ctype.ID, error) {
	value, ok := r.value("id")
	if !ok {
		return 0, fmt.Errorf("canal 行数据缺少 id 列")
	}
	var id ctype.ID
	if err := id.Scan(value); err != nil || id == 0 {
		return 0, fmt.Errorf("canal 行数据 id 解析失败")
	}
	return id, nil
}

func (r rowSnapshot) RequireString(column string) (string, error) {
	value, ok := r.value(column)
	if !ok {
		return "", fmt.Errorf("canal 行数据缺少 %s 列，请确认 binlog_row_image=FULL", column)
	}
	return normalizeString(value), nil
}

func (r rowSnapshot) IsDeleted() bool {
	value, ok := r.value("deleted_at")
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case nil:
		return false
	case time.Time:
		return !typed.IsZero()
	case *time.Time:
		return typed != nil && !typed.IsZero()
	case []byte:
		return strings.TrimSpace(string(typed)) != ""
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return fmt.Sprint(typed) != ""
	}
}

func (r rowSnapshot) value(column string) (any, bool) {
	index, ok := r.layout.columns[column]
	if !ok || index >= len(r.row) {
		return nil, false
	}
	return r.row[index], true
}

func (r rowSnapshot) EqualString(other rowSnapshot, column string) bool {
	left, leftOK := r.value(column)
	right, rightOK := other.value(column)
	if !leftOK && !rightOK {
		return true
	}
	return normalizeString(left) == normalizeString(right)
}

func normalizeString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}
