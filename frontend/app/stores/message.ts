import { computed, ref, shallowRef, watch } from "vue";
import { defineStore } from "pinia";
import { getMessageSummary } from "~/services/user";
import type { MessageSummary } from "~/types/api";

const DEFAULT_SUMMARY: MessageSummary = {
  comment_msg_count: 0,
  digg_favor_msg_count: 0,
  private_msg_count: 0,
  system_msg_count: 0,
  global_msg_count: 0,
};

function normalizeSummary(summary: MessageSummary): MessageSummary {
  return {
    ...DEFAULT_SUMMARY,
    ...summary,
    global_msg_count: summary.global_msg_count ?? 0,
  };
}

export const useMessageStore = defineStore("message", () => {
  const summary = ref<MessageSummary>({ ...DEFAULT_SUMMARY });
  const pending = shallowRef(false);
  const fetched = shallowRef(false);

  const totalUnread = computed(
    () =>
      summary.value.comment_msg_count +
      summary.value.digg_favor_msg_count +
      summary.value.private_msg_count +
      summary.value.system_msg_count +
      summary.value.global_msg_count,
  );

  // 同一次登录里可能有好几处同时要刷新（本 store 的登录 watch、studio layout 的兜底、
  // 收件箱页面的主动刷新）。复用同一个在途请求，避免刚登录就发两次一模一样的 GET。
  let inflight: Promise<MessageSummary> | null = null;

  async function refreshSummary() {
    const authStore = useAuthStore();
    if (!authStore.isLoggedIn) {
      summary.value = { ...DEFAULT_SUMMARY };
      fetched.value = true;
      return summary.value;
    }

    if (inflight) {
      return inflight;
    }

    pending.value = true;
    const task = (async () => {
      const next = normalizeSummary(await getMessageSummary());
      // 请求期间如果登出了，结果必须丢弃：否则 clear() 之后又被写回上一个账号的未读数。
      if (!useAuthStore().isLoggedIn) {
        return summary.value;
      }
      summary.value = next;
      fetched.value = true;
      return next;
    })();
    inflight = task;

    try {
      return await task;
    } finally {
      // 只清自己：clear() 可能已经把 inflight 换掉（登出后立刻换账号登录），
      // 无脑置 null 会把新请求的登记抹掉，导致下一次又多打一个请求。
      if (inflight === task) {
        inflight = null;
      }
      pending.value = false;
    }
  }

  function clear() {
    summary.value = { ...DEFAULT_SUMMARY };
    fetched.value = false;
    // 丢掉在途请求的登记：登出后立刻换账号登录时，不该复用上一个账号那个"注定被丢弃"的请求。
    inflight = null;
  }

  // 登录态一变就自己跟上 —— 不再由 auth store 反过来调用本 store（见 baseline §42）。
  //
  // flush:"sync"：登出时要在界面重渲染之前就清空，否则会闪一帧上一个账号的未读数。
  // 不给 immediate：带着有效 token 直接进站的场景没有"变化"可言，
  // 由 studio layout 的 !fetched 兜底拉一次（见 app/layouts/studio.vue）。
  watch(
    () => useAuthStore().isLoggedIn,
    (loggedIn) => {
      if (loggedIn) {
        void refreshSummary().catch(() => undefined);
      } else {
        clear();
      }
    },
    { flush: "sync" },
  );

  return {
    summary,
    pending,
    fetched,
    totalUnread,
    refreshSummary,
    clear,
  };
});
