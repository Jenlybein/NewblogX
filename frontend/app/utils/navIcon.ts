import {
  IconBriefcase,
  IconHome2,
  IconPencilPlus,
  IconSearch,
  IconStack2,
} from "@tabler/icons-vue";

import type { HeaderNavKey } from "~/utils/chrome";

/**
 * 页头导航的图标映射（key → 图标组件）。
 *
 * 为什么是一张表而不是模板里的三元表达式：以前渲染层写的是
 * `item.key === "home" ? IconHome2 : IconSearch`，也就是"除首页之外一律搜索图标"。
 * 那种写法在只有两个入口时恰好是对的，但**加第三个入口时不会报错**——
 * 新入口会静默拿到搜索图标。加合集（第四项）时把它改成显式映射，
 * 新 key 忘了配图标会在这里的类型上缺一项（`Record<HeaderNavKey, ...>` 直接红）。
 * 加项目（第五项）时同样只补这张表与 chrome.ts 的联合类型，渲染层零改动。
 *
 * 图标只表达"这是哪一档入口"，不承担状态：激活态仍由 `.is-active` 的
 * 内阴影负责（ADR-0012，不改变盒模型）。
 */
export const HEADER_NAV_ICONS: Record<HeaderNavKey, typeof IconHome2> = {
  home: IconHome2,
  search: IconSearch,
  write: IconPencilPlus,
  collections: IconStack2,
  // 项目是独立实体（第 193/198 票）：用公文包剪影，与合集的「层叠」图形明显不同，
  // 避免读者把两个并列入口看成一类。
  project: IconBriefcase,
};

/** 取某个导航项的图标组件。 */
export function headerNavIcon(key: HeaderNavKey) {
  return HEADER_NAV_ICONS[key];
}
