package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"myblogx/conf"
	aipkg "myblogx/internal/ai"
	"myblogx/internal/ai/apptest"
	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
)

// sseStub 是一个 OpenAI 兼容的流式桩：收下请求、把 system prompt 记进
// prompt，再回一帧内容与收尾标记。
//
// 断言模型收到了什么，只能从请求体看；桩模型不参与"回答得好不好"的判定。
func sseStub(t *testing.T) (server *httptest.Server, prompt func() string) {
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
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: %s\n\n", `{"id":"chunk","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null}]}`)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, func() string {
		mu.Lock()
		defer mu.Unlock()
		return captured
	}
}

func newPolicyEnv(t *testing.T, server *httptest.Server) *apptest.Env {
	t.Helper()
	env := apptest.NewEnv(conf.AI{
		Enable: true,
		Active: &conf.AIProvider{
			Vendor: "openai", Model: "test-model",
			Fields: map[string]string{"api_key": "test-key", "base_url": server.URL},
		},
	})
	return env
}

func drainAnswer(t *testing.T, env *apptest.Env, question string) error {
	t.Helper()
	content, errs, err := env.Service.StreamAnswer(context.Background(), question, nil)
	if err != nil {
		return err
	}
	for range content {
	}
	var got error
	for streamErr := range errs {
		if streamErr != nil {
			got = streamErr
		}
	}
	return got
}

// TestSearchAnswerTakesVoiceFromConfiguredSkill 钉住 #310 的第二、三条：
// 场景到 skill 的对应关系来自运行期配置；回答提示里出现的口径文本
// 完全由那份 skill 的正文决定，改文就改行为，不需要改代码。
func TestSearchAnswerTakesVoiceFromConfiguredSkill(t *testing.T) {
	server, prompt := sseStub(t)
	env := newPolicyEnv(t, server)
	env.AI.SceneSkills = map[string][]string{
		aipkg.SceneSearchAnswer: {"jelly-voice"},
	}

	env.Skills.ByName["jelly-voice"] = domain.SceneSkill{
		Name:    "jelly-voice",
		Body:    "自称「捏捏」，称呼读者用「你」。禁用「作为一个 AI」。",
		Version: "hash-v1",
	}

	if err := drainAnswer(t, env, "怎么投稿"); err != nil {
		t.Fatalf("搜索回答失败: %v", err)
	}
	if got := prompt(); !strings.Contains(got, "自称「捏捏」") {
		t.Fatalf("system prompt 应包含配置指定的 skill 正文，实际: %q", got)
	}
	if len(env.Skills.Calls) != 1 || env.Skills.Calls[0] != "jelly-voice" {
		t.Fatalf("应按配置读取 jelly-voice，实际调用: %v", env.Skills.Calls)
	}

	// 运营改掉正文（进程不重启、代码不改）：下一次回答必须用新口径。
	env.Skills.ByName["jelly-voice"] = domain.SceneSkill{
		Name:    "jelly-voice",
		Body:    "自称「小助手」，称呼读者用「您」。",
		Version: "hash-v2",
	}
	if err := drainAnswer(t, env, "怎么投稿"); err != nil {
		t.Fatalf("改文后搜索回答失败: %v", err)
	}
	if got := prompt(); !strings.Contains(got, "自称「小助手」") {
		t.Fatalf("改文后 system prompt 应体现新口径，实际: %q", got)
	}
}

// TestSearchAnswerPromptVersionComesFromSkillHash 钉住"版本号来自策略文本字节哈希"，
// 而不是再写一个手写常量：线上跑的是哪一版口径，必须能从调用记录里回溯。
func TestSearchAnswerPromptVersionComesFromSkillHash(t *testing.T) {
	server, _ := sseStub(t)
	env := newPolicyEnv(t, server)
	env.AI.SceneSkills = map[string][]string{
		aipkg.SceneSearchAnswer: {"jelly-voice"},
	}
	env.Skills.ByName["jelly-voice"] = domain.SceneSkill{
		Name: "jelly-voice", Body: "口径正文", Version: "abc123abc123",
	}

	var reported *aipkg.CallResult
	aipkg.SetCallReporter(func(_ context.Context, res aipkg.CallResult) {
		reported = &res
	})
	t.Cleanup(func() { aipkg.SetCallReporter(nil) })

	if err := drainAnswer(t, env, "怎么投稿"); err != nil {
		t.Fatalf("搜索回答失败: %v", err)
	}
	if reported == nil {
		t.Fatal("AI 调用没有被上报，无法回溯策略版本")
	}
	if reported.Meta.Scene != aipkg.SceneSearchAnswer {
		t.Fatalf("场景 = %q，期望 %q", reported.Meta.Scene, aipkg.SceneSearchAnswer)
	}
	if reported.Meta.PromptVersion != "abc123abc123" {
		t.Fatalf("提示词版本 = %q，期望来自 skill 哈希的 %q", reported.Meta.PromptVersion, "abc123abc123")
	}
}

// TestSearchAnswerIsTheOnlySceneReadingPolicy 钉住第三条的另一半：
// 配置把 skill 绑在别的场景上，搜索回答不会擅自读它；没绑的场景保持原样。
func TestSearchAnswerIsTheOnlySceneReadingPolicy(t *testing.T) {
	server, prompt := sseStub(t)
	env := newPolicyEnv(t, server)
	env.AI.SceneSkills = map[string][]string{
		aipkg.SceneSearchRewrite: {"jelly-voice"},
	}
	env.Skills.ByName["jelly-voice"] = domain.SceneSkill{
		Name: "jelly-voice", Body: "只该给改写场景用的口径", Version: "hash-rewrite",
	}

	if err := drainAnswer(t, env, "怎么投稿"); err != nil {
		t.Fatalf("搜索回答失败: %v", err)
	}
	if len(env.Skills.Calls) != 0 {
		t.Fatalf("搜索回答不应读取绑定到别的场景的 skill，实际调用: %v", env.Skills.Calls)
	}
	if got := prompt(); strings.Contains(got, "只该给改写场景用的口径") {
		t.Fatalf("别的场景的 skill 正文混进了搜索回答: %q", got)
	}
}

// TestSearchAnswerFailsWhenConfiguredSkillIsMissing 钉住"没有兜底文本"：
// 配置引用了不存在的 skill 时明确失败，而不是静默用一份空口径继续回答。
func TestSearchAnswerFailsWhenConfiguredSkillIsMissing(t *testing.T) {
	server, _ := sseStub(t)
	env := newPolicyEnv(t, server)
	env.AI.SceneSkills = map[string][]string{
		aipkg.SceneSearchAnswer: {"no-such-skill"},
	}

	err := drainAnswer(t, env, "怎么投稿")
	if !apperr.IsCode(err, apperr.CodeAIPolicySkillMissing) {
		t.Fatalf("引用不存在的 skill 应报 ai.policy_skill_missing，得到 %v", err)
	}
}
