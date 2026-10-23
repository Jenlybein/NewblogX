package cron_service

import (
	"context"
	"strconv"
	"time"

	"myblogx/conf"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type CronService struct {
	db        *gorm.DB
	redis     *redis.Client
	log       *logrus.Logger
	logConfig conf.Logrus
}

var (
	prepareSyncBucketScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 1 then
	return 1
end
if redis.call("EXISTS", KEYS[1]) == 0 then
	return 0
end
redis.call("RENAME", KEYS[1], KEYS[2])
return 1
`)
)

type hashCounterSyncConfig struct {
	taskName   string
	metricName string
	activeKey  string
	syncKey    string
	idName     string
	applyDelta func(id ctype.ID, delta int) error
}

func NewSchedulerRaw(db *gorm.DB, redisClient *redis.Client, logger *logrus.Logger, logCfg ...conf.Logrus) *CronService {
	s := &CronService{
		db:    db,
		redis: redisClient,
		log:   logger,
	}
	if len(logCfg) > 0 {
		s.logConfig = logCfg[0]
	}
	return s
}

func (s *CronService) syncCounters() {
	s.SyncArticle()
	s.SyncTag()
	s.SyncCommentReply()
	s.SyncCommentDigg()
}

func (s *CronService) runLockedSyncTask(taskName, lockKey string, lockTTL time.Duration, syncFunc func(ctx context.Context) (int, error)) {
	ctx := context.Background()

	unlock, err := redis_service.LockArticleSync(redis_service.Deps{Client: s.redis, Logger: s.log}, ctx, lockKey, lockTTL)
	if err != nil {
		if s.log != nil {
			s.log.Errorf("%s获取锁失败: 错误=%v", taskName, err)
		}
		return
	}
	if unlock == nil {
		if s.log != nil {
			s.log.Infof("%s跳过，本轮已有任务在执行", taskName)
		}
		return
	}
	defer unlock()

	affected, err := syncFunc(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Errorf("%s执行失败: 错误=%v", taskName, err)
		}
		return
	}
	if affected > 0 {
		if s.log != nil {
			s.log.Infof("%s执行成功: 影响数量=%d", taskName, affected)
		}
	}
}

func (s *CronService) syncHashCounterMetric(ctx context.Context, config hashCounterSyncConfig) (int, error) {
	logPrefix := config.taskName
	if config.metricName != "" {
		logPrefix = config.taskName + "同步" + config.metricName
	}

	ret, err := prepareSyncBucketScript.Run(ctx, s.redis, []string{config.activeKey, config.syncKey}).Int()
	if err != nil {
		return 0, err
	}
	if ret != 1 {
		return 0, nil
	}

	rawMap, err := s.redis.HGetAll(ctx, config.syncKey).Result()
	if err != nil {
		return 0, err
	}
	if len(rawMap) == 0 {
		if err := s.redis.Del(ctx, config.syncKey).Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}

	deltaMap := make(map[ctype.ID]int, len(rawMap))
	for idStr, deltaStr := range rawMap {
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			if s.log != nil {
				s.log.Warnf("%s忽略非法%s: Redis键=%s %s=%s", logPrefix, config.idName, config.activeKey, config.idName, idStr)
			}
			continue
		}

		delta, err := strconv.Atoi(deltaStr)
		if err != nil {
			if s.log != nil {
				s.log.Warnf("%s忽略非法增量: Redis键=%s %s=%s 增量=%s", logPrefix, config.activeKey, config.idName, idStr, deltaStr)
			}
			continue
		}

		if delta == 0 {
			continue
		}
		deltaMap[ctype.ID(id)] += delta
	}

	if len(deltaMap) == 0 {
		if err := s.redis.Del(ctx, config.syncKey).Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}

	for id, delta := range deltaMap {
		if err := config.applyDelta(id, delta); err != nil {
			if s.log != nil {
				s.log.Warnf("%s写库失败，准备回补缓存: Redis键=%s %s=%d 增量=%d 错误=%v", logPrefix, config.activeKey, config.idName, id, delta, err)
			}
			if requeueErr := s.redis.HIncrBy(ctx, config.activeKey, strconv.FormatUint(uint64(id), 10), int64(delta)).Err(); requeueErr != nil && s.log != nil {
				s.log.Errorf("%s回补缓存失败: Redis键=%s %s=%d 增量=%d 错误=%v", logPrefix, config.activeKey, config.idName, id, delta, requeueErr)
			}
		}
	}

	if err := s.redis.Del(ctx, config.syncKey).Err(); err != nil {
		return 0, err
	}
	return len(deltaMap), nil
}

func (s *CronService) Start() {
	timezone, _ := time.LoadLocation("Asia/Shanghai")
	scheduler, err := gocron.NewScheduler(
		gocron.WithLocation(timezone),
	)
	if err != nil {
		if s.log != nil {
			s.log.Errorf("创建 gocron 调度器失败: %v", err)
		}
	}

	_, err = scheduler.NewJob(
		gocron.DurationJob(30*time.Minute),
		gocron.NewTask(s.syncCounters),
	)
	if err != nil {
		if s.log != nil {
			s.log.Errorf("添加同步任务失败: %v", err)
		}
	}

	s.registerLogCleanupJob(scheduler)

	if s.log != nil {
		s.log.Infof("成功启动定时任务")
	}
	scheduler.Start()
}
