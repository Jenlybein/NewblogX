import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  HEADER_JUMP_BREAKPOINT_PX,
  HEADER_WORDMARK_BREAKPOINT_PX,
  MOBILE_PANEL_QUERY,
  TABLET_BREAKPOINT_PX,
} from "~/utils/breakpoints";

/**
 * 手机断点的单一来源（票 437 评审）。
 *
 * CSS 不能 import TS 常量，所以这里补一条"两处一致"的断言：
 * `useHeaderMobilePanel` 的查询串由 `TABLET_BREAKPOINT_PX` 派生，
 * `AppHeader.vue` 的 `@media` 必须写同一个数（640 起平板 / 639 及更窄是手机）。
 * 任何一边改了、另一边没跟上，这条就会红 —— 不再靠注释提醒。
 */
const HEADER = readFileSync(
  join(process.cwd(), "app/components/layout/AppHeader.vue"),
  "utf8",
);

describe("页头断点：CSS 与 JS 同源", () => {
  it("查询串由 TABLET_BREAKPOINT_PX 派生（min-width 640 = max-width 639）", () => {
    expect(TABLET_BREAKPOINT_PX).toBe(640);
    expect(MOBILE_PANEL_QUERY).toBe("(max-width: 639px)");
    expect(MOBILE_PANEL_QUERY).toContain(`max-width: ${TABLET_BREAKPOINT_PX - 1}px`);
  });

  it("AppHeader.vue 的 CSS 使用同一个分界（640 起平板、639 及更窄是手机）", () => {
    expect(HEADER).toContain(`@media (min-width: ${TABLET_BREAKPOINT_PX}px)`);
    expect(HEADER).toContain(`@media (max-width: ${TABLET_BREAKPOINT_PX - 1}px)`);
    // 注释里点名了常量的出处：后来者改断点先看这里，而不是再抄一份数字。
    expect(HEADER).toContain("breakpoints.ts");
  });

  it("导航收进「跳转」的断点与常量为同一个数（767 及更窄收，768 起展开）", () => {
    expect(HEADER).toContain(`@media (max-width: ${HEADER_JUMP_BREAKPOINT_PX - 1}px)`);
    expect(HEADER).toContain(`@media (min-width: ${HEADER_JUMP_BREAKPOINT_PX}px)`);
  });

  it("隐去字标的断点与常量为同一个数（899 及更窄隐，900 起显示）", () => {
    expect(HEADER).toContain(`@media (max-width: ${HEADER_WORDMARK_BREAKPOINT_PX - 1}px)`);
  });
});
