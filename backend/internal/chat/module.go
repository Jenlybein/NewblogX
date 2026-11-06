// Package chat 是私信模块的装配入口（#280）。
//
// 装配搬回模块内：九个协作者（会话读写、关注关系、用户展示、陌生人两级开关、
// 按对限流、图片来源、屏蔽 / 拉黑、日志）在这里组装成一个用例服务与一个
// HTTP 处理器；合成根只需要交出基础设施与跨模块能力，不再逐个 new 私信的
// 适配器，也不再经由路由包级变量注入。
//
// 模块间零引入的内核不变：本包不 import social / user / auth 等模块的包，
// 跨模块能力一律以本包定义的窄接口或使用方端口（domain.*）传入。
package chat

import (
	"context"
	"net/url"
	"strings"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"myblogx/conf"
	chathttp "myblogx/internal/chat/adapter/http"
	chatmysql "myblogx/internal/chat/adapter/mysql"
	chatredis "myblogx/internal/chat/adapter/redis"
	chatapp "myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/imageref"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/ws"
	"myblogx/internal/platform/readmodel"
	redis_service "myblogx/internal/platform/redis"

	"github.com/sirupsen/logrus"
)

// UserSettings 是 user 模块暴露的偏好读能力：接收方愿不愿意收陌生人私信。
//
// 只依赖这一条读能力，不碰 user 仓储、也不写 SQL：个人开关的真相留在用户模块里。
type UserSettings interface {
	StrangerChatEnabled(ctx context.Context, id ctype.ID) (bool, error)
}

// Module 是一个装配完成的私信模块。
type Module struct {
	Service *chatapp.Service
	Handler *chathttp.Handler
	Hub     *ws.Hub
}

// New 装配私信模块。
//
// 基础设施（DB / Redis / 运行时站点配置 / 日志）与跨模块能力由合成根给出；
// 私信自己的适配器（仓储、限流、屏蔽策略、图片校验、票据、连接池、处理器）
// 全部由本模块构造，合成根不再知道这些细节。参数是显式的窄能力，不是
// 依赖打包结构体（ADR-0001）。
//
// DB / Redis 缺失时返回 nil：私信没有可用的落库或限流能力，启动应当失败，
// 而不是带着半个模块继续跑（路由注册会因为 Handler 为空而 panic）。
func New(
	db *gorm.DB,
	redisClient *redis.Client,
	logger *logrus.Logger,
	effective *sitecfg.EffectiveConfig,
	relations domain.RelationReader,
	userSettings UserSettings,
	auth chathttp.Authenticator,
) *Module {
	if db == nil || redisClient == nil {
		return nil
	}
	redisOptions := redis_service.NewOptions(redisClient, logger)
	hub := ws.NewHub()

	// 同一个仓储同时实现读端口与写端口；用例层分别注入，读写分离在接口层面成立。
	store := chatmysql.New(db)
	var appLogger domain.Logger
	if logger != nil {
		appLogger = logger
	}

	if effective == nil {
		effective = sitecfg.NewEffectiveConfig(nil)
	}
	svc := chatapp.New(
		store,
		store,
		relations,
		&userDirectory{db: db},
		&strangerPolicy{inner: effective, users: userSettings},
		chatredis.NewLimiter(redisOptions, effective.ChatRateFunc()),
		&imageVerifier{db: db, storage: effective},
		chatmysql.NewBlockPolicy(db),
		appLogger,
	)

	handler := chathttp.NewHandler(
		svc,
		hub,
		chatredis.NewTickets(redisOptions),
		auth,
		appLogger,
	)
	return &Module{Service: svc, Handler: handler, Hub: hub}
}

// userDirectory 读用户展示快照（平台读模型，不 import user 模块）。
type userDirectory struct {
	db *gorm.DB
}

// Displays 实现 domain.UserDirectory。
func (d *userDirectory) Displays(_ context.Context, ids []ctype.ID) (map[ctype.ID]domain.Display, error) {
	raw, err := readmodel.LoadUserDisplayMap(d.db, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[ctype.ID]domain.Display, len(raw))
	for id, item := range raw {
		out[id] = domain.Display{Nickname: item.Nickname, Avatar: item.Avatar}
	}
	return out, nil
}

