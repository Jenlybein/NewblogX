/* eslint-disable no-console -- 这是 CLI 脚本，输出进度就是它的用途 */
// 把契约里的 ID 字段从 integer 改成 string。
//
// # 为什么必须在流水线里修，而不是在后端打 swaggertype 标签
//
// 后端的 ID 类型是 `ctype.ID`，它的 MarshalJSON 返回 `id.String()` ——
// 也就是说**JSON 里这些字段是字符串**（雪花 ID 超出 JS 安全整数，
// 后端故意序列化成字符串）。
//
// 但 swag 只看 Go 类型（uint64），不知道有自定义 MarshalJSON，于是输出
// `type: integer`。前端若照契约把 `id` 当 number 用，运行时会拿到
// `"1834..."` 这样的字符串 —— 这不是"精度不够"，是**契约在事实层面写错了**。
//
// 两种修法：
//   1. 给每个 ctype.ID 字段加 `swaggertype:"string"` 标签 —— 精确，但要动
//      100 处 struct，而且 domain 层结构体会被迫知道 swag 的存在；
//   2. 在图里改一次（本文件）—— 后端零改动，规则集中一处、带自校验。
// 选 2，代价是"按字段名判断"这个约定：本项目所有 ID 都是 ctype.ID，
// 命名统一为 `id` / `*_id` / `*_ids`，所以规则成立。
//
// # 为什么要遍历整个 spec 而不只是 components.schemas
//
// 第一版只走了 schemas，结果漏掉 8 处嵌在 `paths` 里的 `user_id`
// （query/body 参数）。契约里两条路径都得改，否则 TS 里同一个概念有两种类型。
//
// 用法：node scripts/fix-openapi-id-types.mjs .openapi3.json

import fs from "node:fs";

/** 哪些属性名算 ID。 */
// 命名有两套：sql/json 风格（user_id / tag_ids）与没写 json 标签的
// Go 驼峰（userID / avatarImageID）。两种都算 ID —— 漏了后一种，
// 那些 schema 里的 ID 还会是数字（目前前端没用它们，但没理由留个坑）。
const ID_PATTERN = /^(id|.+_ids?|.+IDs?)$/;

/** 判断一个 schema 片段是不是整数（含 int64）。 */
function isInteger(node) {
  return Boolean(node) && typeof node === "object" && (node.type === "integer" || node.type === "number");
}

/**
 * 遍历整个 spec，把 ID 属性改成 string；rewrite=false 时只收集问题位置。
 *
 * @param {any} node
 * @param {string} path
 * @param {string[]} hits
 * @param {boolean} rewrite 是否真的改写
 */
function walk(node, path, hits, rewrite) {
  if (Array.isArray(node)) {
    node.forEach((item, i) => walk(item, `${path}[${i}]`, hits, rewrite));
    return;
  }
  if (!node || typeof node !== "object") {
    return;
  }

  // OpenAPI 3 的参数不是 properties：形态是 { name, in, schema }。
  // 第一版只处理 properties，于是 8 处 `?user_id=` 全都漏了 —— 而且
  // 自校验用的是同一套遍历，所以它也跟着漏，报"0 处剩余"。
  // 教训：改写逻辑和自校验逻辑不能共享同一个盲区，见下面 assert 的独立扫描。
  if (typeof node.name === "string" && node.schema && typeof node.schema === "object" && ID_PATTERN.test(node.name)) {
    if (isInteger(node.schema)) {
      if (rewrite) {
        delete node.schema.format;
        node.schema.type = "string";
      }
      hits.push(`${path}[param ${node.name}]`);
    } else if (node.schema.type === "array" && isInteger(node.schema.items)) {
      if (rewrite) {
        delete node.schema.items.format;
        node.schema.items.type = "string";
      }
      hits.push(`${path}[param ${node.name}][]`);
    }
  }

  const props = node.properties;
  if (props && typeof props === "object") {
    for (const [name, schema] of Object.entries(props)) {
      if (!ID_PATTERN.test(name) || !schema || typeof schema !== "object") {
        continue;
      }

      // 标量 ID：integer -> string
      if (isInteger(schema)) {
        if (rewrite) {
          delete schema.format;
          schema.type = "string";
        }
        hits.push(`${path}.${name}`);
        continue;
      }

      // ID 数组：items 从 integer -> string
      if (schema.type === "array" && isInteger(schema.items)) {
        if (rewrite) {
          delete schema.items.format;
          schema.items.type = "string";
        }
        hits.push(`${path}.${name}[]`);
      }
    }
  }

  // 递归所有子节点（除了已经处理过的 properties 本身）。
  // 这里的白名单只是为了避免重复进入 properties（它的值已在上方处理）。
  for (const [key, value] of Object.entries(node)) {
    if (key === "properties") {
      for (const [name, schema] of Object.entries(value ?? {})) {
        walk(schema, `${path}.properties.${name}`, hits, rewrite);
      }
      continue;
    }
    if (value && typeof value === "object") {
      walk(value, `${path}.${key}`, hits, rewrite);
    }
  }
}

const file = process.argv[2];
if (!file) {
  console.error("用法: node scripts/fix-openapi-id-types.mjs <openapi3.json>");
  process.exit(2);
}

const spec = JSON.parse(fs.readFileSync(file, "utf8"));

const hits = [];
walk(spec, "spec", hits, true);

// 自校验：**独立**扫描整份 spec，不复用 walk 的遍历。
//
// 为什么不用同一个函数：改写与校验共享遍历就会共享盲区 ——
// 第一版只处理 properties，8 处 `?user_id=` 参数既没被改、也没被查出来，
// 脚本还打印了"0 处剩余"。这里的扫描只认"键名像 ID + 值为整数"这两件事，
// 与改写路径无关，所以能兜住改写漏掉的地方。
function assertNoNumericId(node, path, bad) {
  if (Array.isArray(node)) {
    node.forEach((item, i) => assertNoNumericId(item, `${path}[${i}]`, bad));
    return;
  }
  if (!node || typeof node !== "object") {
    return;
  }

  // 形态一：properties 里的 ID 字段
  for (const [name, child] of Object.entries(node.properties ?? {})) {
    if (!ID_PATTERN.test(name) || !child || typeof child !== "object") {
      continue;
    }
    if (isInteger(child) || (child.type === "array" && isInteger(child.items))) {
      bad.push(`${path}.properties.${name}`);
    }
  }

  // 形态二：参数
  if (typeof node.name === "string" && ID_PATTERN.test(node.name) && node.schema) {
    if (isInteger(node.schema) || (node.schema.type === "array" && isInteger(node.schema.items))) {
      bad.push(`${path}[param ${node.name}]`);
    }
  }

  for (const [key, value] of Object.entries(node)) {
    if (value && typeof value === "object") {
      assertNoNumericId(value, `${path}.${key}`, bad);
    }
  }
}

const leftover = [];
assertNoNumericId(spec, "spec", leftover);
if (leftover.length > 0) {
  console.error(`仍有 ${leftover.length} 处 ID 字段是数字（规则没覆盖到）：`);
  for (const item of leftover.slice(0, 10)) {
    console.error("  -", item);
  }
  process.exit(1);
}

fs.writeFileSync(file, `${JSON.stringify(spec, null, 2)}\n`, "utf8");
console.log(`openapi: ID 字段 integer → string，共 ${hits.length} 处`);
