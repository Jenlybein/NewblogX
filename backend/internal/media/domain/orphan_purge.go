package domain

import (
	"context"
)

// 孤儿图真删（issue #140）的配置、结论与端口。
//
// 与 #139 的只读清单分开：清单只拿 OrphanStore（没有任何写方法）；
// 真删链路要写，但写能力放在单独的 OrphanPurgeStore 端口里。
// 而且它不自己实现"删对象、清账本、硬删行"，而是复用既有的删除能力
// （见 Service.Remove / ImageRemover）。

// OrphanPurgeConfig 是一次真删任务的运行时配置。
//
// 由装配层以函数形式注入，每次执行现读：停手开关、冷静期、上限都可以
// 在后台改完立刻生效，不必重启进程。
type OrphanPurgeConfig struct {
	// Enabled 是回收任务的总开关。
	//
	// 默认关闭：删除不可逆，"没配"和"配错了"都不该被解释成"可以删"。
	Enabled bool
	// GraceDays 是冷静期天数：上传不足这么多天的图不进入删除流程。
	GraceDays int
	// BatchLimit 是单次最多处理（以及清单每轮最多检查）的条数。
	BatchLimit int
}

// ScanConfig 把回收配置换算成清单取数配置。
//
// 冷静期与上限和清单共用同一套口径，避免"看得见却删不了 / 清单说能删"
// 两套参数互相打架；归一化规则（默认 14 天、上限 500）也只有一处。
func (c OrphanPurgeConfig) ScanConfig() OrphanScanConfig {
	return OrphanScanConfig{GraceDays: c.GraceDays, BatchLimit: c.BatchLimit}.Normalize()
}

// PurgeSkipReason 是一条候选被跳过的原因（写进日志与结果，便于事后核对）。
type PurgeSkipReason string

const (
	// PurgeSkipLiveRefs 二次确认时重扫到真实引用。
	PurgeSkipLiveRefs PurgeSkipReason = "live_refs"
	// PurgeSkipAvatar 图片被用作某个用户的头像。
	PurgeSkipAvatar PurgeSkipReason = "avatar"
	// PurgeSkipStatus 状态不在可清理白名单（审核中 / 封禁 / 未知）。
	PurgeSkipStatus PurgeSkipReason = "status"
	// PurgeSkipGrace 候选没到冷静期：调用方给的候选可能已经过时。
	PurgeSkipGrace PurgeSkipReason = "grace"
	// PurgeSkipObjectKey 图片行没有对象键：无法重扫真实引用，也就无法确认。
	PurgeSkipObjectKey PurgeSkipReason = "object_key"
	// PurgeSkipDeletedArticleRefs 账本里仍有指向"已删除文章"的引用行。
	//
	// 这类引用只统计、不自动删：文章可能被恢复，图不能先没了。
	PurgeSkipDeletedArticleRefs PurgeSkipReason = "deleted_article_refs"
	// PurgeSkipStorageDelete 存储对象删除失败：本地行必须保留。
	PurgeSkipStorageDelete PurgeSkipReason = "storage_delete"
	// PurgeSkipDelete 删除链路的其它失败（例如清账本/删行的事务失败）。
	PurgeSkipDelete PurgeSkipReason = "delete"
	// PurgeSkipAlreadyGone 条目已经不存在（重复执行时的正常结果，不告警）。
	PurgeSkipAlreadyGone PurgeSkipReason = "already_gone"
)

// OrphanPurgeSkip 是一条跳过记录。
type OrphanPurgeSkip struct {
	// Image 是被跳过的图片。
	Image Image
	// Reason 是跳过原因。
	Reason PurgeSkipReason
	// Detail 是原始错误摘要（只用于日志，不参与流程判断）。
	Detail string
}

// OrphanPurgeResult 是一次真删执行的结论。
//
// 字段刻意拆开"计划删除"与"删除成功"：dry-run 时前者非零、后者恒为 0，
// 运维不必猜"报了数到底写没写"。
type OrphanPurgeResult struct {
	// Enabled 是本轮实际生效的开关。
	Enabled bool
	// DryRun 标记本轮是否只确认不写。
	DryRun bool
	// GraceDays 是本轮实际生效的冷静期（天）。
	GraceDays int
	// BatchLimit 是本轮实际生效的条数上限。
	BatchLimit int
	// Selected 是本轮真正进入二次确认的候选数（受上限约束）。
	Selected int
	// WouldDelete 是通过二次确认、本可删除的条目数。
	WouldDelete int
	// Deleted 是真正删除成功的条目数（dry-run 恒为 0）。
	Deleted int
	// Skipped 是被跳过的条目数（原因见 Skips）。
	Skipped int
	// Remaining 是超过本轮上限、留待下次继续的候选数。
	Remaining int
	// ProtectedRefRows 是指向已删除文章的账本引用行数（只统计，不删除）。
	ProtectedRefRows int
	// Skips 是逐条跳过记录。
	Skips []OrphanPurgeSkip
}

// OrphanPurgeStore 是真删链路的写能力端口。
//
// 它是一个独立端口，而不是往 #139 的 OrphanStore 上加写方法：只读清单
// 那条链路上拿不到写能力，这不依赖"记得别写"的运行时自觉。
//
// 语义与既有的 Service.Remove 一致：先删存储对象、确认成功后才删本地
// 图片行，并在同一事务里清掉引用账本行。实现由 app.ImageRemover 提供。
type OrphanPurgeStore interface {
	Remove(ctx context.Context, ids []ImageID) (int, error)
}

// OrphanPurgeGuard 是二次确认需要的账本保护读数（只读）。
//
// 单独成端口而不是塞进 OrphanStore：它回答的是"这篇引用所属的文章是不是
// 已经进回收站了"，是删除决策的一部分，不属于清单口径。
type OrphanPurgeGuard interface {
	// DeletedArticleRefs 统计账本里指向已软删文章的引用行数（按图片）。
	//
	// 没有这种受保护引用的图片不出现在结果里。读取失败时调用方必须
	// 放弃本轮删除（不能把"查不到"当成"没有"）。
	DeletedArticleRefs(ctx context.Context, imageIDs []ImageID) (map[ImageID]int, error)
}
