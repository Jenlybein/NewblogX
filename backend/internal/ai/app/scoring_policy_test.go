package app_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"myblogx/conf"
	"myblogx/internal/ai"
	aiapp "myblogx/internal/ai/app"
	"myblogx/internal/ai/apptest"
	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// 评分场景同样吃运行期策略文本（ADR-0023）。
//
// 与 search_answer 那一组用例同一个套路：桩模型只负责收下请求、把 system prompt
// 记下来，断言的是"模型到底看到了什么口径"，不是模型会不会打分。运营改一份
// SKILL.md 就该改行为，不需要发版——这条在评分上比在口吻上更重要，因为评分
// 直接决定文章能不能发布。

// scoringStub 返回一个 OpenAI 兼容桩：记下最后一次 system prompt，回一份合法评分 JSON。
func scoringStub(t *testing.T) (server *httptest.Server, prompt func() string) {
	t.Helper()
	var mu sync.Mutex
	var captured string

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err == nil {
			for _, m := range req.Messages {
				if m.Role != "system" {
					continue
				}
				mu.Lock()
				captured = m.Content
				mu.Unlock()
			}
		}

		content := `{"ai_total_score":78,"total_score":78,"score_level":"常规文章",` +
			`"article_type":"说明文","dimensions":[` +
			`{"name":"clarity","score":80,"reason":""},` +
			`{"name":"structure","score":78,"reason":""},` +
			`{"name":"completeness","score":76,"reason":""},` +
			`{"name":"readability","score":79,"reason":""},` +
			`{"name":"persuasiveness","score":72,"reason":""},` +
			`{"name":"language","score":82,"reason":""}],` +
			`"main_issues":[],"overall_comment":"还行。"}`

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": content}},
			},
		})
	}))
	t.Cleanup(server.Close)

	return server, func() string {
		mu.Lock()
		defer mu.Unlock()
		return captured
	}
}

// scoringBody 是"成篇"的正文，避免命中形态封顶、干扰对 prompt 的断言。
func scoringBody() string {
	return strings.Join([]string{
		"最近整理接口层时发现几个反复出现的问题，记下来备查。",
		"第一个问题是返回值形状不一致：同一个模块里有的端点包了一层 envelope，有的直接返回数组。调用方每接一个端点都要先看清它长什么样。",
		"第二个问题是错误码的粒度：所有失败都返回同一个 code，前端只能靠 message 里的中文分支，一旦文案改了就全崩。",
		"第三个问题是分页参数：有的用 page/limit，有的用 offset/count。统一成前者之后，前端列表组件终于可以复用同一份翻页逻辑。",
		"最后留一条经验：接口的边界要在服务端收紧，前端的校验只是提示，不是保障。",
	}, "\n\n")
}

func newScoringEnv(t *testing.T, server *httptest.Server) *apptest.Env {
	t.Helper()
	return apptest.NewEnv(conf.AI{
		Enable: true,
		Active: &conf.AIProvider{
			Vendor: "openai", Model: "test-model",
			Fields: map[string]string{"api_key": "test-key", "base_url": server.URL},
		},
	})
}

// gateInput 是一次闸门评分请求：新建文章还没有落库，ID 由用例自己给。
func gateInput() aiapp.GateScoreInput {
	return aiapp.GateScoreInput{
		ArticleID: ctype.NewID(9001),
		AuthorID:  ctype.NewID(7),
		Title:     "接口层的三个反复问题",
		Content:   scoringBody(),
	}
}

// TestGateScoreInjectsScoringSkill 钉住"评分口径来自配置绑定的 skill 正文"。
func TestGateScoreInjectsScoringSkill(t *testing.T) {
	server, prompt := scoringStub(t)
	env := newScoringEnv(t, server)
	env.AI.SceneSkills = map[string][]string{
		ai.SceneScoring: {"scoring"},
	}
	env.Skills.ByName["scoring"] = domain.SceneSkill{
		Name:    "scoring",
		Body:    "两行祝福语一律不超过 30 分，完整性不超过 20。",
		Version: "score-v1",
	}

	result, err := env.Service.ScoreForGate(context.Background(), gateInput())
	if err != nil {
		t.Fatalf("评分失败: %v", err)
	}
	if result.TotalScore == 0 {
		t.Fatal("评分结果应带回总分")
	}
	// 木桶判定要用维度分：端口必须把六项都带出来，缺一项就等于少卡一项。
	if len(result.Dimensions) != 6 {
		t.Fatalf("应带回六个维度，实际 %d", len(result.Dimensions))
	}

	if got := prompt(); !strings.Contains(got, "两行祝福语一律不超过 30 分") {
		t.Fatalf("system prompt 应包含配置指定的 skill 正文，实际: %q", got)
	}
	if len(env.Skills.Calls) != 1 || env.Skills.Calls[0] != "scoring" {
		t.Fatalf("应按配置读取 scoring 这份 skill，实际调用: %v", env.Skills.Calls)
	}

	// 运营改文（进程不重启）：下一次评分必须用新口径。
	env.Skills.ByName["scoring"] = domain.SceneSkill{
		Name:    "scoring",
		Body:    "新的口径：完整度低于 55 分一律不合格。",
		Version: "score-v2",
	}
	if _, err := env.Service.ScoreForGate(context.Background(), gateInput()); err != nil {
		t.Fatalf("改文后评分失败: %v", err)
	}
	if got := prompt(); !strings.Contains(got, "完整度低于 55 分一律不合格") {
		t.Fatalf("改文后 prompt 应体现新口径，实际: %q", got)
	}
}

// TestGateScoreWorksWithoutBoundSkill 钉住"摘掉 skill 不等于关掉底线"：
// 没绑定时评分照常进行（不让所有人发不出文章），但内置的形态下限仍在。
func TestGateScoreWorksWithoutBoundSkill(t *testing.T) {
	server, prompt := scoringStub(t)
	env := newScoringEnv(t, server)
	env.AI.SceneSkills = map[string][]string{}

	if _, err := env.Service.ScoreForGate(context.Background(), gateInput()); err != nil {
		t.Fatalf("没绑定 skill 时评分应照常进行: %v", err)
	}
	if len(env.Skills.Calls) != 0 {
		t.Fatalf("没绑定就不该去读 skill，实际: %v", env.Skills.Calls)
	}
	got := prompt()
	if strings.Contains(got, "本站在意的评分口径") {
		t.Fatalf("没绑定时不该出现策略段落: %q", got)
	}
	if !strings.Contains(got, "completeness 不得高于 30") {
		t.Fatalf("内置的形态下限不能依赖策略文本: %q", got)
	}
}

// TestGateScoreFailsWhenBoundSkillIsMissing 与其它场景同一条口径：
// 配置写了不存在的 skill 就明确失败，不静默用空口径。
func TestGateScoreFailsWhenBoundSkillIsMissing(t *testing.T) {
	server, _ := scoringStub(t)
	env := newScoringEnv(t, server)
	env.AI.SceneSkills = map[string][]string{
		ai.SceneScoring: {"no-such-skill"},
	}

	_, err := env.Service.ScoreForGate(context.Background(), gateInput())
	if !apperr.IsCode(err, apperr.CodeAIPolicySkillMissing) {
		t.Fatalf("引用不存在的 skill 应报 ai.policy_skill_missing，得到 %v", err)
	}
}
