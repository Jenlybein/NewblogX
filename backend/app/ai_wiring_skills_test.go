package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"myblogx/internal/platform/skills"
)

// writeSkillFixture 在 root/<dir>/SKILL.md 落一份夹具。
//
// 夹具写进临时目录而不是读仓库根那两份真 skill：那两份是运营的文案，
// 会被人改，用例不该跟着红。
func writeSkillFixture(t *testing.T, root, dir, content string) string {
	t.Helper()
	dirPath := filepath.Join(root, dir)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("创建夹具目录失败: %v", err)
	}
	path := filepath.Join(dirPath, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入夹具失败: %v", err)
	}
	return path
}

// TestSceneSkillCatalogReadsBodyOnDemandAndReportsFileHash 钉住装配层的适配：
//
//   - 正文来自 SKILL.md 正文（按需读盘，运营改文后下一次调用即生效）；
//   - 版本来自 SKILL.md 字节哈希（不是手写常量）；
//   - 名称按运行期配置里的 skill 名取，找不到就明确失败。
func TestSceneSkillCatalogReadsBodyOnDemandAndReportsFileHash(t *testing.T) {
	root := t.TempDir()
	content := "---\nname: jelly-voice\ndescription: 捏捏的口吻。\n---\n\n第一版口径。\n"
	path := writeSkillFixture(t, root, "jelly-voice", content)

	catalog, err := skills.Load([]string{root})
	if err != nil {
		t.Fatalf("加载夹具失败: %v", err)
	}
	adapter := sceneSkillCatalog{catalog: catalog}

	first, err := adapter.ReadSceneSkill("jelly-voice")
	if err != nil {
		t.Fatalf("读取 skill 失败: %v", err)
	}
	if first.Body != "第一版口径。" {
		t.Fatalf("正文 = %q，期望 SKILL.md 正文 %q", first.Body, "第一版口径。")
	}
	sum := sha256.Sum256([]byte(content))
	wantVersion := hex.EncodeToString(sum[:])[:12]
	if first.Version != wantVersion {
		t.Fatalf("版本 = %q，期望 SKILL.md 字节哈希前 12 位 %q", first.Version, wantVersion)
	}

	// 模拟运营直接改文案（不发版、不重启）：下一次读取必须拿到新正文。
	changed := "---\nname: jelly-voice\ndescription: 捏捏的口吻。\n---\n\n第二版口径。\n"
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatalf("改写夹具失败: %v", err)
	}
	second, err := adapter.ReadSceneSkill("jelly-voice")
	if err != nil {
		t.Fatalf("改文后读取 skill 失败: %v", err)
	}
	if second.Body != "第二版口径。" {
		t.Fatalf("改文后正文 = %q，期望按需读到磁盘上的新正文", second.Body)
	}
}
