package skills_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myblogx/internal/platform/skills"
)

// writeSkill 在 root/<dir>/SKILL.md 落下内容并返回文件绝对路径。
// 夹具一律由用例自己写进临时目录，不读仓库里那两份真 skill ——
// 那两份会被人改文案，用例不该跟着红。
func writeSkill(t *testing.T, root, dir, content string) string {
	t.Helper()
	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("创建夹具目录失败: %v", err)
	}
	path := filepath.Join(full, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入夹具失败: %v", err)
	}
	return path
}

const validFrontmatter = `---
name: jelly-voice
description: 读者站「捏捏」AI 对话的口吻与措辞约束。
metadata:
  myblogx.scene: "search_answer"
---
`

func TestLoadBuildsCatalogFromDirectoryLayer(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "jelly-voice", validFrontmatter+"捏捏说话要短。\n")
	writeSkill(t, root, "moderation", `---
name: moderation
description: 投稿机审的判定规则。
---
只判合规，不判文笔。
`)

	cat, err := skills.Load([]string{root})
	if err != nil {
		t.Fatalf("合规的 skill 目录不该加载失败: %v", err)
	}

	// 目录层只有名称与描述；顺序按名称升序（不依赖 ReadDir 的偶然顺序）。
	want := []skills.Info{
		{Name: "jelly-voice", Description: "读者站「捏捏」AI 对话的口吻与措辞约束。"},
		{Name: "moderation", Description: "投稿机审的判定规则。"},
	}
	got := cat.List()
	if len(got) != len(want) {
		t.Fatalf("目录条数 = %d，期望 %d：%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("目录第 %d 条 = %+v，期望 %+v", i, got[i], want[i])
		}
	}

	s, ok := cat.Get("jelly-voice")
	if !ok {
		t.Fatal("按名称应能取到 jelly-voice")
	}
	if body, err := s.Body(); err != nil || body != "捏捏说话要短。" {
		t.Errorf("正文 = %q, err = %v；期望 %q", body, err, "捏捏说话要短。")
	}
}

// 目录层与正文必须分开：加载之后改正文，目录不变、正文跟着变 ——
// 说明启动只读了 frontmatter，正文是按需读的。
func TestCatalogHoldsNamesAndDescriptionsWhileBodyIsReadOnDemand(t *testing.T) {
	root := t.TempDir()
	path := writeSkill(t, root, "jelly-voice", validFrontmatter+"第一版正文。\n")

	cat, err := skills.Load([]string{root})
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if got := cat.List()[0].Description; got != "读者站「捏捏」AI 对话的口吻与措辞约束。" {
		t.Fatalf("加载后描述变了: %q", got)
	}

	// 加载之后再改正文：磁盘上的正文变了。
	changed := validFrontmatter + "第二版正文。\n"
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatalf("改写夹具失败: %v", err)
	}
	if got := cat.List()[0].Description; got != "读者站「捏捏」AI 对话的口吻与措辞约束。" {
		t.Errorf("列表不该在加载后重新读盘，描述 = %q", got)
	}
	s, ok := cat.Get("jelly-voice")
	if !ok {
		t.Fatal("按名称应能取到 jelly-voice")
	}
	body, err := s.Body()
	if err != nil {
		t.Fatalf("按需读正文失败: %v", err)
	}
	if body != "第二版正文。" {
		t.Errorf("正文 = %q，期望按需读到磁盘上的新版正文", body)
	}
}

// 版本号必须跟着**这次读到的正文**走：热改挂载的 SKILL.md 之后，调用记录
// 里记的版本不能还是加载时那一个，否则"线上跑的是哪一版"就是假话（#309）。
func TestBodyWithVersionTracksHotEditedContent(t *testing.T) {
	root := t.TempDir()
	first := validFrontmatter + "第一版正文。\n"
	path := writeSkill(t, root, "jelly-voice", first)

	cat, err := skills.Load([]string{root})
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	s, ok := cat.Get("jelly-voice")
	if !ok {
		t.Fatal("按名称应能取到 jelly-voice")
	}

	body, version, err := s.BodyWithVersion()
	if err != nil {
		t.Fatalf("读正文失败: %v", err)
	}
	if body != "第一版正文。" {
		t.Fatalf("首次正文 = %q", body)
	}
	if version != s.Hash() {
		t.Fatalf("未改的正文版本应与加载时一致: got %q want %q", version, s.Hash())
	}

	// 热改：正文换成第二版。
	second := validFrontmatter + "第二版正文。\n"
	if err := os.WriteFile(path, []byte(second), 0o644); err != nil {
		t.Fatalf("改写夹具失败: %v", err)
	}

	body, version, err = s.BodyWithVersion()
	if err != nil {
		t.Fatalf("热改后读正文失败: %v", err)
	}
	if body != "第二版正文。" {
		t.Fatalf("热改后正文 = %q", body)
	}
	if version == s.Hash() {
		t.Fatal("热改后版本仍是加载时的旧值：调用记录会记错版本")
	}
	sum := sha256.Sum256([]byte(second))
	if want := hex.EncodeToString(sum[:])[:12]; version != want {
		t.Fatalf("版本 = %q，期望新正文的 sha256 前 12 位 %q", version, want)
	}
}

