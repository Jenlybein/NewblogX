package cron_service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"myblogx/service/log_service"

	"github.com/go-co-op/gocron/v2"
)

const (
	logDateDirLayout     = "2006-01-02"
	defaultCleanupRunAt  = "03:30:00"
	defaultRetentionDays = 7
	minLogRetentionDays  = 1
)

func (s *CronService) registerLogCleanupJob(scheduler gocron.Scheduler) {
	cfg := s.logConfig.Cleanup
	if !cfg.Enabled {
		if s.log != nil {
			s.log.Infof("日志清理任务未启用，跳过注册")
		}
		return
	}

	retentionDays := cfg.RetentionDays
	if retentionDays < minLogRetentionDays {
		retentionDays = defaultRetentionDays
	}

	hour, minute, second, err := parseDailyAt(cfg.RunAt)
	if err != nil {
		if s.log != nil {
			s.log.Errorf("日志清理任务时间配置非法，改用默认时间: run_at=%q 错误=%v", cfg.RunAt, err)
		}
		hour, minute, second, _ = parseDailyAt(defaultCleanupRunAt)
	}

	_, err = scheduler.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(uint(hour), uint(minute), uint(second)))),
		gocron.NewTask(func() {
			s.cleanupExpiredLogs(retentionDays)
		}),
	)
	if err != nil {
		if s.log != nil {
			s.log.Errorf("添加日志清理任务失败: %v", err)
		}
		return
	}

	if s.log != nil {
		s.log.Infof("已注册日志清理任务: 保留天数=%d 每日执行时间=%02d:%02d:%02d", retentionDays, hour, minute, second)
	}
}

func (s *CronService) cleanupExpiredLogs(retentionDays int) {
	logRoot := log_service.ResolveLogDir(s.logConfig.Dir)
	if strings.TrimSpace(logRoot) == "" {
		if s.log != nil {
			s.log.Warnf("日志清理任务跳过：日志目录为空")
		}
		return
	}

	now := time.Now()
	keepFrom := dayStart(now).AddDate(0, 0, -(retentionDays - 1))
	totalRemoved := 0

	logSubDirs := []string{
		log_service.RuntimeLogDirName,
		log_service.LoginEventLogDirName,
		log_service.ActionAuditLogDirName,
		log_service.CdcEventLogDirName,
		log_service.ReplayEventLogDirName,
	}

	for _, subDir := range logSubDirs {
		removed, err := removeExpiredDateDirs(filepath.Join(logRoot, subDir), keepFrom)
		if err != nil {
			if s.log != nil {
				s.log.Errorf("日志清理失败: 目录=%s 错误=%v", filepath.Join(logRoot, subDir), err)
			}
			continue
		}
		totalRemoved += removed
	}

	if s.log != nil {
		s.log.Infof("日志清理任务执行完成: 保留起始日期=%s 删除目录数=%d", keepFrom.Format(logDateDirLayout), totalRemoved)
	}
}

func removeExpiredDateDirs(parentDir string, keepFrom time.Time) (int, error) {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := strings.TrimSpace(entry.Name())
		dirDate, parseErr := time.ParseInLocation(logDateDirLayout, dirName, time.Local)
		if parseErr != nil {
			continue
		}

		if dayStart(dirDate).Before(keepFrom) {
			if err := os.RemoveAll(filepath.Join(parentDir, dirName)); err != nil {
				return removed, err
			}
			removed++
		}
	}

	return removed, nil
}

func parseDailyAt(raw string) (int, int, int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, 0, 0, fmt.Errorf("run_at 不能为空，格式必须为 HH:MM:SS")
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("run_at 格式错误，当前值=%q", value)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, 0, fmt.Errorf("run_at 小时非法，当前值=%q", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, 0, fmt.Errorf("run_at 分钟非法，当前值=%q", value)
	}
	second, err := strconv.Atoi(parts[2])
	if err != nil || second < 0 || second > 59 {
		return 0, 0, 0, fmt.Errorf("run_at 秒非法，当前值=%q", value)
	}
	return hour, minute, second, nil
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
