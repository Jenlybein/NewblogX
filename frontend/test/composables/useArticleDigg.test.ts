import { ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useArticleDigg } from "~/composables/useArticleDigg";

/**
 * 点赞状态机的测试（ADR-0024 决策 3/4）。
 *
 * 这块逻辑原本埋在 article/[id].vue（791 行、零测试）里。抽出来的直接原因就是
 * 它有几处"看起来对、用起来错"的地方：乐观更新被刷新覆盖、靠中文文案猜方向、
 * 计数减成负数、连点重复发请求。
 *
 * 2026-11-04 起点赞接口改成**幂等目标态**：前端传 `{liked}` 而不是"取反"，
 * 响应回填服务端权威的 `is_digg` / `digg_count`。所以这里不再有"按文案猜方向"
 * 的用例 —— 方向与计数都来自响应，唯一需要守的是"传出去的是目标态"。
 */

describe("useArticleDigg", () => {
  type Snapshot = { isDigg?: boolean; diggCount?: number };
  type SetDiggResult = { is_digg: boolean; digg_count: number };

  function harness(initial: Snapshot = { isDigg: false, diggCount: 3 }) {
    const articleId = ref("a1");
    const snapshot = ref<Snapshot>(initial);
    const patched: Array<{ isDigg: boolean; diggCount: number }> = [];
    const setDigg = vi.fn<(articleId: string, liked: boolean) => Promise<SetDiggResult>>();
    const notify = { success: vi.fn(), error: vi.fn() };
    const openAuthModal = vi.fn();
    const ensureLoggedIn = vi.fn().mockResolvedValue(true);

    const digg = useArticleDigg({
      articleId: () => articleId.value,
      snapshot: () => snapshot.value,
      patchArticle: (isDigg, diggCount) => {
        patched.push({ isDigg, diggCount });
        snapshot.value = { isDigg, diggCount };
      },
      setDigg,
      ensureLoggedIn,
      openAuthModal,
      notify,
      debounceMs: 50,
    });

    return { digg, articleId, snapshot, patched, setDigg, notify, openAuthModal, ensureLoggedIn };
  }

  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("初始值跟随服务端快照", () => {
    const { digg } = harness({ isDigg: true, diggCount: 9 });
    expect(digg.articleIsDigged.value).toBe(true);
    expect(digg.articleDiggCount.value).toBe(9);
  });

  it("计数为负时归零（并发/重复点击不该出现 -1）", () => {
    const { digg } = harness({ isDigg: false, diggCount: -5 });
    expect(digg.articleDiggCount.value).toBe(0);
  });

  it("未登录时不发请求，改为打开登录弹窗", async () => {
    const { digg, setDigg, openAuthModal, ensureLoggedIn } = harness();
    ensureLoggedIn.mockResolvedValue(false);

    await digg.handleLike();

    expect(setDigg).not.toHaveBeenCalled();
    expect(openAuthModal).toHaveBeenCalledTimes(1);
  });

  it("未赞时传目标态 liked=true，并用服务端返回的权威计数回填（不是本地 +1）", async () => {
    const { digg, setDigg, notify, patched } = harness({ isDigg: false, diggCount: 3 });
    // 服务端计数与本地快照故意不同：回填必须以响应里的 digg_count 为准。
    setDigg.mockResolvedValue({ is_digg: true, digg_count: 42 });

    await digg.handleLike();

    expect(setDigg).toHaveBeenCalledWith("a1", true);
    expect(digg.articleIsDigged.value).toBe(true);
    expect(digg.articleDiggCount.value).toBe(42);
    expect(notify.success).toHaveBeenCalledWith("点赞成功");
    // 同时要写回文章对象，否则页面 refresh 后会被覆盖
    expect(patched.at(-1)).toEqual({ isDigg: true, diggCount: 42 });
  });

  it("已赞时再点传目标态 liked=false，并回到未赞（不是再发一次 toggle）", async () => {
    const { digg, setDigg, articleId } = harness({ isDigg: true, diggCount: 9 });
    setDigg.mockResolvedValue({ is_digg: false, digg_count: 8 });

    await digg.handleLike();

    expect(setDigg).toHaveBeenCalledWith("a1", false);
    expect(digg.articleIsDigged.value).toBe(false);
    expect(digg.articleDiggCount.value).toBe(8);
    // 目标态来自本地状态，当前文章没变。
    expect(articleId.value).toBe("a1");
  });

  it("失败时只提示，不改状态", async () => {
    const { digg, setDigg, notify } = harness({ isDigg: false, diggCount: 3 });
    setDigg.mockRejectedValue(new Error("网络炸了"));

    await digg.handleLike();

    expect(digg.articleIsDigged.value).toBe(false);
    expect(digg.articleDiggCount.value).toBe(3);
    expect(notify.error).toHaveBeenCalledWith("网络炸了");
  });

  it("防抖窗口内连点只发一次请求", async () => {
    const { digg, setDigg } = harness();
    setDigg.mockResolvedValue({ is_digg: true, digg_count: 4 });

    await digg.handleLike();
    await digg.handleLike(); // 窗口内，应被 debouncing 挡住

    expect(setDigg).toHaveBeenCalledTimes(1);

    // 窗口过去后可以再点
    vi.advanceTimersByTime(60);
    await digg.handleLike();
    expect(setDigg).toHaveBeenCalledTimes(2);
  });

  it("用户点过之后，服务端刷新不会覆盖界面上的值", async () => {
    const { digg, setDigg, snapshot } = harness({ isDigg: false, diggCount: 3 });
    setDigg.mockResolvedValue({ is_digg: true, digg_count: 4 });
    await digg.handleLike();
    expect(digg.articleIsDigged.value).toBe(true);

    // 模拟 refreshArticle 拿回旧数据（服务端还没算上这次点赞）
    snapshot.value = { isDigg: false, diggCount: 3 };
    await Promise.resolve();

    expect(digg.articleIsDigged.value).toBe(true);
    expect(digg.articleDiggCount.value).toBe(4);
  });

  it("换文章后脏标记复位，重新跟随服务端", async () => {
    const { digg, setDigg, snapshot, articleId } = harness({ isDigg: false, diggCount: 3 });
    setDigg.mockResolvedValue({ is_digg: true, digg_count: 4 });
    await digg.handleLike();

    articleId.value = "a2";
    snapshot.value = { isDigg: false, diggCount: 7 };
    await Promise.resolve();

    expect(digg.articleIsDigged.value).toBe(false);
    expect(digg.articleDiggCount.value).toBe(7);
  });

  it("dispose 会清掉防抖定时器", async () => {
    const { digg, setDigg } = harness();
    setDigg.mockResolvedValue({ is_digg: true, digg_count: 4 });
    await digg.handleLike();
    expect(digg.articleLikeDebouncing.value).toBe(true);

    digg.dispose();

    expect(digg.articleLikeDebouncing.value).toBe(false);
    // 定时器被清掉后，时间推进也不会再改状态
    vi.advanceTimersByTime(60);
    expect(digg.articleLikeDebouncing.value).toBe(false);
  });
});
