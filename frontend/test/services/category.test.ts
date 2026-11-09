import { beforeEach, describe, expect, it, vi } from "vitest";

import { getProfileCategories, deleteProfileCategories, saveProfileCategory } from "~/services/category";
import { readListPayload } from "~/services/listData";

const requestMock = vi.fn();

describe("列表响应 → 条目：data 恒为 {list,count}", () => {
  beforeEach(() => {
    requestMock.mockReset();
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
  });

  it("把 {list,count} 读成条目数组 + 总数（不是把 data 当数组 .map）", () => {
    const payload = {
      list: [
        { id: "c1", title: "工程笔记", article_count: 3 },
        { id: "c2", title: "读书", article_count: 0 },
      ],
      count: 2,
    };

    expect(readListPayload(payload)).toEqual({ list: payload.list, count: 2 });
  });

  it("list 缺失 / 为 null / 整个 data 缺失时返回空列表，而不是抛 list.map is not a function", () => {
    // #320 就是栽在这里：把已解包的 data 当成数组直接 .map。
    for (const degraded of [undefined, null, {}, { count: 0 }, { list: null }]) {
      expect(readListPayload(degraded as never)).toEqual({ list: [], count: 0 });
    }
  });

  it("count 缺失时回落到条目数，不让模板显示 undefined", () => {
    expect(readListPayload({ list: [{ id: "c1" }] })).toEqual({ list: [{ id: "c1" }], count: 1 });
  });
});

describe("分类列表读取（读者站，只读）", () => {
  beforeEach(() => {
    requestMock.mockReset();
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
  });

  it("按 user_id 读这位作者的公开分类列表，匿名可读", async () => {
    requestMock.mockResolvedValue({ list: [], count: 0 });

    await getProfileCategories("42");

    expect(requestMock).toHaveBeenCalledWith("/api/articles/category", {
      auth: false,
      query: {
        user_id: "42",
      },
    });
  });

  it("每个分类带出标题与文章数，供分类栏直接渲染", async () => {
    requestMock.mockResolvedValue({
      list: [
        { id: "c1", title: "工程笔记", article_count: 3, user_id: "42" },
        { id: "c2", title: "读书", article_count: 0, user_id: "42" },
      ],
      count: 2,
    });

    const result = await getProfileCategories("42");

    expect(result.count).toBe(2);
    expect(result.list.map((item) => [item.id, item.title, item.article_count])).toEqual([
      ["c1", "工程笔记", 3],
      ["c2", "读书", 0],
    ]);
  });

  it("分类条目缺字段时回落成可渲染的值（不把 undefined 交给模板）", async () => {
    requestMock.mockResolvedValue({
      list: [{ id: "c1" }, { title: "没有 id 的行" }],
      count: 2,
    });

    const result = await getProfileCategories("42");

    expect(result.list).toEqual([
      { id: "c1", title: "未命名分类", article_count: 0 },
      { id: "", title: "没有 id 的行", article_count: 0 },
    ]);
  });

  it("后端返回降级空 data 时给出空列表 + 0，页面能走空态而不是崩", async () => {
    requestMock.mockResolvedValue({});

    await expect(getProfileCategories("42")).resolves.toEqual({ list: [], count: 0 });
  });
});

// 分类写操作（票 325）：新建 / 改名 / 删除都走既有的两个端点，不新增接口。
// 这里锁的是"打哪个端点、用哪个方法、带什么请求体"，页面上看得见的行为由
// useProfileCategoryManage 与页面的用例覆盖。
describe("分类写操作（读者站）", () => {
  beforeEach(() => {
    requestMock.mockReset();
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
  });

  it("新建：POST /api/articles/category，请求体只有 title（没有 id 才是新建）", async () => {
    requestMock.mockResolvedValue({});

    await saveProfileCategory({ title: "工程笔记" });

    expect(requestMock).toHaveBeenCalledWith("/api/articles/category", {
      method: "POST",
      body: { title: "工程笔记" },
    });
    expect(requestMock.mock.calls[0]![1].body).not.toHaveProperty("id");
  });

  it("改名：同一个端点带上 id，后端按 id 更新而不是新建一条", async () => {
    requestMock.mockResolvedValue({});

    await saveProfileCategory({ id: "c1", title: "改名后的分类" });

    expect(requestMock).toHaveBeenCalledWith("/api/articles/category", {
      method: "POST",
      body: { id: "c1", title: "改名后的分类" },
    });
  });

  it("删除：DELETE 同一个端点，请求体是 {id_list}", async () => {
    requestMock.mockResolvedValue({});

    await deleteProfileCategories(["c1"]);

    expect(requestMock).toHaveBeenCalledWith("/api/articles/category", {
      method: "DELETE",
      body: { id_list: ["c1"] },
    });
  });

  it("写请求不声明 auth:false：这些端点必须带登录态", async () => {
    requestMock.mockResolvedValue({});

    await saveProfileCategory({ title: "工程笔记" });
    await deleteProfileCategories(["c1"]);

    for (const call of requestMock.mock.calls) {
      expect(call[1]).not.toHaveProperty("auth");
    }
  });
});
