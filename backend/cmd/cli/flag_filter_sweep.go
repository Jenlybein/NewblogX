package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	commentmysql "myblogx/internal/comment/adapter/mysql"
	filtermysql "myblogx/internal/filter/adapter/mysql"
	filterapp "myblogx/internal/filter/app"
	filterdomain "myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/config"
	"myblogx/internal/platform/contentfilter"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 本文件是"存量内容清洗"（#402）：扫描六个文本面的历史数据，把命中词按 rune 等长
// 替换成 `*`，并按需补写「关键词风险」记录；默认 dry-run，只报告不改任何数据。
//
// 三条设计：
//   - **默认 dry-run**：与 -filter-import、-t reconcile、-t imagegc 同一口径。
//     存量清洗会改写正文且**误报不可恢复**（ADR-0032 决策 5），没有预演就等于
//     拿线上数据练手。
//   - **幂等，判定依据是"这次替换是否真的改变了文本"**：已经是星号版的文本不会
//     再匹配到命中词，所以第二次执行的"命中"是 0、一条记录也不会多写。没有选
//     "按内容 ID + 命中词去重"那条路 —— 它要先把 history 里的 hits_json 全部读出来
//     再逐条比对，而"文本变没变"本来就是过滤引擎已经算好的答案。
//     代价：**白名单命中的历史文本不会被补写记录**（这类命中不会替换文本，按
//     "文本没变就不记"的口径自然跳过）。这是刻意的取舍，见 ADR-0032 决策 9。
//   - **词表必须已导入**：内容过滤的旧词合并迁移已随票 401 删除（词表机审退场，
//     旧的两段配置结构不存在了）。所以生产上如果从未部署过第 11 段，壳里没有词
//     可匹配 —— 那种情况下这条命令会直接报错，而不是打印一份"0 命中、看起来很干净"
//     的报告让人误以为已经洗过。
//
// 与 ES 的关系：替换后的文本才是 MySQL 里的唯一版本，但 ES 里仍留着旧文档。
// 所以命令结束时**必须**重建索引，见 filterSweepESHint。带 -filter-sweep-reindex
// 时本命令会顺带调用现有 ES 维护命令（-es -s init + -es -s article-sync）完成它，
// 不另写一份重建逻辑。

// sweepBatchSize 是一次扫描装载的行数。
//
// 分批不是为了省内存（存量表通常很小），而是为了让"扫全表但不把 longtext 正文一次
// 装进内存"成为默认行为：正文列是长文本，一次性加载一张大表会真的把进程撑爆。
const sweepBatchSize = 200

// legacyCommentExamining 是评论「审核中」的保留值（票 400 退役后只剩存量行）。
//
// 只作存量判定条件，**没有任何写入路径会再产生它**；真正的转换动作复用评论模块
// 自己的 RetireExamining（见 sweepContent），这里不复制那条 UPDATE。
const legacyCommentExamining int8 = 1

// sweepRecorder 是"补写一条命中记录"的能力。
//
// 生产实现是 *filterapp.Service（方法 Record），测试里用内存实现断言写了什么。
// 抽成接口是因为 dry-run 必须**一次都不写**：这个能力为 nil 就是预演。
type sweepRecorder interface {
	Record(ctx context.Context, hit filterdomain.Hit)
}

// contentSweeper 执行六个文本面的扫描。
type contentSweeper struct {
	db     *gorm.DB
	engine *contentfilter.Engine
	// recorder 为 nil 表示 dry-run（只统计、不补写记录）。
	recorder  sweepRecorder
	apply     bool
	batchSize int
	now       func() time.Time
	// categorySpans 按类目累计替换处数（误伤评估口径）。
	//
	// 为什么要它：词表一变大，"命中 176 处"这个总数无法判断该不该 apply ——
	// 176 处来自涉政宽泛词和来自毒品垃圾词是完全不同的两件事。这里只统计
	// **类目与处数**，不记录任何词条（词表是敏感数据，报告会进运维记录）。
	categorySpans map[contentfilter.Category]int
}

