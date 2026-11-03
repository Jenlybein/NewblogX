import { watch } from "vue";

import { isNewChatMessageNotification } from "~/utils/chat";

/**
 * 全局未读实时订阅（票 #240）。
 *
 * 为什么是"全局"而不是"某个页面"：
 *   - 旧实现把 WS 连接建在私信页的 composable 里，并在 `onBeforeUnmount` 关掉，
 *     于是"离开消息中心"= 断开实时通道 —— 顶栏未读数只能等下次进页面才更新；
 *   - 现在连接属于**整个登录会话**：登录后建连、登出才断开（连接本身由
 *     `useChatSocket` 的模块级单例持有，多处调用共享一条）。
 *
 * 未读为什么在本地先 +1：WS 通知只说明"已落库一条新消息"，不带正文也不带汇总。
 * 等 `GET /api/sitemsg/user` 往返回来，顶栏数字会慢半拍；所以先 +1 保证"到达即
 * 可见"，随后立刻刷新汇总，把数字校准成服务端真相（例如对方在另一端已读）。
 *
 * 重复调用是安全的：模块级 `installed` 保证只装一份订阅，未读不会被 +两次。
 */

// 模块级单例：插件装一次，页面 / 页头再调用也不重复订阅。
let installed = false;

export function useMessageRealtime() {
  if (installed) {
    return;
  }
  installed = true;

  const authStore = useAuthStore();
  const messageStore = useMessageStore();
  const socket = useChatSocket();

  socket.onNotification((notification) => {
    // 已读回执不是"又收到一条"：它也会让发送方 +1 的话，数字会越聊越大。
    if (!isNewChatMessageNotification(notification)) {
      return;
    }

    messageStore.bumpPrivateUnread();
    // 校准：服务端汇总才是真相（多端已读 / 上限裁剪都由它说了算）。
    // 不 await：未读已经本地更新，网络慢不该挡住界面。
    void messageStore.refreshSummary().catch(() => undefined);
  });

  // immediate：账号已经登录着进站（带着有效 token）时也要建连。
  watch(
    () => authStore.isLoggedIn,
    (loggedIn) => {
      if (loggedIn) {
        void socket.connect();
        return;
      }
      socket.close();
    },
    { immediate: true },
  );
}
