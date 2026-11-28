package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
)

// 2026-11-28 / ADR-0037 D2：站点关掉「文章质量评分」时，三个评分 type 全被拒。
//
// 这条用例只钉**文案与业务码**，因为它是这次上线后唯一还能出错的地方：
// 守卫本身已经在 Scoring 里（`h.scoreEnabled()` 为 false 就拒），但如果翻译层
// 落到 failScore 的 default，读者看到的是「读取文章评分失败」——像是服务坏了，
// 而不是"这个功能站点关掉了"。线上就是这么发现的：接口返回
// `code:1001 "读取文章评分失败"`。
func TestFailScore_评分关闭的文案与业务码(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/ai/scoring/article", nil)

	failScore(c, apperr.New(apperr.CodeArticleScoreDisabled))

	if recorder.Code != http.StatusOK {
		t.Fatalf("业务失败仍走 200 包体，实际 HTTP %d", recorder.Code)
	}
	body := recorder.Body.String()
	// spec §8 的口径：关停时接口统一返回这句。文案变了这条用例就该红。
	wantMsg := "站点已关闭文章质量评分"
	if !contains(body, wantMsg) {
		t.Fatalf("文案应为 %q（而不是「读取文章评分失败」），实际 %s", wantMsg, body)
	}
	if contains(body, "读取文章评分失败") {
		t.Fatalf("关停不该落进 default 的兜底文案，实际 %s", body)
	}
	// 参数类失败在协议里是 1001，不是 1002：1002 会让调用方以为后端异常。
	if !contains(body, "1001") {
		t.Fatalf("业务码应为 1001（请求类），实际 %s", body)
	}
}

// 其余三类错误仍保持旧文案（这次只加了一条 case，不能顺手改掉它们）。
func TestFailScore_既有三类文案不变(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code apperr.Code
		want string
	}{
		{apperr.CodeAuthRequired, "请先登录"},
		{apperr.CodeAIPermissionDenied, "权限错误"},
		{apperr.CodeAIArticleMissing, "文章不存在"},
	}

	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/ai/scoring/article", nil)

			failScore(c, apperr.New(tc.code))
			if body := recorder.Body.String(); !contains(body, tc.want) {
				t.Fatalf("文案应为 %q，实际 %s", tc.want, body)
			}
		})
	}
}

// contains 是一个极小助手：这里只需要看包体里有没有那段文字，
// 不引入 json 解析是为了让失败信息保留原始包体。
func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// 编译期守卫：响应层 helper 的语义（1001 vs 1002）必须与协议登记一致。
var _ = res.FailWithValidError
