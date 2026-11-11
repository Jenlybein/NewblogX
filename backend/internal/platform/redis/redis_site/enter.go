package redis_site

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"myblogx/internal/platform/redis"
	"strconv"
	"strings"
	"time"
)

const (
	// siteFlowTotalKey 是累计访问量。
	//
	// 键名带 v2（票 349）：旧口径把每一次配置读取都算一次访问，既不去重也不过滤
	// 爬虫，累积出来的数字与"有多少人来过"没有关系。旧键不再读取 —— 这就是
	// "把旧累计清零"，且不需要一次破坏性的 DEL 就能让界面立刻回到真实口径。
	siteFlowTotalKey = "blog_site_flow:total:v2"
	// siteFlowDailyKey 是按天（Asia/Shanghai 自然日）的访问量。
	siteFlowDailyKey = "blog_site_flow:daily:v2"
	// siteFlowSeenKeyPrefix 是"这一天见过哪些访客"的去重集合。
	siteFlowSeenKeyPrefix = "blog_site_flow:seen:"
	maxDailyFlowDays      = 30
)

// DateCountItem 是"某天有多少次访问"。
type DateCountItem struct {
	Date  string
	Count int
}

// flowTimezone 是"自然日"的边界（票 349）。
//
// 固定东八区而不是服务器本地时区：站点面向中文读者，服务器跑在 UTC 时
// 用本地时区切天，用户在凌晨 0–8 点看到的"今天"其实是昨天。
var flowTimezone = mustLoadLocation("Asia/Shanghai")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// 没有 tzdata 时退回固定偏移：宁可差一次夏令时，也不能让统计整体崩掉。
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// crawlerUserAgents 是爬虫 / 监控的特征串（小写匹配）。
//
// 只挡明显的机器人：宁可漏掉几个小众爬虫，也不要把真人挡在外面 ——
// 少算一次访问只是数字偏小，把真人当爬虫会让"访客数"长期偏低且无从发现。
var crawlerUserAgents = []string{
	"bot", "spider", "crawler", "slurp", "curl/", "wget/", "python-requests",
	"headlesschrome", "lighthouse", "pingdom", "uptime", "monitor",
}

// IsCrawler 判断这次请求是不是爬虫（按 User-Agent）。
func IsCrawler(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	for _, marker := range crawlerUserAgents {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}

// SetFlow 记一次站点访问。
//
// 口径（票 349）：**同一 IP + 同一 User-Agent 在同一自然日内只算一次**，
// 且爬虫不计。去重集合按天存、两天后过期，所以内存占用与"一天的访客数"成正比。
func SetFlow(deps redis_service.Options, ip, userAgent string) {
	if deps.Client == nil {
		return
	}
	if IsCrawler(userAgent) {
		return
	}

	ctx := context.Background()
	now := time.Now().In(flowTimezone)
	today := now.Format("2006-01-02")

	// 访客指纹：IP + UA。没有 IP（例如内网直连）时用 UA 兜底，
	// 否则所有"没有 IP"的请求会被当成同一个访客、一天只算一次。
	visitor := ip + "|" + userAgent
	if strings.TrimSpace(ip) == "" {
		visitor = "noip|" + userAgent
	}
	sum := sha1.Sum([]byte(visitor))
	fingerprint := hex.EncodeToString(sum[:])

	seenKey := siteFlowSeenKeyPrefix + today
	added, err := deps.Client.SAdd(ctx, seenKey, fingerprint).Result()
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("站点访问去重失败: %v", err)
		}
		return
	}
	// 集合两天后过期：今天与"跨零点还在看的那个会话"都够用。
	deps.Client.Expire(ctx, seenKey, 48*time.Hour)

	// 这个访客今天已经算过了：不再累加。
	if added == 0 {
		return
	}

	pipe := deps.Client.TxPipeline()
	pipe.IncrBy(ctx, siteFlowTotalKey, 1)
	pipe.HIncrBy(ctx, siteFlowDailyKey, today, 1)

	if _, err := pipe.Exec(ctx); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("站点流量写入失败: %v", err)
		}
		return
	}
	pruneExpiredDailyFlow(deps, now)
}

// GetFlow 读取累计访问量。
func GetFlow(deps redis_service.Options) int {
	if deps.Client == nil {
		return 0
	}
	num, _ := deps.Client.Get(context.Background(), siteFlowTotalKey).Int()
	return num
}

// GetRecentFlow 获取最近 days 天的站点访问量（按东八区自然日）。
func GetRecentFlow(deps redis_service.Options, days int) []DateCountItem {
	if deps.Client == nil || days <= 0 {
		return nil
	}

	now := time.Now().In(flowTimezone)
	pruneExpiredDailyFlow(deps, now)

	values, err := deps.Client.HGetAll(context.Background(), siteFlowDailyKey).Result()
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("读取站点流量失败: %v", err)
		}
		return nil
	}

	dateCountList := make([]DateCountItem, 0, days)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, flowTimezone).AddDate(0, 0, -(days - 1))
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		count, _ := strconv.Atoi(values[date])
		dateCountList = append(dateCountList, DateCountItem{
			Date:  date,
			Count: count,
		})
	}
	return dateCountList
}

// pruneExpiredDailyFlow 清理过期的站点流量
func pruneExpiredDailyFlow(deps redis_service.Options, now time.Time) {
	if deps.Client == nil {
		return
	}

	keys, err := deps.Client.HKeys(context.Background(), siteFlowDailyKey).Result()
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("读取站点流量日期失败: %v", err)
		}
		return
	}

	expiredDates := buildExpiredDates(now, keys)
	if len(expiredDates) == 0 {
		return
	}

	fields := make([]string, 0, len(expiredDates))
	for _, date := range expiredDates {
		fields = append(fields, date)
	}
	if err := deps.Client.HDel(context.Background(), siteFlowDailyKey, fields...).Err(); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("清理过期站点流量失败: %v", err)
		}
	}
}

func buildExpiredDates(now time.Time, keys []string) []string {
	expiredDates := make([]string, 0)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(maxDailyFlowDays - 1))
	for _, key := range keys {
		date, err := time.ParseInLocation("2006-01-02", key, now.Location())
		if err != nil {
			expiredDates = append(expiredDates, key)
			continue
		}
		if date.Before(cutoff) {
			expiredDates = append(expiredDates, key)
		}
	}
	return expiredDates
}
