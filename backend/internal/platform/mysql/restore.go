package mysql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// ErrRestoreNilValue 表示 RestoreOrCreateUnique 收到空指针。
var ErrRestoreNilValue = errors.New("mysql: nil value for restore-or-create")

// ErrRestoreEmptyMatch 表示没有给匹配字段，无法判断"同一条记录"。
var ErrRestoreEmptyMatch = errors.New("mysql: empty match fields for restore-or-create")

// RestoreOrCreateUnique 按"先恢复软删、再尝试创建"的顺序写入一条唯一记录。
//
// 存在的意义：所有带唯一索引 + 软删的表，在"删除后重新添加同一条"时都会撞唯一索引。
// 调用方不应该各自处理这个分支，所以收在这里。
//
// 返回 true 表示本次真正写入了（恢复或新建）；false 表示活记录已存在、什么都没做。
// 后者不是错误——"已经存在"是正常业务结果。
//
// 逻辑归位自 service/db_service（S4.0）。只依赖 GORM 的 schema 反射，不含业务语义。
func RestoreOrCreateUnique[T any](tx *gorm.DB, value *T, match []string) (bool, error) {
	model, matchConditions, restoreAssignments, err := buildUniqueWriteData(tx, value, match)
	if err != nil {
		return false, err
	}

	// 先尝试恢复被软删除的记录：命中说明"这条以前存在过"，恢复比新建更合适，
	// 因为新建会撞唯一索引。
	restoreResult := tx.Unscoped().
		Model(model).
		Where(matchConditions).
		Where("deleted_at IS NOT NULL").
		Updates(restoreAssignments)
	if restoreResult.Error != nil {
		return false, restoreResult.Error
	}
	if restoreResult.RowsAffected > 0 {
		return true, nil
	}

	// 没有可恢复的行，走新建。并发下别人可能刚创建成功，此时唯一索引冲突
	// 等价于"已存在"，不算错误。
	createResult := tx.Create(value)
	if createResult.Error != nil {
		if errors.Is(createResult.Error, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		return false, createResult.Error
	}
	return true, nil
}

// buildUniqueWriteData 从 value 中提取模型实例、匹配条件和恢复时的赋值集合。
//
// match 里声明的字段只参与 where 条件，不在恢复时被重新赋值——否则会拿调用方
// 传入的值覆盖掉匹配键，语义混乱。
func buildUniqueWriteData[T any](tx *gorm.DB, value *T, match []string) (any, map[string]any, map[string]any, error) {
	if value == nil {
		return nil, nil, nil, ErrRestoreNilValue
	}

	sourceValue := reflect.ValueOf(value).Elem()
	if sourceValue.Kind() != reflect.Struct {
		return nil, nil, nil, fmt.Errorf("mysql: value must be a struct or struct pointer, got %s", sourceValue.Kind())
	}

	modelValue := reflect.New(sourceValue.Type()).Interface()
	stmt := &gorm.Statement{DB: tx}
	if err := stmt.Parse(modelValue); err != nil {
		return nil, nil, nil, fmt.Errorf("mysql: parse schema: %w", err)
	}

	matchConditions, err := buildMatchConditions(stmt.Schema, sourceValue, match)
	if err != nil {
		return nil, nil, nil, err
	}

	assignments := map[string]any{
		"deleted_at": nil,
		"updated_at": time.Now(),
	}
	ctx := context.Background()
	for _, field := range stmt.Schema.Fields {
		if shouldSkipRestoreField(field, matchConditions) {
			continue
		}
		fieldValue, _ := field.ValueOf(ctx, sourceValue)
		assignments[field.DBName] = fieldValue
	}
	return modelValue, matchConditions, assignments, nil
}

// buildMatchConditions 把 Go 字段名翻译成 (列名 → 值)。
func buildMatchConditions(s *schema.Schema, sourceValue reflect.Value, matchFields []string) (map[string]any, error) {
	if len(matchFields) == 0 {
		return nil, ErrRestoreEmptyMatch
	}

	matchConditions := make(map[string]any, len(matchFields))
	ctx := context.Background()
	for _, name := range matchFields {
		field := s.LookUpField(name)
		if field == nil || field.DBName == "" {
			return nil, fmt.Errorf("mysql: match field not found: %s", name)
		}
		fieldValue, _ := field.ValueOf(ctx, sourceValue)
		matchConditions[field.DBName] = fieldValue
	}
	return matchConditions, nil
}

// shouldSkipRestoreField 判断某个列在恢复时是否该跳过。
//
// 跳过的四类：主键（恢复不该换 ID）、时间戳（由 GORM/上面显式赋值管）、
// 匹配键本身、以及四个元数据列。
func shouldSkipRestoreField(field *schema.Field, matchDBNames map[string]any) bool {
	if field == nil || field.DBName == "" {
		return true
	}
	if field.PrimaryKey || field.AutoCreateTime > 0 || field.AutoUpdateTime > 0 {
		return true
	}
	if _, ok := matchDBNames[field.DBName]; ok {
		return true
	}
	switch field.DBName {
	case "id", "created_at", "updated_at", "deleted_at":
		return true
	}
	return false
}