// 版本号是 SKILL.md 字节的 sha256 前 12 位：期望值由用例自己按规范算，
// 不从实现反推（否则等于拿实现给自己作证）。
func TestSkillHashIsFileBytesSHA256Prefix(t *testing.T) {
	root := t.TempDir()
	content := validFrontmatter + "正文。\n"
	path := writeSkill(t, root, "jelly-voice", content)

	cat, err := skills.Load([]string{root})
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	sum := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(sum[:])[:12]
	s, _ := cat.Get("jelly-voice")
	if got := s.Hash(); got != want {
		t.Errorf("Hash() = %q，期望 %q（sha256 前 12 位）", got, want)
	}
	if got := s.Path(); got != path {
		t.Errorf("Path() = %q，期望 %q", got, path)
	}
}

// 不合规的 skill 必须加载失败，且错误里点明是哪一份文件、哪一条不合规。
// 这就是"启动失败并指出具体文件"的行为面。
func TestLoadRejectsBrokenSkillAndNamesTheFile(t *testing.T) {
	tooLongDescription := strings.Repeat("描", 1025)
	tooLongCompatibility := strings.Repeat("兼", 501)

	cases := []struct {
		name    string
		dir     string
		content string
		want    []string // 错误里必须出现的片段
	}{
		{
			name:    "名称与目录名不一致",
			dir:     "jelly-voice",
			content: "---\nname: jelly\ndescription: 口吻约束。\n---\n正文。\n",
			want:    []string{"name", "目录名", "jelly-voice"},
		},
		{
			name:    "描述超长",
			dir:     "jelly-voice",
			content: "---\nname: jelly-voice\ndescription: " + tooLongDescription + "\n---\n正文。\n",
			want:    []string{"description", "1024"},
		},
		{
			name:    "正文为空",
			dir:     "jelly-voice",
			content: "---\nname: jelly-voice\ndescription: 口吻约束。\n---\n\n   \n",
			want:    []string{"正文"},
		},
		{
			name:    "metadata 值不是字符串",
			dir:     "jelly-voice",
			content: "---\nname: jelly-voice\ndescription: 口吻约束。\nmetadata:\n  myblogx.revision: 1\n---\n正文。\n",
			want:    []string{"metadata", "myblogx.revision"},
		},
		{
			name:    "metadata 值是嵌套结构",
			dir:     "jelly-voice",
			content: "---\nname: jelly-voice\ndescription: 口吻约束。\nmetadata:\n  myblogx.scene:\n    - search_answer\n---\n正文。\n",
			want:    []string{"metadata", "myblogx.scene"},
		},
		{
			name:    "缺 name",
			dir:     "jelly-voice",
			content: "---\ndescription: 口吻约束。\n---\n正文。\n",
			want:    []string{"name"},
		},
		{
			name:    "缺 description",
			dir:     "jelly-voice",
			content: "---\nname: jelly-voice\n---\n正文。\n",
			want:    []string{"description"},
		},
		{
			name:    "name 含大写与空格",
			dir:     "jelly-voice",
			content: "---\nname: Jelly Voice\ndescription: 口吻约束。\n---\n正文。\n",
			want:    []string{"name", "小写"},
		},
		{
			name:    "name 以连字符结尾",
			dir:     "jelly-voice-",
			content: "---\nname: jelly-voice-\ndescription: 口吻约束。\n---\n正文。\n",
			want:    []string{"name"},
		},
		{
			name:    "name 超过 64 字符",
			dir:     "jelly-voice",
			content: "---\nname: " + strings.Repeat("a", 65) + "\ndescription: 口吻约束。\n---\n正文。\n",
			want:    []string{"name", "64"},
		},
		{
			name:    "compatibility 超长",
			dir:     "jelly-voice",
			content: "---\nname: jelly-voice\ndescription: 口吻约束。\ncompatibility: " + tooLongCompatibility + "\n---\n正文。\n",
			want:    []string{"compatibility", "500"},
		},
		{
			name:    "没有 frontmatter",
			dir:     "jelly-voice",
			content: "# 只是一篇正文\n",
			want:    []string{"frontmatter"},
		},
		{
			name:    "frontmatter 不是合法 YAML",
			dir:     "jelly-voice",
			content: "---\nname: [unclosed\ndescription: 口吻约束。\n---\n正文。\n",
			want:    []string{"YAML"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := writeSkill(t, root, tc.dir, tc.content)

			_, err := skills.Load([]string{root})
			if err == nil {
				t.Fatalf("不合规的 skill 必须加载失败（%s）", tc.name)
			}
			msg := err.Error()
			if !strings.Contains(msg, path) {
				t.Errorf("错误必须点明文件 %q，实际: %v", path, err)
			}
			for _, frag := range tc.want {
				if !strings.Contains(msg, frag) {
					t.Errorf("错误应说明 %q，实际: %v", frag, err)
				}
			}
		})
	}
}

