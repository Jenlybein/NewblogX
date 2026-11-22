// nginx 模板的 gzip 配置：后台「内容过滤」页能不能打开，取决于这一条。
//
// 2026-11-22 线上故障：6 万条词表的段响应是 ~3.8MB JSON，未压缩走公网，后台页面
// 15s 超时 → 报「内容过滤配置加载失败」。JS/CSS 之前就压过（_nuxt 的 348KB chunk
// → 109KB），所以 gzip 本身是通的，缺的只是 application/json 这个类型。
// 这条用例钉住它，避免以后"顺手精简 gzip_types"又把 JSON 掉出去。
//
// 用 node --test（零依赖），与 scripts/content-filter 的用例同一处置：
// 不进 gates.json，由 ci.yml 的 frontend-lint 与 check.sh 的 gates 段直接执行。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const template = readFileSync(join(repoRoot, "deploy", "docker", "nginx", "nginx.conf.tmpl"), "utf8");

// gzip_types 可能跨行续写：先把整段抠出来再压成一行判断。
function gzipTypes() {
  const match = template.match(/gzip_types\s+([^;]+);/);
  return match ? match[1].replace(/\s+/g, " ") : "";
}

test("gzip_types 里包含 application/json（API 响应全是 JSON）", () => {
  assert.match(gzipTypes(), /application\/json/);
});

test("反代响应允许压缩（gzip_proxied any）", () => {
  assert.match(template, /gzip_proxied\s+any;/);
});

test("gzip 开着且保留最小长度门槛", () => {
  assert.match(template, /gzip\s+on;/);
  assert.match(template, /gzip_min_length/);
});
