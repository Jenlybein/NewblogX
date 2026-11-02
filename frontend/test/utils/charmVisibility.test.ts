import { describe, expect, it } from "vitest";

import {
  CHARM_SHOW_DELAY_MS,
  charmVisibilityAdvance,
  charmVisibilityBusy,
  charmVisibilityEnter,
  charmVisibilityLeave,
  charmVisibilityVisible,
  createCharmVisibilityState,
  type CharmVisibilityState,
} from "~/utils/charmVisibility";
import { MOTION_DURATIONS_MS } from "~/utils/motion";

/**
 * 光标挂件进出场（渐隐 / 渐显 / 防抖）的状态机 —— 纯函数用例。
 *
 * 为什么值得测：这三条观感要求在接线里都极难断言，但全是时间上的状态迁移 ——
 * 「移出就渐隐」「移回满 0.1s 才渐显」「0.1s 内再次移出就取消显示」。
 * 把迁移写成纯函数之后，时钟是参数，不需要真浏览器、不需要真的等 0.1 秒。
 *
 * 关键口径（票面）：
 *   - 移出**立即**离开可见态（渐隐从这一刻开始，不是等渐隐结束才算不可见）；
 *   - 移回先进入「等待渐显」0.1 秒，**满 0.1 秒**才可见（边界值恰好生效）；
 *   - 等待期间再次移出 = 取消这次显示，**永远不会**借这一次移回而可见；
 *   - 沿窗口边缘反复进出、每次都短于 0.1 秒 → 一直不可见（不闪烁）；
 *   - 减少动态：直接切，不经过等待也不经过渐隐（不做过渡）。
 *
 * 首次登场是**例外**：页面打开后的第一次进入直接可见，不走 0.1 秒防抖 ——
 * 那时没有「移回」可言，而且它与既有的 200ms 淡入是配对的登场动作。
 */

/** 快档渐隐时长（ms）：与 `--motion-fast` / 动效预算同源，不写死数字。 */
const FADE_OUT_MS = MOTION_DURATIONS_MS.fast;

/** 从「已在屏幕上」的状态出发，便于每条用例只关心自己要测的那一跳。 */
function shown(): CharmVisibilityState {
  return charmVisibilityEnter(createCharmVisibilityState(), 1000);
}

/** 已经显示过、并已移出且渐隐走完（= 再次移回要防抖的那种隐藏）。 */
function hiddenAfterFirstShow(at = 2000): CharmVisibilityState {
  const leaving = charmVisibilityLeave(shown(), at - FADE_OUT_MS - 1);
  return charmVisibilityAdvance(leaving, at);
}

describe("charmVisibility：初始与首次登场", () => {
  it("初始是隐藏：没有待渐显时刻、没有渐隐结束时刻、也确实不可见", () => {
    const state = createCharmVisibilityState();

    expect(state.phase).toBe("hidden");
    expect(state.showAtMs).toBeNull();
    expect(state.fadeEndAtMs).toBeNull();
    expect(charmVisibilityVisible(state)).toBe(false);
    expect(charmVisibilityBusy(state)).toBe(false);
  });

  it("首次登场直接可见：第一次进入不走 0.1 秒防抖（登场不是「移回」）", () => {
    const state = charmVisibilityEnter(createCharmVisibilityState(), 1000);

    expect(state.phase).toBe("visible");
    expect(state.showAtMs).toBeNull();
    expect(state.hasShown).toBe(true);
    expect(charmVisibilityVisible(state)).toBe(true);
  });

  it("防抖窗口是票面口径的 0.1 秒（100ms）", () => {
    expect(CHARM_SHOW_DELAY_MS).toBe(100);
  });
});