// 开放规范会加字段，我们只认自己用到的那些：顶层未知字段一律容忍，
// 但不合规的已知字段仍然要红（上面那张表）。
func TestLoadToleratesUnknownTopLevelField(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "jelly-voice", `---
name: jelly-voice
description: 口吻约束。
x-future-field: whatever
allowed-tools: Read Grep
license: MIT
compatibility: 需要支持 tool use 的模型
---
正文。
`)

	cat, err := skills.Load([]string{root})
	if err != nil {
		t.Fatalf("未知顶层字段不该让加载失败: %v", err)
	}
	if len(cat.List()) != 1 {
		t.Fatalf("应加载 1 份 skill，实际 %+v", cat.List())
	}
}

// 读不到目录就不启动 —— 不存在代码内兜底文本。
func TestLoadFailsWhenRootIsMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-skills")

	cat, err := skills.Load([]string{missing})
	if err == nil {
		t.Fatalf("缺失的技能目录必须加载失败，实际拿到 %d 条", len(cat.List()))
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("错误必须点明缺失的目录 %q，实际: %v", missing, err)
	}
}

// 目录存在但没配任何 root 也是配置错误，而不是"零条 skill 也算成功"。
func TestLoadFailsWhenNoRootConfigured(t *testing.T) {
	if _, err := skills.Load(nil); err == nil {
		t.Fatal("没有配置 skill 目录时必须失败")
	}
}

// 子目录缺 SKILL.md 同样要失败：静默跳过等于把写歪的目录当成不存在。
func TestLoadFailsWhenSubdirectoryHasNoSkillFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "half-baked"), 0o755); err != nil {
		t.Fatalf("创建夹具失败: %v", err)
	}

	_, err := skills.Load([]string{root})
	if err == nil {
		t.Fatal("子目录缺 SKILL.md 必须加载失败")
	}
	if !strings.Contains(err.Error(), filepath.Join(root, "half-baked", "SKILL.md")) {
		t.Errorf("错误必须点明缺失的 SKILL.md，实际: %v", err)
	}
}

// skills/ 根下会有 README.md 这类普通文件：它们不是 skill，跳过而不是报错。
func TestLoadSkipsPlainFilesAtRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("说明\n"), 0o644); err != nil {
		t.Fatalf("创建夹具失败: %v", err)
	}
	writeSkill(t, root, "jelly-voice", validFrontmatter+"正文。\n")

	cat, err := skills.Load([]string{root})
	if err != nil {
		t.Fatalf("根下的普通文件不该让加载失败: %v", err)
	}
	if got := len(cat.List()); got != 1 {
		t.Fatalf("应只加载 1 份 skill，实际 %d 份", got)
	}
}

// 多个 root 合并成一份目录；同名 skill 出现两次是配置错误（不知道该用哪份）。
func TestLoadMergesRootsAndRejectsDuplicateName(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	writeSkill(t, first, "jelly-voice", validFrontmatter+"正文。\n")
	writeSkill(t, second, "moderation", "---\nname: moderation\ndescription: 机审规则。\n---\n正文。\n")

	cat, err := skills.Load([]string{first, second})
	if err != nil {
		t.Fatalf("两个 root 应合并加载: %v", err)
	}
	if got := len(cat.List()); got != 2 {
		t.Fatalf("合并后应有 2 份 skill，实际 %d 份", got)
	}

	dup := t.TempDir()
	writeSkill(t, dup, "jelly-voice", validFrontmatter+"另一个人写的正文。\n")
	if _, err := skills.Load([]string{first, dup}); err == nil {
		t.Fatal("同名 skill 出现在两个 root 里必须失败")
	}
}
