package imageref_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/cdc/imageref"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"gorm.io/gorm"
)

func seedRef(t *testing.T, db *gorm.DB, refType image_ref_enum.RefType, ownerID ctype.ID) {
	t.Helper()
	row := &schema.ImageRefRow{
		ImageID: ctype.NewID(99),
		RefType: refType,
		OwnerID: ownerID,
		Field:   image_ref_enum.RefFieldArticleCover,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插入图片引用失败: %v", err)
	}
}

func countRefs(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&schema.ImageRefRow{}).Count(&n).Error; err != nil {
		t.Fatalf("统计图片引用失败: %v", err)
	}
	return n
}

// 删除类死信：payload 里有 owner_id + ref_type，重投就是把该对象的残留引用清掉。
func TestReplayDeleteRemovesRefs(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ImageRefRow{})
	ownerID := ctype.NewID(2001)
	seedRef(t, db, image_ref_enum.RefTypeArticle, ownerID)

	r := imageref.NewReplayer(db)
	err := r.Replay(context.Background(), deadletter.Record{
		Stream:      imageref.ReplayStream,
		Action:      "delete",
		CdcJobID:    "image_ref_river:db.article_models:x:1:0",
		PayloadJSON: fmt.Sprintf(`{"owner_id":"%s","ref_type":"article"}`, ownerID.String()),
	})
	if err != nil {
		t.Fatalf("删除类重投不应报错: %v", err)
	}
	if got := countRefs(t, db); got != 0 {
		t.Fatalf("引用应被清空，实际剩 %d 条", got)
	}
}

// 新增/更新类死信：明确拒绝，并说明该改用重新保存对象。
func TestReplayInsertIsRejected(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ImageRefRow{})
	r := imageref.NewReplayer(db)

	err := r.Replay(context.Background(), deadletter.Record{
		Stream:      imageref.ReplayStream,
		CdcJobID:    "image_ref_river:db.article_models:x:1:0",
		PayloadJSON: `{"table":"article_models","row":[1,"标题","内容"]}`,
	})
	if err == nil {
		t.Fatal("新增类死信应拒绝重投")
	}
	if !strings.Contains(err.Error(), "重新保存") {
		t.Fatalf("应给出替代动作，实际 %q", err.Error())
	}
	// 必须是 ErrUnsupported 而不是普通 error：编排层据此判"不支持"（不涨 retry_count、
	// 不改状态、前端显示橙色"不支持重投"），而不是红色"失败"。
	if !errors.Is(err, deadletter.ErrUnsupported) {
		t.Fatalf("应包装 deadletter.ErrUnsupported，实际 %v", err)
	}
}

// 非法输入要明确报错，不要"猜一个 owner_id"去删数据。
func TestReplayRejectsBadPayload(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ImageRefRow{})
	r := imageref.NewReplayer(db)

	cases := map[string]string{
		"空 payload":   "",
		"非法 JSON":     "{oops",
		"未知 ref_type": `{"owner_id":"1","ref_type":"unknown-type"}`,
		"owner 非法":    `{"owner_id":"abc","ref_type":"article"}`,
	}
	for name, payload := range cases {
		if err := r.Replay(context.Background(), deadletter.Record{PayloadJSON: payload}); err == nil {
			t.Fatalf("%s: 期望报错", name)
		}
	}
}

func TestReplayWithoutDBFails(t *testing.T) {
	r := imageref.NewReplayer(nil)
	if err := r.Replay(context.Background(), deadletter.Record{PayloadJSON: `{"owner_id":"1","ref_type":"article"}`}); err == nil {
		t.Fatal("未注入数据库时应报错")
	}
}
