package http

import (
	"encoding/json"
	"myblogx/internal/analytics/adapter/clickhouse"
	analyticsmysql "myblogx/internal/analytics/adapter/mysql"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newDataCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

// newDataHandler 用真实 sqlite 组装统计处理器。
func newDataHandler(t *testing.T) *Handler {
	t.Helper()
	db := testutil.SetupSQLite(t, schema.Models(
		schema.UserCore, schema.ArticleCore, schema.Comment, schema.Chat,
	)...)
	return NewHandler(
		clickhouse.Options{},
		analyticsmysql.NewStats(db),
		nil, // flow
		nil, // deadLetters
		nil, // replayer
	)
}

func readDataResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var body struct {
		Code int    `json:"code"`
		Data T      `json:"data"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	if body.Code != 0 {
		t.Fatalf("接口应成功 body=%s", w.Body.String())
	}
	return body.Data
}

func createPublishedByPublishStatusOnlyArticle(t *testing.T, createdAt time.Time) {
	t.Helper()
	schema.InsertArticle(t, testutil.DB(), schema.ArticleSpec{
		Title:         "publish-status-only",
		Content:       "content",
		AuthorID:      1,
		PublishStatus: schema.ArticleStatusPublished,
		CreatedAt:     createdAt,
	})
}

func TestGrowthDataViewCountsEffectivePublishedArticles(t *testing.T) {
	api := newDataHandler(t)
	now := time.Now()
	createPublishedByPublishStatusOnlyArticle(t, now)

	c, w := newDataCtx()
	c.Set("requestQuery", GrowthDataRequest{Type: 2})
	api.GrowthDataView(c)

	data := readDataResponse[GrowthDataResponse](t, w)
	today := now.Format("2006-01-02")
	countMap := map[string]int{}
	for _, item := range data.DateCountList {
		countMap[item.Date] = item.Count
	}
	if countMap[today] != 1 {
		t.Fatalf("今日文章发布趋势应统计 publish_status=published 的文章 got=%d body=%s", countMap[today], w.Body.String())
	}
}

func TestArticleYearDataViewCountsEffectivePublishedArticles(t *testing.T) {
	api := newDataHandler(t)
	now := time.Now()
	createPublishedByPublishStatusOnlyArticle(t, now)

	c, w := newDataCtx()
	api.ArticleYearDataView(c)

	data := readDataResponse[ArticleYearDataResponse](t, w)
	currentMonth := now.Format("2006-01")
	countMap := map[string]int{}
	for _, item := range data.DateCountList {
		countMap[item.Date] = item.Count
	}
	if countMap[currentMonth] != 1 {
		t.Fatalf("年度文章趋势应统计 publish_status=published 的文章 got=%d body=%s", countMap[currentMonth], w.Body.String())
	}
}
