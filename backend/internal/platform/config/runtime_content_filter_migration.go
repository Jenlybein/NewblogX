package config

import (
	"strings"

	"myblogx/conf"
	"myblogx/conf/site"
)

// legacyMergedCategory 是旧词合并进来时统一归入的类目。
//
// 旧的两份词表只分"阻断 / 观察"，没有类目；合并时不能凭空推断（"博彩网站"到底是
// 赌博还是诈骗，判断依据不足）。统一落到 fake_docs 并由运营在词云里逐词改，
// 比让代码猜一个看起来更聪明但经常错的值要好。
const legacyMergedCategory = "fake_docs"

// mergeLegacyModerationWords 把 site.article.moderation / site.comment.moderation
// 的旧词合并进 content_filter 段（ADR-0032 决策 12）。
//
// 只在 content_filter 段**首次出现**时调用（调用点见 InitRuntimeConfig）。合并后：
//   - 旧词按原来的两档写入（block_words → block，watch_words → watch），去重；
//   - 段里已有的默认种子词保留（不进第二次）；
//   - 写库走 UpdateContentFilter，因此缓存同步刷新、下一次启动不再合并；
//   - 旧字段本票不动，等后续票删除（删除前它们仍可读，只是不再被消费）。
func (s *RuntimeConfigService) mergeLegacyModerationWords() error {
	current := s.GetRuntimeContentFilter()
	article := s.GetRuntimeSite().Article.Moderation
	comment := s.GetRuntimeSite().Comment.Moderation

	merged, added := mergeModerationLists(current, article, comment)
	if added == 0 {
		s.logf("内容过滤：content_filter 段首次出现，旧词表为空，未合并任何词条")
		return nil
	}
	if err := s.UpdateContentFilter(merged); err != nil {
		return err
	}
	s.logf(
		"内容过滤：content_filter 段首次出现，已合并旧词表 %d 条（文章 %d 条 / 评论 %d 条），当前共 %d 条",
		added, len(article.BlockWords)+len(article.WatchWords),
		len(comment.BlockWords)+len(comment.WatchWords), len(merged.Words),
	)
	return nil
}

// mergeModerationLists 纯函数：把旧的两档词表并进当前段，返回合并结果与新增条数。
//
// 去重口径是"归一化后的词"（去首尾空白 + 小写），因为引擎匹配前也会做归一化 ——
// 否则 "PayPal" 与 "paypal" 会各占一条，词云里出现两个看起来一样的词。
func mergeModerationLists(
	current conf.ContentFilter, lists ...site.Moderation,
) (conf.ContentFilter, int) {
	merged := conf.ContentFilter{
		Words:     make([]conf.ContentFilterWord, 0, len(current.Words)),
		Whitelist: append([]string(nil), current.Whitelist...),
	}
	seen := make(map[string]struct{}, len(current.Words))
	for _, entry := range current.Words {
		key := dedupeKey(entry.Word)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		merged.Words = append(merged.Words, entry)
	}

	added := 0
	appendWords := func(words []string, tier string) {
		for _, raw := range words {
			key := dedupeKey(raw)
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			merged.Words = append(merged.Words, conf.ContentFilterWord{
				Word:     strings.TrimSpace(raw),
				Tier:     tier,
				Category: legacyMergedCategory,
			})
			added++
		}
	}
	for _, list := range lists {
		appendWords(list.BlockWords, string(tierBlock))
		appendWords(list.WatchWords, string(tierWatch))
	}
	return merged, added
}

// tierBlock / tierWatch 与 contentfilter 包的档位取值一致（这里不引依赖，只对齐字面量）。
const (
	tierBlock = "block"
	tierWatch = "watch"
)

// dedupeKey 是合并时的去重键。
func dedupeKey(word string) string {
	return strings.ToLower(strings.TrimSpace(word))
}

// logf 写一条 info 日志（logger 由装配层注入，测试里可能为空）。
func (s *RuntimeConfigService) logf(format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Infof(format, args...)
}