// sweepSurfaceStats 是一个文本面的统计。
//
// 口径（也是报告里的三个数字）：
//   - Scanned：参与扫描的实体条数（按行算；昵称面含用户资料与评论里的昵称快照）；
//   - Hit：**需要替换**的实体条数 —— 一条内容里多个字段命中只算一条，白名单豁免
//     的不算（它本来就不改文本）；
//   - Spans：等长星号替换的处数（与「已过滤 N 处」同口径）。
type sweepSurfaceStats struct {
	Surface filterdomain.Surface
	Scanned int
	Hit     int
	Spans   int
}

// newSweepSurfaceStats 构造一个面的统计。
func newSweepSurfaceStats(surface filterdomain.Surface) sweepSurfaceStats {
	return sweepSurfaceStats{Surface: surface}
}

// filterSweepReport 是一次存量清洗的报告。
type filterSweepReport struct {
	// Apply 表示这次是否真的写库。
	Apply bool
	// Words 是本次匹配使用的词条数。
	Words int
	// Surfaces 按固定顺序（article → comment → nickname → abstract → chat →
	// collection）保存六个面的统计，**永远六项**（没有命中也打印一行 0）——
	// 固定形状让报告可断言，也让"某个面没被扫到"直接暴露成缺一行。
	Surfaces []sweepSurfaceStats
	// PendingComments 是存量「审核中」评论的条数（票 400 退役前的遗留行）。
	PendingComments int64
	// RetiredComments 是本次真正转成已删除的条数（dry-run 恒为 0）。
	RetiredComments int64
	// CategorySpans 是**按类目**的替换处数（误伤评估用；只含类目名与数字）。
	CategorySpans map[contentfilter.Category]int
}

// String 渲染报告。
//
// 格式是**稳定契约**：面名、数字位置固定，测试直接断言整段文本。加一个新面要同时
// 改这里与 sweepContent 的扫描列表，否则缺一行会立刻被测试发现。
func (r filterSweepReport) String() string {
	var b strings.Builder
	mode := "预演，未写库"
	if r.Apply {
		mode = "已写库"
	}
	fmt.Fprintf(&b, "── 存量内容清洗（%s）──\n", mode)
	fmt.Fprintf(&b, "词条:         %d 条\n", r.Words)
	fmt.Fprintf(&b, "命中口径:     需要替换的实体条数（白名单豁免的不算）\n")

	total := sweepSurfaceStats{}
	for _, stats := range r.Surfaces {
		fmt.Fprintf(&b, "%-12s 扫描 %d 条 / 命中 %d 条 / 替换 %d 处\n",
			stats.Surface, stats.Scanned, stats.Hit, stats.Spans)
		total.Scanned += stats.Scanned
		total.Hit += stats.Hit
		total.Spans += stats.Spans
	}
	fmt.Fprintf(&b, "%-12s 扫描 %d 条 / 命中 %d 条 / 替换 %d 处\n",
		"合计", total.Scanned, total.Hit, total.Spans)

	if r.Apply {
		fmt.Fprintf(&b, "评论「审核中」存量: %d 条 → 已转为已删除 %d 条\n",
			r.PendingComments, r.RetiredComments)
	} else {
		fmt.Fprintf(&b, "评论「审核中」存量: %d 条（apply 时按已删除处理，不计入上面的命中）\n",
			r.PendingComments)
	}
	return b.String()
}

// filterSweepCategoryLine 渲染"按类目的替换处数"，按处数降序。
//
// 只输出类目名与数字：这条报告会被人贴进工单与运维记录，词条不能出现。
// 没有命中时返回空串（不打印一行 0，避免报告变长）。
func filterSweepCategoryLine(spans map[contentfilter.Category]int) string {
	if len(spans) == 0 {
		return ""
	}
	type row struct {
		category contentfilter.Category
		spans    int
	}
	rows := make([]row, 0, len(spans))
	for category, count := range spans {
		if count <= 0 {
			continue
		}
		rows = append(rows, row{category: category, spans: count})
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].spans != rows[j].spans {
			return rows[i].spans > rows[j].spans
		}
		return rows[i].category < rows[j].category
	})

	var b strings.Builder
	b.WriteString("类目分布:     ")
	for i, item := range rows {
		if i > 0 {
			b.WriteString(" / ")
		}
		fmt.Fprintf(&b, "%s %d 处", item.category, item.spans)
	}
	b.WriteString("\n")
	return b.String()
}

