import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  ADMIN_NAV_TREE,
  activeGroupKeys,
  buildAdminNavMenu,
  navItemByPath,
  navTrail,
  pageFileCandidates,
} from "../../app/utils/menu";
import { SITE_SETTINGS_SECTIONS, siteSettingsSectionByPath } from "../../app/utils/siteSettings";

/**
 * 站点配置八域 → 独立路由的结构对账。
 *
 * 后端把站点配置拆成八个独立的段（读 `GET /site/admin/{name}`、写 `PUT /site/{name}`），
 * 后台原来用一个 `NTabs` 壳把它们装在同一页里：刷新会丢当前域，链接也没法分享。
 * 拆开之后要守住三件事：
 *
 *   1. 八个域各有一个可直接打开的 URL（页面文件真实存在，刷新保持当前域）；
 *   2. 每个页面只渲染自己那一段的配置组件，且组件名与位置不变；
 *   3. 左侧菜单的八个子项与路由一一对应，深链接时父分组展开并高亮。
 *
 * 为什么按源码断言：后台没有 e2e，naive-ui 的表单交互要 mock 一整套。
 * 与 `menu.test.ts` 的页面文件对账、`site-config.test.ts` 的文案断言同一套做法。
 * vitest 以 project root（frontend_admin/）为 cwd。
 */
const projectDir = process.cwd();
const pagesDir = join(projectDir, "app", "pages");
const settingsDir = join(projectDir, "app", "components", "settings");

/** 期望值来自票面与既有页面分区，不复用被测模块的数据（避免自证）。 */
interface ExpectedSection {
  key: string;
  label: string;
  path: string;
  /** 相对 app/pages，不含 .vue。 */
  page: string;
  /** components/settings 下不改名、不移动的配置组件。 */
  component: string;
}

const EXPECTED_SECTIONS: ExpectedSection[] = [
  { key: "site", label: "站点", path: "/site", page: "site/index", component: "SettingsSite" },
  { key: "ai", label: "AI 助手", path: "/site/ai", page: "site/ai", component: "SettingsAi" },
  { key: "mail", label: "邮件", path: "/site/mail", page: "site/mail", component: "SettingsMail" },
  {
    key: "storage",
    label: "对象存储",
    path: "/site/storage",
    page: "site/storage",
    component: "SettingsStorage",
  },
  {
    key: "oauth",
    label: "第三方登录",
    path: "/site/oauth",
    page: "site/oauth",
    component: "SettingsOAuth",
  },
  { key: "log", label: "日志", path: "/site/log", page: "site/log", component: "SettingsLog" },
  {
    key: "upload",
    label: "上传策略",
    path: "/site/upload",
    page: "site/upload",
    component: "SettingsUpload",
  },
  { key: "deco", label: "装饰", path: "/site/deco", page: "site/deco", component: "SettingsDeco" },
];

function sitePageFiles(): string[] {
  return readdirSync(join(pagesDir, "site"), { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".vue"))
    .map((entry) => `site/${entry.name.slice(0, -".vue".length)}`)
    .sort();
}

function pageSource(page: string): string {
  return readFileSync(join(pagesDir, `${page}.vue`), "utf8");
}

describe("站点配置八域清单（路由与组件映射的唯一来源）", () => {
  it("八个域的顺序、标题、路由与组件名固定", () => {
    expect(
      SITE_SETTINGS_SECTIONS.map(({ key, label, path, component }) => ({
        key,
        label,
        path,
        component,
      })),
    ).toEqual(
      EXPECTED_SECTIONS.map(({ key, label, path, component }) => ({
        key,
        label,
        path,
        component,
      })),
    );
    expect(SITE_SETTINGS_SECTIONS).toHaveLength(8);
  });

  it("路由唯一、都是绝对路径，且全部挂在 /site 之下", () => {
    const paths = SITE_SETTINGS_SECTIONS.map((section) => section.path);
    expect(new Set(paths).size).toBe(paths.length);
    for (const path of paths) {
      expect(path.startsWith("/site")).toBe(true);
      // 域必须靠路径区分，而不是查询参数或页面内状态。
      expect(path).not.toContain("?");
    }
  });

  it("siteSettingsSectionByPath 按路由反查；未知路由返回 undefined", () => {
    for (const section of EXPECTED_SECTIONS) {
      expect(siteSettingsSectionByPath(section.path)).toMatchObject({
        label: section.label,
        component: section.component,
      });
    }
    expect(siteSettingsSectionByPath("/site/missing")).toBeUndefined();
    expect(siteSettingsSectionByPath("")).toBeUndefined();
  });
});

