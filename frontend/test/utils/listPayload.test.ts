import { describe, expect, it } from "vitest";

import { readListPayload } from "~/utils/listPayload";

// 列表载荷的形状读取（纯函数）。
//
// 这个模块存在的原因就是"只写一遍"：列表端点的 data 恒为 `{list, count}`，
// 而 `$api.request()` 返回已解包的 data。此前 taxonomyOptions、services/category、
// services/listData 各自写了一遍 `Array.isArray(payload?.list)`，口径一改要改三处，
// 且 utils 不能反向 import services 复用。这里钉住形状规则本身。

describe("readListPayload · 列表载荷的形状读取", () => {
  it("正常载荷：条目原样返回，count 用服务端给的数", () => {
    const payload = { list: [{ id: "c1" }, { id: "c2" }], count: 2 };

    expect(readListPayload(payload)).toEqual({ list: payload.list, count: 2 });
  });

  it("缺 count 时回落到条目数（降级响应不该让总数变 0）", () => {
    expect(readListPayload({ list: [{ id: "c1" }] })).toEqual({ list: [{ id: "c1" }], count: 1 });
  });

  it("缺 list / list 不是数组 / 整个 data 为空时都读成空列表，不抛错", () => {
    expect(readListPayload(null)).toEqual({ list: [], count: 0 });
    expect(readListPayload(undefined)).toEqual({ list: [], count: 0 });
    expect(readListPayload({})).toEqual({ list: [], count: 0 });
    expect(readListPayload({ list: null, count: 3 })).toEqual({ list: [], count: 3 });
    expect(readListPayload({ list: "not-an-array" } as never)).toEqual({ list: [], count: 0 });
  });
});
