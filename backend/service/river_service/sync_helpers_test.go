package river_service

import (
	"myblogx/service/river_service/rule"
	"myblogx/test/testutil"
	"strings"
	"testing"
	"time"

	"github.com/go-mysql-org/go-mysql/schema"
)

func TestGetFieldParts(t *testing.T) {
	testutil.InitGlobals()
	r := &River{}

	mysql, elastic, fieldType := r.getFieldParts("title", "es_title,list")
	if mysql != "title" || elastic != "es_title" || fieldType != "list" {
		t.Fatalf("getFieldParts 解析错误: mysql=%s elastic=%s fieldType=%s", mysql, elastic, fieldType)
	}

	mysql, elastic, fieldType = r.getFieldParts("name", "")
	if mysql != "name" || elastic != "name" || fieldType != "" {
		t.Fatalf("空映射解析错误: mysql=%s elastic=%s fieldType=%s", mysql, elastic, fieldType)
	}
}

func TestMakeReqColumnData(t *testing.T) {
	testutil.InitGlobals()
	r := &River{}

	enumCol := schema.TableColumn{Type: schema.TYPE_ENUM, EnumValues: []string{"A", "B"}}
	if got := r.makeReqColumnData(&enumCol, int64(1)); got != "A" {
		t.Fatalf("enum 转换错误: %v", got)
	}
	if got := r.makeReqColumnData(&enumCol, int64(5)); got != "" {
		t.Fatalf("非法 enum 索引应返回空字符串: %v", got)
	}

	setCol := schema.TableColumn{Type: schema.TYPE_SET, SetValues: []string{"x", "y"}}
	if got := r.makeReqColumnData(&setCol, int64(3)); got != "x,y" {
		t.Fatalf("set 转换错误: %v", got)
	}

	bitCol := schema.TableColumn{Type: schema.TYPE_BIT}
	if got := r.makeReqColumnData(&bitCol, "\x01"); got != int64(1) {
		t.Fatalf("bit=1 转换错误: %v", got)
	}
	if got := r.makeReqColumnData(&bitCol, "\x00"); got != int64(0) {
		t.Fatalf("bit=0 转换错误: %v", got)
	}

	strCol := schema.TableColumn{Type: schema.TYPE_STRING}
	if got := r.makeReqColumnData(&strCol, []byte("hello")); got != "hello" {
		t.Fatalf("string 转换错误: %v", got)
	}

	jsonCol := schema.TableColumn{Type: schema.TYPE_JSON}
	gotJSON := r.makeReqColumnData(&jsonCol, []byte(`{"n":1}`))
	m, ok := gotJSON.(map[string]any)
	if !ok || m["n"].(float64) != 1 {
		t.Fatalf("json 转换错误: %#v", gotJSON)
	}

	dateCol := schema.TableColumn{Type: schema.TYPE_DATE}
	if got := r.makeReqColumnData(&dateCol, "2024-05-20"); got != "2024-05-20" {
		t.Fatalf("date 转换错误: %v", got)
	}
	if got := r.makeReqColumnData(&dateCol, "bad-date"); got != nil {
		t.Fatalf("非法 date 应返回 nil: %v", got)
	}
}

func TestGetFieldValue(t *testing.T) {
	testutil.InitGlobals()
	r := &River{}

	col := schema.TableColumn{Type: schema.TYPE_STRING}
	got := r.getFieldValue(&col, fieldTypeList, "a,b,c")
	list, ok := got.([]string)
	if !ok || len(list) != 3 {
		t.Fatalf("list 字段转换错误: %#v", got)
	}

	numCol := schema.TableColumn{Type: schema.TYPE_NUMBER}
	dt := r.getFieldValue(&numCol, fieldTypeDate, int64(0))
	s, ok := dt.(string)
	if !ok || !strings.Contains(s, "1970-01-01T") {
		t.Fatalf("date 字段转换错误: %#v", dt)
	}

	normal := r.getFieldValue(&col, "", "plain")
	if normal != "plain" {
		t.Fatalf("普通字段不应被修改: %#v", normal)
	}
}

func TestGetFieldValueBool(t *testing.T) {
	testutil.InitGlobals()
	r := &River{}

	numCol := schema.TableColumn{Type: schema.TYPE_NUMBER}
	if got := r.getFieldValue(&numCol, fieldTypeBool, int64(1)); got != true {
		t.Fatalf("bool 字段 int64(1) 转换错误: %#v", got)
	}
	if got := r.getFieldValue(&numCol, fieldTypeBool, int64(0)); got != false {
		t.Fatalf("bool 字段 int64(0) 转换错误: %#v", got)
	}

	strCol := schema.TableColumn{Type: schema.TYPE_STRING}
	if got := r.getFieldValue(&strCol, fieldTypeBool, "true"); got != true {
		t.Fatalf("bool 字段 'true' 转换错误: %#v", got)
	}
	if got := r.getFieldValue(&strCol, fieldTypeBool, "0"); got != false {
		t.Fatalf("bool 字段 '0' 转换错误: %#v", got)
	}

	bitCol := schema.TableColumn{Type: schema.TYPE_BIT}
	if got := r.getFieldValue(&bitCol, fieldTypeBool, "\x01"); got != true {
		t.Fatalf("bool 字段 bit=1 转换错误: %#v", got)
	}
	if got := r.getFieldValue(&bitCol, fieldTypeBool, "\x00"); got != false {
		t.Fatalf("bool 字段 bit=0 转换错误: %#v", got)
	}
}

func TestRulePrepareLowercase(t *testing.T) {
	testutil.InitGlobals()
	rl := &rule.Rule{Schema: "db", Table: "T", Index: "IDX", Type: "TP"}
	if err := rl.Prepare(); err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if rl.Index != "idx" || rl.Type != "tp" {
		t.Fatalf("Prepare 未转小写: index=%s type=%s", rl.Index, rl.Type)
	}
}

func TestMakeReqColumnDataDateTime(t *testing.T) {
	testutil.InitGlobals()
	r := &River{}
	col := schema.TableColumn{Type: schema.TYPE_DATETIME}
	v := r.makeReqColumnData(&col, "2024-01-01 12:13:14")
	s, ok := v.(string)
	if !ok {
		t.Fatalf("datetime 转换类型错误: %#v", v)
	}
	if _, err := time.Parse(time.RFC3339, s); err != nil {
		t.Fatalf("datetime 转换值不是 RFC3339: %s err=%v", s, err)
	}
}
