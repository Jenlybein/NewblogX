package app

import (
	"context"

	"github.com/sirupsen/logrus"

	"myblogx/conf"
	"myblogx/internal/apperr"
	articleapp "myblogx/internal/article/app"
	articledomain "myblogx/internal/article/domain"
	articlecollection "myblogx/internal/article/domain/collection"
	commentapp "myblogx/internal/comment/app"
	commentdomain "myblogx/internal/comment/domain"
	filterdomain "myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	userapp "myblogx/internal/user/app"
	userdomain "myblogx/internal/user/domain"

	filterhttp "myblogx/internal/filter/adapter/http"
	filtermysql "myblogx/internal/filter/adapter/mysql"
	filterapp "myblogx/internal/filter/app"
	"myblogx/internal/platform/contentfilter"
	"myblogx/internal/platform/http/router"

	"fmt"
)

// 内容过滤的装配（ADR-0032 / 票 396、398）。
//
// 分成三段是刻意的，与仓库里"服务在用到的地方构造、handler 在 WireHTTP 注入"的
// 既有分工一致：
//   - InitContentFilter 在 InitRuntimeServices 里跑：它依赖运行时配置，且**所有**
//     进程角色都该拿到同一份引擎（api / worker / cron 都可能写文本）；构建失败
//     直接让启动失败（无过滤上线等于裸奔）。
//   - WireFilter 在 WireHTTP 里跑：只做处理器注入，路由树是 HTTP 角色的事。
//   - WireFilterDisposer 在内容模块装配完之后跑：处置要删的实体归文章 / 评论 /
//     用户三个模块所有，桥接必须先拿到它们的用例服务。

// InitContentFilter 用运行期词表构建过滤引擎，并把命中记录服务挂到 Infra。
func InitContentFilter(infra *Infra) error {
	if infra == nil || infra.RuntimeSite == nil {
		return fmt.Errorf("初始化内容过滤失败: 运行时配置或基础设施未就绪")
	}
	contentFilter := infra.RuntimeSite.GetRuntimeContentFilter()
	words := contentFilterWords(contentFilter)
	engine, err := contentfilter.NewEngine(words, contentFilterWhitelist(contentFilter))
	if err != nil {
		return fmt.Errorf("内容过滤引擎构建失败（无过滤上线等于裸奔）: %w", err)
	}
	infra.ContentFilter = engine
	// 命中记录服务：写失败只记日志（见 filterapp.Service.Record），不阻断调用方。
	infra.Filter = filterapp.New(engine, filtermysql.New(infra.DB), infra.Logger)
	if infra.Logger != nil {
		infra.Logger.Infof(
			"内容过滤引擎已构建：词表 %d 条（引擎去重与短词剔除后 %d 条）；词表类目分布 %s",
			len(words),
			engine.WordCount(),
			contentfilter.FormatCategoryCounts(contentfilter.CategoryCounts(words)),
		)
	}
	warnMissingCategories(infra, words)
	return nil
}

// warnMissingCategories 在"必需类目为空"时打一条 Warn。
//
// 为什么要专门一条日志：这两类词按 ADR-0032 决策 8 只进数据库、不进仓库，
// 仓库默认排除、CI 也看不到，所以**没有任何环节会失败**。生产上真的发生过
// "词库 3 万条、后台看起来很满、涉政与暴恐却是 0 条"，而当时七个文本面
// 对涉政内容完全不拦 —— 这条 Warn 是那个缺口的唯一红灯。
//
// 只打类目名与条数，不打词条（词表是敏感数据，日志里不能出现词）。
func warnMissingCategories(infra *Infra, words []contentfilter.Word) {
	if infra == nil || infra.Logger == nil {
		return
	}
	missing := contentfilter.MissingRequiredCategories(words)
	if len(missing) == 0 {
		return
	}
	infra.Logger.Warnf("%s", contentfilter.CategoryCoverageHint(missing))
}

// WireFilter 把关键词风险处理器注入路由树。
func WireFilter(infra *Infra) {
	if infra == nil || infra.Filter == nil {
		return
	}
	router.SetFilterHandler(filterhttp.NewHandler(infra.Filter))
}

// WireFilterDisposer 把处置写端口接到内容过滤服务（票 398）。
//
// 必须排在 WireArticle / WireComment / WireUser **之后**：这三个模块的用例服务
// 是它们各自的装配产物（不是全局单例），处置桥需要拿到同一批实例，否则会出现
// "删文章用的服务与写文章用的服务不是同一份装配"这种只在生产才暴露的分歧。
func WireFilterDisposer(infra *Infra) {
	if infra == nil || infra.Filter == nil {
		return
	}
	infra.Filter.WithDisposer(disposerBridge{
		articles: infra.ArticleService,
		comments: infra.CommentService,
		users:    infra.UserService,
	})
	// 账号处置（封禁 / 禁言，票 399）：与内容处置共用同一份 user 用例服务。
	infra.Filter.WithAccountDisposer(accountDisposerBridge{users: infra.UserService})
}

// disposerBridge 是 Disposer 的实现：把"删除某类内容"翻译成对应模块的软删用例。
//
// 每个方法只做一件事：把 (actor, id) 拼成那个模块既有的命令结构。这样做而不是
// 直接写 SQL，是为了让删除仍然经过各模块的状态机与副作用（文章删除要清评论 /
// 点赞 / 收藏并写 outbox，评论删除要改计数，用户资料要同步展示快照）。
type disposerBridge struct {
	articles *articleapp.Service
	comments *commentapp.Service
	users    *userapp.Service
	logger   *logrus.Logger
}

var _ filterdomain.Disposer = disposerBridge{}

