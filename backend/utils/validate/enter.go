package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zhTranslations "github.com/go-playground/validator/v10/translations/zh"
)

var trans ut.Translator

func init() {
	uni := ut.New(zh.New())
	trans, _ = uni.GetTranslator("zh")

	v, ok := binding.Validator.Engine().(*validator.Validate)
	if ok {
		_ = zhTranslations.RegisterDefaultTranslations(v, trans)
	}

	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		label := field.Tag.Get("label")
		if label == "" {
			label = field.Name
		}
		name := ""
		tagList := []string{"json", "form", "uri"}
		for _, tag := range tagList {
			name = field.Tag.Get(tag)
			if name != "" {
				break
			}
		}
		return fmt.Sprintf("%s---%s", name, label)
	})
}

func ValidateErr(err error) string {
	_, msg := ValidateError(err)
	return msg
}

func ValidateError(err error) (data map[string]any, msg string) {
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		return translateValidationErrors(validationErrs)
	}
	return translateCommonError(err)
}

func translateValidationErrors(errs validator.ValidationErrors) (data map[string]any, msg string) {
	data = make(map[string]any)
	msgList := make([]string, 0, len(errs))
	for _, e := range errs {
		m := e.Translate(trans)
		parts := strings.Split(m, "---")
		if len(parts) != 2 {
			msgList = append(msgList, m)
			continue
		}
		data[parts[0]] = parts[1]
		msgList = append(msgList, parts[1])
	}
	msg = strings.Join(msgList, ";")
	return
}

func translateCommonError(err error) (data map[string]any, msg string) {
	if err == nil {
		return map[string]any{}, ""
	}

	if errors.Is(err, io.EOF) {
		msg = "请求体不能为空"
		return map[string]any{"body": msg}, msg
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		msg = fmt.Sprintf("JSON 格式错误，位置 %d 附近存在非法内容", syntaxErr.Offset)
		return map[string]any{"body": msg}, msg
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := typeErr.Field
		if field == "" {
			field = "body"
		}
		msg = fmt.Sprintf("字段 %s 类型错误，应为 %s", field, typeErr.Type.String())
		return map[string]any{field: msg}, msg
	}

	if strings.Contains(err.Error(), "unexpected EOF") {
		msg = "JSON 格式不完整，请检查请求体"
		return map[string]any{"body": msg}, msg
	}

	if strings.Contains(err.Error(), "invalid character") {
		msg = "JSON 格式错误，请检查逗号、引号和括号"
		return map[string]any{"body": msg}, msg
	}

	msg = err.Error()
	return map[string]any{}, msg
}