// filterSweepESHint 是"清洗之后要重建 ES"的显式指引。
//
// 独立成函数而不是直接写在 FlagFilterSweep 里：这段文案是运维顺序的一部分，
// 测试要能断言它存在（"清洗完了没重建索引"是这次最容易漏掉的一步）。
func filterSweepESHint(apply bool) string {
	var b strings.Builder
	b.WriteString("\n下一步：用现有 ES 维护命令重建索引，替换后的文本才会进搜索（旧文档里仍是旧文本）。\n")
	if apply {
		b.WriteString("  ./server -f config/settings.yaml -filter-sweep -filter-sweep-apply -filter-sweep-reindex\n")
		b.WriteString("或者分两步：\n")
	}
	b.WriteString("  ./server -f config/settings.yaml -es -s init          # 先删后建索引与流水线\n")
	b.WriteString("  ./server -f config/settings.yaml -es -s article-sync  # 从 MySQL 全量回灌\n")
	return b.String()
}

// FlagFilterSweep 执行存量清洗，按 apply 决定是否写库。
//
// configFile 用来定位 site_default_settings.yaml：词表段缺字段时由默认值补齐
// （与 -filter-import 同一个理由与同一套运行时配置读取方式）。
func FlagFilterSweep(deps Options, configFile string, apply, reindex bool) error {
	if deps.DB == nil {
		return fmt.Errorf("存量清洗失败: 数据库未初始化")
	}
	if reindex && !apply {
		// 预演什么都不改，重建索引没有意义，反而会让人以为"清洗过了"。
		return fmt.Errorf("存量清洗失败: -filter-sweep-reindex 需要同时带 -filter-sweep-apply")
	}
	if reindex && deps.ESClient == nil {
		return fmt.Errorf("存量清洗失败: ES 未启用，无法在清洗后重建索引（去掉 -filter-sweep-reindex，改用 -es 子命令）")
	}

	defaults, defaultsErr := config.LoadDefaults(configFile)
	if defaultsErr != nil && deps.Logger != nil {
		deps.Logger.Warnf("读取站点默认配置失败，存量清洗将只用库里那份运行时配置: %v", defaultsErr)
	}
	svc := config.NewRuntimeConfigService(defaults, deps.Logger, deps.DB)
	if err := svc.InitRuntimeConfig(); err != nil {
		return fmt.Errorf("存量清洗失败: 读取运行时配置: %w", err)
	}

	contentFilter := svc.GetRuntimeContentFilter()
	if len(contentFilter.Words) == 0 {
		// 旧词合并迁移已随票 401 删除：生产上如果第 11 段从未部署过，这里就是空表。
		// 继续跑会得到一份"0 命中"的漂亮报告，而数据其实没洗过 —— 宁可报错。
		return fmt.Errorf(
			"存量清洗失败: content_filter 词表为空，没有可匹配的词。" +
				"旧的两份词表不会自动迁移，请先跑 -filter-import 导入词表，再执行清洗",
		)
	}
	engine, err := contentfilter.NewEngine(toFilterWords(contentFilter.Words), contentFilter.Whitelist)
	if err != nil {
		return fmt.Errorf("存量清洗失败: 词表不合法，无法构建过滤引擎: %w", err)
	}

	var recorder sweepRecorder
	if apply {
		recorder = filterapp.New(engine, filtermysql.New(deps.DB), deps.Logger)
	}

	report, err := sweepContent(context.Background(), deps.DB, engine, recorder, apply)
	if err != nil {
		return fmt.Errorf("存量清洗失败: %w", err)
	}

	fmt.Print(report.String())
	if line := filterSweepCategoryLine(report.CategorySpans); line != "" {
		fmt.Print(line)
	}
	if deps.Logger != nil {
		// 只记数量与面分布，不记命中词与上下文（ADR-0032 安全约束）。
		var hit, spans int
		for _, stats := range report.Surfaces {
			hit += stats.Hit
			spans += stats.Spans
		}
		deps.Logger.Infof("存量清洗完成: apply=%t hit=%d spans=%d pendingComments=%d retiredComments=%d",
			apply, hit, spans, report.PendingComments, report.RetiredComments)
	}

	if reindex {
		fmt.Println("\n清洗完成，开始重建 ES 索引…")
		if err := FlagESIndex(deps); err != nil {
			return fmt.Errorf("存量清洗后重建索引失败: %w", err)
		}
		if err := FlagESArticleSync(deps); err != nil {
			return fmt.Errorf("存量清洗后回灌 ES 失败: %w", err)
		}
		fmt.Println("\nES 索引已重建：旧文本不会再出现在搜索结果里。")
		return nil
	}

	fmt.Print(filterSweepESHint(apply))
	if !apply {
		fmt.Println("\n这是预演：文本与命中记录都没有改动。带 -filter-sweep-apply 才会写库。")
	}
	return nil
}

