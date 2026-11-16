package app

import (
	"fmt"

	"myblogx/conf"
	filterhttp "myblogx/internal/filter/adapter/http"
	filtermysql "myblogx/internal/filter/adapter/mysql"
	filterapp "myblogx/internal/filter/app"
	"myblogx/internal/platform/contentfilter"
	"myblogx/internal/platform/http/router"
)

// 内容过滤的装配（ADR-0032 / 票 396）。
//
// 分成两半是刻意的，与仓库里"服务在用到的地方构造、handler 在 WireHTTP 注入"的
// 既有分工一致：
//   - InitContentFilter 在 InitRuntimeServices 里跑：它依赖运行时配置，且**所有**
//     进程角色都该拿到同一份引擎（api / worker / cron 都可能写文本）；构建失败
//     直接让启动失败（无过滤上线等于裸奔）。
//   - WireFilter 在 WireHTTP 里跑：只做处理器注入，路由树是 HTTP 角色的事。

// InitContentFilter 用运行期词表构建过滤引擎，并把命中记录服务挂到 Infra。
//
// 引擎构建失败返回 error（调用方让启动失败）：词表里的档位/类目写错了却照常上线，
// 等于全站裸奔——这正是 ADR-0032 决策 11 要避免的那件事。
func InitContentFilter(infra *Infra) error {
	if infra == nil || infra.RuntimeSite == nil {
		return fmt.Errorf("初始化内容过滤失败: 运行时配置或基础设施未就绪")
	}
	contentFilter := infra.RuntimeSite.GetRuntimeContentFilter()
	engine, err := contentfilter.NewEngine(contentFilterWords(contentFilter), contentFilter.Whitelist)
	if err != nil {
		return fmt.Errorf("内容过滤引擎构建失败（无过滤上线等于裸奔）: %w", err)
	}
	infra.ContentFilter = engine
	// 命中记录服务：写失败只记日志（见 filterapp.Service.Record），不阻断调用方。
	infra.Filter = filterapp.New(engine, filtermysql.New(infra.DB), infra.Logger)
	if infra.Logger != nil {
		infra.Logger.Infof(
			"内容过滤引擎已构建：词条 %d 条（含白名单命中的统计口径见风险页）",
			engine.WordCount(),
		)
	}
	return nil
}

// WireFilter 把关键词风险处理器注入路由树。
func WireFilter(infra *Infra) {
	if infra == nil || infra.Filter == nil {
		return
	}
	router.SetFilterHandler(filterhttp.NewHandler(infra.Filter))
}

// contentFilterWords 把配置段的词条转成引擎形状。
//
// 只做形状转换，不做校验：合法性已在保存用例（siteapp.UpdateContentFilter）与
// 初始词表导入命令里各过一次，这里再校验一遍只会让"坏数据"的来源变得难查。
func contentFilterWords(contentFilter conf.ContentFilter) []contentfilter.Word {
	words := make([]contentfilter.Word, 0, len(contentFilter.Words))
	for _, entry := range contentFilter.Words {
		words = append(words, contentfilter.Word{
			Word:     entry.Word,
			Tier:     contentfilter.Tier(entry.Tier),
			Category: contentfilter.Category(entry.Category),
		})
	}
	return words
}

// rebuildContentFilter 按新词表原子重建引擎（词表保存后由站点用例回调）。
//
// 重建失败只记日志、不切换：旧匹配器继续生效。后台保存一份坏词表不该把线上
// 正在用的词表一起弄丢（引擎的 Rebuild 语义也是如此）。
func rebuildContentFilter(infra *Infra, contentFilter conf.ContentFilter) {
	if infra == nil || infra.ContentFilter == nil {
		return
	}
	if err := infra.ContentFilter.Rebuild(contentFilterWords(contentFilter), contentFilter.Whitelist); err != nil {
		if infra.Logger != nil {
			infra.Logger.Errorf("内容过滤引擎重建失败，继续使用旧词表: %v", err)
		}
		return
	}
	if infra.Logger != nil {
		infra.Logger.Infof("内容过滤引擎已按新词表重建：词条 %d 条", infra.ContentFilter.WordCount())
	}
}
