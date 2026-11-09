import { useNuxtApp } from "#imports";
import { readListPayload } from "~/services/listData";
import type { Schema } from "~/types/contract";
import type { ProfileCategoryItem, ProfileCategoryListData } from "~/types/api";

/**
 * 分类的读者侧读取（#321）。
 *
 * 只读：个人主页的分类栏要的是"这个人有哪些分类、每个分类几篇文章"，数据来自既有的
 * `GET /api/articles/category?user_id=`（公开读，匿名可访问）。**不新增端点**，写操作
 * （新建 / 改名 / 删除）不在这里 —— 那是后台/个人中心的票。
 *
 * 响应形状是 `kernel.ListData[CategoryListItem]`，即 `{list, count}`；`$api.request()`
 * 返回已解包的 data，所以必须走 `readListPayload` 读条目，不能把 data 当数组 `.map`
 * （#320 的坑）。
 */

/** 归一化后的分类列表（`kernel.ListData` 的 {list, count} 口径）。 */
export interface ProfileCategoryList {
  list: ProfileCategoryItem[];
  count: number;
}

/** 分类条目的最后一道防线：缺字段时给模板可渲染的值，不让 undefined 漏到页面上。 */
function toProfileCategory(item: Schema<"internal_article_adapter_http.CategoryListItem">): ProfileCategoryItem {
  return {
    id: item.id ?? "",
    title: item.title?.trim() || "未命名分类",
    article_count: item.article_count ?? 0,
  };
}

/** 读这位作者（user_id）的公开分类列表；匿名访客同样可读。 */
export function getProfileCategories(userId: string | number) {
  return useNuxtApp()
    .$api.request<ProfileCategoryListData>("/api/articles/category", {
      auth: false,
      query: {
        user_id: String(userId),
      },
    })
    .then((payload): ProfileCategoryList => {
      const { list, count } = readListPayload<Schema<"internal_article_adapter_http.CategoryListItem">>(payload);

      return {
        list: list.map(toProfileCategory),
        count,
      };
    });
}
