import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ProfileHeroCard from "~/components/profile/ProfileHeroCard.vue";
import type { UserBaseInfo } from "~/types/api";

const profile = {
  id: "u1",
  code_age: 3,
  avatar: "https://image.example.com/avatar.png",
  nickname: "管理员02",
  abstract: "简介",
  view_count: 12000,
  article_visited_count: 0,
  article_count: 5,
  fans_count: 6,
  follow_count: 7,
  favor_count: 8,
  digg_count: 9,
  comment_count: 10,
  favorites_visibility: true,
  followers_visibility: true,
  fans_visibility: true,
  home_style_id: null,
  relation: 0,
  place: "",
} satisfies UserBaseInfo;

function mountHero(props: Partial<InstanceType<typeof ProfileHeroCard>["$props"]> = {}) {
  return mount(ProfileHeroCard, {
    props: {
      profile,
      ...props,
    },
    global: {
      stubs: {
        IconEye: true,
        IconHeart: true,
        IconMessageCircle2: true,
        IconThumbUp: true,
      },
    },
  });
}

describe("ProfileHeroCard", () => {
  it("renders profile content and passes the avatar url to the avatar component", () => {
    const wrapper = mountHero({ abstractText: "公开简介", relationText: "关注作者" });

    expect(wrapper.text()).toContain("管理员02");
    expect(wrapper.text()).toContain("公开简介");
    expect(wrapper.text()).toContain("阅读 1.2w");
    expect(wrapper.get('img[data-image-src="https://image.example.com/avatar.png"]').exists()).toBe(true);
  });

  it("falls back to the first nickname character when avatar is empty", () => {
    const wrapper = mountHero({
      profile: {
        ...profile,
        avatar: "",
        nickname: "River",
      },
    });

    expect(wrapper.get(".n-avatar").text()).toContain("R");
  });

  it("disables the follow action when requested", () => {
    const wrapper = mountHero({
      relationText: "回关",
      actionDisabled: true,
    });

    const buttons = wrapper.findAll("button");
    expect(buttons).toHaveLength(2);
    expect(buttons.every((button) => button.attributes("disabled") != null)).toBe(true);
  });

  it("renders follow and private-message actions for other users", () => {
    const wrapper = mountHero({ relationText: "回关" });
    const buttons = wrapper.findAll("button");

    expect(buttons).toHaveLength(2);
    expect(buttons[0]?.text()).toContain("私信");
    expect(buttons[1]?.text()).toContain("回关");
  });

  it("emits follow for other users", async () => {
    const wrapper = mountHero({ relationText: "回关" });
    const buttons = wrapper.findAll("button");

    await buttons[1]!.trigger("click");
    expect(wrapper.emitted("follow")).toHaveLength(1);
  });

  it("emits message when the private-message action is clicked", async () => {
    const wrapper = mountHero({ relationText: "回关" });
    const buttons = wrapper.findAll("button");

    await buttons[0]!.trigger("click");
    expect(wrapper.emitted("message")).toHaveLength(1);
  });
});