// sweepContent 扫描六个面并（按 apply）写库，返回报告。
//
// 与命令入口解耦：测试直接喂 sqlite 与测试词表，就能端到端断言"预演不写、apply
// 幂等、六个面都被扫到"。
func sweepContent(
	ctx context.Context, db *gorm.DB, engine *contentfilter.Engine,
	recorder sweepRecorder, apply bool,
) (filterSweepReport, error) {
	if db == nil {
		return filterSweepReport{}, fmt.Errorf("数据库未初始化")
	}
	if engine == nil {
		return filterSweepReport{}, fmt.Errorf("过滤引擎未初始化")
	}
	if apply && recorder == nil {
		// 写库却没有记录入口 = 提示语说要写、实际一条也写不进去。宁可当场报错。
		return filterSweepReport{}, fmt.Errorf("apply 模式缺少命中记录入口")
	}

	sweeper := &contentSweeper{
		db:        db,
		engine:    engine,
		recorder:  recorder,
		apply:     apply,
		batchSize: sweepBatchSize,
		now:       time.Now,
	}
	report := filterSweepReport{Apply: apply, Words: engine.WordCount()}

	// 顺序即报告顺序：文章 → 评论 → 昵称 → 简介 → 私信 → 合集。
	scans := []func(context.Context) (sweepSurfaceStats, error){
		sweeper.sweepArticles,
		sweeper.sweepComments,
		sweeper.sweepNicknames,
		sweeper.sweepAbstracts,
		sweeper.sweepChats,
		sweeper.sweepCollections,
	}
	report.Surfaces = make([]sweepSurfaceStats, 0, len(scans))
	for _, scan := range scans {
		stats, err := scan(ctx)
		if err != nil {
			return report, err
		}
		report.Surfaces = append(report.Surfaces, stats)
	}

	// 事故遗留的三条「审核中」评论（票 400 的退役迁移是它们**唯一**的正确去处）。
	// 这里复用同一条仓储入口：幂等（只命中 status=1）、0 行时不发 UPDATE。
	pending, err := countLegacyExaminingComments(ctx, db)
	if err != nil {
		return report, err
	}
	report.PendingComments = pending
	if apply && pending > 0 {
		retired, err := commentmysql.New(db).RetireExamining(ctx)
		if err != nil {
			return report, fmt.Errorf("存量「审核中」评论转已删除失败: %w", err)
		}
		report.RetiredComments = retired
	}
	report.CategorySpans = sweeper.categorySpans
	return report, nil
}

// countLegacyExaminingComments 数存量「审核中」评论（dry-run 也要能看到它们）。
func countLegacyExaminingComments(ctx context.Context, db *gorm.DB) (int64, error) {
	var pending int64
	err := db.WithContext(ctx).Model(&commentSweepRow{}).
		Where("status = ?", legacyCommentExamining).
		Count(&pending).Error
	if err != nil {
		return 0, fmt.Errorf("统计存量「审核中」评论失败: %w", err)
	}
	return pending, nil
}

