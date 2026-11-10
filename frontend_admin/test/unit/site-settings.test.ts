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
import {
  ALL_SITE_SETTINGS_SECTIONS,
  SITE_SETTINGS_PAGES,
  SITE_SETTINGS_SECTIONS,
  siteSettingsSectionByPath,
} from "../../app/utils/siteSettings";

/**
 * 站点配置编辑域 → 独立路由的结构对账（ADR-0039）。
 *
 * 2026-11 之前「站点」是一个页面（8 张卡、32 个控件），现在拆成四页：
 * 站点基础 / 登录与发码 / 发布与审核 / 私信，挂在「站点配置 → 站点」这个三层菜单下。
 * 其余七个编辑域仍是直接挂「站点配置」的二级子项。
 *
 * 要守住的事：
 *
 *   1. 十一个编辑域各有一个可直接打开的 URL（页面文件真实存在，刷新保持当前域）；
 *   2. 每个页面只渲染自己那一个配置组件（不再有页内 tab，也不能顺手改兄弟页的字段）；
 *   3. 左侧菜单的归属与层级与路由一致：站点四页深度 2、其余域深度 1；
 *   4. 站点四页共用同一个 `site` 段端点，所以保存必须走读-改-写（见 siteSectionSave 的用例）。
 *
 * 为什么按源码断言：后台没有 e2e，naive-ui 的表单交互要 mock 一整套。
 * 与 `menu.test.ts` 的页面文件对账、`site-config.test.ts` 的文案断言同一套做法。
 * vitest 以 project root（frontend_admin/）为 cwd。
 */
const projectDir = process.cwd();
const pagesDir = join(projectDir, "app", "pages");
const settingsDir = join(projectDir, "app", "components", "settings");

/** 期望值来自票面与拆页方案，不复用被测模块的数据（避免自证）。 */
interface ExpectedSection {
  key: string;
  label: string;
  path: string;
  /** 相对 app/pages，不含 .vue。 */
  page: string;
  /** components/settings 下的配置组件。 */
  component: string;
  /** 菜单层级：站点四页在「站点」分组下，深度 2。 */
  depth: number;
}

