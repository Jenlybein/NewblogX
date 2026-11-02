import { computed } from "vue";

import { useAuthStore } from "~/stores/auth";
import { useSiteStore } from "~/stores/site";
import {
  articleWritePolicy,
  editorReadOnly,
  writeEntryVisible,
  writePolicyNotices,
} from "~/utils/writePolicy";

/** 后端 role 列里管理员的取值（1 管理员 / 2 注册用户）。 */
const ROLE_ADMIN = 1;

/**
 * 读者站对站点「发布与展示」两个写作开关的消费入口（#64）。
 *
 * 只有一处读站点配置：组件不再各自去翻 `runtimeConfig.article`，
 * 否则"发布关了就隐藏入口"这类判断会散到页头、个人中心、写作页三份。
 *
 * 顺带补一次公开引导取数：创作页（`layout: write`）不经过 default/studio 布局，
 * 不在引导批次里，直接打开编辑器时配置会是空的。store 内部有单飞闸门，
 * 与布局里的调用重复也不会多发请求。
 */
export function useArticleWritePolicy() {
  const siteStore = useSiteStore();
  const authStore = useAuthStore();
  void siteStore.ensurePublicBootstrap();

  // 站点开关限制的是**非管理员**，管理员在后端被短路放行。
  // 前端的读法必须与服务端一致，否则关掉开关会把管理员自己也锁成只读。
  const isAdmin = computed(() => authStore.currentUser?.role === ROLE_ADMIN);

  const policy = computed(() => articleWritePolicy(siteStore.runtimeConfig, { isAdmin: isAdmin.value }));
  const entryVisible = computed(() => writeEntryVisible(policy.value));
  const readOnly = computed(() => editorReadOnly(policy.value));
  const notices = computed(() => writePolicyNotices(policy.value));

  return { policy, entryVisible, readOnly, notices, isAdmin };
}
