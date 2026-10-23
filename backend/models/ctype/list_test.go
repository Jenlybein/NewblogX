package ctype_test

import (
	"myblogx/models/ctype"
	"testing"
)

func TestListScanAndValue(t *testing.T) {
	var l ctype.List
	if err := l.Scan([]byte("a,b,c")); err != nil {
		t.Fatalf("Scan 失败: %v", err)
	}
	if len(l) != 3 || l[0] != "a" {
		t.Fatalf("Scan 结果异常: %+v", l)
	}

	if err := l.Scan(nil); err != nil {
		t.Fatalf("Scan nil 失败: %v", err)
	}
	if len(l) != 0 {
		t.Fatalf("Scan nil 后应为空: %+v", l)
	}

	if err := l.Scan(123); err == nil {
		t.Fatal("非法类型应报错")
	}

	v, err := (ctype.List{"x", "y"}).Value()
	if err != nil {
		t.Fatalf("Value 失败: %v", err)
	}
	if v.(string) != "x,y" {
		t.Fatalf("Value 结果异常: %v", v)
	}
}
