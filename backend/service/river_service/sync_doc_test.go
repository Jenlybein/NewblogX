package river_service

import (
	"myblogx/service/es_service"
	riverrule "myblogx/service/river_service/rule"
	"myblogx/test/testutil"
	"testing"

	"github.com/go-mysql-org/go-mysql/schema"
)

func newTestRule() *riverrule.Rule {
	return &riverrule.Rule{
		Schema: "db",
		Table:  "articles",
		Index:  "idx",
		Type:   "_doc",
		TableInfo: &schema.Table{
			Schema: "db",
			Name:   "articles",
			Columns: []schema.TableColumn{
				{Name: "id", Type: schema.TYPE_NUMBER},
				{Name: "parent_id", Type: schema.TYPE_NUMBER},
				{Name: "title", Type: schema.TYPE_STRING},
			},
			PKColumns: []int{0},
		},
		FieldMapping: map[string]string{
			"title": "es_title",
		},
	}
}

func TestGetDocIDAndParentID(t *testing.T) {
	testutil.InitGlobals()
	r := &River{}
	rule := newTestRule()
	row := []interface{}{int64(10), int64(7), "hello"}

	id, err := r.getDocID(rule, row)
	if err != nil || id != "10" {
		t.Fatalf("getDocID 主键模式错误: id=%s err=%v", id, err)
	}

	rule.ID = []string{"id", "parent_id"}
	id2, err := r.getDocID(rule, row)
	if err != nil || id2 != "10:7" {
		t.Fatalf("getDocID 指定 ID 错误: id=%s err=%v", id2, err)
	}

	pid, err := r.getParentID(rule, row, "parent_id")
	if err != nil || pid != "7" {
		t.Fatalf("getParentID 错误: pid=%s err=%v", pid, err)
	}

	if _, err = r.getParentID(rule, row, "missing"); err == nil {
		t.Fatal("缺失 parent 列应返回错误")
	}
}

func TestMakeInsertAndUpdateRequests(t *testing.T) {
	testutil.InitGlobals()
	r := &River{}
	rule := newTestRule()

	insertRows := [][]interface{}{
		{int64(1), int64(0), "t1"},
	}
	reqs, err := r.makeInsertRequest(rule, insertRows)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("makeInsertRequest 错误: len=%d err=%v", len(reqs), err)
	}
	if reqs[0].Action != es_service.ActionIndex {
		t.Fatalf("插入 action 异常: %s", reqs[0].Action)
	}
	if reqs[0].Data["es_title"] != "t1" {
		t.Fatalf("字段映射未生效: %#v", reqs[0].Data)
	}

	delReqs, err := r.makeDeleteRequest(rule, insertRows)
	if err != nil || len(delReqs) != 1 {
		t.Fatalf("makeDeleteRequest 错误: len=%d err=%v", len(delReqs), err)
	}
	if delReqs[0].Action != es_service.ActionDelete {
		t.Fatalf("删除 action 异常: %s", delReqs[0].Action)
	}

	// update rows 必须成对
	if _, err = r.makeUpdateRequest(rule, [][]interface{}{{int64(1), int64(0), "a"}}); err == nil {
		t.Fatal("奇数 update rows 应报错")
	}

	updateRows := [][]interface{}{
		{int64(1), int64(0), "old"},
		{int64(1), int64(0), "new"},
	}
	upReqs, err := r.makeUpdateRequest(rule, updateRows)
	if err != nil || len(upReqs) != 1 {
		t.Fatalf("makeUpdateRequest 错误: len=%d err=%v", len(upReqs), err)
	}
	if upReqs[0].Action != es_service.ActionUpdate {
		t.Fatalf("更新 action 异常: %s", upReqs[0].Action)
	}
	if upReqs[0].Data["es_title"] != "new" {
		t.Fatalf("更新数据异常: %#v", upReqs[0].Data)
	}
}