// DeleteArticle 软删一篇文章。
func (d disposerBridge) DeleteArticle(ctx context.Context, actor, articleID ctype.ID) error {
	if d.articles == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	_, err := d.articles.Delete(ctx, articleapp.DeleteCmd{
		Actor:     articledomain.AdminUser(articledomain.UserID(actor)),
		ArticleID: articleID,
	})
	return err
}

// DeleteComment 软删一条评论。
func (d disposerBridge) DeleteComment(ctx context.Context, actor, commentID ctype.ID) error {
	if d.comments == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	return d.comments.Remove(ctx,
		commentdomain.Viewer{ID: actor, Admin: true},
		commentID.Uint64())
}

// DeleteCollection 软删一个合集（不删合集中的文章本体）。
func (d disposerBridge) DeleteCollection(ctx context.Context, actor, collectionID ctype.ID) error {
	if d.articles == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	return d.articles.DeleteCollection(ctx, articleapp.DeleteCollectionCmd{
		Actor:        articledomain.AdminUser(articledomain.UserID(actor)),
		CollectionID: articlecollection.ID(collectionID),
	})
}

// ResetProfile 重置被处置用户的资料：昵称改为「问题昵称 + uid 末 4 位」、简介清空。
//
// 走用户模块的 Update 用例而不是直接改仓储：资料变更要经过格式校验与展示快照
// 同步，绕过它会让公开主页继续渲染旧昵称（那正是这次事故的形状）。
func (d disposerBridge) ResetProfile(ctx context.Context, actor, userID ctype.ID, nickname string) error {
	if d.users == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	empty := ""
	update := userdomain.ProfileUpdate{Nickname: &nickname, Abstract: &empty}
	_, err := d.users.Update(ctx,
		userdomain.Viewer{ID: userdomain.UserID(actor), Role: userdomain.RoleAdmin},
		userdomain.UserID(userID),
		update)
	return err
}

// accountDisposerBridge 是 AccountDisposer 的实现：把"封禁 / 禁言"翻译成
// user 模块的账号用例（票 399）。
//
// 两个动作都走 user 模块已经有权限判定与会话处理的用例，而不是直接写 SQL：
//   - 封禁 = AdminUpdate(status=3)：与后台账号页的「封禁」同一口径，
//     会把令牌版本 +1 并吊销全部会话（被封的人手里那张未过期令牌立即失效）；
//   - 禁言 = Mute(days)：只写 mute_until，不动会话（禁言不拦登录）。
//
// 时长校验（1–365 / 缺省 7）也由 user 模块做：内容过滤只负责传原始天数。
type accountDisposerBridge struct {
	users *userapp.Service
}

var _ filterdomain.AccountDisposer = accountDisposerBridge{}

// BanUser 封禁账号（账号禁用，不可自助恢复）。
func (d accountDisposerBridge) BanUser(ctx context.Context, actor, userID ctype.ID) error {
	if d.users == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	banned := userdomain.StatusBanned
	return d.users.AdminUpdate(ctx,
		userdomain.Viewer{ID: userdomain.UserID(actor), Role: userdomain.RoleAdmin},
		userdomain.UserID(userID),
		userapp.AdminPatchInput{Status: &banned},
	)
}

// MuteUser 禁言账号 days 天（0 表示没传，user 模块按默认 7 天处理）。
func (d accountDisposerBridge) MuteUser(ctx context.Context, actor, userID ctype.ID, days int) error {
	if d.users == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	_, err := d.users.Mute(ctx,
		userdomain.Viewer{ID: userdomain.UserID(actor), Role: userdomain.RoleAdmin},
		userdomain.UserID(userID),
		days)
	return err
}

// contentFilterWords 把配置段的词条转成引擎形状。
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

// contentFilterWhitelist 合并词级与短语级白名单（引擎只认一张表，见 MergeWhitelist）。
//
// 两处构建（启动与保存后重建）必须走同一个函数：只要有一处只传了词级那份，
// 短语白名单就会在"保存一次词表之后悄悄失效"—— 这种 bug 只在后台点过保存才出现，
// 本地起服务怎么试都是好的。
func contentFilterWhitelist(contentFilter conf.ContentFilter) []string {
	return contentfilter.MergeWhitelist(contentFilter.Whitelist, contentFilter.WhitelistPhrases)
}

// rebuildContentFilter 按新词表原子重建引擎（词表保存后由站点用例回调）。
//
// 重建失败只记日志、不切换：旧匹配器继续生效。后台保存一份坏词表不该把线上
// 正在用的词表一起弄丢（引擎的 Rebuild 语义也是如此）。
func rebuildContentFilter(infra *Infra, contentFilter conf.ContentFilter) {
	if infra == nil || infra.ContentFilter == nil {
		return
	}
	words := contentFilterWords(contentFilter)
	if err := infra.ContentFilter.Rebuild(words, contentFilterWhitelist(contentFilter)); err != nil {
		if infra.Logger != nil {
			infra.Logger.Errorf("内容过滤引擎重建失败，继续使用旧词表: %v", err)
		}
		return
	}
	if infra.Logger != nil {
		infra.Logger.Infof("内容过滤引擎已按新词表重建：词表 %d 条（引擎收 %d 条）；词表类目分布 %s",
			len(words),
			infra.ContentFilter.WordCount(),
			contentfilter.FormatCategoryCounts(contentfilter.CategoryCounts(words)),
		)
	}
	// 保存词表时也自查一次：后台把某一类删空、或换了台机器重新导入，
	// 这是最容易再次出现空类目的时刻。
	warnMissingCategories(infra, words)
}