// scan 过滤一段文本：返回替换后的文本、命中明细与替换处数。只算不写。
func (s *contentSweeper) scan(text string) (string, []contentfilter.Hit, int) {
	result := s.engine.Filter(text)
	if len(result.Hits) > 0 {
		if s.categorySpans == nil {
			s.categorySpans = make(map[contentfilter.Category]int, len(contentfilter.AllCategories))
		}
		for _, hit := range result.Hits {
			s.categorySpans[hit.Category] += len(hit.Snippets)
		}
	}
	return result.Masked, result.Hits, result.MaskedSpans
}

// recordHit 在 apply 模式补写一条命中记录。
//
// 只在**替换真的改变了文本**之后调用（见各面扫描循环），所以重复执行不会写出
// 重复记录。写失败由 filterapp.Service.Record 自己吞掉并记日志，不阻断这里 ——
// 清洗命令的价值在"文本变干净"，审计记录写不进去是次要问题。
func (s *contentSweeper) recordHit(
	ctx context.Context, surface filterdomain.Surface, userID, contentID ctype.ID,
	hits []contentfilter.Hit,
) {
	if !s.apply || s.recorder == nil || len(hits) == 0 {
		return
	}
	s.recorder.Record(ctx, filterdomain.Hit{
		CreatedAt: s.now(),
		Surface:   surface,
		ContentID: contentID,
		UserID:    userID,
		Words:     sweepWords(hits),
		HitCount:  sweepHitCount(hits),
		Status:    filterdomain.StatusPending,
	})
}

// sweepWords 把引擎的命中明细转成领域形状。
//
// 与 filterapp 内部的同名转换同构：那边不导出是因为它只服务在线路径；存量清洗
// 按 NOTES 的约定走 Record 这个纯写入口，自己负责形状转换。
func sweepWords(hits []contentfilter.Hit) []filterdomain.Word {
	words := make([]filterdomain.Word, 0, len(hits))
	for _, hit := range hits {
		snippets := make([]filterdomain.Snippet, 0, len(hit.Snippets))
		for _, snippet := range hit.Snippets {
			snippets = append(snippets, filterdomain.Snippet{
				Start: snippet.Start,
				End:   snippet.End,
				Text:  snippet.Text,
			})
		}
		words = append(words, filterdomain.Word{
			Word:     hit.Word,
			Tier:     hit.Tier,
			Category: hit.Category,
			Snippets: snippets,
		})
	}
	return words
}

// sweepHitCount 统计命中处数：同一个词出现多次时每条上下文都算一处（与在线路径
// 的 HitCount 口径一致，白名单命中在这里已经含在内了）。
func sweepHitCount(hits []contentfilter.Hit) int {
	total := 0
	for _, hit := range hits {
		total += len(hit.Snippets)
	}
	return total
}

// ── 六个面的扫描 ──────────────────────────────────────────────────
//
// 每个面都是一个形状：分批读 → 逐字段过滤 → 命中的字段先写库再补记录 → 记统计。
// 文章与合集是多字段面（一条内容命中多个字段只算一条"命中条数"）；昵称面多一张
// "评论里的昵称快照"表，原因见 sweepNicknames。

