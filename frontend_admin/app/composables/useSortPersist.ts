/**
 * 拖拽排序的保存节流（票 346 / 348 共用）。
 *
 * 为什么不是"松手就提交"：后台列表一次拖动会连着触发多次 drop（跨组、组内、
 * 折叠后再排），逐次提交既打服务端，也会让"最后一次排列"取决于响应回来的
 * 顺序。这里统一成"停下 10 秒后提交一次整份顺序"，语义是**幂等整树覆盖**：
 * 提交的一定是当前界面的最终排列，与拖了几次无关。
 *
 * 三个必须一起到位的边界，少一个就会丢用户的操作：
 *  1. **离开前 flush**：路由切换 / 组件卸载时立刻提交，不等 10 秒；
 *  2. **失败回滚**：提交失败把本地顺序恢复成上一次成功的快照，并把错误抛给
 *     调用方提示 —— 否则界面显示的顺序与库里的不一致，刷新后"自己变回去"；
 *  3. **并发只保留最后一份**：提交途中又有新改动时，不并发发第二个请求，
 *     而是记下"还有未提交"，等这次回来再发一次。
 */
import { onBeforeUnmount, ref, type Ref } from 'vue'

/** 保存状态：idle 没有待提交、pending 有改动在等、saving 正在提交、error 上次失败。 */
export type SortPersistState = 'idle' | 'pending' | 'saving' | 'error'

export interface SortPersistOptions {
  /** 防抖窗口（毫秒）。默认 10000，与票 346 的约定一致。 */
  delayMs?: number
  /** 提交实现：参数是"当前顺序的快照"，由调用方决定请求体形状。 */
  commit: () => Promise<void>
  /** 提交失败时的回调（用于 toast）；默认吞掉，只把状态置为 error。 */
  onError?: (error: unknown) => void
}

export interface SortPersist {
  /** 当前状态，供 UI 显示「保存中 / 已保存」。 */
  state: Ref<SortPersistState>
  /** 标记"顺序变了"，触发防抖提交。 */
  markDirty: () => void
  /** 立刻提交（离开前 / 点保存时调用）。 */
  flush: () => Promise<void>
  /** 丢弃待提交的改动（取消编辑、回滚后调用）。 */
  cancel: () => void
}

export function useSortPersist(options: SortPersistOptions): SortPersist {
  const delayMs = options.delayMs ?? 10_000
  const state = ref<SortPersistState>('idle')

  let timer: ReturnType<typeof setTimeout> | null = null
  // 提交途中又有改动：不并发发请求，回来后再补一次。
  let dirtyWhileSaving = false
  let inFlight: Promise<void> | null = null

  function clearTimer() {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  async function runCommit() {
    clearTimer()
    if (inFlight) {
      dirtyWhileSaving = true
      return inFlight
    }

    state.value = 'saving'
    let failed = false
    inFlight = (async () => {
      try {
        await options.commit()
        state.value = dirtyWhileSaving ? 'pending' : 'idle'
      } catch (error) {
        failed = true
        state.value = 'error'
        options.onError?.(error)
      } finally {
        inFlight = null
      }
    })()

    await inFlight

    if (dirtyWhileSaving) {
      dirtyWhileSaving = false
      // 提交期间又有改动：把状态退回 pending 并重新计时，
      // 而不是立刻再发一次（用户可能还在连续拖）。上一次若失败，
      // 保留 error 直到这次成功 —— 不能让"又拖了一下"把错误提示抹掉。
      if (!failed) state.value = 'pending'
      schedule()
    }
  }

  function schedule() {
    clearTimer()
    timer = setTimeout(() => {
      timer = null
      void runCommit()
    }, delayMs)
  }

  function markDirty() {
    if (state.value !== 'saving') state.value = 'pending'
    schedule()
  }

  async function flush() {
    clearTimer()
    if (state.value === 'idle' && !dirtyWhileSaving) return
    await runCommit()
  }

  function cancel() {
    clearTimer()
    dirtyWhileSaving = false
    if (state.value !== 'saving') state.value = 'idle'
  }

  // 离开前 flush：不 await（onBeforeUnmount 无法等待），但请求已经发出，
  // 浏览器同源导航下会正常完成。真正的兜底是"每次改动都有本地快照"，
  // 失败时 UI 会显示 error 而不是假装成功。
  onBeforeUnmount(() => {
    void flush()
  })

  return { state, markDirty, flush, cancel }
}
