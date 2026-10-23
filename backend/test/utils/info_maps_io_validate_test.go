package utils_test

import (
	"io"
	"myblogx/middleware"
	"myblogx/utils/info_check"
	"myblogx/utils/io_util"
	"myblogx/utils/maps"
	"myblogx/utils/validate"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSensitiveWordAndUsernameCheck(t *testing.T) {
	if word, ok := info_check.IsSensitiveWord("A_d_m_i_n_001"); !ok || word != "admin" {
		t.Fatalf("敏感词检测失败 word=%s ok=%v", word, ok)
	}
	if _, ok := info_check.IsSensitiveWord("normal_user_123"); ok {
		t.Fatal("正常用户名不应被识别为敏感词")
	}

	valid := []string{"hello_12", "abc12345", "my_name_1"}
	for _, v := range valid {
		if err := info_check.CheckUsername(v); err != nil {
			t.Fatalf("合法用户名被拒绝: %s, err=%v", v, err)
		}
	}

	invalid := []string{"", "abc", "a-b-c-123", "_abcdef", "admin123", "a___bcdef"}
	for _, v := range invalid {
		if err := info_check.CheckUsername(v); err == nil {
			t.Fatalf("非法用户名未报错: %s", v)
		}
	}
}

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

func TestGetBody(t *testing.T) {
	body := io.NopCloser(strings.NewReader(`{"a":1}`))
	data, err := io_util.GetBody(&body)
	if err != nil {
		t.Fatalf("GetBody 失败: %v", err)
	}
	if string(data) != `{"a":1}` {
		t.Fatalf("body 不一致: %s", string(data))
	}
	data2, _ := io.ReadAll(body)
	if string(data2) != `{"a":1}` {
		t.Fatalf("body 未恢复: %s", string(data2))
	}
}

func TestShouldBindJSONWithRecover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x":"ok"}`))

	var obj struct {
		X string `json:"x"`
	}
	if err := io_util.ShouldBindJSONWithRecover(c, &obj); err != nil {
		t.Fatalf("ShouldBindJSONWithRecover 失败: %v", err)
	}
	if obj.X != "ok" {
		t.Fatalf("绑定值错误: %+v", obj)
	}

	var obj2 struct {
		X string `json:"x"`
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x"`))
	if err := io_util.ShouldBindJSONWithRecover(c, &obj2); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestValidateErrorHelpers(t *testing.T) {
	type Req struct {
		Name string `json:"name" binding:"required" label:"名称"`
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v", middleware.BindJson[Req], func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP 状态码异常: %d", w.Code)
	}

	msg := validate.ValidateErr(io.EOF)
	if msg != "请求体不能为空" {
		t.Fatalf("EOF 提示错误: %s", msg)
	}

	data, msg2 := validate.ValidateError(io.EOF)
	if data["body"] != "请求体不能为空" {
		t.Fatalf("EOF data 错误: %+v", data)
	}
	if msg2 != "请求体不能为空" {
		t.Fatalf("ValidateError msg 错误: %s", msg2)
	}
}
