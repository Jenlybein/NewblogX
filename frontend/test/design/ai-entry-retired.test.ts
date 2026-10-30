import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// 票 #10 的「退场断言」：旧的 AI 入口必须真的消失，而不是只在页面上藏起来。
//
// 为什么用源码扫描而不是渲染断言：这两处旧入口的退场是**删除**，删除后的正确状态
// 是"文件/引用根本不存在"。渲染断言只能证明某个页面当前没画出来，证明不了
// 下一个页面不会又把它引回来 —— 门禁式的静态断言才能挡住回潮。

const projectDir = process.cwd();
const appDir = join(projectDir, "app");

function listSourceFiles(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    const info = statSync(full);
    if (info.isDirectory()) {
      if (name === "node_modules" || name === ".nuxt") continue;
      listSourceFiles(full, out);
      continue;
    }
    if (name.endsWith(".vue") || name.endsWith(".ts")) {
      out.push(full);
    }
  }
  return out;
}

const sourceFiles = listSourceFiles(appDir);

function read(relativePath: string): string {
  return readFileSync(join(appDir, relativePath), "utf8");
}

describe("旧 AI 入口已退场（票 #10）", () => {
  it("旧居中模态组件文件已删除", () => {
    expect(existsSync(join(appDir, "components/home/HomeAiChatDialog.vue"))).toBe(false);
  });

  it("全站源码不再引用 HomeAiChatDialog（组件或调用点都不能回来）", () => {
    const referencing = sourceFiles.filter((file) =>
      readFileSync(file, "utf8").includes("HomeAiChatDialog"),
    );

    expect(referencing.map((file) => relative(appDir, file))).toEqual([]);
  });

  it("首页右栏不再有 AI 卡片（卡片区块 / 进行对话按钮都不在）", () => {
    const sidebar = read("components/home/HomeSidebar.vue");

    expect(sidebar).not.toContain("home-ai-card");
    expect(sidebar).not.toContain("进行对话");
    expect(sidebar).not.toContain("HomeAiChatDialog");
    expect(sidebar).not.toContain("aiInfo");
  });

  it("首页不再把 aiInfo 塞给右栏（入口只剩捏捏）", () => {
    const homePage = read("pages/index.vue");

    expect(homePage).not.toMatch(/:ai-info=/u);
    expect(homePage).not.toContain("HomeAiChatDialog");
  });

  it("旧对话框的样式类名没有残留（ai-chat-dialog / ai-chat-thread / ai-chat-bubble）", () => {
    const leftovers = sourceFiles.filter((file) => {
      const source = readFileSync(file, "utf8");
      return /ai-chat-(dialog|thread|bubble|composer)/u.test(source);
    });

    expect(leftovers.map((file) => relative(appDir, file))).toEqual([]);
  });

  it("AI 入口收敛到捏捏：布局同时挂角色与面板，且两者共享同一实例", () => {
    const layout = read("layouts/default.vue");

    expect(layout).toContain("JellyLauncher");
    expect(layout).toContain("JellyAiPanel");
    // 面板与角色必须吃同一份实例（谁也别自己 new）。
    expect(layout).toMatch(/:assistant="jellyAssistant"/u);
    expect(layout).not.toMatch(/useJellyAssistant\(\)/u);
  });

  it("AnimatePresence 退场有客户端挂载守卫（SSR 不会把面板内联成 opacity:0）", () => {
    const panel = read("components/jelly/JellyAiPanel.vue");
    const layout = read("layouts/default.vue");

    expect(panel).toContain("AnimatePresence");
    // 挂载后才渲染动画容器：SSR 阶段不产出 initial 内联样式（#3 报告踩过的坑）。
    expect(panel).toMatch(/<AnimatePresence v-if="hydrated">/u);
    // 布局侧再包一层 ClientOnly 双保险。
    expect(layout).toMatch(/<ClientOnly>[\s\S]*JellyAiPanel[\s\S]*<\/ClientOnly>/u);
  });
});
