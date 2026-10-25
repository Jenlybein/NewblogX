package app

import (
	notificationhttp "myblogx/internal/notification/adapter/http"
	notificationmysql "myblogx/internal/notification/adapter/mysql"
	notificationapp "myblogx/internal/notification/app"
	"myblogx/internal/platform/http/router"
)

// notificationService 供评论模块与 outbox 中继生产站内信。
//
// 生产侧在运行期才被调用，所以这里用包级变量而不是构造参数，
// 避免"评论装配必须先于通知装配"的顺序约束。
var notificationService *notificationapp.Service

// WireNotification 装配通知模块，并注入路由处理器。
func WireNotification(infra *Infra) {
	if infra == nil {
		return
	}
	svc := notificationapp.New(
		notificationmysql.NewMessage(infra.DB),
		notificationmysql.NewGlobal(infra.DB),
		notificationmysql.NewConf(infra.DB),
		notificationmysql.NewUsers(infra.DB),
		notificationmysql.NewChatUnread(infra.DB),
		nil, // 用默认时钟
		infra.Logger,
	)
	notificationService = svc
	router.SetNotificationHandler(notificationhttp.NewHandler(svc))
}
