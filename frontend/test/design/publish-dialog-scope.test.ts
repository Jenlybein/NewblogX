import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 发布弹窗的尺寸约束必须写在**组件自己**的 scoped 样式里。
 *
 * 这些规则曾经放在 `pages/studio/write.vue` 引用的 `studio-write.css` 里。页面的
 * scoped 样式只作用于该页模板，而 `NModal` 把内容 teleport 到 body，弹窗内部的元素
 * 拿不到页面的 data-v 作用域 —— 宽度规则整条失效，NCard 按默认宽度铺满视口，
 * 运营看到的就是"发布信息铺满全屏"而不是一个弹窗。
 *
 * 这条用例钉住归属：组件里有、页面引用的那份 css 里没有。
 */
const projectDir = process.cwd();
const dialogSource = readFileSync(
  join(projectDir, "app/components/studio/StudioWritePublishDialog.vue"),
  "utf8",
);
const pageCss = readFileSync(join(projectDir, "app/assets/css/studio-write.css"), "utf8");

describe("发布弹窗 · 尺寸约束归属", () => {
  it("宽度写在组件自己的 <style scoped> 里，而不是页面引用的全局 css", () => {
    expect(dialogSource).toContain("<style scoped>");
    expect(dialogSource).toContain(".publish-modal-shell {");
    expect(dialogSource).toContain("width: min(760px, calc(100vw - 96px))");

    // 页面那份 css 里不许再留同一批规则：留着会让人以为它才是生效的那一处。
    expect(pageCss).not.toContain(".publish-modal-shell");
    expect(pageCss).not.toContain(".publish-form__row");
    expect(pageCss).not.toContain(".publish-modal__footer");
  });

  it("高度也有上限并内部滚动：弹窗不会把整屏占满", () => {
    expect(dialogSource).toMatch(/max-height:\s*calc\(100vh - 96px\)/u);
    expect(dialogSource).toMatch(/overflow:\s*auto/u);
  });

  it("窄屏退化成整宽卡片（仍留边距），不是贴边全屏", () => {
    expect(dialogSource).toMatch(/width:\s*calc\(100vw - 28px\)/u);
  });
});
