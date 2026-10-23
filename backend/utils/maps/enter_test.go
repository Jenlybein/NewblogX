package maps_test

import (
	"myblogx/utils/maps"
	"testing"
)

type mapSrc struct {
	Name *string
	Age  int
	Skip string
}

type mapDst struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestFieldsStructToStruct(t *testing.T) {
	name := "alice"
	src := &mapSrc{Name: &name, Age: 18, Skip: "x"}
	dst := &mapDst{}

	if err := maps.FieldsStructToStruct(src, dst); err != nil {
		t.Fatalf("FieldsStructToStruct 失败: %v", err)
	}
	if dst.Name != "alice" || dst.Age != 18 {
		t.Fatalf("字段映射错误: %+v", dst)
	}

	if err := maps.FieldsStructToStruct(*src, dst); err == nil {
		t.Fatal("非指针入参应报错")
	}
}

func TestFieldsStructToMap(t *testing.T) {
	name := "bob"
	src := &mapSrc{Name: &name, Age: 20}
	dst := &mapDst{}

	res, err := maps.FieldsStructToMap(src, dst)
	if err != nil {
		t.Fatalf("FieldsStructToMap 失败: %v", err)
	}
	if res["name"] != "bob" || res["age"] != 20 {
		t.Fatalf("结果异常: %+v", res)
	}
}
