package app

import (
	sitecfg "myblogx/internal/platform/config"
)

// onlyAdminAuthors 把生效值模块的「显示非管理员发布文章」开关翻成**读过滤开关**。
//
// 语义见 ADR-0015 决策 4：开关关闭时，读者端所有公开读面一致过滤掉"作者不是
// 管理员"的文章；后台（type 5）与自己的文章（type 4）不受影响。取反只在
// platform/config 的生效值模块里做一次（#284），本函数只是装配层转交闭包。
//
// 返回取值函数而不是布尔：开关必须**每请求现读**，管理员在后台改完下一次请求
// 即生效（GLOSSARY 的「Provider 闭包（现读）」）。没有配置服务时按默认值 true（展示）。
func onlyAdminAuthors(effective *sitecfg.EffectiveConfig) func() bool {
	return effective.OnlyAdminAuthorsFunc()
}