const EXPECTED_SECTIONS: ExpectedSection[] = [
  { key: "site-basics", label: "站点基础", path: "/site", page: "site/index", component: "SettingsSiteBasics", depth: 2 },
  { key: "site-login", label: "登录与发码", path: "/site/login", page: "site/login", component: "SettingsSiteLogin", depth: 2 },
  {
    key: "site-publish",
    label: "发布与审核",
    path: "/site/publish",
    page: "site/publish",
    component: "SettingsSitePublish",
    depth: 2,
  },
  { key: "site-chat", label: "私信", path: "/site/chat", page: "site/chat", component: "SettingsSiteChat", depth: 2 },
  { key: "ai", label: "AI 助手", path: "/site/ai", page: "site/ai", component: "SettingsAi", depth: 1 },
  { key: "mail", label: "邮件", path: "/site/mail", page: "site/mail", component: "SettingsMail", depth: 1 },
  {
    key: "storage",
    label: "对象存储",
    path: "/site/storage",
    page: "site/storage",
    component: "SettingsStorage",
    depth: 1,
  },
  {
    key: "oauth",
    label: "第三方登录",
    path: "/site/oauth",
    page: "site/oauth",
    component: "SettingsOAuth",
    depth: 1,
  },
  { key: "log", label: "日志", path: "/site/log", page: "site/log", component: "SettingsLog", depth: 1 },
  {
    key: "upload",
    label: "上传策略",
    path: "/site/upload",
    page: "site/upload",
    component: "SettingsUpload",
    depth: 1,
  },
  { key: "deco", label: "装饰", path: "/site/deco", page: "site/deco", component: "SettingsDeco", depth: 1 },
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

describe("站点配置编辑域清单（路由与组件映射的唯一来源）", () => {
  it("十一个编辑域的顺序、标题、路由与组件名固定", () => {
    expect(
      ALL_SITE_SETTINGS_SECTIONS.map(({ key, label, path, component }) => ({
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
    expect(ALL_SITE_SETTINGS_SECTIONS).toHaveLength(11);
    // 站点四页与其余七域在清单里的切分也是固定口径。
    expect(SITE_SETTINGS_PAGES.map((section) => section.key)).toEqual([
      "site-basics",
      "site-login",
      "site-publish",
      "site-chat",
    ]);
    expect(SITE_SETTINGS_SECTIONS).toHaveLength(7);
  });

  it("站点四页各自声明自己拥有的 site 段顶层块，且互不重叠", () => {
    const owned = SITE_SETTINGS_PAGES.flatMap((section) => section.blocks);
    expect(new Set(owned).size).toBe(owned.length);
    expect(owned.sort()).toEqual(["article", "chat", "comment", "login", "seo", "site_info"]);

    for (const section of SITE_SETTINGS_PAGES) {
      expect(section.blocks.length, section.key).toBeGreaterThan(0);
    }
    // 其余七域各用自己整段端点，不参与 site 段的读-改-写。
    for (const section of SITE_SETTINGS_SECTIONS) {
      expect(section.blocks, section.key).toEqual([]);
    }
  });

  it("路由唯一、都是绝对路径，且全部挂在 /site 之下", () => {
    const paths = ALL_SITE_SETTINGS_SECTIONS.map((section) => section.path);
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

describe("站点配置菜单层级（三级）", () => {
  // 站点配置下：1 个「站点」分组 + 7 个编辑域 + 1 个运营位。
  const OPERATIONAL_CHILDREN = [{ label: "右栏卡片", path: "/site/sidebar-cards" }];

  it("「站点配置」是纯分组，子项 = 站点分组 + 七个域 + 右栏卡片", () => {
    const group = ADMIN_NAV_TREE.find((node) => node.label === "站点配置");
    expect(group).toBeDefined();
    expect(group!.path).toBeUndefined();

    const children = group!.children ?? [];
    const siteGroup = children.find((child) => child.label === "站点");
    expect(siteGroup).toBeDefined();
    // 「站点」自身也是纯分组：没有 landing page，`/site` 归「站点基础」。
    expect(siteGroup!.path).toBeUndefined();

    expect((siteGroup!.children ?? []).map(({ label, path }) => ({ label, path }))).toEqual(
      SITE_SETTINGS_PAGES.map(({ label, path }) => ({ label, path })),
    );

    const domainChildren = children.filter(
      (child) => child.label !== "站点" && !OPERATIONAL_CHILDREN.some((extra) => extra.path === child.path),
    );
    expect(domainChildren.map(({ label, path }) => ({ label, path }))).toEqual(
      SITE_SETTINGS_SECTIONS.map(({ label, path }) => ({ label, path })),
    );

    for (const extra of OPERATIONAL_CHILDREN) {
      expect(children.map(({ label, path }) => ({ label, path }))).toContainEqual(extra);
    }
    expect(children.length).toBe(1 + SITE_SETTINGS_SECTIONS.length + OPERATIONAL_CHILDREN.length);
  });

  it("十一个编辑域都能在菜单里查到，层级与父级归属正确", () => {
    for (const section of EXPECTED_SECTIONS) {
      const item = navItemByPath(section.path);
      expect(item).toMatchObject({ label: section.label, depth: section.depth });
      if (section.depth === 2) {
        // 站点四页的父级是「站点」分组（父分组没有自身 path）。
        expect(item!.parentLabel).toBe("站点");
        expect(item!.parentPath).toBeNull();
      } else {
        expect(item!.parentLabel).toBe("站点配置");
        expect(item!.parentPath).toBeNull();
      }
    }
  });

  it("深链接到站点四页：面包屑三层，两层分组都展开并高亮", () => {
    for (const section of SITE_SETTINGS_PAGES) {
      expect(navTrail(section.path).map((item) => item.label)).toEqual(["站点配置", "站点"]);
      expect(activeGroupKeys(section.path)).toEqual(["group:站点配置", "group:站点配置 / 站点"]);

      const entries = buildAdminNavMenu(ADMIN_NAV_TREE, section.path, activeGroupKeys(section.path));
      const siteConfig = entries.find((entry) => entry.label === "站点配置");
      if (siteConfig?.kind !== "group") throw new Error("「站点配置」应渲染成分组");
      expect(siteConfig.active).toBe(true);
      expect(siteConfig.expanded).toBe(true);

      const siteGroup = siteConfig.children.find((child) => child.label === "站点");
      if (siteGroup?.kind !== "group") throw new Error("「站点」应渲染成分组");
      expect(siteGroup.active).toBe(true);
      expect(siteGroup.expanded).toBe(true);
      // 只亮当前页，兄弟页不亮。
      expect(siteGroup.children.filter((child) => child.active).map((child) => child.label)).toEqual([
        section.label,
      ]);
    }
  });

  it("深链接到其它域：仍是一层面包屑，只有「站点配置」展开", () => {
    for (const section of SITE_SETTINGS_SECTIONS) {
      expect(navTrail(section.path).map((item) => item.label)).toEqual(["站点配置"]);
      expect(activeGroupKeys(section.path)).toEqual(["group:站点配置"]);
    }
  });

  it("两层同名分组的展开态互不串味（key 带层级链）", () => {
    const expanded = activeGroupKeys("/site/chat");
    // 只展开两层自己，不把别的分组一起打开。
    expect(expanded).not.toContain("group:站点");
    expect(reconcile(expanded, "/site/deco")).toEqual([
      "group:站点配置",
      "group:站点配置 / 站点",
      // 切换到二级页面后，「站点」分组保持原状（它已经在列表里）。
    ]);
  });
});

/** 小工具：把导航到另一个路由后的展开态结算一次，避免测试里重复手写。 */
function reconcile(expanded: string[], path: string): string[] {
  return [...new Set([...expanded, ...activeGroupKeys(path)])];
}

describe("站点配置编辑域页面文件（刷新保持当前域）", () => {
  it("app/pages/site 下十一个编辑域各一个页面文件，且多一个「右栏卡片」运营位页面", () => {
    const domainPages = EXPECTED_SECTIONS.map((section) => section.page).sort();
    const actual = sitePageFiles();
    expect(actual.filter((page) => domainPages.includes(page))).toEqual(domainPages);

    expect(actual).toContain("site/sidebar-cards");
    expect(actual.length).toBe(domainPages.length + 1);
  });

  it("每个路由都能解析到一个真实页面文件", () => {
    for (const section of EXPECTED_SECTIONS) {
      const candidates = pageFileCandidates(section.path)
        .map((candidate) => `${candidate}.vue`)
        .filter((file) => existsSync(join(pagesDir, file)));
      expect(candidates, section.path).toEqual([`${section.page}.vue`]);
    }
  });

  it("每个页面只渲染自己那一个配置组件，不再有页内 tab 切换", () => {
    for (const section of EXPECTED_SECTIONS) {
      const source = pageSource(section.page);

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

  it("十一个编辑域的配置组件都在原位置", () => {
    for (const section of EXPECTED_SECTIONS) {
      expect(
        existsSync(join(settingsDir, `${section.component}.vue`)),
        section.component,
      ).toBe(true);
    }
  });

  it("站点四页的保存都走读-改-写，只覆盖自己拥有的块", () => {
    for (const section of SITE_SETTINGS_PAGES) {
      const source = readFileSync(join(settingsDir, `${section.component}.vue`), "utf8");
      // 保存前必须重新取一次最新整段（否则会覆盖其它页/无 UI 块）。
      expect(source, section.component).toContain("applySiteSectionPatch");
      const blocks = [...source.matchAll(/OWNED_BLOCKS\s*=\s*\[([^\]]*)\]/gu)].map((match) =>
        match[1]!.split(",").map((raw) => raw.trim().replace(/^["']|["']$/gu, "")).filter(Boolean),
      );
      expect(blocks, section.component).toHaveLength(1);
      expect(blocks[0], section.component).toEqual([...section.blocks]);
    }
  });
});
