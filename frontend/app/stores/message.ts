import { computed, ref, shallowRef } from "vue";
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

  async function refreshSummary() {
    const authStore = useAuthStore();
    if (!authStore.isLoggedIn) {
      summary.value = { ...DEFAULT_SUMMARY };
      fetched.value = true;
      return summary.value;
    }

    pending.value = true;
    try {
      summary.value = normalizeSummary(await getMessageSummary());
      fetched.value = true;
      return summary.value;
    } finally {
      pending.value = false;
    }
  }

  function clear() {
    summary.value = { ...DEFAULT_SUMMARY };
    fetched.value = false;
  }

  return {
    summary,
    pending,
    fetched,
    totalUnread,
    refreshSummary,
    clear,
  };
});
