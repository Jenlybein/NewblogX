package article_api

import (
	"encoding/json"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"testing"
)

func readArticleReviewTaskList(t *testing.T, body []byte) ArticleReviewTaskListResponse {
	t.Helper()
	var resp struct {
		Code int                           `json:"code"`
		Data ArticleReviewTaskListResponse `json:"data"`
		Msg  string                        `json:"msg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("解析审核任务列表响应失败: %v body=%s", err, string(body))
	}
	if resp.Code != 0 {
		t.Fatalf("审核任务列表应成功 body=%s", string(body))
	}
	return resp.Data
}

func createArticleReviewTaskForStatus(t *testing.T, userID ctype.ID, status models.ArticleReviewTaskStatus) models.ArticleReviewTaskModel {
	t.Helper()
	article := models.ArticleModel{
		Title:         "review-" + string(status),
		Content:       "content",
		AuthorID:      userID,
		PublishStatus: enum.ArticleStatusExamining,
	}
	if err := testutil.DB().Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败 status=%s err=%v", status, err)
	}
	task := models.ArticleReviewTaskModel{
		ArticleID:            article.ID,
		AuthorID:             article.AuthorID,
		ArticleTitle:         article.Title,
		AuthorName:           "审核作者",
		ArticlePublishStatus: article.EffectivePublishStatus(),
		Stage:                models.ArticleReviewTaskStageManual,
		Source:               models.ArticleReviewTaskSourceCreate,
		Status:               status,
	}
	if err := testutil.DB().Create(&task).Error; err != nil {
		t.Fatalf("创建审核任务失败 status=%s err=%v", status, err)
	}
	return task
}

func TestArticleReviewTaskListViewFiltersByStatus(t *testing.T) {
	user := setupArticleEnv(t)
	api := setupArticleAPI(t)

	statusList := []models.ArticleReviewTaskStatus{
		models.ArticleReviewTaskPending,
		models.ArticleReviewTaskApproved,
		models.ArticleReviewTaskRejected,
		models.ArticleReviewTaskCanceled,
	}
	for _, status := range statusList {
		createArticleReviewTaskForStatus(t, user.ID, status)
	}

	for _, status := range statusList {
		t.Run(string(status), func(t *testing.T) {
			c, w := newCtx()
			c.Set("requestQuery", ArticleReviewTaskListRequest{
				Page:   1,
				Limit:  5,
				Status: status,
			})

			api.ArticleReviewTaskListView(c)

			data := readArticleReviewTaskList(t, w.Body.Bytes())
			if data.Count != 1 || len(data.List) != 1 {
				t.Fatalf("筛选结果数量异常 status=%s data=%+v body=%s", status, data, w.Body.String())
			}
			if data.List[0].Status != status {
				t.Fatalf("筛选结果状态异常 got=%s want=%s", data.List[0].Status, status)
			}
		})
	}
}

func TestArticleReviewTaskListViewUsesTaskSnapshotsWithoutJoin(t *testing.T) {
	setupArticleEnv(t)
	api := setupArticleAPI(t)

	task := models.ArticleReviewTaskModel{
		ArticleID:            9001,
		AuthorID:             9002,
		ArticleTitle:         "冗余标题",
		AuthorName:           "冗余作者",
		ArticlePublishStatus: enum.ArticleStatusExamining,
		Stage:                models.ArticleReviewTaskStageManual,
		Source:               models.ArticleReviewTaskSourceCreate,
		Status:               models.ArticleReviewTaskPending,
	}
	if err := testutil.DB().Create(&task).Error; err != nil {
		t.Fatalf("创建孤立审核任务失败: %v", err)
	}

	c, w := newCtx()
	c.Set("requestQuery", ArticleReviewTaskListRequest{
		Page:  1,
		Limit: 5,
	})

	api.ArticleReviewTaskListView(c)

	data := readArticleReviewTaskList(t, w.Body.Bytes())
	if data.Count != 1 || len(data.List) != 1 {
		t.Fatalf("审核任务列表数量异常 data=%+v body=%s", data, w.Body.String())
	}
	got := data.List[0]
	if got.ArticleTitle != task.ArticleTitle || got.AuthorName != task.AuthorName || got.PublishStatus != task.ArticlePublishStatus {
		t.Fatalf("审核任务未使用任务表冗余快照 got=%+v task=%+v", got, task)
	}
}
