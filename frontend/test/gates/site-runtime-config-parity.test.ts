import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 「读者站站点运行时配置投影与后端 `conf/site` 同源」门禁（#39）。
 *
 * 为什么需要它：`SiteRuntimeConfig` 是手写投影，不在 OpenAPI 契约里（`/site/{name}`
 * 的 data 是 `map[string]any`，没有可派生的类型）。手写就会漂移 —— #39 之前它声明了
 * 后端根本没有的 `site_info.subtitle`，却漏了后端确实有的 `logo` / `beian`，
 * 而 `subtitle` 还被首页侧栏当成简介兜底，于是那段回落在生产里永远是死分支。
 *
 * 门禁的判定口径是**逐字段相等**而不是"子集"：`site_info` 在读者站是完整投影，
 * 多一个字段意味着有人照着想象中的后端在写前端，少一个意味着新字段没接进来。
 *
 * 两侧都从源码取（Go 结构体的 json tag / TS 接口的键），不维护第三份字段清单 ——
 * 否则清单本身又会漂移，门禁就成了自证。
 *
 * vitest 以 project root（frontend/）为 cwd；仓库根是上一级目录。
 */
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");
const GO_SITE_CONF = join(repoRoot, "backend", "conf", "site", "enter.go");
const TS_PROJECTION = join(projectDir, "app", "types", "api.ts");

/** 从 `openIndex` 处的 `{` 开始，返回与之配对的 `}` 的下标。 */
function matchingBrace(source: string, openIndex: number): number {
  let depth = 0;
  for (let index = openIndex; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) return index;
    }
  }
  throw new Error("花括号没有闭合");
}

/** Go 结构体 `type X struct {...}` 里按源码顺序出现的 json 字段名。 */
function goStructJSONKeys(source: string, typeName: string): string[] {
  const marker = `type ${typeName} struct`;
  const start = source.indexOf(marker);
  if (start < 0) throw new Error(`找不到 ${marker}`);
  const open = source.indexOf("{", start);
  const block = source.slice(open + 1, matchingBrace(source, open));
  return [...block.matchAll(/\bjson:"([^",]+)"/gu)].map((match) => match[1]);
}

/** TS 投影里某个 `键: {...}` 对象的字段名；`site_info` / `project` 都是扁平块。 */
function tsObjectKeys(source: string, marker: string): string[] {
  const start = source.indexOf(marker);
  if (start < 0) throw new Error(`找不到 ${marker}`);
  const open = source.indexOf("{", start);
  const block = source.slice(open + 1, matchingBrace(source, open));
  return [...block.matchAll(/^\s*([A-Za-z_$][\w$]*)\??:/gmu)].map((match) => match[1]);
}

const goSource = readFileSync(GO_SITE_CONF, "utf8");
const tsSource = readFileSync(TS_PROJECTION, "utf8");

describe("读者站投影 site_info 与后端 SiteInfo 逐字段一致（真实仓库为绿）", () => {
  it("两侧都真的读到了字段（防止解析器空转后门禁假绿）", () => {
    const goKeys = goStructJSONKeys(goSource, "SiteInfo");
    const tsKeys = tsObjectKeys(tsSource, "site_info: {");

    expect(goKeys.length).toBeGreaterThan(0);
    expect(tsKeys.length).toBeGreaterThan(0);
    expect(goKeys).toContain("title");
    expect(tsKeys).toContain("title");
  });

  it("字段名与顺序完全相等：不多（没有 subtitle）也不少（有 logo / beian）", () => {
    expect(tsObjectKeys(tsSource, "site_info: {")).toEqual(goStructJSONKeys(goSource, "SiteInfo"));
  });

  it("project 投影只声明后端 Project 真有的字段（不再有 abstract）", () => {
    const goKeys = goStructJSONKeys(goSource, "Project");
    const tsKeys = tsObjectKeys(tsSource, "project: {");

    expect(goKeys.length).toBeGreaterThan(0);
    expect(tsKeys.length).toBeGreaterThan(0);
    // 投影可以是子集（图标 / 静态入口读者站用不到），但不许出现后端没有的字段。
    expect(tsKeys.filter((key) => !goKeys.includes(key))).toEqual([]);
    expect(tsKeys).not.toContain("abstract");
  });

  it("login 投影不出现后端没有的键（qq / email / pwd 这类自造名不许回来）", () => {
    const goKeys = goStructJSONKeys(goSource, "Login");
    const tsKeys = tsObjectKeys(tsSource, "login: {");

    expect(goKeys.length).toBeGreaterThan(0);
    expect(tsKeys.length).toBeGreaterThan(0);
    expect(tsKeys.filter((key) => !goKeys.includes(key))).toEqual([]);
    expect(tsKeys).not.toContain("qq");
    expect(tsKeys).not.toContain("email");
    expect(tsKeys).not.toContain("pwd");
  });

  it("login 投影覆盖公告卡实际读取的三个开关", () => {
    expect(tsObjectKeys(tsSource, "login: {")).toEqual(
      expect.arrayContaining(["qq_login", "username_pwd_login", "email_login"]),
    );
  });

  it("index_right.list 的条目只声明后端 ComponentInfo 真有的字段（不再有 abstract）", () => {
    const goKeys = goStructJSONKeys(goSource, "ComponentInfo");
    const tsKeys = tsObjectKeys(tsSource, "list?: Array<{");

    expect(goKeys.length).toBeGreaterThan(0);
    expect(tsKeys.length).toBeGreaterThan(0);
    expect(tsKeys.filter((key) => !goKeys.includes(key))).toEqual([]);
    expect(tsKeys).not.toContain("abstract");
  });
});

describe("canary：投影一旦漂移，门禁必须红", () => {
  it("多写一个后端没有的字段（subtitle）→ 与后端字段不相等", () => {
    const goKeys = goStructJSONKeys(goSource, "SiteInfo");
    const drifted = tsSource.replace("    title?: string;\n", "    title?: string;\n    subtitle?: string;\n");
    const driftedKeys = tsObjectKeys(drifted, "site_info: {");

    expect(driftedKeys).toContain("subtitle");
    expect(driftedKeys).not.toEqual(goKeys);
  });

  it("漏掉一个后端确实有的字段（logo）→ 与后端字段不相等", () => {
    const goKeys = goStructJSONKeys(goSource, "SiteInfo");
    const drifted = tsSource.replace(/^ *logo\?: string;\n/mu, "");
    const driftedKeys = tsObjectKeys(drifted, "site_info: {");

    expect(driftedKeys).not.toContain("logo");
    expect(driftedKeys).not.toEqual(goKeys);
  });

  it("把 login 改回 qq / email / pwd（#56 的真实事故）→ 三个自造键必须被抓出", () => {
    const goKeys = goStructJSONKeys(goSource, "Login");
    const drifted = tsSource.replace(
      /( *)login: \{\n(?:.*\n)*? {2}\};/u,
      "  login: {\n    qq?: boolean;\n    email?: boolean;\n    pwd?: boolean;\n  };\n",
    );
    const driftedKeys = tsObjectKeys(drifted, "login: {");

    expect(driftedKeys).toEqual(["qq", "email", "pwd"]);
    expect(driftedKeys.filter((key) => !goKeys.includes(key))).toEqual(["qq", "email", "pwd"]);
  });
});
