import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import AppAvatar from "~/components/common/AppAvatar.vue";

describe("AppAvatar", () => {
  it("renders an image when an avatar url is available", () => {
    const wrapper = mount(AppAvatar, {
      props: {
        src: "https://image.example.com/u.png",
        name: "Alice",
        fallback: "我",
      },
    });

    expect(wrapper.get('img[data-image-src="https://image.example.com/u.png"]').exists()).toBe(true);
  });

  it("renders a stable initial when avatar url is empty", () => {
    const wrapper = mount(AppAvatar, {
      props: {
        src: "",
        name: "River",
        fallback: "我",
      },
    });

    expect(wrapper.get(".n-avatar").text()).toContain("R");
  });

  it("uses the provided fallback when both avatar and name are missing", () => {
    const wrapper = mount(AppAvatar, {
      props: {
        fallback: "系",
      },
    });

    expect(wrapper.get(".n-avatar").text()).toContain("系");
  });
});
