import { beforeEach, describe, expect, it, vi } from "vitest";

import { useProfileCategoryManage } from "~/composables/useProfileCategoryManage";

// 个人主页分类栏的写操作（票 325）。
//
// 这一层锁的是"用户在分类栏上能做的事"：新建 / 改名 / 删除，以及三件最容易
// 退化的细节：
//
//   1. **删除前必须告知受影响篇数**（不然用户不知道这一下会动几篇文章）；
//      零文章的分类不该编一个"0 篇文章会变成未分类"出来。
//   2. **删除 = 文章变未分类**，所以确认文案必须说清楚文章不会消失。
//   3. **失败不假装成功**：后端给的可读文案（如「名称重复」）原样展示，
//      弹窗不关、列表不重拉。

const saveProfileCategoryMock = vi.hoisted(() => vi.fn());
const deleteProfileCategoriesMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/category", () => ({
  saveProfileCategory: saveProfileCategoryMock,
  deleteProfileCategories: deleteProfileCategoriesMock,
}));

function makeMessage() {
  return { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() };
}

function makeConfirm() {
  return vi.fn((request: { title: string; content: string; onConfirm: () => Promise<void> }) => request);
}

function setup() {
  const message = makeMessage();
  const confirm = makeConfirm();
  const reload = vi.fn(async () => undefined);
  const onDeleted = vi.fn();
  const manage = useProfileCategoryManage({ message, confirm, reload, onDeleted });

  return { manage, message, confirm, reload, onDeleted };
}

/** 取出确认弹窗那次调用的参数（顺带断言它真的被弹出来了）。 */
function lastConfirmRequest(confirm: ReturnType<typeof makeConfirm>) {
  const call = confirm.mock.calls[0]?.[0];
  if (!call) {
    throw new Error("确认弹窗没有弹出");
  }
  return call;
}

const engineering = { id: "c1", title: "工程笔记", article_count: 3 };

beforeEach(() => {
  vi.clearAllMocks();
  saveProfileCategoryMock.mockResolvedValue(undefined);
  deleteProfileCategoriesMock.mockResolvedValue(undefined);
});

describe("新建分类", () => {
  it("提交后带上去掉首尾空白的名字，不带 id（才是新建），成功后关窗并重拉列表", async () => {
    const { manage, message, reload } = setup();

    manage.openCreate();
    expect(manage.formMode.value).toBe("create");
    expect(manage.formOpen.value).toBe(true);

    manage.formTitle.value = "  工程笔记  ";
    await manage.submit();

    expect(saveProfileCategoryMock).toHaveBeenCalledWith({ title: "工程笔记" });
    expect(saveProfileCategoryMock.mock.calls[0]![0]).not.toHaveProperty("id");
    expect(manage.formOpen.value).toBe(false);
    expect(message.success).toHaveBeenCalledWith("分类已新建");
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("空名被拒绝并提示，不发任何写请求、不关窗", async () => {
    const { manage, message } = setup();

    manage.openCreate();
    manage.formTitle.value = "   ";
    await manage.submit();

    expect(saveProfileCategoryMock).not.toHaveBeenCalled();
    expect(message.warning).toHaveBeenCalledWith("分类名称不能为空");
    expect(manage.formOpen.value).toBe(true);
  });

  it("后端可读文案（名称重复）原样展示，弹窗保持打开", async () => {
    const { manage, message, reload } = setup();
    saveProfileCategoryMock.mockRejectedValue(new Error("名称重复"));

    manage.openCreate();
    manage.formTitle.value = "工程笔记";
    await manage.submit();

    expect(message.error).toHaveBeenCalledWith("名称重复");
    expect(message.success).not.toHaveBeenCalled();
    // 弹窗保持打开：用户能就地改个名字重试，而不是从头再点一遍。
    expect(manage.formOpen.value).toBe(true);
    expect(reload).not.toHaveBeenCalled();
  });
});

describe("分类改名", () => {
  it("打开时回填原名字，提交带上 id 与去掉空白的名字", async () => {
    const { manage, message, reload } = setup();

    manage.openRename(engineering);
    expect(manage.formMode.value).toBe("rename");
    expect(manage.formOpen.value).toBe(true);
    expect(manage.formTitle.value).toBe("工程笔记");

    manage.formTitle.value = " 工程随笔 ";
    await manage.submit();

    expect(saveProfileCategoryMock).toHaveBeenCalledWith({ id: "c1", title: "工程随笔" });
    expect(message.success).toHaveBeenCalledWith("分类已改名");
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe("删除分类", () => {
  it("先二次确认，并告知该分类下的文章数（文章会变成未分类、不会消失）", () => {
    const { manage, confirm } = setup();

    manage.remove(engineering);

    const request = lastConfirmRequest(confirm);
    expect(request.title).toBe("删除分类");
    expect(request.content).toContain("工程笔记");
    expect(request.content).toContain("3 篇文章");
    expect(request.content).toContain("未分类");
    // 说清楚文章不删：否则用户会以为连文章一起没了。
    expect(request.content).toContain("文章本身不会删除");
    // 还没确认，什么都不该发生。
    expect(deleteProfileCategoriesMock).not.toHaveBeenCalled();
  });

  it("零文章的分类不编「0 篇文章会变成未分类」，但仍然要确认", () => {
    const { manage, confirm } = setup();

    manage.remove({ id: "c2", title: "空分类", article_count: 0 });

    const request = lastConfirmRequest(confirm);
    expect(request.content).toContain("空分类");
    expect(request.content).not.toContain("0 篇文章");
    expect(request.content).toContain("还没有文章");
  });

  it("确认后调删除接口、提示、通知被删的 ID 并重拉列表", async () => {
    const { manage, confirm, message, reload, onDeleted } = setup();

    manage.remove(engineering);
    await lastConfirmRequest(confirm).onConfirm();

    expect(deleteProfileCategoriesMock).toHaveBeenCalledWith(["c1"]);
    expect(message.success).toHaveBeenCalledWith("分类已删除");
    expect(onDeleted).toHaveBeenCalledWith("c1");
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("删除失败：原样展示后端文案，不通知被删、不重拉", async () => {
    const { manage, confirm, message, reload, onDeleted } = setup();
    deleteProfileCategoriesMock.mockRejectedValue(new Error("分类不存在"));

    manage.remove(engineering);
    await lastConfirmRequest(confirm).onConfirm();

    expect(message.error).toHaveBeenCalledWith("分类不存在");
    expect(message.success).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();
    expect(reload).not.toHaveBeenCalled();
  });
});
