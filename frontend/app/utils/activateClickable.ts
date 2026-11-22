/**
 * 让带 `role="button"` 的 `<label>`（CSS checkbox 开关）也能被键盘激活。
 *
 * 原生 label 只响应鼠标点击；补 tabindex 后焦点能到，但 Enter / Space 默认仍然
 * 什么都不做。这里把两个键翻译成一次点击，行为与鼠标一致，不再需要为每个开关
 * 复制一遍 keydown 分支。
 */
export function activateClickable(event: KeyboardEvent): void {
  if (event.key !== "Enter" && event.key !== " ") return;
  event.preventDefault();
  (event.currentTarget as HTMLElement | null)?.click();
}
