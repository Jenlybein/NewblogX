import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ProfileCategoryFormModal from "~/components/profile/ProfileCategoryFormModal.vue";

// 分类栏的新建 / 改名弹窗（票 325）。
//
// 只锁"用户在弹窗里看得见、点得到的东西"：标题随模式变、改名回填旧名字、
// 保存把输入原样交出去（去空白与空名校验在 composable 里，那层有用例）。
// 弹窗本身不做业务判断，也不自己发请求。

type ModalProps = {
  show?: boolean;
  mode?: "create" | "rename";
  initialTitle?: string;
  pending?: boolean;
};

function mountModal(props: ModalProps = {}) {
  return mount(ProfileCategoryFormModal, {
    props: {
      show: props.show ?? true,
      mode: props.mode ?? "create",
      initialTitle: props.initialTitle ?? "",
      pending: props.pending ?? false,
    },
    // NModal 内部是 Teleport：存根掉才能在同一棵树里断言弹窗内容。
    global: { stubs: { teleport: true } },
  });
}

describe("分类弹窗：新建与改名", () => {
  it("新建：标题是「新建分类」，输入框是空的，保存把输入交出去", async () => {
    const wrapper = mountModal();

    expect(wrapper.text()).toContain("新建分类");
    expect(wrapper.get('[data-role="category-form-title"] input').element).toHaveProperty("value", "");

    await wrapper.get('[data-role="category-form-title"] input').setValue("工程笔记");
    await wrapper.get('[data-role="category-form-save"]').trigger("click");

    expect(wrapper.emitted("submit")).toEqual([["工程笔记"]]);
  });

  it("改名：标题是「分类改名」，输入框回填当前名字，保存带上新名字", async () => {
    const wrapper = mountModal({ mode: "rename", initialTitle: "工程笔记" });

    expect(wrapper.text()).toContain("分类改名");
    expect(wrapper.get('[data-role="category-form-title"] input').element).toHaveProperty("value", "工程笔记");

    await wrapper.get('[data-role="category-form-title"] input').setValue("工程随笔");
    await wrapper.get('[data-role="category-form-save"]').trigger("click");

    expect(wrapper.emitted("submit")).toEqual([["工程随笔"]]);
  });

  it("取消：只要求关闭弹窗，不提交任何东西", async () => {
    const wrapper = mountModal({ mode: "rename", initialTitle: "工程笔记" });

    await wrapper.get('[data-role="category-form-cancel"]').trigger("click");

    expect(wrapper.emitted("submit")).toBeUndefined();
    expect(wrapper.emitted("update:show")).toEqual([[false]]);
  });

  it("提示了「同名会被拒绝」，用户不必自己猜为什么保存失败", () => {
    const wrapper = mountModal();

    expect(wrapper.text()).toContain("同名");
  });
});
