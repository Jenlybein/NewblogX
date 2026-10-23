package maps

import (
	"errors"
	"reflect"
)

// MapFieldsToStruct 将src结构体的字段值映射到dest结构体的对应字段中，不支持嵌套指针处理
func FieldsStructToStruct(src, dest any) error {
	// 转换为reflect.Value
	srcVal := reflect.ValueOf(src)
	destVal := reflect.ValueOf(dest)

	// 校验是否为指针
	if srcVal.Kind() != reflect.Ptr || destVal.Kind() != reflect.Ptr {
		return errors.New("src 和 dest 必须是指针类型")
	}

	// 取指针指向的结构体元素
	srcElem := srcVal.Elem()
	destElem := destVal.Elem()

	// 校验是否为结构体
	if srcElem.Kind() != reflect.Struct || destElem.Kind() != reflect.Struct {
		return errors.New("src 和 dest 指针必须指向结构体类型")
	}

	// 遍历dest结构体的所有字段
	for i := 0; i < destElem.NumField(); i++ {
		// 获取dest字段的元信息和值
		destField := destElem.Field(i)
		destFieldName := destElem.Type().Field(i).Name

		// 校验dest字段是否可赋值（避免未导出字段panic）
		if !destField.CanSet() {
			continue
		}

		// 查找src中同名的字段
		srcField := srcElem.FieldByName(destFieldName)
		if !srcField.IsValid() {
			continue
		}

		// 处理src字段（单层指针解引用）
		var srcRealValue reflect.Value
		if srcField.Kind() == reflect.Ptr {
			if srcField.IsNil() {
				continue
			}
			srcRealValue = srcField.Elem()
		} else {
			srcRealValue = srcField // 非指针直接使用
		}

		// 赋值到dest字段（适配指针/非指针）
		if destField.Kind() == reflect.Ptr {
			ptr := reflect.New(srcRealValue.Type())
			ptr.Elem().Set(srcRealValue)
			destField.Set(ptr)
		} else if srcRealValue.Type().AssignableTo(destField.Type()) {
			destField.Set(srcRealValue)
		}
	}

	return nil
}

// MapFieldsToMap 将src结构体的字段值映射到dest结构体的对应字段中，不支持嵌套指针处理
func FieldsStructToMap(src, dest any) (res map[string]any, err error) {
	// 转换为reflect.Value
	srcVal := reflect.ValueOf(src)
	destVal := reflect.ValueOf(dest)

	// 校验是否为指针
	if srcVal.Kind() != reflect.Ptr || destVal.Kind() != reflect.Ptr {
		return nil, errors.New("src 和 dest 必须是指针类型")
	}

	// 初始化返回值
	res = make(map[string]any)

	// 取指针指向的结构体元素
	srcElem := srcVal.Elem()
	destElem := destVal.Elem()

	// 校验是否为结构体
	if srcElem.Kind() != reflect.Struct || destElem.Kind() != reflect.Struct {
		return nil, errors.New("src 和 dest 指针必须指向结构体类型")
	}

	// 遍历dest结构体的所有字段
	for i := 0; i < destElem.NumField(); i++ {
		// 获取dest字段的元信息
		destFieldType := destElem.Type().Field(i)
		destFieldName := destFieldType.Name

		// 查找src中同名的字段
		srcField := srcElem.FieldByName(destFieldName)
		if !srcField.IsValid() {
			continue
		}

		// 使用JSON Tag作为map的key
		if jsonTag := destFieldType.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			destFieldName = jsonTag
		} else {
			continue
		}

		// 处理src字段
		if srcField.Kind() == reflect.Ptr {
			if srcField.IsNil() {
				continue
			}
			res[destFieldName] = srcField.Elem().Interface()
		} else {
			res[destFieldName] = srcField.Interface()
		}
	}

	return
}
