package cli

import (
	"context"
	"fmt"
	"time"

	"myblogx/conf"
	mediamysql "myblogx/internal/media/adapter/mysql"
	mediaqiniu "myblogx/internal/media/adapter/qiniu"
	mediaapp "myblogx/internal/media/app"
	"myblogx/internal/media/domain"
	sitecfg "myblogx/internal/platform/config"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 本文件是"无人引用的候选图"（issue #139 清单 / issue #140 真删）。
//
// 两条路径共用同一个入口：
//   - 不带 -apply：只读清单，外加一次"二次确认预演"，绝不写任何数据；
//   - 带 -apply：重扫真实引用做二次确认后真删，受 image_gc.enabled 开关约束。
//
// 判定不信任账本：账本由 CDC 异步维护、有死信，引用又靠从 URL 反解
// object_key。清单会并列给出"账本引用数"与"重扫真实引用数"，并把
// "账本 0 条 / 真实 >0" 的不一致条目单独列出，供人工复核。
//
// 真删不自己实现删除顺序：走 Service.Remove 的同一段实现（先删存储对象、
// 确认成功才清账本并硬删行），任一步失败整条跳过。

// FlagImageGCOrphans 输出候选孤儿图清单，并按需执行真删。
//
// configFile 用来定位 site_default_settings.yaml：运行时配置段缺字段时
// 由那里的默认值补齐（与站点配置的合并语义一致）。
func FlagImageGCOrphans(db *gorm.DB, logger *logrus.Logger, configFile string, apply bool) error {
	if db == nil {
		return fmt.Errorf("孤儿图清单失败: 数据库未初始化")
	}

	ctx := context.Background()

	// 运行时配置现读：一次性命令每次启动都重新查库。
	// 改完冷静期/上限/回收开关（或换存储前缀）不需要重启任何常驻进程。
	defaults, defaultsErr := sitecfg.LoadDefaults(configFile)
	if defaultsErr != nil && logger != nil {
		logger.Warnf("读取站点默认配置失败，孤儿图清单将只用库里那份运行时配置: %v", defaultsErr)
	}

	// 真实引用靠 URL 反解 object_key，反解依赖当前存储前缀 ——
	// 所以这里读的是 storage 段，而不是命令行的某个开关。
	storageCfg, err := sitecfg.ReadSection(ctx, db, sitecfg.SectionStorage, defaults.Storage)
	if err != nil {
		return fmt.Errorf("读取对象存储配置失败: %w", err)
	}
	imageGCCfg, err := sitecfg.ReadSection(ctx, db, sitecfg.SectionImageGC, defaults.ImageGC)
	if err != nil {
		return fmt.Errorf("读取孤儿图回收配置失败: %w", err)
	}
	scanCfg := domain.OrphanScanConfig{
		GraceDays:  imageGCCfg.GraceDays,
		BatchLimit: imageGCCfg.BatchLimit,
	}.Normalize()
	purgeCfg := domain.OrphanPurgeConfig{
		Enabled:    imageGCCfg.Enabled,
		GraceDays:  imageGCCfg.GraceDays,
		BatchLimit: imageGCCfg.BatchLimit,
	}

	storage := mediaqiniu.New(func() conf.QiNiu { return storageCfg })
	store := mediamysql.NewOrphanStore(db, func() conf.QiNiu { return storageCfg })
	scanner := mediaapp.NewOrphanScanner(store, cliClock{}, logger, func() domain.OrphanScanConfig { return scanCfg })

	res, err := scanner.Scan(ctx)
	if err != nil {
		return fmt.Errorf("扫描候选孤儿图失败: %w", err)
	}
	printOrphanScan(res)

	// 真删前再确认一次：Purge 会重扫真实引用、重读账本头像与"已删除文章
	// 引用"，不通过就跳过。不带 -apply 时 dryRun=true，只确认不写。
	remover := mediaapp.NewImageRemover(mediamysql.New(db), storage)
	purger := mediaapp.NewOrphanPurger(store, store, remover, cliClock{}, logger, func() domain.OrphanPurgeConfig { return purgeCfg })
	purgeResult, err := purger.Purge(ctx, res.Candidates, !apply)
	if err != nil {
		return fmt.Errorf("孤儿图二次确认失败，本轮未删除任何条目: %w", err)
	}
	printOrphanPurge(purgeResult)
	return nil
}

// printOrphanScan 把清单打到标准输出（日志里的结论由用例层负责）。
func printOrphanScan(res domain.OrphanScanResult) {
	fmt.Println("── 孤儿图候选清单（只读）──")
	fmt.Printf("冷静期:       %d 天（只选中上传早于 %s 的图）\n", res.GraceDays, res.Cutoff.Format("2006-01-02 15:04:05"))
	fmt.Printf("单次上限:     %d 条（本轮检查 %d 条）\n", res.BatchLimit, res.Scanned)
	fmt.Printf("候选:         %d 条\n", len(res.Candidates))
	fmt.Printf("排除:         %d 条（头像 %d、仍被引用 %d）\n", res.Excluded, res.AvatarExcluded, res.RefExcluded)
	fmt.Printf("剩余:         %d 条\n\n", res.Remaining)

	if len(res.Candidates) > 0 {
		fmt.Println("候选（真实引用为 0，可进入二次确认）：")
		for _, candidate := range res.Candidates {
			fmt.Printf("  - id=%s 对象=%s 上传=%s 状态=%s 账本引用=%d 真实引用=%d\n",
				candidate.ID, candidate.ObjectKey,
				candidate.CreatedAt.Format("2006-01-02 15:04:05"),
				candidate.Status, candidate.LedgerRefs, candidate.LiveRefs,
			)
		}
	} else if res.Scanned > 0 {
		fmt.Println("本轮没有候选图。")
	}

	if len(res.Mismatches) > 0 {
		fmt.Println("\n账本不一致（账本 0 条 / 真实引用 >0，供人工复核）：")
		for _, mismatch := range res.Mismatches {
			fmt.Printf("  - id=%s 对象=%s 上传=%s 状态=%s 账本引用=%d 真实引用=%d\n",
				mismatch.ID, mismatch.ObjectKey,
				mismatch.CreatedAt.Format("2006-01-02 15:04:05"),
				mismatch.Status, mismatch.LedgerRefs, mismatch.LiveRefs,
			)
		}
	}

	if res.Remaining > 0 {
		fmt.Printf("\n还有剩余 %d 条未检查，下次继续（可调大 image_gc.batch_limit）。\n", res.Remaining)
	}
}

// printOrphanPurge 打印二次确认与真删的结论。
func printOrphanPurge(res domain.OrphanPurgeResult) {
	fmt.Println("\n── 真删执行（二次确认后）──")
	if !res.Enabled {
		fmt.Println("回收任务已关闭（image_gc.enabled=false）：本轮没有删除任何东西。")
		fmt.Println("改运行时配置里的这一行后，下一次执行立即生效，不必重启。")
		return
	}
	if res.DryRun {
		fmt.Println("模式:         预演（未带 -apply，只做二次确认，不写任何数据）")
	} else {
		fmt.Println("模式:         落库（-apply）")
	}
	fmt.Printf("二次确认:     %d 条\n", res.Selected)
	fmt.Printf("计划删除:     %d 条\n", res.WouldDelete)
	fmt.Printf("删除成功:     %d 条\n", res.Deleted)
	fmt.Printf("跳过:         %d 条\n", res.Skipped)
	if res.ProtectedRefRows > 0 {
		fmt.Printf("指向已删除文章的引用行: %d 条（只统计、不自动删：恢复文章后图不能先没了）\n", res.ProtectedRefRows)
	}
	if res.Remaining > 0 {
		fmt.Printf("还有剩余 %d 条未处理，下次继续（可调大 image_gc.batch_limit）。\n", res.Remaining)
	}
	if len(res.Skips) > 0 {
		fmt.Println("跳过明细（原因代码见 domain/orphan_purge.go）：")
		for _, skip := range res.Skips {
			fmt.Printf("  - id=%s 对象=%s 原因=%s", skip.Image.ID, skip.Image.ObjectKey, skip.Reason)
			if skip.Detail != "" {
				fmt.Printf(" err=%s", skip.Detail)
			}
			fmt.Println()
		}
	}
	if res.DryRun {
		fmt.Println("\n这是预演：图片行、引用账本、存储对象都没有改动。")
	}
}

// cliClock 是媒体用例需要的墙钟实现。
type cliClock struct{}

// Now 实现 domain.Clock。
func (cliClock) Now() time.Time { return time.Now() }
