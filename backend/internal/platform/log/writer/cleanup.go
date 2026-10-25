package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// 本文件负责"删除过期日志目录"。
//
// 之所以放在 writer 而不是调度层：日期目录的结构（哪些子目录、目录名格式）
// 是日志写入侧的私有约定，清理逻辑跟着目录结构走。调度层只负责"每天几点跑"。

const (
	// logDateDirLayout 是日期目录的命名格式，与写入侧创建目录时一致。
	logDateDirLayout = "2006-01-02"
	// minRetentionDays 是保留天数的下限：0 或负数会让"今天刚写的日志"也被删。
	minRetentionDays = 1
)

// 全部日志子目录。新增一类日志时要同步加进这里，否则那类日志永远不会被清理。
var logSubDirs = []string{
	RuntimeLogDirName,
	LoginEventLogDirName,
	ActionAuditLogDirName,
	CdcEventLogDirName,
	ReplayEventLogDirName,
}

// CleanupExpiredLogs 删除早于保留窗口的日志日期目录，返回删除的目录总数。
//
// 保留窗口按"含今天在内共 N 天"计算：retentionDays=7 时保留今天及往前 6 天。
// 单个子目录出错不中断其余子目录 —— 一个目录权限异常不该让整次清理放弃。
func CleanupExpiredLogs(logDir string, retentionDays int, logger *logrus.Logger) (int, error) {
	logRoot := ResolveLogDir(logDir)
	if strings.TrimSpace(logRoot) == "" {
		if logger != nil {
			logger.Warnf("日志清理跳过：日志目录为空")
		}
		return 0, nil
	}
	if retentionDays < minRetentionDays {
		retentionDays = minRetentionDays
	}

	keepFrom := dayStart(time.Now()).AddDate(0, 0, -(retentionDays - 1))
	totalRemoved := 0
	var firstErr error

	for _, sub := range logSubDirs {
		dir := filepath.Join(logRoot, sub)
		removed, err := removeExpiredDateDirs(dir, keepFrom)
		if err != nil {
			if logger != nil {
				logger.Errorf("日志清理失败: 目录=%s 错误=%v", dir, err)
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		totalRemoved += removed
	}

	if logger != nil {
		logger.Infof("日志清理完成: 保留起始日期=%s 删除目录数=%d",
			keepFrom.Format(logDateDirLayout), totalRemoved)
	}
	return totalRemoved, firstErr
}

// removeExpiredDateDirs 删除 parentDir 下早于 keepFrom 的日期目录。
//
// 目录名不是合法日期的（比如手建的临时目录）一律跳过：宁可留着也不能误删。
func removeExpiredDateDirs(parentDir string, keepFrom time.Time) (int, error) {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			// 这类日志还没写过，不算错误。
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirDate, parseErr := time.ParseInLocation(logDateDirLayout, strings.TrimSpace(entry.Name()), time.Local)
		if parseErr != nil {
			continue
		}
		if dayStart(dirDate).Before(keepFrom) {
			if err := os.RemoveAll(filepath.Join(parentDir, entry.Name())); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

// ParseDailyAt 解析 "HH:MM:SS" 形式的每日执行时间。
//
// 配置写错时返回 error 让调用方决定回退到默认值，而不是在这里静默兜底 ——
// 静默兜底会让人以为配置生效了。
func ParseDailyAt(raw string) (hour, minute, second int, err error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, 0, 0, fmt.Errorf("run_at 为空，格式必须是 HH:MM:SS")
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("run_at 格式错误，当前值=%q", value)
	}

	if hour, err = strconv.Atoi(parts[0]); err != nil || hour < 0 || hour > 23 {
		return 0, 0, 0, fmt.Errorf("run_at 小时非法，当前值=%q", value)
	}
	if minute, err = strconv.Atoi(parts[1]); err != nil || minute < 0 || minute > 59 {
		return 0, 0, 0, fmt.Errorf("run_at 分钟非法，当前值=%q", value)
	}
	if second, err = strconv.Atoi(parts[2]); err != nil || second < 0 || second > 59 {
		return 0, 0, 0, fmt.Errorf("run_at 秒非法，当前值=%q", value)
	}
	return hour, minute, second, nil
}

// dayStart 取当天零点。
func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