// articleSweepRow 是文章面的扫描投影。
type articleSweepRow struct {
	ID        ctype.ID       `gorm:"primaryKey;column:id"`
	Title     string         `gorm:"column:title"`
	Abstract  string         `gorm:"column:abstract"`
	Content   string         `gorm:"column:content"`
	AuthorID  ctype.ID       `gorm:"column:author_id"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

// TableName 固定表名。
func (articleSweepRow) TableName() string { return "article_models" }

// sweepArticles 扫描文章标题 / 摘要 / 正文（正文是 Markdown 原文列）。
func (s *contentSweeper) sweepArticles(ctx context.Context) (sweepSurfaceStats, error) {
	const surface = filterdomain.SurfaceArticle
	stats := newSweepSurfaceStats(surface)

	var batch []articleSweepRow
	err := s.db.WithContext(ctx).
		Model(&articleSweepRow{}).
		Select("id, title, abstract, content, author_id").
		FindInBatches(&batch, s.batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range batch {
				stats.Scanned++
				fields := []struct {
					column string
					text   string
				}{
					{"title", row.Title},
					{"abstract", row.Abstract},
					{"content", row.Content},
				}
				spans := 0
				for _, field := range fields {
					masked, hits, count := s.scan(field.text)
					if count == 0 {
						continue
					}
					spans += count
					if !s.apply {
						continue
					}
					if err := tx.Model(&articleSweepRow{}).
						Where("id = ?", row.ID).
						Update(field.column, masked).Error; err != nil {
						return fmt.Errorf("更新文章 %d 的 %s 失败: %w", row.ID.Uint64(), field.column, err)
					}
					s.recordHit(ctx, surface, row.AuthorID, row.ID, hits)
				}
				if spans > 0 {
					stats.Hit++
					stats.Spans += spans
				}
			}
			return nil
		}).Error
	if err != nil {
		return stats, fmt.Errorf("扫描文章面失败: %w", err)
	}
	return stats, nil
}

// commentSweepRow 是评论面的扫描投影（内容面与昵称快照面共用）。
type commentSweepRow struct {
	ID                ctype.ID           `gorm:"primaryKey;column:id"`
	Content           string             `gorm:"column:content"`
	UserID            ctype.ID           `gorm:"column:user_id"`
	UserNickname      string             `gorm:"column:user_nickname"`
	ReplyUserID       ctype.ID           `gorm:"column:reply_user_id"`
	ReplyUserNickname string             `gorm:"column:reply_user_nickname"`
	Status            enum.CommentStatus `gorm:"column:status"`
	DeletedAt         gorm.DeletedAt     `gorm:"column:deleted_at"`
}

// TableName 固定表名。
func (commentSweepRow) TableName() string { return "comment_models" }

// sweepComments 扫描评论正文。
//
// 只扫已发布（status=2）的评论：已删除的不是公开面，而「审核中」(status=1) 的
// 存量行由票 400 的退役迁移转成已删除（见 sweepContent），扫它们等于白洗一遍
// 马上就要消失的文本。
func (s *contentSweeper) sweepComments(ctx context.Context) (sweepSurfaceStats, error) {
	const surface = filterdomain.SurfaceComment
	stats := newSweepSurfaceStats(surface)

	var batch []commentSweepRow
	err := s.db.WithContext(ctx).
		Model(&commentSweepRow{}).
		Select("id, content, user_id, status").
		Where("status = ?", enum.CommentStatusPublished).
		FindInBatches(&batch, s.batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range batch {
				stats.Scanned++
				masked, hits, count := s.scan(row.Content)
				if count == 0 {
					continue
				}
				stats.Hit++
				stats.Spans += count
				if !s.apply {
					continue
				}
				if err := tx.Model(&commentSweepRow{}).
					Where("id = ?", row.ID).
					Update("content", masked).Error; err != nil {
					return fmt.Errorf("更新评论 %d 的正文失败: %w", row.ID.Uint64(), err)
				}
				s.recordHit(ctx, surface, row.UserID, row.ID, hits)
			}
			return nil
		}).Error
	if err != nil {
		return stats, fmt.Errorf("扫描评论面失败: %w", err)
	}
	return stats, nil
}

// userProfileSweepRow 是用户资料（昵称 / 简介）的扫描投影。
type userProfileSweepRow struct {
	ID        ctype.ID       `gorm:"primaryKey;column:id"`
	Nickname  string         `gorm:"column:nickname"`
	Abstract  string         `gorm:"column:abstract"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

// TableName 固定表名。
func (userProfileSweepRow) TableName() string { return "user_models" }

// sweepNicknames 扫描昵称面：用户资料昵称 + **评论里存的昵称快照**。
//
// 为什么连评论表的 user_nickname / reply_user_nickname 一起洗：评论行落库时会
// 把作者昵称抄一份进去（这是读面在作者改名后仍能显示当年名字的原因），读的时候
// 优先用这份快照。只洗 user_models.nickname 的话，事故里那个昵称在历史评论串上
// 还会继续展示 —— 而这次事故的违规文本正是昵称。快照属于昵称面，不属于评论面：
// 命中记录按"这条昵称属于谁"回指用户（ContentID = 用户 ID，与在线路径的
// FilterNicknameText 同一个口径）。
func (s *contentSweeper) sweepNicknames(ctx context.Context) (sweepSurfaceStats, error) {
	const surface = filterdomain.SurfaceNickname
	stats := newSweepSurfaceStats(surface)

	var users []userProfileSweepRow
	err := s.db.WithContext(ctx).
		Model(&userProfileSweepRow{}).
		Select("id, nickname").
		FindInBatches(&users, s.batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range users {
				stats.Scanned++
				masked, hits, count := s.scan(row.Nickname)
				if count == 0 {
					continue
				}
				stats.Hit++
				stats.Spans += count
				if !s.apply {
					continue
				}
				if err := tx.Model(&userProfileSweepRow{}).
					Where("id = ?", row.ID).
					Update("nickname", masked).Error; err != nil {
					return fmt.Errorf("更新用户 %d 的昵称失败: %w", row.ID.Uint64(), err)
				}
				s.recordHit(ctx, surface, row.ID, row.ID, hits)
			}
			return nil
		}).Error
	if err != nil {
		return stats, fmt.Errorf("扫描用户昵称失败: %w", err)
	}

	var comments []commentSweepRow
	err = s.db.WithContext(ctx).
		Model(&commentSweepRow{}).
		Select("id, user_id, user_nickname, reply_user_id, reply_user_nickname").
		Where("status = ?", enum.CommentStatusPublished).
		FindInBatches(&comments, s.batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range comments {
				stats.Scanned++
				fields := []struct {
					column string
					text   string
					owner  ctype.ID
				}{
					// 评论作者的昵称快照归属于评论作者。
					{"user_nickname", row.UserNickname, row.UserID},
					// 被回复者的昵称快照归属于被回复者；ID 缺失时退回评论作者，
					// 至少让记录能挂到一个真实用户上。
					{"reply_user_nickname", row.ReplyUserNickname, replyNicknameOwner(row)},
				}
				spans := 0
				for _, field := range fields {
					masked, hits, count := s.scan(field.text)
					if count == 0 {
						continue
					}
					spans += count
					if !s.apply {
						continue
					}
					if err := tx.Model(&commentSweepRow{}).
						Where("id = ?", row.ID).
						Update(field.column, masked).Error; err != nil {
						return fmt.Errorf("更新评论 %d 的 %s 失败: %w", row.ID.Uint64(), field.column, err)
					}
					s.recordHit(ctx, surface, field.owner, field.owner, hits)
				}
				if spans > 0 {
					stats.Hit++
					stats.Spans += spans
				}
			}
			return nil
		}).Error
	if err != nil {
		return stats, fmt.Errorf("扫描评论昵称快照失败: %w", err)
	}
	return stats, nil
}