// strangerPolicy 把两级开关合成一个结论（#243）。
//
//	有效判决 = 站点总闸(site.chat.stranger_msg_enabled)
//	           AND 接收方个人开关(user_conf_models.stranger_chat_enabled，默认开)
//
// 站点一级来自运行时站点配置，每次现读缓存，后台改完保存后立即生效；
// 接收方一级走 user 模块读端口，每次现读，账号设置改完立即生效。
// 任一级关闭即拒绝；站点关闭时不再多查一次用户偏好。
type strangerPolicy struct {
	inner *sitecfg.EffectiveConfig
	users UserSettings
}

var _ domain.StrangerPolicy = (*strangerPolicy)(nil)

// CheckStranger 实现 domain.StrangerPolicy。
//
// 返回值不只回答"能不能发"，还带上拒绝来自哪一级（#247）：站点级与接收方级
// 在读者侧对应不同文案与不同正确动作。
func (p *strangerPolicy) CheckStranger(ctx context.Context, receiverID ctype.ID) (domain.StrangerVerdict, error) {
	if p.inner == nil {
		// 未装配运行时配置时按"关"处理（与默认值一致，失败关闭）。
		// 这是部署/配置问题，归到站点级，避免把用户引向"对方关了开关"。
		return domain.StrangerBlockedBySite, nil
	}
	if !p.inner.Chat().StrangerMsgEnabled {
		return domain.StrangerBlockedBySite, nil
	}
	if p.users == nil {
		// 用户模块未装配时无法确认接收方意愿：按"关"处理，失败关闭。
		// 同样归到站点级：读不到接收方偏好不等于接收方关掉了开关。
		return domain.StrangerBlockedBySite, nil
	}
	enabled, err := p.users.StrangerChatEnabled(ctx, receiverID)
	if err != nil {
		return domain.StrangerBlockedBySite, err
	}
	if !enabled {
		return domain.StrangerBlockedByRecipient, nil
	}
	return domain.StrangerAllowed, nil
}

// imageVerifier 判定图片私信里的 URL 是不是本站上传产物（票 267）。
//
// 两步，都复用既有实现，不新增表也不加列：
//  1. 校验域名：`imageref.ExtractObjectKey` 只看路径，外链域名拼一个长得一样
//     的路径照样能反解出 object_key。所以这里先按 storage.Uri 归一化后的域名
//     比对 host（与 `qiniu.Storage.mediaBase` 同一套 http→https 收敛规则）。
//  2. `imageref.ExtractObjectKey` 按当前存储配置（前缀）反解 object_key，
//     再去 `image_models` 查一行 —— 上传确认时才会落这张表，所以"能查到"
//     就等价于"确实是本站走上传任务传上去的"。
//
// 失败关闭：存储没启用、域名不匹配、反解失败、查库报错都按"不是本站图片"处理，
// 不让校验成为绕过口子。
type imageVerifier struct {
	db      *gorm.DB
	storage *sitecfg.EffectiveConfig
}

var _ domain.ImageURLVerifier = (*imageVerifier)(nil)

// IsOwnedImageURL 实现 domain.ImageURLVerifier。
func (v *imageVerifier) IsOwnedImageURL(ctx context.Context, rawURL string) (bool, error) {
	if v == nil || v.db == nil || v.storage == nil {
		return false, nil
	}
	cfg := v.storage.Storage()
	if !cfg.Enable {
		return false, nil
	}
	if !hostMatchesStorage(rawURL, cfg.Uri) {
		return false, nil
	}
	objectKey := imageref.ExtractObjectKey(rawURL, conf.QiNiu(cfg))
	if objectKey == "" {
		return false, nil
	}
	var count int64
	if err := v.db.WithContext(ctx).Table("image_models").
		Where("object_key = ? AND deleted_at IS NULL", objectKey).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// hostMatchesStorage 判断 URL 的 host 是不是当前配置的存储域名。
//
// 规则与 qiniu.Storage.mediaBase 一致：配置里写裸域名 / http:// / // 都收敛成
// https://，比较时忽略 scheme 与端口差异中的 scheme 部分（站点在 HTTPS 下，
// 浏览器会自动把 http 图片升级成 https）。配置为空时一律不匹配（失败关闭）。
func hostMatchesStorage(rawURL, configured string) bool {
	configured = strings.TrimSpace(configured)
	if rawURL == "" || configured == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	base := configured
	if !strings.Contains(base, "://") {
		base = "//" + base
	}
	baseURL, err := url.Parse(base)
	if err != nil || baseURL.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), baseURL.Hostname())
}
