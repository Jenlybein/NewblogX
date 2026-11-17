package app

import (
	"context"

	"myblogx/internal/media/adapter/mysql"
	mediaqiniu "myblogx/internal/media/adapter/qiniu"
	"myblogx/internal/platform/imagedisplay"
)

// imageDisplay 是全局唯一的图片展示派生器（装配期构造一次）。
//
// 为什么是包级变量：它要被五个模块的 HTTP 处理器共用（文章、站点、用户、私信、
// 媒体），而每个模块的装配函数各自独立。与 notificationService 同一做法 ——
// 装配层是唯一的写入方，测试里可以显式覆盖。
var imageDisplay *imagedisplay.Resolver

// WireImageDisplay 构造图片展示派生器。
//
// 必须在 WireMedia / WireSite / WireArticle / WireUser / WireChat **之前**调用：
// 那些模块的处理器要在构造时拿到它（拿不到就是 nil，表现为"不派生、原样返回"）。
func WireImageDisplay(infra *Infra) *imagedisplay.Resolver {
	if infra == nil {
		return nil
	}
	storage := mediaqiniu.New(infra.RuntimeSite.GetRuntimeStorage)
	visibility := mysql.NewVisibilityStore(infra.DB)

	imageDisplay = imagedisplay.New(
		// 配置传函数：后台保存样式名后要立刻生效，不能要求重启。
		func() imagedisplay.Config {
			return imagedisplay.ConfigFrom(infra.RuntimeSite.GetRuntimeStorage())
		},
		imageDisplayPolicy{visibility: visibility, storage: storage},
	)
	return imageDisplay
}

// ImageDisplay 返回装配好的派生器（可能是 nil，调用方按"不派生"处理）。
func ImageDisplay() *imagedisplay.Resolver { return imageDisplay }

// imageDisplayPolicy 把"能不能看"与"怎么签名"两个能力接到具体适配器上。
//
// 拆成一个类型而不是两个函数字段：它只有两个方法，实现体一眼看得完，
// 也能在测试里整体替换。
type imageDisplayPolicy struct {
	visibility *mysql.VisibilityStore
	storage    *mediaqiniu.Storage
}

// Visible 批量判定这批地址当前是否可对外展示（只有审核通过才算）。
func (p imageDisplayPolicy) Visible(ctx context.Context, urls []string) (map[string]bool, error) {
	return p.visibility.VisibilityByURL(ctx, urls)
}

// Sign 对已拼好样式后缀的对象 key 签名（私有空间）。
func (p imageDisplayPolicy) Sign(keyWithStyle string) string {
	return p.storage.PrivateURL(keyWithStyle)
}