describe("charmVisibility：移出 → 渐隐", () => {
  it("移出立即进入渐隐：可见态当场结束，并记下快档渐隐的结束时刻", () => {
    const leaving = charmVisibilityLeave(shown(), 2000);

    expect(leaving.phase).toBe("fading-out");
    expect(charmVisibilityVisible(leaving)).toBe(false);
    expect(leaving.showAtMs).toBeNull();
    expect(leaving.fadeEndAtMs).toBe(2000 + FADE_OUT_MS);
    expect(charmVisibilityBusy(leaving), "渐隐期间循环要保活到渐隐结束").toBe(true);
  });

  it("渐隐未走完之前仍是渐隐态；到点才落到隐藏（不是移出就立刻隐藏）", () => {
    const leaving = charmVisibilityLeave(shown(), 2000);

    const almost = charmVisibilityAdvance(leaving, 2000 + FADE_OUT_MS - 1);
    expect(almost.phase).toBe("fading-out");
    // 渐隐期间的重复推进不该把结束时刻往后拖。
    expect(almost.fadeEndAtMs).toBe(2000 + FADE_OUT_MS);

    const done = charmVisibilityAdvance(leaving, 2000 + FADE_OUT_MS);
    expect(done.phase).toBe("hidden");
    expect(done.fadeEndAtMs).toBeNull();
    expect(charmVisibilityBusy(done)).toBe(false);
  });

  it("渐隐途中重复收到移出事件是幂等的：不重启、也不延长渐隐", () => {
    const leaving = charmVisibilityLeave(shown(), 2000);
    const again = charmVisibilityLeave(leaving, 2100);

    expect(again.phase).toBe("fading-out");
    expect(again.fadeEndAtMs).toBe(leaving.fadeEndAtMs);
  });

  it("隐藏态收到移出事件什么都不做（本来就不在屏幕上）", () => {
    const hidden = hiddenAfterFirstShow(2000);
    const again = charmVisibilityLeave(hidden, 2100);

    expect(again.phase).toBe("hidden");
    expect(again.fadeEndAtMs).toBeNull();
  });
});

describe("charmVisibility：移回 → 等 0.1 秒再渐显", () => {
  it("移回先进入「等待渐显」：满 0.1 秒之前都不可见", () => {
    const back = charmVisibilityEnter(hiddenAfterFirstShow(2000), 3000);

    expect(back.phase).toBe("awaiting-show");
    expect(back.showAtMs, "渐显时刻 = 移回时刻 + 0.1 秒").toBe(3000 + CHARM_SHOW_DELAY_MS);
    expect(charmVisibilityVisible(back)).toBe(false);

    const justBefore = charmVisibilityAdvance(back, 3000 + CHARM_SHOW_DELAY_MS - 1);
    expect(justBefore.phase, "差 1ms 还没到点").toBe("awaiting-show");
    expect(charmVisibilityVisible(justBefore)).toBe(false);
  });

  it("满 0.1 秒当场渐显（边界值生效，不早也不晚）", () => {
    const back = charmVisibilityEnter(hiddenAfterFirstShow(2000), 3000);
    const atBoundary = charmVisibilityAdvance(back, 3000 + CHARM_SHOW_DELAY_MS);

    expect(atBoundary.phase).toBe("visible");
    expect(charmVisibilityVisible(atBoundary)).toBe(true);
    expect(atBoundary.showAtMs).toBeNull();
    expect(atBoundary.hasShown).toBe(true);
    expect(charmVisibilityBusy(atBoundary), "已经显示出来了，循环不必再为它保活").toBe(false);
  });

  it("等待渐显期间再来一次进入事件：不重置 0.1 秒时钟（鼠标挪动不该无限推迟）", () => {
    const back = charmVisibilityEnter(hiddenAfterFirstShow(2000), 3000);
    const again = charmVisibilityEnter(back, 3050);

    expect(again.phase).toBe("awaiting-show");
    expect(again.showAtMs, "渐显时刻仍是第一次移回的时刻 + 0.1 秒").toBe(
      3000 + CHARM_SHOW_DELAY_MS,
    );
    expect(charmVisibilityAdvance(again, 3100).phase).toBe("visible");
  });

  it("已可见时再来进入事件不动状态（沿边缘反复划过不该有副作用）", () => {
    const state = shown();
    const again = charmVisibilityEnter(state, 1500);

    expect(again.phase).toBe("visible");
    expect(again.showAtMs).toBeNull();
  });
});

