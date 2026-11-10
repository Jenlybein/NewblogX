import { readFileSync } from "node:fs";
import { join } from "node:path";
import { mount } from "@vue/test-utils";
import { NAvatar } from "naive-ui";
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

  it("默认渲染成正圆（round 默认 true）", () => {
    const wrapper = mount(AppAvatar, {
      props: {
        src: "https://image.example.com/u.png",
        name: "Alice",
      },
    });

    expect(wrapper.findComponent(NAvatar).props("round")).toBe(true);
  });

  it("头像图片按短边裁切、头像盒不被 flex 压缩（不会变椭圆或被拉宽）", () => {
    // CSS 不会被 happy-dom 计算，所以这一条钉的是样式契约本身：
    //   - 没有 object-fit: cover 时，naive-ui 的 <img> 默认 fill 会把非 1:1 的
    //     旧头像/第三方头像横向拉宽（阅读页作者卡与个人资料页都踩过）；
    //   - 没有 flex: 0 0 auto 时，作者卡在窄栏里会把头像宽度压缩，
    //     border-radius: 50% 就画成椭圆。
    const source = readFileSync(join(process.cwd(), "app/components/common/AppAvatar.vue"), "utf8");

    expect(source).toMatch(/object-fit:\s*cover/u);
    expect(source).toMatch(/flex:\s*0 0 auto/u);
  });
});
