package http_test

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"myblogx/conf"
	sitehttp "myblogx/internal/site/adapter/http"
	sitetest "myblogx/internal/site/apptest"

	"github.com/gin-gonic/gin"
)

// 本用例跨唯一行为接缝验「一张参数表 → 契约（开放接口契约）+ 后端 HTTP 接口」：
// 契约印出每个参数的 min/max/default，GET /site/deco 返回参数表的默认值，
// PUT /site/deco 把越界值钳回参数表的边界。它不读任何实现源码文本。
type decoParameterSpec struct {
	Key     string  `json:"key"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
}

type decoParameterTable struct {
	Section    string              `json:"section"`
	Subsection string              `json:"subsection"`
	Parameters []decoParameterSpec `json:"parameters"`
}

const decoParameterTablePath = "../../../../../config/deco-charm-parameters.json"
const decoContractPath = "../../../../docs/swagger.json"

func readDecoParameterTable(t *testing.T) decoParameterTable {
	t.Helper()
	raw, err := os.ReadFile(decoParameterTablePath)
	if err != nil {
		t.Fatalf("读取仓库级装饰参数表失败: %v", err)
	}
	var table decoParameterTable
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("解析仓库级装饰参数表失败: %v", err)
	}
	if table.Section != "deco" || table.Subsection != "charm" {
		t.Fatalf("参数表应指向运行时配置的 deco.charm 段，实际 %s.%s", table.Section, table.Subsection)
	}
	if len(table.Parameters) != 12 {
		t.Fatalf("装饰挂件应有 12 个数值参数，实际 %d", len(table.Parameters))
	}
	return table
}

func readDecoContractProperties(t *testing.T) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(decoContractPath)
	if err != nil {
		t.Fatalf("读取契约（开放接口契约）失败: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("解析契约（开放接口契约）失败: %v", err)
	}
	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("契约缺少 components")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("契约缺少 components.schemas")
	}
	charm, ok := schemas["conf.Charm"].(map[string]any)
	if !ok {
		t.Fatal("契约缺少 conf.Charm 具名形状")
	}
	properties, ok := charm["properties"].(map[string]any)
	if !ok {
		t.Fatal("契约里的 conf.Charm 缺少 properties")
	}
	out := make(map[string]map[string]any, len(properties))
	for key, value := range properties {
		property, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("契约里的 conf.Charm.%s 不是对象", key)
		}
		out[key] = property
	}
	return out
}

func contractNumber(t *testing.T, property map[string]any, field string, parameter string) float64 {
	t.Helper()
	value, ok := property[field]
	if !ok {
		t.Fatalf("契约里的 conf.Charm.%s 缺少 %s", parameter, field)
	}
	number, ok := value.(float64)
	if !ok {
		t.Fatalf("契约里的 conf.Charm.%s.%s 应是数字，实际 %T(%v)", parameter, field, value, value)
	}
	return number
}

func decoCharmFromNumericValues(t *testing.T, values map[string]float64) conf.Charm {
	t.Helper()
	payload := map[string]any{"enable": true}
	for key, value := range values {
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("构造 deco.charm 请求载荷失败: %v", err)
	}
	var charm conf.Charm
	if err := json.Unmarshal(raw, &charm); err != nil {
		t.Fatalf("解析 deco.charm 请求载荷失败: %v", err)
	}
	return charm
}

func decoCharmNumericValues(t *testing.T, charm conf.Charm) map[string]float64 {
	t.Helper()
	raw, err := json.Marshal(charm)
	if err != nil {
		t.Fatalf("序列化 deco.charm 失败: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("解析 deco.charm 失败: %v", err)
	}
	values := make(map[string]float64, len(decoded))
	for key, value := range decoded {
		if key == "enable" {
			continue
		}
		number, ok := value.(float64)
		if !ok {
			t.Fatalf("deco.charm.%s 应是数字，实际 %T(%v)", key, value, value)
		}
		values[key] = number
	}
	return values
}

func callDecoSection(t *testing.T, handler *sitehttp.Handler, fn func(*gin.Context), method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	fn(ctx)
	return recorder
}

func decodeDecoResponse(t *testing.T, recorder *httptest.ResponseRecorder) conf.Deco {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("HTTP 状态应为 200，实际 %d，body: %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data conf.Deco `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("解析 Envelope[deco] 失败: %v，body: %s", err, recorder.Body.String())
	}
	return envelope.Data
}

