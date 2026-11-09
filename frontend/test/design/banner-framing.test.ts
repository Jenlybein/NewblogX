import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter } from "../helpers/designFiles";

/**
 * 封面的固定比例（#189 封面定形 / #204 取景退场）：16:9 只有一个形状，跨端锁定一致。
 *
 * #204 之前这里还锁"取景窗的取景框"，取景体系退场后只剩两类会露出封面的框：
 *   阅读站：首屏轮播面板（所有断点同一比例）。
 *   后台：上传面板的预览框。
 *
 * 同时这里也负责"取景窗确实没了"的静态断言：组件、算术模块、后台样式块
 * 一旦被重新引入，这道门禁立刻变红。为什么是静态断言而不是运行时配置：
 * 比例一旦可调、取景一旦回到渲染路径，历史图与旧封面的裁切就会随环境漂移。
 */
const projectDir = process.cwd();
const readerIndex = readFileSync(join(projectDir, "app/pages/index.vue"), "utf8");
const adminCss = readFileSync(join(projectDir, "../frontend_admin/app/assets/css/admin.css"), "utf8");
// 票 330 起轮播的新增 / 编辑合成一个弹窗组件，上传预览框随之从 admin.css 的常驻面板
// 搬进了组件自己的 scoped 样式。这里读组件而不是样式表：被锁的不变量（预览框 16:9）
// 没变，变的是它现在写在哪个文件里。
const adminBannerModal = readFileSync(
  join(projectDir, "../frontend_admin/app/components/media/BannerFormModal.vue"),
  "utf8",
);

/** 取块里的比例声明，并把空白压成单空格（只消除排版差异，不放过取值差异）。 */
function aspectRatioOf(block: string): string {
  return (/aspect-ratio:\s*([^;]+);/u.exec(block)?.[1] ?? "").replace(/\s+/gu, " ").trim();
}

const SPEC_RATIO = "16 / 9";

describe("封面固定 16:9：跨端只有一份形状、两端取值一致", () => {
  it("阅读站面板用 aspect-ratio: 16 / 9 + width: 100%，不再有高度档", () => {
    const panel = blockAfter(readerIndex, ".home-cover__panel {");

    expect(panel).toContain("width: 100%");
    expect(aspectRatioOf(panel)).toBe(SPEC_RATIO);
    expect(panel).not.toContain("height:");
  });

  it("窄屏断点不另换比例（面板比例任何断点都相同）", () => {
    const narrow = blockAfter(readerIndex, "@media (max-width: 767px)");
    const narrowPanel = blockAfter(narrow, ".home-cover__panel {");

    expect(narrowPanel).not.toContain("aspect-ratio");
    expect(narrowPanel).not.toContain("height:");
    // 阅读站首屏里只允许这一处比例定义 —— 加一条覆盖规则就等于留了两套。
    expect(readerIndex.match(/aspect-ratio\s*:/gu) ?? []).toHaveLength(1);
  });

  it("后台上传预览框与阅读站面板同一比例（跨端常量一致性）", () => {
    const readerPanel = blockAfter(readerIndex, ".home-cover__panel {");
    // 后台只剩一个会露出轮播封面的 16:9 框（上传预览框）；取景框已随 #204 退场。
    const preview = blockAfter(adminBannerModal, ".banner-form__preview {");

    expect(aspectRatioOf(preview), `上传预览框应是 ${SPEC_RATIO}`).toBe(SPEC_RATIO);
    expect(aspectRatioOf(preview), "上传预览框应与阅读站面板一致").toBe(aspectRatioOf(readerPanel));
  });
});

/**
 * 取景窗全量退场（#204）。
 *
 * 两道删除项各锁一层：文件不存在（组件与算术被删）与源码不再引用（渲染路径与
 * 后台流程不消费取景参数）。只锁文件会在"重建到别的路径"时漏掉；只锁引用会
 * 在"留着死代码"时漏掉。
 */
describe("取景窗已退场：组件与算术模块不再存在", () => {
  const removed = [
    "app/utils/coverFraming.ts",
    "app/utils/bannerFraming.ts",
    "app/components/studio/StudioCoverFramingView.vue",
    "app/components/studio/StudioCoverFramingDialog.vue",
    "../frontend_admin/app/utils/framing.ts",
    "../frontend_admin/app/components/media/BannerFramingView.vue",
    "../frontend_admin/app/components/media/BannerFramingDialog.vue",
  ];

  it.each(removed)("%s 已删除（重新引入取景体系时这里会红）", (path) => {
    expect(existsSync(join(projectDir, path)), `${path} 应已删除`).toBe(false);
  });

  it("后台样式里没有取景窗的样式块", () => {
    expect(adminCss).not.toMatch(/\.admin-framing/u);
  });

  it("读者站渲染路径不再引用取景样式出口", () => {
    const files = [
      readFileSync(join(projectDir, "app/components/article/ArticleCover.vue"), "utf8"),
      readFileSync(join(projectDir, "app/components/article/ArticleFeedItem.vue"), "utf8"),
      readFileSync(join(projectDir, "app/components/article/BannerCarousel.vue"), "utf8"),
      readFileSync(join(projectDir, "app/pages/article/[id].vue"), "utf8"),
    ];

    for (const source of files) {
      expect(source).not.toMatch(/framingStyle|bannerFramingStyle/u);
    }
  });
});

/**
 * 视差外扩不能把容器的比例改掉（US9「封面只有一个形状」）。
 *
 * 底图比面板四边各外扩 12px 是为 ≤10px 视差留余量；四边**等量**外扩会让容器比例
 * 从 16:9 变成 1.7397，cover 之后线上可见的原图区域会比 16:9 容器紧约 5%，
 * 同一张封面在首屏与其它页面看起来就不一样了。正确做法是横向按比例多扩（12 × 16/9）。
 */
describe("视差外扩保持 16:9 配比", () => {
  const carousel = readFileSync(join(projectDir, "app/components/article/BannerCarousel.vue"), "utf8");
  const photo = blockAfter(carousel, ".banner-photo {");
  const insetDecl = (/inset:\s*([^;]+);/u.exec(photo)?.[1] ?? "").replace(/\s+/gu, " ").trim();
  const [verticalPx, horizontalPx] = (insetDecl.match(/-?\d+(?:\.\d+)?/gu) ?? []).map((value) =>
    Math.abs(Number(value)),
  ) as [number, number];

  it("横向外扩按 16/9 配比放大，而不是与纵向相同", () => {
    expect(verticalPx).toBeGreaterThan(0);
    expect(horizontalPx).toBeGreaterThan(verticalPx);
    expect(horizontalPx / verticalPx).toBeCloseTo(16 / 9, 3);
  });

  it("纵向外扩不少于视差上限（位移不会从边缘漏出兜底底色）", () => {
    // 上限来自 --parallax-max 的同源常量；位移是纵向 translateY。
    const parallaxMax = Number(
      /PARALLAX_MAX_PX = (\d+)/u.exec(
        readFileSync(join(projectDir, "app/utils/motion.ts"), "utf8"),
      )?.[1] ?? "0",
    );

    expect(verticalPx).toBeGreaterThanOrEqual(parallaxMax);
  });
});
