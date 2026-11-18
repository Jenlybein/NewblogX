import { describe, expect, it, vi } from "vitest";

import { confirmDestructive } from "~/utils/confirmDestructive";

/**
 * 统一的不可逆操作确认助手（票 437 评审：四处近逐字重复收口）。
 *
 * 这里只钉结构与默认值：标题 / 正文由调用方给（每处的范围不同），
 * 固定的部分是 `dialog.warning` + 「动作 / 取消」+ 只有点确认才执行。
 */

function fakeDialog() {
  return { warning: vi.fn() };
}

describe("confirmDestructive：统一确认口径", () => {
  it("走 dialog.warning，默认按钮文案是「删除 / 取消」", () => {
    const dialog = fakeDialog();
    confirmDestructive(dialog as never, {
      title: "删除消息",
      content: "删除后不可恢复。",
      onConfirm: vi.fn(),
    });

    expect(dialog.warning).toHaveBeenCalledTimes(1);
    const options = dialog.warning.mock.calls[0]![0] as {
      title: string;
      content: string;
      positiveText: string;
      negativeText: string;
    };
    expect(options).toMatchObject({
      title: "删除消息",
      content: "删除后不可恢复。",
      positiveText: "删除",
      negativeText: "取消",
    });
  });

  it("点确认才执行回调；取消（negativeText）不挂任何副作用", async () => {
    const dialog = fakeDialog();
    const onConfirm = vi.fn();
    confirmDestructive(dialog as never, {
      title: "清空分组",
      content: "清空后不可恢复。",
      confirmText: "清空",
      onConfirm,
    });

    const options = dialog.warning.mock.calls[0]![0] as {
      positiveText: string;
      negativeText: string;
      onPositiveClick: () => Promise<void>;
      onNegativeClick?: () => void;
    };
    expect(options.positiveText).toBe("清空");
    expect(options.negativeText).toBe("取消");
    expect(onConfirm).not.toHaveBeenCalled();
    // 取消不提供回调：对话框关闭即结束，不动任何状态。
    expect(options.onNegativeClick).toBeUndefined();

    await options.onPositiveClick();
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