// replyNicknameOwner 返回被回复昵称快照的主人。
func replyNicknameOwner(row commentSweepRow) ctype.ID {
	if !row.ReplyUserID.IsZero() {
		return row.ReplyUserID
	}
	return row.UserID
}

// sweepAbstracts 扫描个人简介面。
func (s *contentSweeper) sweepAbstracts(ctx context.Context) (sweepSurfaceStats, error) {
	const surface = filterdomain.SurfaceAbstract
	stats := newSweepSurfaceStats(surface)

	var batch []userProfileSweepRow
	err := s.db.WithContext(ctx).
		Model(&userProfileSweepRow{}).
		Select("id, abstract").
		FindInBatches(&batch, s.batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range batch {
				stats.Scanned++
				masked, hits, count := s.scan(row.Abstract)
				if count == 0 {
					continue
				}
				stats.Hit++
				stats.Spans += count
				if !s.apply {
					continue
				}
				if err := tx.Model(&userProfileSweepRow{}).
					Where("id = ?", row.ID).
					Update("abstract", masked).Error; err != nil {
					return fmt.Errorf("更新用户 %d 的简介失败: %w", row.ID.Uint64(), err)
				}
				s.recordHit(ctx, surface, row.ID, row.ID, hits)
			}
			return nil
		}).Error
	if err != nil {
		return stats, fmt.Errorf("扫描简介面失败: %w", err)
	}
	return stats, nil
}

