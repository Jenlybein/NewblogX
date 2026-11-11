/**
 * 合集 / 项目分组与拖排的页面级接线（票 348）。
 *
 * 两个页面的这段逻辑完全一致，抽成 composable 而不是各写一份：
 *   - 读"卡片列表 + 分组树"，按分组树把卡片归位；
 *   - 拖完顺序后用 useSortPersist 做 10 秒防抖 + 离开前 flush；
 *   - 分组 CRUD 立即提交（不等防抖）；
 *   - 保存失败时把顺序恢复成上一次成功的快照。
 *
 * 归位规则（与 ADR-0041 一致）：分组树里有归属的卡片进对应分组；
 * 树里没有的（新建成还没进过分组）落到"未分组"，并接在未分组末尾 ——
 * 它们不该因为"没有归属行"就消失。
 */
import { ref, shallowRef } from 'vue'
import { useSortPersist } from '~/composables/useSortPersist'
import type { GroupedCardItem, GroupedGroup } from '~/components/articles/GroupedEntityGrid.vue'
import type { GroupTreeData } from '~/types/api'

export interface UseEntityGroupsOptions {
  kind: 'collection' | 'project'
  /** 读卡片列表（页面自己的 admin 列表接口）。 */
  loadCards: () => Promise<GroupedCardItem[]>
  /** 读分组树。 */
  loadGroups: () => Promise<GroupTreeData>
  /** 提交整树顺序。 */
  saveOrder: (payload: { groups: { id: string; items: string[] }[]; ungrouped: string[] }) => Promise<void>
  /** 分组 CRUD 立即提交的回调（页面负责弹窗与提示）。 */
  onError: (message: string) => void
}

export function useEntityGroups(options: UseEntityGroupsOptions) {
  const cards = ref<GroupedCardItem[]>([])
  const groups = ref<GroupedGroup[]>([])
  const ungrouped = ref<GroupedCardItem[]>([])
  const loading = ref(false)
  const tree = shallowRef<GroupTreeData | null>(null)

  /**
   * 上一次**提交成功**的顺序快照。
   *
   * 只存"分组 id + 组内 id"的结构，不存卡片本体：期间若有人改了标题或展示开关，
   * 那些字段变更不该被这次回滚覆盖。
   */
  let lastSaved: { groups: { id: string; items: string[] }[]; ungrouped: string[] } | null = null

  function snapshotOrder() {
    lastSaved = {
      groups: groups.value.map(g => ({ id: g.id, items: g.items.map(i => i.id) })),
      ungrouped: ungrouped.value.map(i => i.id),
    }
  }

  const sortPersist = useSortPersist({
    commit: async () => {
      await options.saveOrder(currentOrder())
      snapshotOrder()
    },
    onError: () => {
      options.onError('顺序保存失败，正在恢复上一次成功的顺序')
      restoreSnapshot()
    },
  })

  /** 当前界面上的整树快照（提交与回滚都用它）。 */
  function currentOrder() {
    return {
      groups: groups.value.map(g => ({ id: g.id, items: g.items.map(i => i.id) })),
      ungrouped: ungrouped.value.map(i => i.id),
    }
  }

  /** 按快照回滚：卡片本体从当前数据里按 id 找回，字段变更保留。 */
  function restoreSnapshot() {
    if (!lastSaved) return
    const byId = new Map(cards.value.map(card => [card.id, card]))
    groups.value = lastSaved.groups.map(group => ({
      id: group.id,
      title: groups.value.find(g => g.id === group.id)?.title ?? '',
      items: group.items.map(id => byId.get(id)).filter((c): c is GroupedCardItem => Boolean(c)),
    }))
    ungrouped.value = lastSaved.ungrouped.map(id => byId.get(id)).filter((c): c is GroupedCardItem => Boolean(c))
  }

  /** 把卡片按分组树归位。 */
  function applyTree(data: GroupTreeData | null) {
    const byId = new Map(cards.value.map(card => [card.id, card]))
    const placed = new Set<string>()

    const nextGroups: GroupedGroup[] = (data?.groups ?? []).map(group => {
      const items = (group.items ?? [])
        .map(item => byId.get(item.owner_id))
        .filter((card): card is GroupedCardItem => Boolean(card))
      items.forEach(card => placed.add(card.id))
      return { id: group.id, title: group.title, items }
    })

    const nextUngrouped = (data?.ungrouped ?? [])
      .map(item => byId.get(item.owner_id))
      .filter((card): card is GroupedCardItem => Boolean(card))
    nextUngrouped.forEach(card => placed.add(card.id))

    // 树里没有的卡片（新建成还没进过分组）落到未分组末尾。
    for (const card of cards.value) {
      if (!placed.has(card.id)) nextUngrouped.push(card)
    }

    groups.value = nextGroups
    ungrouped.value = nextUngrouped
    tree.value = data
  }

  async function refresh() {
    loading.value = true
    try {
      const [cardList, groupTree] = await Promise.all([options.loadCards(), options.loadGroups()])
      cards.value = cardList
      applyTree(groupTree)
      snapshotOrder()
    } catch {
      options.onError('列表加载失败')
    } finally {
      loading.value = false
    }
  }

  /** 拖动结束：标记脏，由 useSortPersist 做 10 秒防抖。 */
  function markOrderDirty() {
    sortPersist.markDirty()
  }

  /**
   * 落账拖拽后的排列（网格抛上来的整树快照）。
   *
   * 网格不直接改父组件的数据，所以这里负责把快照写回本地状态：
   * 顺序变化必须落在"提交时读的那份数据"上，否则拖完看起来动了、保存的还是旧顺序。
   */
  function applySnapshot(snapshot: { groups: { id: string; items: string[] }[]; ungrouped: string[] }) {
    const byId = new Map(cards.value.map(card => [card.id, card]))
    groups.value = snapshot.groups.map(group => ({
      id: group.id,
      title: groups.value.find(g => g.id === group.id)?.title ?? '',
      items: group.items.map(id => byId.get(id)).filter((c): c is GroupedCardItem => Boolean(c)),
    }))
    ungrouped.value = snapshot.ungrouped.map(id => byId.get(id)).filter((c): c is GroupedCardItem => Boolean(c))
    markOrderDirty()
  }



  /** 离开页面前把待提交的顺序发出去。 */
  async function flushOrder() {
    await sortPersist.flush()
  }


  return {
    cards,
    groups,
    ungrouped,
    loading,
    tree,
    sortState: sortPersist.state,
    refresh,
    markOrderDirty,
    applySnapshot,
    flushOrder,
    currentOrder,
    applyTree,
  }
}
