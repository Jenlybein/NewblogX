import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import InboxSitePanel from "~/components/inbox/InboxSitePanel.vue";
import type { SiteMessageItem } from "~/types/api";

const categories = [
  { key: 1 as const, label: "评论与回复", hint: "", count: 2 },
  { key: 2 as const, label: "点赞与收藏", hint: "", count: 0 },
  { key: "global" as const, label: "全局通知", hint: "", count: 1 },
];

const messageItem = {
  id: "m1",
  created_at: "2026-04-16T08:00:00.000Z",
  updated_at: "2026-04-16T08:00:00.000Z",
  type: 1,
  receiver_id: "u1",
  action_user_id: "u2",
  action_user_nickname: "River",
  action_user_avatar: "https://image.example.com/river.png",
  content: "评论了你的文章",
  article_id: "a1",
  comment_id: "c1",
  article_title: "文章标题",
  link_title: "评论提醒",
  link_herf: "/article/a1",
  is_read: false,
  read_at: null,
} satisfies SiteMessageItem;

function mountPanel(props: Partial<InstanceType<typeof InboxSitePanel>["$props"]> = {}) {
  return mount(InboxSitePanel, {
    props: {
      categories,
      activeGroup: 1,
      items: [messageItem],
      pending: false,
      hasMore: false,
      ...props,
    },
    global: {
      stubs: {
        NButton: {
          emits: ["click"],
          template: `<button @click="$emit('click')"><slot /></button>`,
        },
        NList: {
          template: `<div><slot /></div>`,
        },
        NListItem: {
          template: `<article><slot /><slot name="suffix" /></article>`,
        },
        NSpace: {
          template: `<span><slot /></span>`,
        },
        NTag: {
          template: `<span><slot /></span>`,
        },
        NThing: {
          props: ["title", "description"],
          template: `
            <section>
              <slot name="avatar" />
              <h3>{{ title }}</h3>
              <p>{{ description }}</p>
              <slot name="header-extra" />
              <slot name="footer" />
            </section>
          `,
        },
        NuxtLink: {
          props: ["to"],
          template: `<a data-testid="origin-link" :href="typeof to === 'string' ? to : to.path"><slot /></a>`,
        },
        StudioEmptyState: {
          props: ["title", "description"],
          template: `<div data-testid="empty">{{ title }} {{ description }}</div>`,
        },
      },
    },
  });
}

describe("InboxSitePanel", () => {
  it("renders message content, avatar url, unread state, and origin link", () => {
    const wrapper = mountPanel();

    expect(wrapper.text()).toContain("评论提醒");
    expect(wrapper.text()).toContain("评论了你的文章");
    expect(wrapper.text()).toContain("未读");
    expect(wrapper.get('img[data-image-src="https://image.example.com/river.png"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="origin-link"]').attributes("href")).toBe("/article/a1");
  });

  it("emits public actions from toolbar, category buttons, and delete buttons", async () => {
    const wrapper = mountPanel();
    const buttons = wrapper.findAll("button");

    await buttons[1]?.trigger("click");
    await buttons[3]?.trigger("click");
    await buttons[4]?.trigger("click");

    expect(wrapper.emitted("update:activeGroup")?.[0]).toEqual([2]);
    expect(wrapper.emitted("markAllRead")).toHaveLength(1);
    expect(wrapper.emitted("clearGroup")).toHaveLength(1);

    await buttons.at(-1)?.trigger("click");
    expect(wrapper.emitted("remove")?.[0]).toEqual(["m1"]);
  });

  it("emits loadMore when the rendered list can accept more items", async () => {
    const wrapper = mountPanel({ hasMore: true });

    await flushPromises();

    expect(wrapper.emitted("loadMore")).toHaveLength(1);
  });

  it("does not load more while a request is already pending", async () => {
    const wrapper = mountPanel({
      hasMore: true,
      pending: true,
    });

    await flushPromises();

    expect(wrapper.emitted("loadMore")).toBeUndefined();
    expect(wrapper.text()).toContain("正在加载更多消息");
  });
});
