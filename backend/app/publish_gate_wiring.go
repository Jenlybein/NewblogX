package app

import (
	"context"

	articleapp "myblogx/internal/article/app"
	chatroot "myblogx/internal/chat"
	chatdomain "myblogx/internal/chat/domain"
	commentdomain "myblogx/internal/comment/domain"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	userapp "myblogx/internal/user/app"
	userdomain "myblogx/internal/user/domain"
)

// 发布闸门的装配（票 399 / ADR-0032 决策 8）。
//
// 禁言的生效范围只有发布类动作，且判定口径只有一份（user 模块的
// EnsureCanPublish → domain.CanPublish）。各内容模块因此都只依赖一个窄端口：
// "问 user 模块这个人现在能不能发布"，而不是自己去读 user_models.mute_until。
//
// 为什么不在各模块的 adapter 里直接读那一列：一是会复制"now < mute_until"的
// 判定（四处实现迟早漂移），二是错误码与到期时间文案必须只有一处来源。
//
// 为什么必须最后装配：闸门要接到 WireArticle / WireComment / WireChat 构造出来
// 的**同一份**服务实例（与 WireFilterDisposer 同一条理由：不能出现"写文章用的
// 服务与闸门检查的那份不是同一个装配"）。

// WirePublishGate 把 user 模块的发布闸门接到各内容模块。
func WirePublishGate(infra *Infra, chat *chatroot.Module) {
	if infra == nil {
		return
	}
	gate := publishGateBridge{users: infra.UserService}
	if infra.ArticleService != nil {
		infra.ArticleService.WithPublishGate(gate)
	}
	if infra.CommentService != nil {
		infra.CommentService.WithPublishGate(gate)
	}
	if chat != nil && chat.Service != nil {
		chat.Service.WithPublishGate(gate)
	}
}

// publishGateBridge 是三个模块 PublishGate 端口的共同实现。
//
// 三个端口的方法签名相同（userID 都是 ctype.ID 的别名），因此一个实现就能同时
// 满足三方；分成三个类型只会多出三份一模一样的转发代码。
type publishGateBridge struct {
	users *userapp.Service
}

var (
	_ articleapp.PublishGate    = publishGateBridge{}
	_ commentdomain.PublishGate = publishGateBridge{}
	_ chatdomain.PublishGate    = publishGateBridge{}
)

// EnsureCanPublish 把一次"能不能发布"的问询转给 user 模块。
func (b publishGateBridge) EnsureCanPublish(ctx context.Context, userID ctype.ID) error {
	if b.users == nil {
		// 装配缺失时失败关闭：静默放行等于禁言失效，而且这种缺失没有任何
		// 可观测信号（不会报错、不会 panic），只能靠这条兜底。
		return apperr.New(apperr.CodeCommonServiceFailed)
	}
	return b.users.EnsureCanPublish(ctx, userdomain.UserID(userID))
}
