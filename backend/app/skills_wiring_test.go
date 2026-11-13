package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myblogx/app"
	"myblogx/conf"
)

const validSkill = `---
name: jelly-voice
description: 读者站「捏捏」AI 对话的口吻与措辞约束。
---

捏捏说话要短。
`

func writeSkill(t *testing.T, root, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
		t.Fatalf("创建夹具目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入夹具失败: %v", err)
	}
}

// 启动阶段只有两种结局：拿到目录层，或拿到一个点名文件的错误。
// 没有"跳过并继续"的第三条路 —— 读不到就不启动。
func TestLoadSkills(t *testing.T) {
	t.Run("目录配置正确时拿到目录层", func(t *testing.T) {
		root := t.TempDir()
		writeSkill(t, root, "jelly-voice", validSkill)

		cat, err := app.LoadSkills(&conf.Config{Skills: conf.Skills{Dirs: root}})
		if err != nil {
			t.Fatalf("不该失败: %v", err)
		}
		if cat.Len() != 1 || cat.List()[0].Name != "jelly-voice" {
			t.Fatalf("目录层 = %+v，期望 1 份 jelly-voice", cat.List())
		}
	})

	t.Run("目录读不到时失败并点明路径", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "no-such-skills")

		_, err := app.LoadSkills(&conf.Config{Skills: conf.Skills{Dirs: missing}})
		if err == nil {
			t.Fatal("目录读不到必须失败")
		}
		if !strings.Contains(err.Error(), missing) {
			t.Fatalf("错误应点明缺失目录 %q，实际: %v", missing, err)
		}
	})

	t.Run("不合规的 skill 让启动失败", func(t *testing.T) {
		root := t.TempDir()
		// 名称与目录名不一致：这是最容易写歪、又最难在运行期发现的一种。
		writeSkill(t, root, "jelly-voice", "---\nname: jelly\ndescription: 口吻约束。\n---\n正文。\n")

		_, err := app.LoadSkills(&conf.Config{Skills: conf.Skills{Dirs: root}})
		if err == nil {
			t.Fatal("不合规的 skill 必须让启动失败")
		}
		if !strings.Contains(err.Error(), filepath.Join(root, "jelly-voice", "SKILL.md")) {
			t.Fatalf("错误应点明具体文件，实际: %v", err)
		}
	})

	t.Run("没配置目录时按默认 skills 相对进程工作目录解析", func(t *testing.T) {
		root := t.TempDir()
		writeSkill(t, filepath.Join(root, "skills"), "jelly-voice", validSkill)
		t.Chdir(root)

		cat, err := app.LoadSkills(&conf.Config{})
		if err != nil {
			t.Fatalf("默认 skills 目录应能加载: %v", err)
		}
		if cat.Len() != 1 {
			t.Fatalf("应加载 1 份 skill，实际 %+v", cat.List())
		}
	})

	t.Run("默认目录不存在时失败", func(t *testing.T) {
		t.Chdir(t.TempDir())

		_, err := app.LoadSkills(&conf.Config{})
		if err == nil {
			t.Fatal("默认 skills 目录不存在时必须失败")
		}
	})
}

// 场景引用一份不存在的 skill 必须在启动期就失败（ADR-0021）。
//
// 这条防的是"配置写错但进程照常起来"：线上表现是那个场景第一次被调用时
// 报 ai.policy_skill_missing，而那时用户已经在等回答。
func TestValidateSceneSkillBindings(t *testing.T) {
	t.Run("引用存在的 skill 通过", func(t *testing.T) {
		root := t.TempDir()
		writeSkill(t, root, "jelly-voice", validSkill)
		cat, err := app.LoadSkills(&conf.Config{Skills: conf.Skills{Dirs: root}})
		if err != nil {
			t.Fatalf("加载失败: %v", err)
		}

		aiConf := conf.AI{SceneSkills: map[string][]string{"search_answer": {"jelly-voice"}}}
		if err := app.ValidateSceneSkillBindings(aiConf, cat); err != nil {
			t.Fatalf("引用存在的 skill 不该报错: %v", err)
		}
	})

	t.Run("引用不存在的 skill 返回错误并点名", func(t *testing.T) {
		root := t.TempDir()
		writeSkill(t, root, "jelly-voice", validSkill)
		cat, err := app.LoadSkills(&conf.Config{Skills: conf.Skills{Dirs: root}})
		if err != nil {
			t.Fatalf("加载失败: %v", err)
		}

		aiConf := conf.AI{SceneSkills: map[string][]string{"moderation": {"nope"}}}
		err = app.ValidateSceneSkillBindings(aiConf, cat)
		if err == nil {
			t.Fatal("引用不存在的 skill 应启动失败")
		}
		if !strings.Contains(err.Error(), "moderation") || !strings.Contains(err.Error(), "nope") {
			t.Fatalf("错误里应点名场景与 skill: %v", err)
		}
	})

	t.Run("空名单是合法的（场景不绑定策略文本）", func(t *testing.T) {
		aiConf := conf.AI{SceneSkills: map[string][]string{"moderation": nil}}
		if err := app.ValidateSceneSkillBindings(aiConf, nil); err != nil {
			t.Fatalf("空名单不该报错: %v", err)
		}
	})
}
