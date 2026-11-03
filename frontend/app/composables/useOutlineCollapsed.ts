import { computed, onMounted, ref, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

import { parseOutlineExpanded, serializeOutlineExpanded } from "~/utils/outlineState";

/**
 * 目录树折叠状态（localStorage）。
 *
 * 树只在客户端取数：首次渲染用「全部展开」的骨架，挂载后读 localStorage，
 * 之后每次折叠都写回。不同树用 `<kind>:<id>` 分开存，避免同名文件夹串状态。
 * 存储的 key 是节点 ID（重命名不会丢状态），坏数据由纯函数兜住。
 *
 * kind / ownerId 收成 `MaybeRefOrGetter`（#225）：旧实现把 key 在 setup 时算死
 * 一次，而站内从 `/collections/1` 切到 `/collections/2` 时页面组件**不会重新挂载**，
 * 于是新树沿用旧树的折叠状态——读者会看到自己从未收起的文件夹莫名消失，刷新又
 * 一切正常。现在 key 是 computed，参数一变就换成新树的存储；切换时也**不会**把
 * 旧树的状态写进新 key（那等于把串状态换了个方向）。
 */
export function outlineCollapsedStorageKey(kind: string, ownerId: string | number): string {
  return `reader-outline-collapsed:${kind}:${String(ownerId)}`;
}

export function useOutlineCollapsed(
  kind: MaybeRefOrGetter<string>,
  ownerId: MaybeRefOrGetter<string | number>,
) {
  const key = computed(() => outlineCollapsedStorageKey(toValue(kind), toValue(ownerId)));
  const collapsed = ref<Set<string>>(new Set());
  /**
   * 当前这份折叠状态属于哪个 key。
   *
   * 与 `key` 分开记：换树时 `key` 会先变，而 `collapsed` 里的还是旧树的状态。
   * 写回一律写 `activeKey`，否则一次换树 + 一次折叠会把旧树状态写进新 key
   * （把"串状态"换了个方向，更难查）。
   */
  let activeKey = key.value;
  /** 正在读新 key：这一刻的 `collapsed` 变更来自加载，不该回写。 */
  let loading = false;

  /** 读某个 key 的折叠状态；隐私模式 / 禁用存储时按"全部展开"处理。 */
  function readStorage(storageKey: string): Set<string> {
    try {
      return parseOutlineExpanded(window.localStorage.getItem(storageKey));
    } catch {
      // 隐私模式 / 禁用存储：保持默认展开，不让整个详情页 500。
      return new Set();
    }
  }

  /** 切到某个 key 并载入它的折叠状态。 */
  function load(storageKey: string): void {
    activeKey = storageKey;
    loading = true;
    collapsed.value = readStorage(storageKey);
    loading = false;
  }

  onMounted(() => {
    load(activeKey);
  });

  // 换树（路由参数变化）时重新读：不重挂载的站内跳转全靠这一步。
  // 同步刷新，保证"key 已经变了、状态还是旧的"这个中间态不跨过任何一次 flush。
  watch(key, load, { flush: "sync" });

  watch(
    collapsed,
    (value) => {
      if (loading) return;
      try {
        window.localStorage.setItem(activeKey, serializeOutlineExpanded(value));
      } catch {
        // 同上：写不进去只影响下次进入的展开状态，不影响本次阅读。
      }
    },
    { deep: true },
  );

  function toggle(nodeId: string): void {
    const next = new Set(collapsed.value);
    if (next.has(nodeId)) {
      next.delete(nodeId);
    } else {
      next.add(nodeId);
    }
    collapsed.value = next;
  }

  return { collapsed, toggle };
}