describe("charmVisibility：0.1 秒内再次移出 → 取消显示（防抖）", () => {
  it("等待渐显期间移出 = 取消这一次显示：回到渐隐，到原定时刻也不会冒出来", () => {
    const back = charmVisibilityEnter(hiddenAfterFirstShow(2000), 3000);
    const cancelled = charmVisibilityLeave(back, 3050);

    expect(cancelled.phase).toBe("fading-out");
    expect(cancelled.showAtMs, "取消之后不再有任何待渐显时刻").toBeNull();
    expect(cancelled.fadeEndAtMs).toBe(3050 + FADE_OUT_MS);

    // 原定渐显时刻（3100）已经到了，但它不该出现。
    const atOriginalShowTime = charmVisibilityAdvance(cancelled, 3100);
    expect(charmVisibilityVisible(atOriginalShowTime), "取消显示后永远不该可见").toBe(false);

    const settled = charmVisibilityAdvance(cancelled, 3050 + FADE_OUT_MS);
    expect(settled.phase).toBe("hidden");
  });

  it("沿窗口边缘反复划过（每次进出都短于 0.1 秒）→ 全程不可见（不闪烁）", () => {
    // 从"已经显示过、现在隐藏"出发：之后每次移回都受 0.1 秒防抖约束。
    let state = hiddenAfterFirstShow(2000);
    let time = 3000;

    for (let crossing = 0; crossing < 5; crossing += 1) {
      state = charmVisibilityEnter(state, time);
      time += 50;
      expect(charmVisibilityVisible(state), `第 ${crossing + 1} 次移回不到 0.1 秒：不该可见`).toBe(
        false,
      );
      state = charmVisibilityLeave(state, time);
      expect(charmVisibilityVisible(state), `第 ${crossing + 1} 次移出：不该可见`).toBe(false);
      time += 50;
    }

    // 反复划完之后仍然是渐隐 / 隐藏，没有一次"闪一下"。
    expect(charmVisibilityVisible(charmVisibilityAdvance(state, time + 1000))).toBe(false);
  });

  it("移回满 0.1 秒后再移出：正常走渐隐（防抖只拦「没站满 0.1 秒」的那种）", () => {
    const back = charmVisibilityEnter(hiddenAfterFirstShow(2000), 3000);
    const visibleAgain = charmVisibilityAdvance(back, 3000 + CHARM_SHOW_DELAY_MS);
    const leavingAgain = charmVisibilityLeave(visibleAgain, 3200);

    expect(leavingAgain.phase).toBe("fading-out");
    expect(leavingAgain.fadeEndAtMs).toBe(3200 + FADE_OUT_MS);
  });
});

describe("charmVisibility：减少动态 → 直接切，不做过渡", () => {
  it("减少动态时移回直接可见：既不等待 0.1 秒，也不经过等待态", () => {
    const hidden = hiddenAfterFirstShow(2000);
    const back = charmVisibilityEnter(hidden, 3000, { reducedMotion: true });

    expect(back.phase).toBe("visible");
    expect(back.showAtMs).toBeNull();
    expect(charmVisibilityVisible(back)).toBe(true);
  });

  it("减少动态时移出直接隐藏：不经过渐隐态，也没有渐隐结束时刻", () => {
    const leaving = charmVisibilityLeave(shown(), 2000, { reducedMotion: true });

    expect(leaving.phase).toBe("hidden");
    expect(leaving.fadeEndAtMs).toBeNull();
    expect(charmVisibilityBusy(leaving)).toBe(false);
    expect(charmVisibilityVisible(leaving)).toBe(false);
  });

  it("减少动态下的进入 / 移出都不需要动画帧保活（循环可以立刻停）", () => {
    const hidden = hiddenAfterFirstShow(2000);
    const back = charmVisibilityEnter(hidden, 3000, { reducedMotion: true });
    const leaving = charmVisibilityLeave(back, 3100, { reducedMotion: true });

    expect(charmVisibilityBusy(back)).toBe(false);
    expect(charmVisibilityBusy(leaving)).toBe(false);
  });
});

describe("charmVisibility：保活判定（循环只在过渡窗口里多跑几帧）", () => {
  it("只有等待渐显与渐隐算「忙」：可见与隐藏都不该拖住循环", () => {
    expect(charmVisibilityBusy(charmVisibilityEnter(createCharmVisibilityState(), 1000))).toBe(false);
    expect(charmVisibilityBusy(hiddenAfterFirstShow(2000))).toBe(false);
    expect(charmVisibilityBusy(charmVisibilityEnter(hiddenAfterFirstShow(2000), 3000))).toBe(true);
    expect(charmVisibilityBusy(charmVisibilityLeave(shown(), 2000))).toBe(true);
  });
});