describe("站点配置八域菜单子项", () => {
  it("「站点配置」是纯分组：没有自身页面，八个子项一条不少", () => {
    const group = ADMIN_NAV_TREE.find((node) => node.label === "站点配置");
    expect(group).toBeDefined();
    // 父项只分组：它没有页面，所以不会跳空页。
    expect(group!.path).toBeUndefined();
    expect(group!.children?.map(({ label, path }) => ({ label, path }))).toEqual(
      EXPECTED_SECTIONS.map(({ label, path }) => ({ label, path })),
    );
  });

  it("八个子项都能在菜单里查到，且父级归属正确", () => {
    for (const section of EXPECTED_SECTIONS) {
      expect(navItemByPath(section.path)).toMatchObject({
        label: section.label,
        parentLabel: "站点配置",
        parentPath: null,
        depth: 1,
      });
    }
  });

  it("深链接到任一域：标题取子项名，父分组高亮并默认展开", () => {
    for (const section of EXPECTED_SECTIONS) {
      // 标题：AdminShell 用 navItemByPath 的 label 填页面标题与浏览器标题。
      expect(navItemByPath(section.path)?.label).toBe(section.label);
      // 祖先链：面包屑 / 分组展开都从这里取。
      expect(navTrail(section.path).map((item) => item.label)).toEqual(["站点配置"]);
      expect(activeGroupKeys(section.path)).toEqual(["group:站点配置"]);

      const entries = buildAdminNavMenu(ADMIN_NAV_TREE, section.path, ["group:站点配置"]);
      const group = entries.find((entry) => entry.label === "站点配置");
      if (group?.kind !== "group") throw new Error("「站点配置」应渲染成分组");

      expect(group.active).toBe(true);
      expect(group.expanded).toBe(true);
      // 只亮当前域，兄弟子项不亮。
      expect(group.children.filter((child) => child.active).map((child) => child.label)).toEqual([
        section.label,
      ]);
    }
  });
});

describe("站点配置八域页面文件（刷新保持当前域）", () => {
  it("app/pages/site 下恰好八个页面文件，每个域一个，旧的 tab 壳已消失", () => {
    expect(sitePageFiles()).toEqual(EXPECTED_SECTIONS.map((section) => section.page).sort());
  });

  it("每个路由都能解析到一个真实页面文件", () => {
    for (const section of EXPECTED_SECTIONS) {
      const candidates = pageFileCandidates(section.path)
        .map((candidate) => `${candidate}.vue`)
        .filter((file) => existsSync(join(pagesDir, file)));
      expect(candidates, section.path).toEqual([`${section.page}.vue`]);
    }
  });

  it("每个页面只渲染自己那一段的配置组件，不再有页内 tab 切换", () => {
    for (const section of EXPECTED_SECTIONS) {
      const source = pageSource(section.page);

      // 组件按原名引用（不改名、不移动）；页面只挂一个域。
      expect(source).toContain(
        `import ${section.component} from '~/components/settings/${section.component}.vue'`,
      );
      expect(source).toMatch(new RegExp(`<${section.component}(\\s|/|>)`, "u"));

      const importedSettings = [...source.matchAll(/~\/components\/settings\/(\w+)\.vue/gu)].map(
        (match) => match[1],
      );
      expect(importedSettings).toEqual([section.component]);

      // 域由 URL 决定：页面里不应再出现 tab 容器或活动态内存变量。
      expect(source).not.toMatch(/NTabs|NTabPane|useState/u);
    }
  });

  it("八个配置组件仍按原名待在原位置", () => {
    for (const section of EXPECTED_SECTIONS) {
      expect(
        existsSync(join(settingsDir, `${section.component}.vue`)),
        section.component,
      ).toBe(true);
    }
  });
});
