import type { DialogApi } from "naive-ui";

/**
 * 不可逆操作的统一二次确认（票 435 / spec 423 第 23 项；票 437 评审收口）。
 *
 * 全站删除类动作只有一个口径：**`dialog.warning` + 「动作 / 取消」**，点确认之前
 * 不发请求、不改本地状态。此前「删除会话 / 删除单条消息 / 清空分组」四处各写一份
 * 近逐字重复的 `dialog.warning({...})`，文案与按钮名已经开始分叉；这里把结构收成
 * 唯一来源，调用方只给标题、正文与真正要执行的动作。
 *
 * 为什么是工具函数而不是 composable：它不持有状态、不需要响应式，`useDialog()` 的
 * 实例由页面 / 组件传进来（也便于测试直接断言 `dialog.warning` 收到的参数）。
 */
export interface DestructiveConfirmOptions {
  /** 对话框标题（动词开头，如「删除消息」「清空分组」）。 */
  title: string;
  /** 正文：必须说清"不可恢复"，以及这次动作影响的范围。 */
  content: string;
  /** 确认按钮文案；默认「删除」。 */
  confirmText?: string;
  /** 取消按钮文案；默认「取消」。 */
  cancelText?: string;
  /** 点确认后执行的动作（异步也行，对话框不等它）。 */
  onConfirm: () => void | Promise<void>;
}

/** 弹出统一口径的二次确认；取消时什么都不发生。 */
export function confirmDestructive(dialog: DialogApi, options: DestructiveConfirmOptions): void {
  dialog.warning({
    title: options.title,
    content: options.content,
    positiveText: options.confirmText ?? "删除",
    negativeText: options.cancelText ?? "取消",
    onPositiveClick: () => options.onConfirm(),
  });
}
