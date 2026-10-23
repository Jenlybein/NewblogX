package db_service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// RestoreOrCreateUnique 按“先恢复软删，再尝试创建”的顺序执行。
// 返回 true 表示本次请求真正完成了“恢复”或“创建”，返回 false 表示活记录已存在。
func RestoreOrCreateUnique[T any](tx *gorm.DB, value *T, match []string) (bool, error) {
	model, matchConditions, restoreAssignments, err := buildUniqueWriteData(tx, value, match)
	if err != nil {
		return false, err
	}

	// 先尝试恢复被软删除的记录
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

	// 如果创建时活记录已存在，或并发下别人刚创建成功，就返回 false。
	createResult := tx.Create(value)
	if createResult.Error != nil {
		if errors.Is(createResult.Error, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		return false, createResult.Error
	}
	return true, nil
}

// buildUniqueWriteData 从 Value 中提取模型、匹配条件和恢复赋值。
// Match 中声明的字段会参与 where 条件生成，但不会在恢复时再次更新。
func buildUniqueWriteData[T any](tx *gorm.DB, value *T, match []string) (any, map[string]any, map[string]any, error) {
	if value == nil {
		return nil, nil, nil, errors.New("value 不能为空指针")
	}

	sourceValue := reflect.ValueOf(value).Elem()
	if sourceValue.Kind() != reflect.Struct {
		return nil, nil, nil, errors.New("value 必须是结构体或结构体指针")
	}

	modelValue := reflect.New(sourceValue.Type()).Interface()
	stmt := &gorm.Statement{DB: tx}
	if err := stmt.Parse(modelValue); err != nil {
		return nil, nil, nil, err
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

// buildMatchConditions 生成匹配条件。
func buildMatchConditions(s *schema.Schema, sourceValue reflect.Value, matchFields []string) (map[string]any, error) {
	if len(matchFields) == 0 {
		return nil, errors.New("match 不能为空")
	}

	matchConditions := make(map[string]any, len(matchFields))
	ctx := context.Background()
	for _, name := range matchFields {
		field := s.LookUpField(name)
		if field == nil || field.DBName == "" {
			return nil, fmt.Errorf("match 字段不存在: %s", name)
		}
		fieldValue, _ := field.ValueOf(ctx, sourceValue)
		matchConditions[field.DBName] = fieldValue
	}
	return matchConditions, nil
}

// shouldSkipRestoreField 判断是否应该跳过恢复某个字段。
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
