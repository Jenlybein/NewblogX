package validate_test

import (
	"encoding/json"
	"io"
	"myblogx/utils/validate"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func TestValidateErrorHelpers(t *testing.T) {
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

func TestValidateErrorWithJSONSyntaxError(t *testing.T) {
	var obj map[string]any
	err := json.Unmarshal([]byte(`{"name":}`), &obj)
	if err == nil {
		t.Fatal("期望产生 JSON 语法错误")
	}

	data, msg := validate.ValidateError(err)
	if data["body"] == nil {
		t.Fatalf("JSON 语法错误应返回 body 提示: %+v", data)
	}
	if msg == "" {
		t.Fatal("JSON 语法错误提示不能为空")
	}
}

type validateReq struct {
	Name string `json:"name" label:"名称" binding:"required,min=2"`
	Age  int    `json:"age" label:"年龄" binding:"gte=1"`
}

func TestValidateErrWithValidationErrors(t *testing.T) {
	v := binding.Validator.Engine().(*validator.Validate)
	err := v.Struct(validateReq{Name: "a", Age: 0})
	if err == nil {
		t.Fatal("期望产生校验错误")
	}

	msg := validate.ValidateErr(err)
	if msg == "" {
		t.Fatal("ValidateErr 返回不能为空")
	}

	data, m := validate.ValidateError(err)
	if len(data) == 0 || m == "" {
		t.Fatalf("ValidateError 返回异常: data=%v msg=%s", data, m)
	}
	if _, ok := data["name"]; !ok {
		t.Fatalf("应包含 name 错误, data=%v", data)
	}
	if _, ok := data["age"]; !ok {
		t.Fatalf("应包含 age 错误, data=%v", data)
	}
}
