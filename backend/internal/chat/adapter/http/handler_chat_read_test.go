package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	chathttp "myblogx/internal/chat/adapter/http"
	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/chat/domain/msgid"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/http/ws"
	"myblogx/utils/jwts"
)

// 本文件守 POST /chat/read 的两个契约性质：
//
//  1. msg_id_list 的元素是 **JSON 字符串**（ctype.ID），不是 JS number。
//     曾经把 DTO 写成 []uint64，前端传 "123" 直接 400；而且 2^62 区间的
//     客户端幂等键 ID 经过 JS number 会静默丢精度；
//  2. session_id 模式整条会话已读，且优先级高于 msg_id_list；
//  3. 0 条命中是幂等成功（前端静默重试不该看到假失败）。
//
// R12：测试里的 ID 语义字段一律用 ctype.ID。

// readFakeRepo 记录已读调用，并可以给出一批"命中"的消息。
type readFakeRepo struct {
	readIDs           []ctype.ID
	readSessions      []string
	affectedByList    []domain.Message
	affectedBySession []domain.Message
}

func (f *readFakeRepo) Send(context.Context, domain.SendCommand, domain.Display, domain.Display, string) (domain.Message, bool, error) {
	return domain.Message{}, false, nil
}

func (f *readFakeRepo) ListSessions(context.Context, domain.SessionQuery) ([]domain.Session, int, error) {
	return nil, 0, nil
}

func (f *readFakeRepo) FindSession(context.Context, string, ctype.ID, bool) (domain.Session, error) {
	return domain.Session{}, apperr.New(apperr.CodeChatSessionNotFound)
}

func (f *readFakeRepo) ListMessages(context.Context, domain.MessageQuery) (domain.MessageQueryResult, error) {
	return domain.MessageQueryResult{}, nil
}

func (f *readFakeRepo) UserDeletedAt(context.Context, ctype.ID, string, []ctype.ID) (map[ctype.ID]time.Time, error) {
	return nil, nil
}

func (f *readFakeRepo) MarkRead(_ context.Context, _ ctype.ID, ids []ctype.ID) ([]domain.Message, error) {
	f.readIDs = append(f.readIDs, ids...)
	return f.affectedByList, nil
}

func (f *readFakeRepo) MarkReadSession(_ context.Context, _ ctype.ID, sessionID string) ([]domain.Message, error) {
	f.readSessions = append(f.readSessions, sessionID)
	return f.affectedBySession, nil
}

func (f *readFakeRepo) DeleteMessagesForUser(context.Context, ctype.ID, []ctype.ID) ([]domain.Message, error) {
	return nil, nil
}

func (f *readFakeRepo) DeleteSessions(context.Context, ctype.ID, []string) ([]string, error) {
	return nil, nil
}

// newReadEngine 只挂 POST /chat/read，并注入 uid=1 的登录态。
func newReadEngine(repo *readFakeRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
			UserID:    ctype.NewID(1),
			SessionID: ctype.NewID(1),
			Role:      enum.RoleUser,
		}})
	})
	svc := app.New(app.Deps{
		Read: repo, Write: repo, Relations: fakeRelations{}, Users: fakeUsers{},
		Stranger: fakeStranger{verdict: domain.StrangerAllowed},
		Limiter:  permitLimiter{}, Blocks: permitBlocks{},
	})
	handler := chathttp.NewHandler(svc, ws.NewHub(),
		fakeTickets{user: ctype.NewID(1), session: ctype.NewID(1)}, fakeAuth{}, nil)
	engine.POST("/chat/read", handler.Read)
	return engine
}

func postRead(t *testing.T, engine *gin.Engine, body string) res.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/chat/read", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var envelope res.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("响应不是 JSON 壳: %v，原文 %s", err, rec.Body.String())
	}
	return envelope
}

