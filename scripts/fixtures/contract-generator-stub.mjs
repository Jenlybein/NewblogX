#!/usr/bin/env node
// 契约未过期判定的受控生成器（仅用于行为用例与门禁自检演练）。
//
// 与 swag CLI 同形：接受 -o <输出目录>，把 swagger.json 写进去。
// STUB_MODE 决定外部可观察行为，而不是判定实现：
//   copy            正常重生成：把 STUB_SOURCE 逐字节复制到输出目录
//   stale           正常写文件，但内容与已提交契约不同（模拟未重新生成）
//   drop-entry      少写一个 path 条目（条目数量下降到基线以下）
//   drop-added      少写一个本次新增 schema（清单里点名的那一个）
//   silent-skip     退出 0，但完全不写产物（生成器静默跳过，假绿形态）
//   command-failed  退出非 0（命令失败形态）
import * as fs from 'node:fs';
import * as path from 'node:path';

const argv = process.argv.slice(2);
const outDir = argv[argv.indexOf('-o') + 1];
const source = process.env.STUB_SOURCE || '';
const mode = process.env.STUB_MODE || 'copy';

if (!outDir) {
  console.error('stub: 缺少 -o <dir>');
  process.exit(90);
}
if (mode === 'command-failed') {
  console.error('stub: 模拟生成命令失败');
  process.exit(17);
}
// 关键：退出码 0，产物不存在——这正是 swag 注解写歪时的失效形态。
if (mode === 'silent-skip') {
  console.log('stub: 生成器静默跳过（exit 0，未写产物）');
  process.exit(0);
}

const bytes = fs.readFileSync(source);
const doc = JSON.parse(bytes.toString('utf8'));
fs.mkdirSync(outDir, { recursive: true });
const target = path.join(outDir, 'swagger.json');

if (mode === 'copy') {
  // 正常路径：逐字节复制，保证不会因 JSON 重新序列化而出现"假陈旧"。
  fs.writeFileSync(target, bytes);
} else {
  if (mode === 'stale') doc.info.title = 'BlogX API (stale fixture)';
  if (mode === 'drop-entry') delete doc.paths[Object.keys(doc.paths)[0]];
  // 把点名的那个键换成另一个名字，条目总数不变：单独验「新增结构在场」，
  // 否则条目数下降会先触发另一条原因，两种红就分不开了。
  if (mode === 'drop-added') {
    delete doc.components.schemas['myblogx_internal_chat_domain.MessageListData'];
    doc.components.schemas['__stub_padding__'] = { type: 'object' };
  }
  fs.writeFileSync(target, JSON.stringify(doc, null, 4));
}
console.log(`stub: 已写出 ${target}（${mode}）`);
