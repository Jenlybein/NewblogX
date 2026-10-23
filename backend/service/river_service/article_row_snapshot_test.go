package river_service

import (
	"testing"
	"time"

	"myblogx/models/enum"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/schema"
)

func TestCollectArticleRowSnapshots(t *testing.T) {
	createdAt := time.Date(2026, 4, 13, 12, 0, 0, 0, time.Local)
	updatedAt := createdAt.Add(2 * time.Minute)
	e := &canal.RowsEvent{
		Action: canal.InsertAction,
		Table: &schema.Table{
			Name: "article_models",
			Columns: []schema.TableColumn{
				{Name: "id"},
				{Name: "created_at"},
				{Name: "updated_at"},
				{Name: "title"},
				{Name: "abstract"},
				{Name: "content"},
				{Name: "category_id"},
				{Name: "cover"},
				{Name: "author_id"},
				{Name: "view_count"},
				{Name: "digg_count"},
				{Name: "comment_count"},
				{Name: "favor_count"},
				{Name: "comments_toggle"},
				{Name: "publish_status"},
				{Name: "visibility_status"},
				{Name: "deleted_at"},
			},
		},
		Rows: [][]any{{
			int64(301900000000000001),
			createdAt,
			updatedAt,
			"测试文章",
			"摘要",
			"# 标题\n正文",
			int64(301900000000000002),
			"/cover.png",
			int64(301900000000000003),
			int64(11),
			int64(12),
			int64(13),
			int64(14),
			int8(1),
			int64(enum.ArticleStatusPublished),
			string(enum.ArticleVisibilityVisible),
			nil,
		}},
	}

	snapshots, err := collectArticleRowSnapshots(e)
	if err != nil {
		t.Fatalf("collectArticleRowSnapshots 返回错误: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("应解析出 1 条快照, got=%d", len(snapshots))
	}

	snapshot := snapshots[0]
	if snapshot.ID.String() != "301900000000000001" {
		t.Fatalf("文章 ID 解析错误: %s", snapshot.ID.String())
	}
	if snapshot.CreatedAt != createdAt || snapshot.UpdatedAt != updatedAt {
		t.Fatalf("时间字段解析错误: created=%v updated=%v", snapshot.CreatedAt, snapshot.UpdatedAt)
	}
	if snapshot.Title != "测试文章" || snapshot.Abstract != "摘要" || snapshot.Content != "# 标题\n正文" {
		t.Fatalf("字符串字段解析错误: %+v", snapshot)
	}
	if snapshot.CategoryID == nil || snapshot.CategoryID.String() != "301900000000000002" {
		t.Fatalf("分类 ID 解析错误: %#v", snapshot.CategoryID)
	}
	if snapshot.AuthorID.String() != "301900000000000003" {
		t.Fatalf("作者 ID 解析错误: %s", snapshot.AuthorID.String())
	}
	if snapshot.ViewCount != 11 || snapshot.DiggCount != 12 || snapshot.CommentCount != 13 || snapshot.FavorCount != 14 {
		t.Fatalf("计数字段解析错误: %+v", snapshot)
	}
	if !snapshot.CommentsToggle {
		t.Fatal("comments_toggle 解析错误")
	}
	if snapshot.PublishStatus != enum.ArticleStatusPublished {
		t.Fatalf("publish_status 解析错误: %v", snapshot.PublishStatus)
	}
	if snapshot.VisibilityStatus != enum.ArticleVisibilityVisible {
		t.Fatalf("visibility_status 解析错误: %v", snapshot.VisibilityStatus)
	}
}
