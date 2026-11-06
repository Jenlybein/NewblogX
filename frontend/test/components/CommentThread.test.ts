import { readFileSync } from "node:fs";
import { join } from "node:path";

import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import AppAvatar from "~/components/common/AppAvatar.vue";
import CommentThread from "~/components/comment/CommentThread.vue";
import type { CommentReplyItem, CommentRootItem } from "~/types/api";

import { blockAfter } from "../helpers/designFiles";

/** 只保留 NuxtLink 的对外语义：真实渲染成带 href 的 `<a>`（测试要断言链接地址）。 */
const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(props, { slots }) {
    return () => h("a", { href: String(props.to) }, slots.default?.());
  },
});

/**
 * 评论楼层级（#238 范围 1/2/4/5）。
 *
 * 这里只测组件的对外协议（渲染了什么、点了发什么事件、无障碍名是什么），
 * 不碰页面怎么处理事件（那是 `useArticleComments.test.ts` 的事）：
 *
 *  - 子评论点赞必须**可点**（此前是纯文本 `<span>点赞 N</span>`，后端其实早就支持二级点赞）；
 *  - 子评论「回复」要发**被回复的那条评论 id**（此前发根评论 id，回复落错楼层）；
 *  - 点赞/回复是图标版：实心/空心两态 + 数字、回复箭头，全部带 `aria-label` 且是原生按钮；
 *  - 「展开 N 条回复」是胶囊按钮（浅底 + 主题色文字 + chevron）；
 *  - 作者区（头像 + 昵称同一个链接进个人主页）也是对外协议的一部分（票 292）。
 */

function makeComment(overrides: Partial<CommentRootItem> = {}): CommentRootItem {
  return {
    id: "c1",
    user_id: "u1",
    user_nickname: "读者甲",
    user_avatar: "",
    content: "这是一条评论",
    created_at: "2026-10-30T10:00:00.000Z",
    digg_count: 3,
    is_digg: false,
    reply_count: 2,
    status: 1,
    ...overrides,
  } as CommentRootItem;
}

function makeReply(overrides: Partial<CommentReplyItem> = {}): CommentReplyItem {
  return {
    id: "r1",
    user_id: "u2",
    user_nickname: "读者乙",
    user_avatar: "",
    content: "楼中楼回复",
    created_at: "2026-10-30T11:00:00.000Z",
    digg_count: 1,
    is_digg: false,
    status: 1,
    reply_user_nickname: "读者甲",
    ...overrides,
  } as CommentReplyItem;
}