// TestRead_BindsJSONStringIDs 回归：{"msg_id_list":["123"]} 必须绑定成功。
func TestRead_BindsJSONStringIDs(t *testing.T) {
	repo := &readFakeRepo{affectedByList: []domain.Message{{ID: ctype.NewID(123)}}}
	engine := newReadEngine(repo)

	got := postRead(t, engine, `{"msg_id_list":["123"]}`)
	if got.Code != res.SuccessCode {
		t.Fatalf("字符串 ID 列表必须绑定成功，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	if len(repo.readIDs) != 1 || repo.readIDs[0] != ctype.NewID(123) {
		t.Fatalf("用例层应收到 ctype.ID(123)，实际 %v", repo.readIDs)
	}
	if !strings.Contains(got.Msg, "1条") {
		t.Fatalf("成功文案应带命中条数，实际 %q", got.Msg)
	}
}

// TestRead_BigHashIDIsNotTruncated 派生号段（2^62 以上）的哈希 ID 必须原样透传（不转 JS number）。
func TestRead_BigHashIDIsNotTruncated(t *testing.T) {
	// 用真实的派生入口造 ID：这条用例守的是"大 ID 原样透传"，
	// 所以 ID 必须真的是模块会产出的那种值，而不是手写的相近数字。
	big := msgid.FromClientKey(ctype.NewID(100), "client-key-big")
	repo := &readFakeRepo{affectedByList: []domain.Message{{ID: big}}}
	engine := newReadEngine(repo)

	body, _ := json.Marshal(map[string]any{"msg_id_list": []ctype.ID{big}})
	got := postRead(t, engine, string(body))
	if got.Code != res.SuccessCode {
		t.Fatalf("大 ID 字符串必须绑定成功，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	if len(repo.readIDs) != 1 || repo.readIDs[0] != big {
		t.Fatalf("大 ID 必须无精度损失透传: got %v want %s", repo.readIDs, big)
	}
}

// TestRead_SessionModeMarksWholeSession session_id 存在时整条会话已读且优先。
func TestRead_SessionModeMarksWholeSession(t *testing.T) {
	repo := &readFakeRepo{
		affectedBySession: []domain.Message{{ID: ctype.NewID(11)}, {ID: ctype.NewID(12)}},
	}
	engine := newReadEngine(repo)

	got := postRead(t, engine, `{"session_id":"chat:1:2","msg_id_list":["999"]}`)
	if got.Code != res.SuccessCode {
		t.Fatalf("整会话已读应成功，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	if len(repo.readSessions) != 1 || repo.readSessions[0] != "chat:1:2" {
		t.Fatalf("应走整会话模式，实际 %v", repo.readSessions)
	}
	if len(repo.readIDs) != 0 {
		t.Fatalf("session_id 优先时不应再按 ID 列表标记，实际 %v", repo.readIDs)
	}
	if !strings.Contains(got.Msg, "2条") {
		t.Fatalf("成功文案应带命中条数，实际 %q", got.Msg)
	}
}

// TestRead_ZeroAffectedIsBenignSuccess 前端静默重试 / 重复标记已读必须看到成功。
func TestRead_ZeroAffectedIsBenignSuccess(t *testing.T) {
	for _, body := range []string{`{"session_id":"chat:1:2"}`, `{"msg_id_list":["123"]}`} {
		repo := &readFakeRepo{}
		got := postRead(t, newReadEngine(repo), body)
		if got.Code != res.SuccessCode {
			t.Fatalf("0 条命中必须是良性成功: body=%s code=%d msg=%q", body, got.Code, got.Msg)
		}
		if !strings.Contains(got.Msg, "0条") {
			t.Fatalf("0 条命中文案应为『批量标记已读0条消息』，实际 %q", got.Msg)
		}
	}
}

// TestRead_EmptyBodyIsParameterError 两者都不给时是参数错误，不能进用例层。
func TestRead_EmptyBodyIsParameterError(t *testing.T) {
	repo := &readFakeRepo{}
	got := postRead(t, newReadEngine(repo), `{}`)
	if got.Code != res.FailValidCode {
		t.Fatalf("空请求应回参数错误 1001，实际 code=%d msg=%q", got.Code, got.Msg)
	}
	if len(repo.readIDs) != 0 || len(repo.readSessions) != 0 {
		t.Fatalf("空请求不该调用已读用例: ids=%v sessions=%v", repo.readIDs, repo.readSessions)
	}
}
