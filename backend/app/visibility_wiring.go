package app

import (
	sitecfg "myblogx/internal/platform/config"
)

// onlyAdminAuthors 把站点运行时配置的「显示非管理员发布文章」开关翻成**读过滤开关**。
//
// 语义见 ADR-0017 决策 4：开关关闭时，读者端所有公开读面一致过滤掉"作者不是
// 管理员"的文章；后台（type 5）与自己的文章（type 4）不受影响。开关的正反面
// 在这里翻转一次，下游（search 与 article）只认"只出管理员作者"这个正向布尔，
// 免得每处各写一次取反、也免得零值把内容静默隐藏。
//
// 返回取值函数而不是布尔：开关必须**每请求现读**，管理员在后台改完下一次请求
// 即生效（house 的 provider 闭包模式）。没有配置服务时按默认值 true（展示）。
func onlyAdminAuthors(site *sitecfg.RuntimeConfigService) func() bool {
	return func() bool {
		if site == nil {
			return false
		}
		return !site.GetRuntimeArticle().ShowNonAdminArticles
	}
}