// chatMsgSweepRow 是私信正文的扫描投影。
type chatMsgSweepRow struct {
	ID        ctype.ID       `gorm:"primaryKey;column:id"`
	Content   string         `gorm:"column:content"`
	SenderID  ctype.ID       `gorm:"column:sender_id"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

// TableName 固定表名。
func (chatMsgSweepRow) TableName() string { return "chat_msg_models" }

// sweepChats 扫描私信正文。
//
// 命中记录不带内容 ID：私信不在四个处置动作里（ADR-0032 决策 10），在线路径
// 的 FilterChatText 同样不传 ContentID —— 两边口径保持一致。
func (s *contentSweeper) sweepChats(ctx context.Context) (sweepSurfaceStats, error) {
	const surface = filterdomain.SurfaceChat
	stats := newSweepSurfaceStats(surface)

	var batch []chatMsgSweepRow
	err := s.db.WithContext(ctx).
		Model(&chatMsgSweepRow{}).
		Select("id, content, sender_id").
		FindInBatches(&batch, s.batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range batch {
				stats.Scanned++
				masked, hits, count := s.scan(row.Content)
				if count == 0 {
					continue
				}
				stats.Hit++
				stats.Spans += count
				if !s.apply {
					continue
				}
				if err := tx.Model(&chatMsgSweepRow{}).
					Where("id = ?", row.ID).
					Update("content", masked).Error; err != nil {
					return fmt.Errorf("更新私信 %d 的正文失败: %w", row.ID.Uint64(), err)
				}
				s.recordHit(ctx, surface, row.SenderID, "", hits)
			}
			return nil
		}).Error
	if err != nil {
		return stats, fmt.Errorf("扫描私信面失败: %w", err)
	}
	return stats, nil
}

// collectionSweepRow 是合集标题 / 描述的扫描投影。
type collectionSweepRow struct {
	ID          ctype.ID       `gorm:"primaryKey;column:id"`
	Title       string         `gorm:"column:title"`
	Description string         `gorm:"column:description"`
	CreatorID   ctype.ID       `gorm:"column:creator_id"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at"`
}

// TableName 固定表名。
func (collectionSweepRow) TableName() string { return "collection_models" }

// sweepCollections 扫描合集标题与描述。
func (s *contentSweeper) sweepCollections(ctx context.Context) (sweepSurfaceStats, error) {
	const surface = filterdomain.SurfaceCollection
	stats := newSweepSurfaceStats(surface)

	var batch []collectionSweepRow
	err := s.db.WithContext(ctx).
		Model(&collectionSweepRow{}).
		Select("id, title, description, creator_id").
		FindInBatches(&batch, s.batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range batch {
				stats.Scanned++
				fields := []struct {
					column string
					text   string
				}{
					{"title", row.Title},
					{"description", row.Description},
				}
				spans := 0
				for _, field := range fields {
					masked, hits, count := s.scan(field.text)
					if count == 0 {
						continue
					}
					spans += count
					if !s.apply {
						continue
					}
					if err := tx.Model(&collectionSweepRow{}).
						Where("id = ?", row.ID).
						Update(field.column, masked).Error; err != nil {
						return fmt.Errorf("更新合集 %d 的 %s 失败: %w", row.ID.Uint64(), field.column, err)
					}
					s.recordHit(ctx, surface, row.CreatorID, row.ID, hits)
				}
				if spans > 0 {
					stats.Hit++
					stats.Spans += spans
				}
			}
			return nil
		}).Error
	if err != nil {
		return stats, fmt.Errorf("扫描合集面失败: %w", err)
	}
	return stats, nil
}

// filterSweepLogger 让 *logrus.Logger 满足 filterdomain.Logger（编译期检查）。
//
// 实际传的是 deps.Logger 本身；这里不包一层，只是留一个断言，防止将来换日志接口
// 时 filterapp.New 的入参静默变成 nil。
var _ filterdomain.Logger = (*logrus.Logger)(nil)

// sweepSweepRecorderAssert 编译期断言生产实现满足 sweepRecorder。
var _ sweepRecorder = (*filterapp.Service)(nil)
