package river

// es_projection 死信的重投执行器。
//
// # 这条链路以前为什么不能重投
//
// 死信 payload 只有 `{schema, table, action, rowCount}` —— 知道"哪张表出过错"，
// 但不知道"是哪几行"，所以补不了，只能全量重建索引。现在落死信时把行主键一起记下来
// （见 sync_bulk.go 的 saveProjectionErrorToDLQ），重投就能走现成的按主键重建入口。
//
// # 为什么按 (表, 动作) 分派，而不是"重放当时那次调用"
//
// 重投要的是**结果**：让这些文档变成当前正确的状态。所以它是"重算"，不是"重放"。
// 这也让它天然幂等——重复点两次，第二次还是把同样的几篇算成同样的结果。
//
// 文章表的 insert/update 统一走 article_upsert（按 ID 从库里重读整篇）：
// 当时的 update 是"只改变更列"的增量，但重投手里没有那份增量，也不需要——
// 全量重算比增量更稳，而且同样只影响这几篇。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/es/articlesync"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-mysql-org/go-mysql/canal"
	"gorm.io/gorm"
)

// ProjectionStream 是 river 投影链路写进死信表的 stream 值。
const ProjectionStream = "es_projection"

// ProjectionReplayer 把 es_projection 的死信按主键重算一遍。
type ProjectionReplayer struct {
	db     *gorm.DB
	client *elasticsearch.Client
}

// NewProjectionReplayer 构造执行器。client 为 nil 时每次重投都会明确报错。
func NewProjectionReplayer(db *gorm.DB, client *elasticsearch.Client) *ProjectionReplayer {
	return &ProjectionReplayer{db: db, client: client}
}

// Streams 实现 dlq.Replayer。
func (r *ProjectionReplayer) Streams() []string { return []string{ProjectionStream} }

// projectionReplayPayload 是 es_projection 死信 payload 的形状。
//
// 与写入端共用同一份字段名（写入端是 map，这里是结构体——两边都在本包内，
// 字段名写在同一个文件组里，改名时至少要同时改两处，不容易漏）。
type projectionReplayPayload struct {
	Schema   string     `json:"schema"`
	Table    string     `json:"table"`
	Action   string     `json:"action"`
	RowCount int        `json:"rowCount"`
	IDs      []ctype.ID `json:"ids"`
}

// Replay 实现 dlq.Replayer。
//
// 校验顺序刻意是"payload 先、依赖后"：一条没有 ids 的旧记录，无论数据库是否可用，
// 结论都是"这条补不了、请去全量重建"。先报"数据库未初始化"会把真正该做的事盖住。
func (r *ProjectionReplayer) Replay(ctx context.Context, record deadletter.Record) error {
	if r == nil {
		return fmt.Errorf("river: 重投执行器未初始化")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	var payload projectionReplayPayload
	raw := strings.TrimSpace(record.PayloadJSON)
	if raw == "" || raw == "{}" {
		return fmt.Errorf("river: 死信 payload 为空，无法重投（cdc_job_id=%s）", record.CdcJobID)
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("river: 死信 payload 不是合法 JSON: %w", err)
	}

	if len(payload.IDs) == 0 {
		// 旧版本落库的记录：只有表名与动作，没法定位到行。
		// 这是"语义上不能重投"，不是"这次失败" —— 用 ErrUnsupported 表达，
		// 编排层不会把它记成 failed、也不会涨 retry_count。
		return fmt.Errorf("river: 这条投影死信没有记录行主键（旧版本落库），无法定位要重算哪些文档。"+
			"可靠的补法是全量重建文章搜索投影：`./server -es -s article-sync`"+
			"（索引需已存在，否则先 -es -s init）: %w", deadletter.ErrUnsupported)
	}
	if r.client == nil {
		return fmt.Errorf("river: ES 客户端未初始化，无法重投")
	}
	if r.db == nil {
		return fmt.Errorf("river: 数据库未初始化，无法重投")
	}

	events, err := projectionReplayEvents(payload.Table, payload.Action)
	if err != nil {
		return err
	}

	db := r.db
	if ctx != nil {
		db = db.WithContext(ctx)
	}
	for _, eventType := range events {
		if err := articlesync.SyncArticleSearchProjection(db, r.client, articlesync.ArticleSearchProjectionEvent{
			Type: eventType,
			IDs:  payload.IDs,
		}); err != nil {
			return fmt.Errorf("river: 投影重算失败（table=%s action=%s type=%s ids=%d 个）: %w",
				payload.Table, payload.Action, eventType, len(payload.IDs), err)
		}
	}
	return nil
}

// projectionReplayEvents 把 (表, 动作) 翻译成要重算的投影事件。
//
// 一个表可能对应多个事件：用户行变化同时影响"作者快照"和"置顶作者名"，
// 原链路就是连发两个事件，重投必须保持一致，否则会漏掉一半。
//
// 文章表的 insert/update 都归一到 article_upsert：按 ID 重读整篇。
// 只有 delete 需要区分——它要把文档从索引里删掉，用 upsert 会重新建出来。
func projectionReplayEvents(table, action string) ([]articlesync.ArticleSearchProjectionEventType, error) {
	switch strings.ToLower(strings.TrimSpace(table)) {
	case "article_models":
		if strings.EqualFold(strings.TrimSpace(action), canal.DeleteAction) {
			return []articlesync.ArticleSearchProjectionEventType{
				articlesync.ArticleSearchProjectionArticleDelete,
			}, nil
		}
		return []articlesync.ArticleSearchProjectionEventType{
			articlesync.ArticleSearchProjectionArticleUpsert,
		}, nil
	case "user_models":
		return []articlesync.ArticleSearchProjectionEventType{
			articlesync.ArticleSearchProjectionAuthorSnapshot,
			articlesync.ArticleSearchProjectionTopUserChanged,
		}, nil
	case "category_models":
		return []articlesync.ArticleSearchProjectionEventType{
			articlesync.ArticleSearchProjectionCategorySnapshot,
		}, nil
	case "tag_models":
		return []articlesync.ArticleSearchProjectionEventType{
			articlesync.ArticleSearchProjectionTagSnapshot,
		}, nil
	case "article_tag_models":
		return []articlesync.ArticleSearchProjectionEventType{
			articlesync.ArticleSearchProjectionArticleTagsChanged,
		}, nil
	case "user_top_article_models":
		return []articlesync.ArticleSearchProjectionEventType{
			articlesync.ArticleSearchProjectionArticleTopChanged,
		}, nil
	default:
		// 未知表：不能当成 unsupported 混过去 —— 这说明写入端与读取端对不上，
		// 是真故障，需要人看（unsupported 的含义是"这条本来就不该重投"）。
		return nil, fmt.Errorf("river: 不认识的投影表 %q（写入端与重投端的表清单不一致）", table)
	}
}

// projectionIDColumn 返回各投影表里"要重算的主键列"。
//
// 关联表/置顶表的主键对重算没有意义——它们影响的是**文章**文档，
// 所以取 article_id。原链路就是这么取的（见 article_search_projection.go）。
func projectionIDColumn(table string) string {
	switch strings.ToLower(strings.TrimSpace(table)) {
	case "article_tag_models", "user_top_article_models":
		return "article_id"
	default:
		return "id"
	}
}
