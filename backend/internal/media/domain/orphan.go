package domain

import (
	"context"
	"time"
)

// 孤儿图清单（issue #139）的判定口径与取数契约。
//
// 术语见 GLOSSARY：「引用账本」是 image_ref_models 的明细行；「孤儿图」是
// **判定结果**，不是图片状态的一个值。账本由 CDC 异步维护、有死信，
// 引用又靠从 URL 反解 object_key，所以账本可能漏记 —— 清单只把它当线索，
// 最终判定以重扫真实引用为准。

const (
	// DefaultOrphanGraceDays 是默认冷静期（天）。
	//
	// 14 天覆盖两件事：「本周传图、下周才发文」的更新节奏，以及 CDC 积压
	// 造成的账本延迟。太短会把"已经传了但还没用上"的图当成孤儿。
	DefaultOrphanGraceDays = 14

	// DefaultOrphanBatchLimit 是单次默认检查条数。
	DefaultOrphanBatchLimit = 50

	// MaxOrphanBatchLimit 是单次检查条数的硬上限。
	//
	// 配置是管理员可改的，但"一次扫爆数据库/存储"不该只靠人自觉：
	// 超过上限的值一律夹回来（见 OrphanScanConfig.Normalize）。
	MaxOrphanBatchLimit = 500
)

// OrphanScanConfig 是一次清单扫描的运行时配置。
//
// 由装配层以函数形式注入（`func() OrphanScanConfig`），每次扫描现读：
// 后台改完冷静期或上限，下一次扫描立即生效，不必重启进程。
type OrphanScanConfig struct {
	// GraceDays 是冷静期天数：上传时间不足这么多天的图不进入候选。
	GraceDays int
	// BatchLimit 是单次最多检查的图片条数。
	BatchLimit int
}

// Normalize 把缺失或越界的配置收敛成可用值。
//
// 冷静期 <= 0 一律回落默认值，而不是当成"没有冷静期"：这个值是误删的
// 最后一道防线，零值更可能是配置写漏了，不该被当成"关掉保护"。
func (c OrphanScanConfig) Normalize() OrphanScanConfig {
	if c.GraceDays <= 0 {
		c.GraceDays = DefaultOrphanGraceDays
	}
	if c.BatchLimit <= 0 {
		c.BatchLimit = DefaultOrphanBatchLimit
	}
	if c.BatchLimit > MaxOrphanBatchLimit {
		c.BatchLimit = MaxOrphanBatchLimit
	}
	return c
}

// GracePeriod 把冷静期换算成时长。
func (c OrphanScanConfig) GracePeriod() time.Duration {
	return time.Duration(c.GraceDays) * 24 * time.Hour
}

// CleanableStatuses 返回允许进入候选清单的状态。
//
// 只排除三种：审核中（尚未定性）、封禁（可能还要留作证据）、已删除。
// 未知状态也排除：状态字段正常只会是 pass / orphaned，出现别的值说明
// 数据有问题，此时宁可少列一张图，也不要把它推给不可逆的删除流程。
func CleanableStatuses() []Status {
	return []Status{StatusPass, StatusOrphaned}
}

// OrphanScanSpec 是本轮扫描的取数条件。
type OrphanScanSpec struct {
	// UploadedBefore 是冷静期截止时间：上传时间必须严格早于它。
	UploadedBefore time.Time
	// Statuses 是允许进入检查范围的状态。
	Statuses []Status
	// Limit 是本轮最多检查的条数。
	Limit int
}

// LedgerSnapshot 是一批图片的账本读数。
type LedgerSnapshot struct {
	// Refs 是账本引用行数（按图片）。
	Refs map[ImageID]int
	// Avatars 标出被引用为用户头像的图片。
	//
	// 单独标出来是因为头像要一律排除：它可能只在账本里记着、而重扫因为
	// CDN 域名变过匹配不上，光看真实引用会把它误判成孤儿。
	Avatars map[ImageID]bool
}

// OrphanCandidate 是一条候选清单（真实引用为 0）。
//
// 两个引用数并列展示：账本是 CDC 记的，真实引用是重扫业务表得到的，
// 站长要能一眼看出"这条候选是否被账本支持"，而不是盲信单一数据源。
type OrphanCandidate struct {
	Image
	// LedgerRefs 是账本引用行数。
	LedgerRefs int
	// LiveRefs 是重扫真实引用得到的引用数（候选恒为 0）。
	LiveRefs int
}

// OrphanMismatch 是账本不可信的证据：账本零引用，但真实引用非零。
//
// 它**不是候选**，只是供人工复核账本哪里漏记了（常见原因是 CDC 死信，
// 或历史 URL 的域名/前缀与当前存储配置不匹配）。
type OrphanMismatch struct {
	Image
	// LedgerRefs 是账本引用行数（本类条目恒为 0）。
	LedgerRefs int
	// LiveRefs 是重扫得到的真实引用数（> 0）。
	LiveRefs int
}

// OrphanScanResult 是一次清单扫描的结论。
type OrphanScanResult struct {
	// GraceDays 是本轮实际生效的冷静期（天）。
	GraceDays int
	// BatchLimit 是本轮实际生效的单次上限。
	BatchLimit int
	// Cutoff 是本轮实际使用的冷静期截止时间。
	Cutoff time.Time
	// Scanned 是本轮检查的图片数（含被排除的）。
	Scanned int
	// Excluded 是本轮检查过、但没进候选的图片数。
	Excluded int
	// AvatarExcluded 是 Excluded 里因"被用作头像"排除的。
	AvatarExcluded int
	// RefExcluded 是 Excluded 里因"仍有真实引用"排除的。
	RefExcluded int
	// Remaining 是超过本轮上限、留待下次继续的图片数。
	Remaining int
	// Candidates 是候选清单。
	Candidates []OrphanCandidate
	// Mismatches 是账本不一致条目（账本 0 条、真实引用非零）。
	Mismatches []OrphanMismatch
}

// OrphanStore 是孤儿图清单所需的只读数据源：图片表、引用账本、真实引用。
//
// 端口里**没有任何写方法**。只读不是运行时约定（"记得别写"），而是
// 接口层面就拿不到写能力 —— 这是 issue #139 最重要的约束。
type OrphanStore interface {
	// ListCandidates 取本轮要检查的图片：状态可清理、上传早于冷静期，
	// 按上传时间升序最多 spec.Limit 张；第二个返回值是符合条件的总数。
	ListCandidates(ctx context.Context, spec OrphanScanSpec) ([]Image, int64, error)
	// LedgerSnapshot 按图片反查账本引用行数，并标出被用作头像的图片。
	LedgerSnapshot(ctx context.Context, imageIDs []ImageID) (LedgerSnapshot, error)
	// LiveRefs 重扫四类业务表（文章正文/封面、轮播封面、收藏封面、用户头像），
	// 返回 object_key → 真实引用数。没有引用的 key 不出现。
	LiveRefs(ctx context.Context, objectKeys []string) (map[string]int, error)
}
