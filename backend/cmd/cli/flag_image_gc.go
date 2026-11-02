package cli

import (
	"context"
	"fmt"
	"time"

	"myblogx/conf"
	mediamysql "myblogx/internal/media/adapter/mysql"
	mediaapp "myblogx/internal/media/app"
	"myblogx/internal/media/domain"
	sitecfg "myblogx/internal/platform/config"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 本文件是"无人引用的候选图清单"（issue #139）。
//
// 全程**只读**：不带任何开关也不会改数据，因为清单用例只拿到一个没有写方法的
// 读端口。删除能力（issue #140）会在同一入口上加显式开关；在此之前 -apply
// 不启用，只会打印一行提醒。
//
// 判定不信任账本：账本由 CDC 异步维护、有死信，引用又靠从 URL 反解
// object_key。清单会并列给出"账本引用数"与"重扫真实引用数"，并把
// "账本 0 条 / 真实 >0" 的不一致条目单独列出，供人工复核。

// FlagImageGCOrphans 输出候选孤儿图清单。
//
// configFile 用来定位 site_default_settings.yaml：运行时配置段缺字段时
// 由那里的默认值补齐（与站点配置的合并语义一致）。
func FlagImageGCOrphans(db *gorm.DB, logger *logrus.Logger, configFile string, apply bool) error {
	if db == nil {
		return fmt.Errorf("孤儿图清单失败: 数据库未初始化")
	}

	ctx := context.Background()

	// 运行时配置现读：一次性命令每次启动都重新查库。
	// 改完冷静期/上限（或换存储前缀）不需要重启任何常驻进程。
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
		return fmt.Errorf("读取孤儿图扫描配置失败: %w", err)
	}
	scanCfg := domain.OrphanScanConfig{
		GraceDays:  imageGCCfg.GraceDays,
		BatchLimit: imageGCCfg.BatchLimit,
	}.Normalize()

	store := mediamysql.NewOrphanStore(db, func() conf.QiNiu { return storageCfg })
	scanner := mediaapp.NewOrphanScanner(store, cliClock{}, logger, func() domain.OrphanScanConfig { return scanCfg })

	res, err := scanner.Scan(ctx)
	if err != nil {
		return fmt.Errorf("扫描候选孤儿图失败: %w", err)
	}

	printOrphanScan(res, apply)
	return nil
}

// printOrphanScan 把清单打到标准输出（日志里的结论由用例层负责）。
func printOrphanScan(res domain.OrphanScanResult, apply bool) {
	fmt.Println("── 孤儿图候选清单（只读）──")
	fmt.Printf("冷静期:       %d 天（只选中上传早于 %s 的图）\n", res.GraceDays, res.Cutoff.Format("2006-01-02 15:04:05"))
	fmt.Printf("单次上限:     %d 条（本轮检查 %d 条）\n", res.BatchLimit, res.Scanned)
	fmt.Printf("候选:         %d 条\n", len(res.Candidates))
	fmt.Printf("排除:         %d 条（头像 %d、仍被引用 %d）\n", res.Excluded, res.AvatarExcluded, res.RefExcluded)
	fmt.Printf("剩余:         %d 条\n\n", res.Remaining)

	if len(res.Candidates) > 0 {
		fmt.Println("候选（真实引用为 0，可进入后续删除流程）：")
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

	fmt.Println("\n这是只读清单：图片行、引用账本、存储对象都没有改动。")
	if apply {
		fmt.Println("注意：本版本只提供只读清单，-apply 尚未启用（删除能力见后续票）。")
	}
}

// cliClock 是媒体用例需要的墙钟实现。
type cliClock struct{}

// Now 实现 domain.Clock。
func (cliClock) Now() time.Time { return time.Now() }
