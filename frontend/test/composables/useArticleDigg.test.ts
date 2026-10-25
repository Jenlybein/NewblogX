import { ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resolveDiggMutation, useArticleDigg } from "~/composables/useArticleDigg";

/**
 * 点赞状态机的测试。
 *
 * 这块逻辑原本埋在 article/[id].vue（791 行、零测试）里。抽出来的直接原因就是
 * 它有几处"看起来对、用起来错"的地方：乐观更新被刷新覆盖、靠中文文案猜方向、
 * 计数减成负数、连点重复发请求。这些都是纯逻辑，不该只能靠手点验证。
 */

describe("resolveDiggMutation", () => {
  // 后端只返回一句 msg，方向靠文案判断 —— 这类逻辑最怕"改文案就静默错"，
  // 所以把当前行为逐条钉住。
  it("文案含「取消」判为取消点赞", () => {
    expect(resolveDiggMutation("取消点赞成功", true)).toEqual({ isDigged: false, countDelta: -1 });
  });

  it("文案含「点赞」判为点赞", () => {
    expect(resolveDiggMutation("点赞成功", false)).toEqual({ isDigged: true, countDelta: 1 });
  });

  it("文案认不出来时按取反兜底（不会把方向弄反）", () => {
    expect(resolveDiggMutation("操作成功", true)).toEqual({ isDigged: false, countDelta: -1 });
    expect(resolveDiggMutation("操作成功", false)).toEqual({ isDigged: true, countDelta: 1 });
  });
});

describe("useArticleDigg", () => {
  type Snapshot = { isDigg?: boolean; diggCount?: number };

  function harness(initial: Snapshot = { isDigg: false, diggCount: 3 }) {
    const articleId = ref("a1");
    const snapshot = ref<Snapshot>(initial);
    const patched: Array<{ isDigg: boolean; diggCount: number }> = [];
    const toggle = vi.fn<() => Promise<{ msg?: string }>>();
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
      toggle,
      ensureLoggedIn,
      openAuthModal,
      notify,
      debounceMs: 50,
    });

    return { digg, articleId, snapshot, patched, toggle, notify, openAuthModal, ensureLoggedIn };
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
    const { digg, toggle, openAuthModal, ensureLoggedIn } = harness();
    ensureLoggedIn.mockResolvedValue(false);

    await digg.handleLike();

    expect(toggle).not.toHaveBeenCalled();
    expect(openAuthModal).toHaveBeenCalledTimes(1);
  });

  it("成功时乐观更新并提示后端文案", async () => {
    const { digg, toggle, notify, patched } = harness({ isDigg: false, diggCount: 3 });
    toggle.mockResolvedValue({ msg: "点赞成功" });

    await digg.handleLike();

    expect(digg.articleIsDigged.value).toBe(true);
    expect(digg.articleDiggCount.value).toBe(4);
    expect(notify.success).toHaveBeenCalledWith("点赞成功");
    // 同时要写回文章对象，否则页面 refresh 后会被覆盖
    expect(patched.at(-1)).toEqual({ isDigg: true, diggCount: 4 });
  });

  it("失败时只提示，不改状态", async () => {
    const { digg, toggle, notify } = harness({ isDigg: false, diggCount: 3 });
    toggle.mockRejectedValue(new Error("网络炸了"));

    await digg.handleLike();

    expect(digg.articleIsDigged.value).toBe(false);
    expect(digg.articleDiggCount.value).toBe(3);
    expect(notify.error).toHaveBeenCalledWith("网络炸了");
  });

  it("防抖窗口内连点只发一次请求", async () => {
    const { digg, toggle } = harness();
    toggle.mockResolvedValue({ msg: "点赞成功" });

    await digg.handleLike();
    await digg.handleLike(); // 窗口内，应被 debouncing 挡住

    expect(toggle).toHaveBeenCalledTimes(1);

    // 窗口过去后可以再点
    vi.advanceTimersByTime(60);
    await digg.handleLike();
    expect(toggle).toHaveBeenCalledTimes(2);
  });

  it("用户点过之后，服务端刷新不会覆盖界面上的值", async () => {
    const { digg, toggle, snapshot } = harness({ isDigg: false, diggCount: 3 });
    toggle.mockResolvedValue({ msg: "点赞成功" });
    await digg.handleLike();
    expect(digg.articleIsDigged.value).toBe(true);

    // 模拟 refreshArticle 拿回旧数据（服务端还没算上这次点赞）
    snapshot.value = { isDigg: false, diggCount: 3 };
    await Promise.resolve();

    expect(digg.articleIsDigged.value).toBe(true);
    expect(digg.articleDiggCount.value).toBe(4);
  });

  it("换文章后脏标记复位，重新跟随服务端", async () => {
    const { digg, toggle, snapshot, articleId } = harness({ isDigg: false, diggCount: 3 });
    toggle.mockResolvedValue({ msg: "点赞成功" });
    await digg.handleLike();

    articleId.value = "a2";
    snapshot.value = { isDigg: false, diggCount: 7 };
    await Promise.resolve();

    expect(digg.articleIsDigged.value).toBe(false);
    expect(digg.articleDiggCount.value).toBe(7);
  });

  it("dispose 会清掉防抖定时器", async () => {
    const { digg, toggle } = harness();
    toggle.mockResolvedValue({ msg: "点赞成功" });
    await digg.handleLike();
    expect(digg.articleLikeDebouncing.value).toBe(true);

    digg.dispose();

    expect(digg.articleLikeDebouncing.value).toBe(false);
    // 定时器被清掉后，时间推进也不会再改状态
    vi.advanceTimersByTime(60);
    expect(digg.articleLikeDebouncing.value).toBe(false);
  });
});