function mountThread(comments: CommentRootItem[]) {
  return mount(CommentThread, {
    props: { comments },
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

function mountWithReplies(comment: Partial<CommentRootItem> = {}, replies: CommentReplyItem[] = [makeReply()]) {
  return mountThread([{ ...makeComment(comment), replies }] as CommentRootItem[]);
}

/** 展开楼层的回复，返回渲染后的 wrapper（回复行默认收起）。 */
async function mountExpanded(comment: Partial<CommentRootItem> = {}, replies: CommentReplyItem[] = [makeReply()]) {
  const wrapper = mountWithReplies(comment, replies);
  await wrapper.get(".comment-capsule").trigger("click");
  return wrapper;
}

describe("CommentThread（评论楼层级）", () => {
  it("渲染楼层：作者、内容、状态标签与操作按钮", () => {
    const wrapper = mountThread([makeComment()]);

    expect(wrapper.findAll(".comment")).toHaveLength(1);
    expect(wrapper.get(".comment__author").text()).toBe("读者甲");
    expect(wrapper.get(".comment__content").text()).toContain("这是一条评论");
    // 待审核状态用语义写法的独立标签（不再借用 Naive 的 warning 橙色）。
    expect(wrapper.get(".comment__status").text()).toBe("审核中");
    // 根评论的操作区：点赞 + 回复 + 展开胶囊（不再有纯文本操作）。
    expect(wrapper.findAll(".comment-action")).toHaveLength(2);
    expect(wrapper.find(".comment-capsule").exists()).toBe(true);
  });

  it("根评论点赞是图标按钮：空心/实心两态 + 数字，aria-label 两态清晰", async () => {
    const wrapper = mountThread([makeComment({ is_digg: false, digg_count: 7 })]);
    const diggButton = wrapper.get(".comment-action--digg");

    expect(diggButton.element.tagName).toBe("BUTTON");
    expect(diggButton.attributes("type")).toBe("button");
    expect(diggButton.attributes("aria-label")).toBe("点赞");
    expect(diggButton.attributes("aria-pressed")).toBe("false");
    expect(diggButton.get(".comment-action__count").text()).toBe("7");
    // 未点赞是空心图标（filled 变体带 -filled 的名字，通过 svg 的 aria-hidden 判断图标存在即可）。
    expect(diggButton.findAll("svg")).toHaveLength(1);

    await diggButton.trigger("click");
    expect(wrapper.emitted("digg")?.[0]).toEqual(["c1", false]);

    const digged = mountThread([makeComment({ is_digg: true })]);
    expect(digged.get(".comment-action--digg").attributes("aria-label")).toBe("取消点赞");
    expect(digged.get(".comment-action--digg").attributes("aria-pressed")).toBe("true");
  });

  it("回复按钮是箭头图标 + 原生按钮，点了发根评论 id", async () => {
    const wrapper = mountThread([makeComment()]);
    const replyButton = wrapper.get(".comment-action--reply");

    expect(replyButton.element.tagName).toBe("BUTTON");
    expect(replyButton.attributes("aria-label")).toBe("回复");
    expect(replyButton.find("svg").exists()).toBe(true);
    expect(replyButton.text()).toContain("回复");

    await replyButton.trigger("click");
    expect(wrapper.emitted("reply")?.[0]).toEqual(["c1"]);
  });

  it("「展开 N 条回复」是胶囊按钮：浅底 + 主题色文字 + chevron，展开后文案与 aria 状态一起变", async () => {
    const wrapper = mountThread([makeComment({ reply_count: 2 })]);
    const capsule = wrapper.get(".comment-capsule");

    expect(capsule.element.tagName).toBe("BUTTON");
    expect(capsule.attributes("aria-expanded")).toBe("false");
    expect(capsule.text()).toContain("展开 2 条回复");
    expect(capsule.find(".comment-capsule__chevron").exists()).toBe(true);

    await capsule.trigger("click");

    expect(wrapper.emitted("loadReplies")?.[0]).toEqual(["c1"]);
    expect(wrapper.get(".comment-capsule").text()).toContain("收起回复");
    expect(wrapper.get(".comment-capsule").attributes("aria-expanded")).toBe("true");
  });

  it("没有回复时不出现展开胶囊", () => {
    const wrapper = mountThread([makeComment({ reply_count: 0 })]);

    expect(wrapper.find(".comment-capsule").exists()).toBe(false);
  });

  it("展开后回复列表按嵌套层级渲染", async () => {
    const wrapper = await mountExpanded({ reply_count: 1 });

    expect(wrapper.findAll(".comment")).toHaveLength(2);
    expect(wrapper.get(".comment--reply").text()).toContain("楼中楼回复");
  });
});

describe("CommentThread：子评论互动（不再落到根楼层）", () => {
  it("子评论点赞可点：按**子评论 id** 发 digg，并带上它自己的点赞态", async () => {
    const wrapper = await mountExpanded();
    const replyRow = wrapper.get(".comment--reply");
    const diggButton = replyRow.get(".comment-action--digg");

    expect(diggButton.element.tagName).toBe("BUTTON");
    expect(diggButton.attributes("aria-label")).toBe("点赞");
    expect(diggButton.get(".comment-action__count").text()).toBe("1");

    await diggButton.trigger("click");

    expect(wrapper.emitted("digg")?.[0]).toEqual(["r1", false]);
  });

  it("子评论已点赞：实心图标与「取消点赞」语义，点后发取消", async () => {
    const wrapper = await mountExpanded({}, [makeReply({ is_digg: true, digg_count: 4 })]);
    const diggButton = wrapper.get(".comment--reply").get(".comment-action--digg");

    expect(diggButton.attributes("aria-label")).toBe("取消点赞");
    expect(diggButton.attributes("aria-pressed")).toBe("true");

    await diggButton.trigger("click");

    expect(wrapper.emitted("digg")?.[0]).toEqual(["r1", true]);
  });

  it("子评论「回复」发的是子评论 id（不是根评论 c1）", async () => {
    const wrapper = await mountExpanded();

    await wrapper.get(".comment--reply").get(".comment-action--reply").trigger("click");

    expect(wrapper.emitted("reply")?.[0]).toEqual(["r1"]);
  });
});

describe("CommentThread：作者区进个人主页（票 292）", () => {
  it("根评论：头像与昵称在**同一个**链接里，指向 /users/{user_id}", () => {
    const wrapper = mountThread([makeComment({ user_id: "u1" })]);
    const link = wrapper.get(".comment__author-link");

    expect(link.element.tagName).toBe("A");
    expect(link.attributes("href")).toBe("/users/u1");
    // 头像和昵称同属一个链接（不是两个各自可点的入口），链接的可访问名就是昵称。
    expect(link.findComponent(AppAvatar).exists()).toBe(true);
    expect(link.get(".comment__author").text()).toBe("读者甲");
    expect(wrapper.findAll("a.comment__author-link")).toHaveLength(1);
  });

  it("子评论：作者区同样指向子评论作者的个人主页", async () => {
    const wrapper = await mountExpanded({}, [makeReply({ user_id: "u2" })]);
    const link = wrapper.get(".comment--reply").get(".comment__author-link");

    expect(link.element.tagName).toBe("A");
    expect(link.attributes("href")).toBe("/users/u2");
    expect(link.findComponent(AppAvatar).exists()).toBe(true);
    expect(link.get(".comment__author").text()).toBe("读者乙");
  });

  it("根评论缺失 user_id：退化为纯文本，不可点、不可聚焦、读屏也不当链接", () => {
    const wrapper = mountThread([makeComment({ user_id: "" })]);
    const author = wrapper.get(".comment__author-link");

    // 不是 <a>，没有 href，也不靠 tabindex 进 Tab 序（纯文本 = 不可聚焦）。
    expect(author.element.tagName).toBe("SPAN");
    expect(author.attributes("href")).toBeUndefined();
    expect(author.attributes("tabindex")).toBeUndefined();
    expect(author.attributes("role")).toBeUndefined();
    expect(wrapper.findAll("a")).toHaveLength(0);
    // 文案与墨色字不变：昵称仍是 .comment__author。
    expect(wrapper.get(".comment__author").text()).toBe("读者甲");
    expect(author.classes()).toContain("comment__author-link--static");
  });

  it("子评论缺失 user_id：同样退化为纯文本", async () => {
    const wrapper = await mountExpanded({}, [makeReply({ user_id: "" })]);
    const author = wrapper.get(".comment--reply").get(".comment__author-link");

    expect(author.element.tagName).toBe("SPAN");
    expect(author.attributes("href")).toBeUndefined();
    expect(author.attributes("tabindex")).toBeUndefined();
    // 只看这一层回复：根评论自己的作者链接照旧存在。
    expect(wrapper.get(".comment--reply").findAll("a")).toHaveLength(0);
    expect(wrapper.get(".comment--reply").get(".comment__author").text()).toBe("读者乙");
  });

  it("互动按钮不在作者链接里：点赞与「回复」照常发事件，不被外层链接抢走", async () => {
    const wrapper = mountThread([makeComment({ user_id: "u1" })]);
    const link = wrapper.get(".comment__author-link");

    expect(link.find(".comment-action--digg").exists()).toBe(false);
    expect(link.find(".comment-action--reply").exists()).toBe(false);

    await wrapper.get(".comment-action--digg").trigger("click");
    expect(wrapper.emitted("digg")?.[0]).toEqual(["c1", false]);

    await wrapper.get(".comment-action--reply").trigger("click");
    expect(wrapper.emitted("reply")?.[0]).toEqual(["c1"]);
  });
});

describe("CommentThread：作者链接的样式口径（票 292）", () => {
  const css = readFileSync(join(process.cwd(), "app/assets/css/tailwind.css"), "utf8");

  it("hover 出现下划线，字色仍是墨色（不改链接色）", () => {
    const block = blockAfter(css, ".comment__author-link:hover {");

    expect(block).toContain("text-decoration: underline");
    // 链接态不许改成功能蓝：评论区的身份入口保持正文墨色。
    expect(blockAfter(css, ".comment__author-link {")).toContain("text-ink");
  });

  it("键盘聚焦时焦点环沿用全站 --color-focus", () => {
    expect(blockAfter(css, ".comment__author-link:focus-visible {")).toContain("var(--color-focus)");
  });

  it("退化态没有 hover 下划线（不是死链的假反馈）", () => {
    expect(css).not.toContain(".comment__author-link--static:hover");
  });
});

/** 条目上的内联延迟：它是「逐条淡入」的唯一节奏来源（CSS 里不许写 40ms 字面量）。 */
function delayOf(item: { element: Element }): string {
  return (item.element as HTMLElement).style.transitionDelay;
}

/** 把 matchMedia 伪装成「系统要求减少动态」。 */
function stubReduceMotion() {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query.includes("prefers-reduced-motion"),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    onchange: null,
    dispatchEvent: () => false,
  }));
}