func assertDecoNumericValues(t *testing.T, label string, got map[string]float64, table decoParameterTable, expected func(decoParameterSpec) float64) {
	t.Helper()
	for _, parameter := range table.Parameters {
		value, ok := got[parameter.Key]
		if !ok {
			t.Fatalf("%s 缺少 %s", label, parameter.Key)
		}
		want := expected(parameter)
		if math.Abs(value-want) > 1e-9 {
			t.Fatalf("%s 的 %s 应为 %v，实际 %v", label, parameter.Key, want, value)
		}
	}
}

func TestGeneratedDecoParametersAreExposedByContractAndHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	table := readDecoParameterTable(t)

	t.Run("契约印出参数表的 min/max/default", func(t *testing.T) {
		properties := readDecoContractProperties(t)
		for _, parameter := range table.Parameters {
			property, ok := properties[parameter.Key]
			if !ok {
				t.Fatalf("契约缺少 conf.Charm.%s", parameter.Key)
			}
			if got := contractNumber(t, property, "minimum", parameter.Key); got != parameter.Min {
				t.Fatalf("契约 conf.Charm.%s.minimum 应为 %v，实际 %v", parameter.Key, parameter.Min, got)
			}
			if got := contractNumber(t, property, "maximum", parameter.Key); got != parameter.Max {
				t.Fatalf("契约 conf.Charm.%s.maximum 应为 %v，实际 %v", parameter.Key, parameter.Max, got)
			}
			if got := contractNumber(t, property, "default", parameter.Key); got != parameter.Default {
				t.Fatalf("契约 conf.Charm.%s.default 应为 %v，实际 %v", parameter.Key, parameter.Default, got)
			}
		}
	})

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	handler := sitehttp.NewHandler(env.Service, nil)

	defaults := make(map[string]float64, len(table.Parameters))
	tooLarge := make(map[string]float64, len(table.Parameters))
	for _, parameter := range table.Parameters {
		defaults[parameter.Key] = parameter.Default
		tooLarge[parameter.Key] = parameter.Max + 1
	}
	if err := env.Runtime.UpdateDeco(conf.Deco{Charm: decoCharmFromNumericValues(t, defaults)}); err != nil {
		t.Fatalf("预置装饰配置失败: %v", err)
	}

	t.Run("GET /site/deco 返回参数表的默认值", func(t *testing.T) {
		recorder := callDecoSection(t, handler, handler.PublicDecoSection, "GET", "/site/deco", "")
		deco := decodeDecoResponse(t, recorder)
		assertDecoNumericValues(t, "GET /site/deco", decoCharmNumericValues(t, deco.Charm), table,
			func(parameter decoParameterSpec) float64 { return parameter.Default })
	})

	t.Run("PUT /site/deco 按参数表边界钳位并可再次读出", func(t *testing.T) {
		body, err := json.Marshal(map[string]any{
			"charm": decoCharmFromNumericValues(t, tooLarge),
		})
		if err != nil {
			t.Fatalf("构造 PUT 请求体失败: %v", err)
		}
		recorder := callDecoSection(t, handler, handler.UpdateDecoSection, "PUT", "/site/deco", string(body))
		if recorder.Code != 200 {
			t.Fatalf("PUT /site/deco 应成功，实际 %d，body: %s", recorder.Code, recorder.Body.String())
		}
		assertDecoNumericValues(t, "PUT /site/deco 后的生效值", decoCharmNumericValues(t, env.Runtime.Deco().Charm), table,
			func(parameter decoParameterSpec) float64 { return parameter.Max })

		readBack := callDecoSection(t, handler, handler.AdminDecoSection, "GET", "/site/admin/deco", "")
		deco := decodeDecoResponse(t, readBack)
		assertDecoNumericValues(t, "GET /site/admin/deco", decoCharmNumericValues(t, deco.Charm), table,
			func(parameter decoParameterSpec) float64 { return parameter.Max })
	})
}
