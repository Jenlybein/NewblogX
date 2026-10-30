import { describe, expect, it } from "vitest";

import { themeBodyClass, themeRootClass } from "~/utils/themeClass";

describe("themeClass：站点根类", () => {
  it("暗色时 body 同时带 theme-dark 与 dark（后者才是 Tailwind class 策略的开关）", () => {
    const classes = themeBodyClass("dark").split(" ");

    // 两个类缺一不可：theme-dark 给旧 CSS 覆盖层用，dark 给 Tailwind 的 dark: 工具类用。
    // 历史上只有 theme-dark、Tailwind 走 media 策略，所以站内开关管不住 dark: 工具类。
    expect(classes).toContain("theme-dark");
    expect(classes).toContain("dark");
  });

  it("浅色时不得带任何 dark 类（否则 dark: 工具类会在浅色下被误触发）", () => {
    const classes = themeBodyClass("light").split(" ");

    expect(classes).toEqual(["theme-light"]);
    expect(classes.some((name) => name === "dark" || name.startsWith("theme-dark"))).toBe(false);
  });

  it("元素级根类只给 theme-*（dark 只需要在 body 上出现一次）", () => {
    expect(themeRootClass("dark")).toBe("theme-dark");
    expect(themeRootClass("light")).toBe("theme-light");
  });
});