function mountRepliesPage(replies: CommentReplyItem[], page = 1) {
  return mount(CommentThread, {
    props: {
      comments: [{ ...makeComment({ reply_count: 6 }), replies }] as CommentRootItem[],
      replyPages: { c1: page },
      replyHasPrevious: { c1: page > 1 },
      replyHasNext: { c1: page < 2 },
    },
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

/**
 * 子评论展开动效（票 294）。
 *
 * 外部行为契约（都在这里断死，不碰实现细节）：
 *  - 折叠态：容器常驻 DOM（高度动效要有东西可以做过渡），但对读屏与 Tab 是**不存在**的；
 *  - 展开先出骨架（3 行 = 每页 3 条），数据到了条目按索引错峰淡入；
 *  - 收起干脆、无逐条延迟；翻页只交叉淡入，容器高度不二次变化；
 *  - 减少动态：延迟归零、条目没有入场起始态（即时出现）。
 */
describe("CommentThread：子评论展开动效（票 294）", () => {
  it("折叠态：容器 inert + 对读屏隐藏，展开后两者一并撤掉", async () => {
    const wrapper = mountWithReplies({ reply_count: 3 });

    const collapsed = wrapper.get(".comment__replies");
    // inert：整棵子树不可聚焦、不进读屏遍历（原生语义，不靠 tabindex 逐个抠）。
    expect(collapsed.attributes("inert")).toBeDefined();
    expect(collapsed.attributes("aria-hidden")).toBe("true");
    expect(collapsed.classes()).not.toContain("comment__replies--open");
    // 里面确实有可聚焦的控件（翻页按钮）—— 它们此时被 inert 罩住，而不是不存在。
    expect(collapsed.findAll("button").length).toBeGreaterThan(0);

    await wrapper.get(".comment-capsule").trigger("click");

    const opened = wrapper.get(".comment__replies");
    expect(opened.attributes("inert")).toBeUndefined();
    expect(opened.attributes("aria-hidden")).toBeUndefined();
    expect(opened.classes()).toContain("comment__replies--open");
    // 胶囊的 aria-expanded 与文案是既有契约，动效不改它。
    expect(wrapper.get(".comment-capsule").attributes("aria-expanded")).toBe("true");
    expect(wrapper.get(".comment-capsule").text()).toContain("收起回复");
  });

  it("展开先出骨架：3 行，与每页 3 条回复一致，此时还没有条目", async () => {
    const wrapper = mount(CommentThread, {
      props: { comments: [makeComment({ reply_count: 3 })], loadingReplies: { c1: true } },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    await wrapper.get(".comment-capsule").trigger("click");

    const container = wrapper.get(".comment__replies");
    expect(container.findAll('[data-role="skeleton-bar"]')).toHaveLength(3);
    expect(wrapper.find(".comment--reply").exists()).toBe(false);
    // 骨架只是占位，读屏不该逐条读它；真正的加载说明走 sr-only 文案。
    expect(container.get('[data-role="skeleton"]').text()).toContain("回复正在加载");
  });

  it("数据到达：条目按索引写内联延迟（0 / 40 / 80ms），骨架退场", async () => {
    const wrapper = await mountExpanded({ reply_count: 3 }, [
      makeReply(),
      makeReply({ id: "r2" }),
      makeReply({ id: "r3" }),
    ]);

    const items = wrapper.findAll(".comment__replies-item");
    expect(items).toHaveLength(3);
    expect(items.map(delayOf)).toEqual(["0ms", "40ms", "80ms"]);
    expect(wrapper.find('[data-role="skeleton-bar"]').exists()).toBe(false);
  });

  it("减少动态：延迟归零、条目没有入场起始态（即时出现，不是慢慢淡）", async () => {
    stubReduceMotion();

    const wrapper = await mountExpanded({ reply_count: 3 }, [makeReply(), makeReply({ id: "r2" })]);

    const items = wrapper.findAll(".comment__replies-item");
    expect(items.map(delayOf)).toEqual(["0ms", "0ms"]);
    // 没有 opacity/transform 起始态 = 挂载即落位。
    expect((items[0]!.element as HTMLElement).style.opacity).toBe("");
    expect((items[0]!.element as HTMLElement).style.transform).toBe("");
  });

  it("收起：容器回到折叠态，条目不再带逐条延迟", async () => {
    const wrapper = await mountExpanded({ reply_count: 3 }, [makeReply(), makeReply({ id: "r2" })]);
    expect(wrapper.findAll(".comment__replies-item").map(delayOf)).toEqual(["0ms", "40ms"]);

    await wrapper.get(".comment-capsule").trigger("click");

    const container = wrapper.get(".comment__replies");
    expect(container.classes()).not.toContain("comment__replies--open");
    expect(container.attributes("inert")).toBeDefined();
    // 收起不排队：延迟一律归零（时长由 CSS 的 fast 档负责）。
    expect(wrapper.findAll(".comment__replies-item").map(delayOf)).toEqual(["0ms", "0ms"]);
  });

  it("翻页：容器不重挂载、高度不二次变化，新一页只交叉淡入（无逐条延迟）", async () => {
    const wrapper = mountRepliesPage([makeReply(), makeReply({ id: "r2" }), makeReply({ id: "r3" })]);
    await wrapper.get(".comment-capsule").trigger("click");

    const containerBefore = wrapper.get(".comment__replies").element;
    expect(wrapper.findAll(".comment__replies-item").map(delayOf)).toEqual(["0ms", "40ms", "80ms"]);

    await wrapper.findAll(".comment-page-button")[1]!.trigger("click");
    expect(wrapper.emitted("nextReplies")?.[0]).toEqual(["c1"]);

    await wrapper.setProps({
      comments: [{ ...makeComment({ reply_count: 6 }), replies: [makeReply({ id: "r4" }), makeReply({ id: "r5" })] }] as CommentRootItem[],
      replyPages: { c1: 2 },
      replyHasPrevious: { c1: true },
      replyHasNext: { c1: false },
    });

    const container = wrapper.get(".comment__replies");
    // 同一个 DOM 节点 = 容器没有被重建（重建才会重新播高度过渡）。
    expect(container.element).toBe(containerBefore);
    expect(container.classes()).toContain("comment__replies--open");
    // 高度只由 CSS 的 0fr/1fr 轨道决定，组件不写内联高度。
    expect((container.element as HTMLElement).style.height).toBe("");
    expect((container.element as HTMLElement).style.gridTemplateRows).toBe("");

    const items = wrapper.findAll(".comment__replies-item");
    expect(items).toHaveLength(2);
    expect(items.map(delayOf)).toEqual(["0ms", "0ms"]);
  });

  it("别的楼层到货不让已展开的楼层重播入场（每层只管自己这一批）", async () => {
    const wrapper = mount(CommentThread, {
      props: {
        comments: [
          { ...makeComment({ id: "c1", reply_count: 2 }), replies: [makeReply({ id: "r1" })] },
          { ...makeComment({ id: "c2", reply_count: 2 }), replies: [] },
        ] as CommentRootItem[],
      },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    const capsules = wrapper.findAll(".comment-capsule");
    await capsules[0]!.trigger("click");
    await capsules[1]!.trigger("click");
    const c1Row = wrapper.findAll(".comment--reply")[0]!;
    expect((c1Row.element as HTMLElement).style.opacity).toBe("1");

    await wrapper.setProps({
      comments: [
        { ...makeComment({ id: "c1", reply_count: 2 }), replies: [makeReply({ id: "r1" })] },
        { ...makeComment({ id: "c2", reply_count: 2 }), replies: [makeReply({ id: "r9" })] },
      ] as CommentRootItem[],
    });

    // c1 这一批没变：它不该被重新置成 opacity 0（否则会莫名闪一下）。
    expect((wrapper.findAll(".comment--reply")[0]!.element as HTMLElement).style.opacity).toBe("1");
    expect(wrapper.findAll(".comment--reply").length).toBe(2);
  });

  it("加载失败兜底仍可见：重试按钮在容器之外，不会随折叠容器一起消失", async () => {
    const wrapper = mountWithReplies({ reply_count: 3 }, []);

    await wrapper.get(".comment-capsule").trigger("click");

    expect(wrapper.get(".comment__empty-replies").text()).toContain("加载回复");
    expect(wrapper.find(".comment__replies .comment__empty-replies").exists()).toBe(false);
  });
});

describe("CommentThread：展开动效的样式口径（票 294）", () => {
  const css = readFileSync(join(process.cwd(), "app/assets/css/tailwind.css"), "utf8");

  it("折叠态是 0fr 轨道 + 透明，收起走 fast 档（120ms）", () => {
    const block = blockAfter(css, ".comment__replies {");

    expect(block).toContain("grid-template-rows: 0fr");
    expect(block).toContain("opacity: 0");
    expect(block).toContain("grid-template-rows var(--motion-fast) var(--motion-ease)");
  });

  it("展开态是 1fr 轨道 + 不透明，时长取 base 档（200ms）", () => {
    const block = blockAfter(css, ".comment__replies--open {");

    expect(block).toContain("grid-template-rows: 1fr");
    expect(block).toContain("opacity: 1");
    expect(block).toContain("transition-duration: var(--motion-base)");
  });

  it("内层裁剪让 0fr 轨道真的收成 0 高度", () => {
    const block = blockAfter(css, ".comment__replies-clip {");

    expect(block).toContain("overflow: hidden");
    expect(block).toContain("min-height: 0");
  });

  it("条目只动 opacity / transform，不碰容器高度", () => {
    const block = blockAfter(css, ".comment__replies-item {");

    expect(block).toContain("opacity var(--motion-base)");
    expect(block).toContain("transform var(--motion-base)");
    expect(block).not.toContain("grid-template-rows");
  });

  it("逐条延迟只由 motion.ts 算：CSS 无 40ms 字面量、无 --motion-stagger 变量", () => {
    const region = css.slice(css.indexOf(".comment__replies {"), css.indexOf(".comment__pagination {"));

    expect(css).not.toContain("--motion-stagger");
    expect(region).not.toContain("40ms");
  });

  it("减少动态：这一区域自己带兜底（整块 transition: none）", () => {
    const region = css.slice(css.indexOf(".comment__replies {"), css.indexOf(".comment__pagination {"));

    expect(region).toContain("@media (prefers-reduced-motion: reduce)");
    expect(region).toContain("transition: none");
  });
});
