import { useNuxtApp } from "#imports";
import { readListPayload } from "~/services/listData";
import type { Schema } from "~/types/contract";
import type { ProfileCategoryItem, ProfileCategoryListData } from "~/types/api";

/**
 * 分类的读者侧读取（#321）与写操作（#325）。
 *
 * 读是公开的：个人主页的分类栏要的是"这个人有哪些分类、每个分类几篇文章"，
 * 数据来自既有的 `GET /api/articles/category?user_id=`（匿名可访问）。
 *
 * 写是登录后的私有操作，同样**不新增端点**：新建与改名都走
 * `POST /api/articles/category`（带 id 是改名、不带是新建），删除走
 * `DELETE /api/articles/category`（请求体 `{id_list}`）。后端对"空标题"、
 * "同名"、"删别人的分类"都有可读文案，前端原样展示。
 *
 * 响应形状是 `kernel.ListData[CategoryListItem]`，即 `{list, count}`；`$api.request()`
 * 返回已解包的 data，所以读取必须走 `readListPayload` 读条目，不能把 data 当数组 `.map`
 * （#320 的坑）。
 */

/** 归一化后的分类列表（`kernel.ListData` 的 {list, count} 口径）。 */
export interface ProfileCategoryList {
  list: ProfileCategoryItem[];
  count: number;
}

/**
 * 分类写操作的请求体，直接从契约派生（`CategoryRequest` = `{id, title}`）。
 *
 * 不手写 interface：字段名与类型由契约编译期校验，后端改签名会立刻红。
 */
export type CategoryWritePayload = Schema<"internal_article_adapter_http.CategoryRequest">;

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

/**
 * 新建或改名一个分类。
 *
 * 不带 `id` 是新建（后端会先找同名软删行并恢复它，所以**不要**用返回的 ID
 * 做后续写入——写入接口本来也不返回它，建完重拉列表即可）；
 * 带 `id` 是改名。
 */
export function saveProfileCategory(payload: { id?: string; title: string }) {
  const body: CategoryWritePayload = {
    title: payload.title,
  };
  if (payload.id) {
    body.id = payload.id;
  }
  return useNuxtApp().$api.request<unknown>("/api/articles/category", {
    method: "POST",
    body,
  });
}

/**
 * 删除分类。
 *
 * 后端在同一事务里先解除文章归属、再软删分类：这些文章会变成"未分类"，
 * 文章本身不删。删除前必须先告知受影响篇数并二次确认（由调用方的确认弹窗负责）。
 */
export function deleteProfileCategories(idList: string[]) {
  return useNuxtApp().$api.request<unknown>("/api/articles/category", {
    method: "DELETE",
    body: {
      id_list: idList,
    },
  });
}
