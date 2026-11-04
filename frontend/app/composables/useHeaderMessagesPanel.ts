import { onBeforeUnmount, ref } from "vue";

/**
 * 顶栏「消息」悬浮面板的开合状态（2026-11-05 从 AppHeader 抽出）。
 *
 * 面板是触发按钮所在容器的子元素，但两者之间有一段透明间距：指针跨过去时
 * 会先触发 pointerleave。所以关闭走 200ms 宽限 + 重新进入取消，容器另补一条
 * 透明桥接带（CSS ::after）。健康状态由模板门控，异常才渲染。
 */
export const MESSAGES_CLOSE_DELAY_MS = 200;

export function useHeaderMessagesPanel() {
  const messagesOpen = ref(false);
  const messagesOpenTimer = ref<ReturnType<typeof setTimeout> | null>(null);
  const messagesTriggerRef = ref<{ $el?: HTMLElement } | HTMLElement | null>(null);

  function cancelMessagesClose() {
    if (messagesOpenTimer.value !== null) {
      clearTimeout(messagesOpenTimer.value);
      messagesOpenTimer.value = null;
    }
  }

  function openMessages() {
    cancelMessagesClose();
    messagesOpen.value = true;
  }

  function closeMessages() {
    cancelMessagesClose();
    messagesOpen.value = false;
  }

  function scheduleMessagesClose() {
    cancelMessagesClose();
    messagesOpenTimer.value = setTimeout(() => {
      messagesOpenTimer.value = null;
      messagesOpen.value = false;
    }, MESSAGES_CLOSE_DELAY_MS);
  }

  function handleMessagesFocusOut(event: FocusEvent) {
    const container = event.currentTarget as HTMLElement | null;
    const next = event.relatedTarget as Node | null;
    if (container && next && container.contains(next)) {
      cancelMessagesClose();
      return;
    }
    closeMessages();
  }

  function handleMessagesKeydown(event: KeyboardEvent) {
    if (event.key !== "Escape") return;
    closeMessages();
    const trigger = messagesTriggerRef.value;
    const element = trigger && "$el" in trigger ? trigger.$el : (trigger as HTMLElement | null);
    if (element && typeof element.focus === "function") element.focus();
  }

  function moveMessageFocus(event: KeyboardEvent, index: number, count: number) {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    event.preventDefault();
    const step = event.key === "ArrowDown" ? 1 : -1;
    const nextIndex = (index + step + count) % count;
    const container = event.currentTarget as HTMLElement | null;
    const items = container?.parentElement?.querySelectorAll<HTMLElement>('[role="menuitem"]');
    items?.[nextIndex]?.focus();
  }

  onBeforeUnmount(cancelMessagesClose);

  return {
    messagesOpen,
    messagesTriggerRef,
    openMessages,
    closeMessages,
    scheduleMessagesClose,
    handleMessagesFocusOut,
    handleMessagesKeydown,
    moveMessageFocus,
  };
}
