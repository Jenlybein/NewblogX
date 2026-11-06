#!/usr/bin/env node
// 把仓库级参数表的 12 组 min/max/default 注入 make openapi 生成的契约。
// swag v2 的字段 minimum/maximum 标签不接受小数；契约事实仍只来自参数表，
// 生成顺序固定为 make openapi → 本脚本 → 两端 pnpm openapi:types。
import * as fs from 'node:fs';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');
const swaggerArgIndex = process.argv.indexOf('--swagger');
const contractPath = swaggerArgIndex >= 0
  ? path.resolve(process.argv[swaggerArgIndex + 1])
  : path.join(root, 'backend/docs/swagger.json');
const table = JSON.parse(fs.readFileSync(path.join(root, 'config/deco-charm-parameters.json'), 'utf8'));
const doc = JSON.parse(fs.readFileSync(contractPath, 'utf8'));
const charm = doc.components?.schemas?.['conf.Charm'];
if (!charm?.properties) throw new Error('契约缺少 conf.Charm.properties');
for (const p of table.parameters) {
  if (!charm.properties[p.key]) throw new Error(`契约缺少 conf.Charm.${p.key}`);
  charm.properties[p.key].minimum = p.min;
  charm.properties[p.key].maximum = p.max;
  charm.properties[p.key].default = p.default;
}
fs.writeFileSync(contractPath, `${JSON.stringify(doc, null, 4)}\n`);
console.log(`decorated ${table.parameters.length} generated deco parameters in ${contractPath}`);
