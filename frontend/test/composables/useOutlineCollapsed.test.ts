import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it } from "vitest";
import { defineComponent, nextTick, ref, type Component, type Ref } from "vue";

import {
  outlineCollapsedStorageKey,
  useOutlineCollapsed,
} from "~/composables/useOutlineCollapsed";
import { serializeOutlineExpanded } from "~/utils/outlineState";

/**
 * 目录树折叠状态与「当前是哪棵树」的绑定（#225）。
 *
 * 折叠状态按 `<kind>:<id>` 存在 localStorage。旧实现把 key 在 setup 时按路由
 * 参数算死一次：站内从 `/collections/1` 切到 `/collections/2` 时页面组件**不会
 * 重新挂载**，于是新合集沿用旧合集的折叠状态——读者会看到自己从未收起的文件夹
 * 莫名消失（反过来也一样），而刷新页面又一切正常，属于最难复现的那类。
 *
 * 这里测的是接线而不是纯函数：挂载时读 key、参数变化后换 key、折叠写回当前 key。
 */

type CollapsedApi = ReturnType<typeof useOutlineCollapsed>;

/** 把 composable 放进真实的组件实例里（onMounted 与 watch 才有效）。 */
function mountCollapsed(
  kind: string | Ref<string>,
  ownerId: string | Ref<string>,
): { wrapper: ReturnType<typeof mount>; api: CollapsedApi } {
  let api: CollapsedApi | undefined;
  const Comp: Component = defineComponent({
    setup() {
      api = useOutlineCollapsed(kind, ownerId);
      return () => null;
    },
  });
  const wrapper = mount(Comp);
  return { wrapper, api: api as CollapsedApi };
}

function seed(kind: string, ownerId: string, collapsed: readonly string[]): void {
  window.localStorage.setItem(
    outlineCollapsedStorageKey(kind, ownerId),
    serializeOutlineExpanded(new Set(collapsed)),
  );
}

function stored(kind: string, ownerId: string): string[] | null {
  const raw = window.localStorage.getItem(outlineCollapsedStorageKey(kind, ownerId));
  if (raw === null) return null;
  return JSON.parse(raw) as string[];
}

describe("useOutlineCollapsed 的存储 key", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("挂载时读当前树的折叠状态", () => {
    seed("collection", "1", ["f-chapter-1"]);

    const { api } = mountCollapsed("collection", "1");

    expect([...api.collapsed.value]).toEqual(["f-chapter-1"]);
  });

  it("路由参数变化后换成新的那棵树的折叠状态（不沿用旧 key）", async () => {
    seed("collection", "1", ["f-from-collection-1"]);
    seed("collection", "2", ["f-from-collection-2"]);
    const ownerId = ref("1");

    const { api } = mountCollapsed("collection", ownerId);
    expect([...api.collapsed.value]).toEqual(["f-from-collection-1"]);

    ownerId.value = "2";
    await nextTick();

    expect([...api.collapsed.value]).toEqual(["f-from-collection-2"]);
  });

  it("切换后折叠写回新 key，旧 key 保持原样", async () => {
    seed("collection", "1", ["f-from-collection-1"]);
    const ownerId = ref("1");

    const { api } = mountCollapsed("collection", ownerId);
    ownerId.value = "2";
    await nextTick();

    api.toggle("f-new");
    await nextTick();

    expect(stored("collection", "2")).toEqual(["f-new"]);
    expect(stored("collection", "1")).toEqual(["f-from-collection-1"]);
  });

  it("参数变化时不把旧状态写进新 key（串状态的那条回归）", async () => {
    seed("collection", "1", ["f-from-collection-1"]);
    const ownerId = ref("1");

    mountCollapsed("collection", ownerId);
    ownerId.value = "2";
    await nextTick();

    // 新合集从来没有折叠过：它的 key 里绝不能出现旧合集的 f-from-collection-1。
    expect(stored("collection", "2") ?? []).not.toContain("f-from-collection-1");
  });

  it("kind 与 ownerId 都随参数变化（阅读页左栏换树同理）", async () => {
    seed("project", "7", ["p-1"]);
    seed("collection", "7", ["c-1"]);
    const kind = ref("project");
    const ownerId = ref("7");

    const { api } = mountCollapsed(kind, ownerId);
    expect([...api.collapsed.value]).toEqual(["p-1"]);

    kind.value = "collection";
    await nextTick();

    expect([...api.collapsed.value]).toEqual(["c-1"]);
  });
});
