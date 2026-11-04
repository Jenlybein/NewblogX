import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ArticleActionsBar from "~/components/article/ArticleActionsBar.vue";

/**
 * 互动条组件（#236 / 决定清单第 2 节）。
 *
 * 这是右栏互动条与 <1024px 底部固定条的**同一个组件**：两处不同样式、同一份状态。
 * 测试只从组件的公开契约出发（props 进来、事件出去、渲染可观察），不碰页面：
 *   1. 四个动作按固定顺序（点赞 → 收藏 → 分享 → 评论），每个都带图标与无障碍名称；
 *   2. 已赞 / 已藏可见可读（`aria-pressed` + is-active），不是只有颜色差异；
 *   3. 计数走站内 `formatCount` 口径，分享不造假的计数；
 *   4. 点赞在途时禁用并 `aria-busy`，防止连发；
 *   5. 点击只 `emit`，不发请求——状态真相在页面的 `useArticleDigg` 里（#235）。
 */

function actionNames(wrapper: ReturnType<typeof mount>): (string | undefined)[] {
  return wrapper.findAll("[data-action]").map((node) => node.attributes("data-action"));
}

function button(wrapper: ReturnType<typeof mount>, action: string) {
  return wrapper.get(`[data-action="${action}"]`);
}

describe("ArticleActionsBar：四个动作的结构", () => {
  it("固定顺序：点赞 → 收藏 → 分享 → 评论，容器标出变体（右栏 / 底部条）", () => {
    const wrapper = mount(ArticleActionsBar);

    expect(wrapper.get("[data-role='article-actions-bar']").attributes("data-variant")).toBe("aside");
    expect(actionNames(wrapper)).toEqual(["digg", "favor", "share", "comment"]);
  });

  it("每个动作都有图标（svg），分享与评论也带图标——不是文字按钮", () => {
    const wrapper = mount(ArticleActionsBar);

    for (const action of actionNames(wrapper)) {
      expect(button(wrapper, action!).find("svg").exists(), `${action} 缺图标`).toBe(true);
    }
  });

  it("点赞 / 收藏 / 评论带数字，分享没有计数（纯前端 clipboard，不造一个假数字）", () => {
    const wrapper = mount(ArticleActionsBar, {
      props: { diggCount: 1234, favorCount: 8, commentCount: 12000 },
    });

    expect(button(wrapper, "digg").get("[data-role='count']").text()).toBe("1.2k");
    expect(button(wrapper, "favor").get("[data-role='count']").text()).toBe("8");
    expect(button(wrapper, "comment").get("[data-role='count']").text()).toBe("1.2w");
    expect(button(wrapper, "share").find("[data-role='count']").exists()).toBe(false);
  });

  it("底部固定条变体：同一套按钮，容器类与 data-variant 换成 fixed", () => {
    const wrapper = mount(ArticleActionsBar, { props: { variant: "fixed" } });

    const bar = wrapper.get("[data-role='article-actions-bar']");
    expect(bar.attributes("data-variant")).toBe("fixed");
    expect(bar.classes()).toContain("article-actions-bar--fixed");
    expect(actionNames(wrapper)).toEqual(["digg", "favor", "share", "comment"]);
  });
});

describe("ArticleActionsBar：已赞 / 已藏态与无障碍名称", () => {
  it("未赞未藏：aria-pressed=false，名称是「点赞」「收藏」", () => {
    const wrapper = mount(ArticleActionsBar, { props: { diggCount: 12, favorCount: 5 } });

    expect(button(wrapper, "digg").attributes("aria-pressed")).toBe("false");
    expect(button(wrapper, "digg").attributes("aria-label")).toBe("点赞，当前 12 个赞");
    expect(button(wrapper, "digg").attributes("title")).toBe("点赞");
    expect(button(wrapper, "favor").attributes("aria-pressed")).toBe("false");
    expect(button(wrapper, "favor").attributes("aria-label")).toBe("收藏，当前 5 次收藏");
  });

  it("已赞：aria-pressed=true + is-active，名称说「取消点赞」，图标换成实心", () => {
    const wrapper = mount(ArticleActionsBar, { props: { isDigged: true, diggCount: 12 } });

    const digg = button(wrapper, "digg");
    expect(digg.attributes("aria-pressed")).toBe("true");
    expect(digg.attributes("aria-label")).toBe("取消点赞，当前 12 个赞");
    expect(digg.classes()).toContain("is-active");
  });

  it("已藏：aria-pressed=true + is-active，名称说「点按可移除」", () => {
    const wrapper = mount(ArticleActionsBar, { props: { isFavor: true, favorCount: 5 } });

    expect(button(wrapper, "favor").attributes("aria-pressed")).toBe("true");
    expect(button(wrapper, "favor").attributes("aria-label")).toBe("已收藏，点按可移除，当前 5 次收藏");
    expect(button(wrapper, "favor").classes()).toContain("is-active");
  });

  it("评论按钮说明会把焦点送进评论框（未登录时打开登录框的行为在页面接线里）", () => {
    const wrapper = mount(ArticleActionsBar, { props: { commentCount: 7 } });

    expect(button(wrapper, "comment").attributes("aria-label")).toBe("跳到评论区写评论，当前 7 条评论");
    expect(button(wrapper, "share").attributes("aria-label")).toBe("分享这篇文章");
  });

  it("点赞在途：禁用 + aria-busy，名称改「处理中」", () => {
    const wrapper = mount(ArticleActionsBar, { props: { likePending: true, isDigged: true, diggCount: 9 } });

    const digg = button(wrapper, "digg");
    expect(digg.attributes("disabled")).toBeDefined();
    expect(digg.attributes("aria-busy")).toBe("true");
    expect(digg.attributes("aria-label")).toBe("点赞处理中，当前 9 个赞");
  });

  it("防抖窗口内同样禁用（连点不连发请求）", () => {
    const wrapper = mount(ArticleActionsBar, { props: { likeDebouncing: true } });

    expect(button(wrapper, "digg").attributes("disabled")).toBeDefined();
    expect(button(wrapper, "digg").attributes("aria-busy")).toBe("false");
  });
});

describe("ArticleActionsBar：点击只抛事件", () => {
  it("四个动作各自 emit 对应事件", async () => {
    const wrapper = mount(ArticleActionsBar);

    await button(wrapper, "digg").trigger("click");
    await button(wrapper, "favor").trigger("click");
    await button(wrapper, "share").trigger("click");
    await button(wrapper, "comment").trigger("click");

    expect(wrapper.emitted("like")).toHaveLength(1);
    expect(wrapper.emitted("favorite")).toHaveLength(1);
    expect(wrapper.emitted("share")).toHaveLength(1);
    expect(wrapper.emitted("comment")).toHaveLength(1);
  });

  it("点赞在途时按下去不抛事件（禁用不是摆设）", async () => {
    const wrapper = mount(ArticleActionsBar, { props: { likePending: true } });

    await button(wrapper, "digg").trigger("click");

    expect(wrapper.emitted("like")).toBeUndefined();
  });
});
