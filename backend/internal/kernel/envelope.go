// Package kernel 放跨模块共用的值对象与请求结构。
//
// 这里的代码必须是"零业务规则"的：一旦出现"只有文章能用"或"只有用户能用"
// 的判断，它就属于某个模块，不该放在这里。
package kernel

// 本文件是**契约类型**，不是运行时结构。
//
// # 为什么需要它们
//
// res 包的三个成功返回形状是这样拼的：
//
//	res.OkWithData(x)          → {code, msg, data: x}
//	res.OkWithList(xs, n)      → {code, msg, data: {list: xs, count: n}}
//	res.OkWithHasMoreList(x, b) → {code, msg, data: {list: x, has_more: b}}
//
// 但 res 的参数类型是 `any` —— 类型在运行时被擦掉了。swag 只读注解、
// 不读函数调用处，于是注解只能把形状手抄一遍：
//
//	（注解里写一段手抄的 object 形状，里面没有类型信息）
//
// 手抄的后果是：`binding:"required"` 全部失效（required 44/636）、
// 列表元素没有类型（TS 里是 Record<string, never>[]）、改了 struct 契约不变。
//
// 这里定义泛型版本，让注解可以**引用**而不是**手抄**：
//
//	// @Success 200 {object} kernel.Envelope[kernel.ListData[domain.TagListItem]]
//
// swag v2 能解析注解里的泛型（已实测），会生成嵌套正确的 schema。
//
// # 它不是给业务代码用的
//
// 业务代码继续用 res.OkWithList 等函数，不要为了"类型好看"改用这些结构体
// 去构造响应 —— 那会让 `data` 变成强类型，反而把"任意 data"的灵活性弄丢，
// 且没有任何收益（swag 不会因为你用了它而少写一行注解）。

// Envelope 是统一响应外壳，对应 res.OkWithData 的返回形状。
//
//	{"code":0,"msg":"成功","data":{...}}
type Envelope[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

// ListData 是"分页列表"的 data 载荷，对应 res.OkWithList 的返回形状。
//
//	{"list":[...],"count":42}
type ListData[T any] struct {
	List  []T `json:"list"`
	Count int `json:"count"`
}

// HasMoreListData 是"游标/无限滚动"的 data 载荷，对应 res.OkWithHasMoreList。
//
//	{"list":[...],"has_more":true}
type HasMoreListData[T any] struct {
	List    []T  `json:"list"`
	HasMore bool `json:"has_more"`
}

// EmptyData 是"只有提示语、没有数据体"的 data 载荷，对应 res.OkWithMsg。
//
// res.OkWithMsg 实际返回 `data: {}`，所以这里用空结构体。
type EmptyData struct{}

// SiteConfigData 是站点配置的键值块。
//
// 为什么是 map：`/site/{name}` 按 name 返回不同配置块（seo / ai_info / ...），
// 结构随 name 变，契约无法用固定字段表达。
//
// 注意：注解里**不能直接写 `map[string]any`** —— swag 会报
// `cannot find type definition: map[string]any` 并**静默跳过整份契约的写入**
// （exit code 仍是 0，只有翻文件时间才能发现）。所以必须用这个具名类型。
type SiteConfigData map[string]any
