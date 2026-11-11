import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 全局样式不许撞组件作用域里的类名（票 359 的回归）。
 *
 * 真实事故：票 345 给工作室收藏夹在 `tailwind.css` 里加了全局 `.folder-card` /
 * `.folder-card__title`，而读者站的 `FolderCard.vue`（合集/项目文件夹卡）用的
 * 正是同一批类名。组件样式是 `scoped` 的（`.folder-card[data-v-x]`，特异性更高），
 * 所以**组件自己声明过的属性会赢**；但全局那份还额外带了
 * `background` / `border` / `border-radius` / `white-space: nowrap`——
 * 组件没声明的属性就由全局补齐，文件夹卡于是变成一块灰板，标题被压成单行截断。
 *
 * 这类缺陷 vitest 看不见（DOM 结构没变）、typecheck 看不见（类名是字符串）、
 * 设计 token 门禁也看不见（它只管颜色与圆角取值）。所以在这里用一条静态检查钉住：
 * **只要某个组件在自己的 `<style scoped>` 里声明了某类名，全局 CSS 就不许再声明同名类。**
 *
 * 判据刻意做得窄（只比类名、只看 scoped 样式）：目的是拦住"命名撞车"，
 * 不是禁掉全局样式本身。全局工具类（`.mt-5` 之类）由 Tailwind 生成，不在此列。
 */

const APP_ROOT = join(process.cwd(), "app");
const GLOBAL_CSS = join(APP_ROOT, "assets/css/tailwind.css");

/**
 * 共享工具类与状态钩子的前缀：**允许**全局与组件同时声明。
 *
 * 这几类是刻意共用的：
 *   - `glass-*`：全局玻璃工具类，组件在自己的 scoped 里补修饰是正常用法；
 *   - `is-*` / `has-*`：状态钩子，本来就跨组件复用；
 *   - `n-*`：naive-ui 的内部类，组件做定点覆盖；
 *   - `empty-state*`：空态的语义类，页面与组件都引用。
 *
 * 除这些之外的类名必须**唯一归属于一个组件**：撞车的代价不是样式优先级，
 * 而是"组件没声明的属性被全局补齐"（票 359 的 `.folder-card` 事故）。
 */
const SHARED_PREFIXES = ["glass-", "is-", "has-", "n-", "empty-state", "js-"];

function isShared(name: string): boolean {
  return SHARED_PREFIXES.some(prefix => name.startsWith(prefix));
}

/** 递归收集 .vue 文件。 */
function collectVueFiles(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    const info = statSync(full);
    if (info.isDirectory()) {
      out.push(...collectVueFiles(full));
    } else if (name.endsWith(".vue")) {
      out.push(full);
    }
  }
  return out;
}

/** 取出 `<style scoped>` 块里的类名。 */
function scopedClassNames(source: string): Set<string> {
  const out = new Set<string>();
  const styleBlocks = source.matchAll(/<style[^>]*scoped[^>]*>([\s\S]*?)<\/style>/gu);
  for (const block of styleBlocks) {
    const css = block[1] ?? "";
    for (const match of css.matchAll(/\.([A-Za-z_][\w-]*)/gu)) {
      out.add(match[1]!);
    }
  }
  return out;
}

/** 取出全局 CSS 里声明的类名（只看选择器，不看声明块里的取值）。 */
function globalClassNames(css: string): Set<string> {
  const out = new Set<string>();
  // 去掉注释，避免把注释里举例的类名当成真规则。
  const stripped = css.replace(/\/\*[\s\S]*?\*\//gu, "");
  for (const match of stripped.matchAll(/\.([A-Za-z_][\w-]*)/gu)) {
    out.add(match[1]!);
  }
  return out;
}

describe("全局样式与组件作用域类名不冲突", () => {
  it("组件 `<style scoped>` 里声明的类名，全局 CSS 不再重复声明", () => {
    const globalNames = globalClassNames(readFileSync(GLOBAL_CSS, "utf8"));

    const collisions: string[] = [];
    for (const file of collectVueFiles(APP_ROOT)) {
      const source = readFileSync(file, "utf8");
      for (const name of scopedClassNames(source)) {
        if (globalNames.has(name) && !isShared(name)) {
          collisions.push(`${file.replace(APP_ROOT, "app")} → .${name}`);
        }
      }
    }

    expect(
      collisions,
      `这些类名同时被组件（scoped）与全局 tailwind.css 声明，会给组件叠上意料之外的属性；` +
        `给页面侧换一个带前缀的名字：\n${collisions.join("\n")}`,
    ).toEqual([]);
  });

  it("门禁自身有效：故意造一个冲突必须被抓到", () => {
    // 用内联源码验证判据，而不是去改仓库文件：这条 canary 证明上面的用例
    // 不是"永远通过"的空断言。
    const scoped = `<style scoped>\n.folder-card { display: block; }\n</style>`;
    const global = `.folder-card { background: red; }\n.mt-5 { margin-top: 1.25rem; }`;

    const names = scopedClassNames(scoped);
    const globals = globalClassNames(global);
    const hit = [...names].filter(name => globals.has(name));

    expect(hit).toEqual(["folder-card"]);
  });
});
