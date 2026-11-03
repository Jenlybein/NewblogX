import { onMounted, ref, watch } from "vue";

import { parseOutlineExpanded, serializeOutlineExpanded } from "~/utils/outlineState";

/**
 * 目录树折叠状态（localStorage）。
 *
 * 树只在客户端取数：首次渲染用「全部展开」的骨架，挂载后读 localStorage，
 * 之后每次折叠都写回。不同树用 `<kind>:<id>` 分开存，避免同名文件夹串状态。
 * 存储的 key 是节点 ID（重命名不会丢状态），坏数据由纯函数兜住。
 */
export function outlineCollapsedStorageKey(kind: string, ownerId: string | number): string {
  return `reader-outline-collapsed:${kind}:${String(ownerId)}`;
}

export function useOutlineCollapsed(kind: string, ownerId: string | number) {
  const key = outlineCollapsedStorageKey(kind, ownerId);
  const collapsed = ref<Set<string>>(new Set());

  onMounted(() => {
    try {
      collapsed.value = parseOutlineExpanded(window.localStorage.getItem(key));
    } catch {
      // 隐私模式 / 禁用存储：保持默认展开，不让整个详情页 500。
      collapsed.value = new Set();
    }
  });

  watch(
    collapsed,
    (value) => {
      try {
        window.localStorage.setItem(key, serializeOutlineExpanded(value));
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
