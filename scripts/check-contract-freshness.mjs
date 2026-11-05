#!/usr/bin/env node
// 「契约（开放接口契约）未过期」判定：持续集成、本地门禁、门禁自检三处共用这一份实现。
//
// 判据（不再用 git diff --exit-code）：
//   1. 生成工具必须按仓库级 tool-versions.env 的安装 ref 装好；
//   2. 生成器把契约写到**临时目录**，退出码非 0 = COMMAND_FAILED；
//   3. 退出码 0 但临时产物不存在 = SILENT_SKIP（与命令失败输出不同原因）；
//   4. 条目数量（paths / operations / schemas）不得低于 contract-structures.json
//      登记的基线（独立事实源），下降 = ENTRY_COUNT_DROPPED；
//   5. 临时产物与已提交契约逐字比对，不同 = STALE；
//   6. 清单里点名的本次新增结构确实在场，缺席 = ADDED_STRUCTURE_MISSING。
//
// 生成器通过 --generator 注入 argv（默认 `make openapi`，由 Makefile 读 SWAG_OUTPUT_DIR）。
// 占位符 __OUTPUT_DIR__ / __ROOT__ / __SOURCE__ 由本模块替换为临时目录 / 仓库根 / 已提交契约。
// 这样行为用例与门禁自检可以换一个受控生成器，而真门禁跨的是同一个判定接口。
//
// 输出里的 [reason] 标记是机器可判的失败原因；不同失效形态给不同标记。
import { spawnSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_ROOT = path.resolve(here, '..');
const CONTRACT_REL = path.join('backend', 'docs', 'swagger.json');
const STRUCTURES_FILE = path.join(here, 'contract-structures.json');
const VERSIONS_FILE = path.join(here, 'tool-versions.env');
const METHODS = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options', 'trace'];

function parseArgs(argv) {
  const out = { root: DEFAULT_ROOT, generator: null, skipVersion: false, versionsFile: VERSIONS_FILE };
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--root') out.root = path.resolve(argv[++i]);
    else if (argv[i] === '--versions-file') out.versionsFile = path.resolve(argv[++i]);
    else if (argv[i] === '--generator') {
      out.generator = [];
      while (i + 1 < argv.length && !argv[i + 1].startsWith('--')) out.generator.push(argv[++i]);
    } else if (argv[i] === '--skip-version-check') out.skipVersion = true;
    else fail('BAD_ARGS', `未知参数 ${argv[i]}`);
  }
  out.root = path.resolve(out.root);
  if (!out.generator || out.generator.length === 0) out.generator = ['make', 'openapi'];
  return out;
}

function report(reason, detail) {
  console.log(`[reason] ${reason}`);
  console.log(detail);
  process.exit(reason === 'OK' ? 0 : 1);
}
function fail(reason, detail) { report(reason, `✗ 契约未过期判定失败：${detail}`); }

function readVersions(file) {
  const text = fs.readFileSync(file, 'utf8');
  const get = key => text.match(new RegExp(`^\\s*${key}\\s*=\\s*(\\S+)\\s*$`, 'm'))?.[1];
  const module = get('SWAG_MODULE');
  const version = get('SWAG_VERSION');
  if (!module || !version) throw new Error('tool-versions.env 缺少 SWAG_MODULE / SWAG_VERSION');
  return { module, version };
}

function operationCount(doc) {
  return Object.values(doc.paths || {}).reduce(
    (n, item) => n + Object.keys(item || {}).filter(k => METHODS.includes(k)).length,
    0,
  );
}

function counts(doc) {
  return {
    paths: Object.keys(doc.paths || {}).length,
    operations: operationCount(doc),
    schemas: Object.keys(doc.components?.schemas || {}).length,
  };
}

function manifest() {
  const data = JSON.parse(fs.readFileSync(STRUCTURES_FILE, 'utf8'));
  const baseline = data.minimumEntries || null;
  const paths = [];
  const schemas = [];
  for (const batch of data.addedIn || []) {
    for (const [p, methods] of Object.entries(batch.paths || {})) {
      for (const m of methods) paths.push({ path: p, method: m });
    }
    schemas.push(...(batch.schemas || []));
  }
  return { baseline, paths, schemas };
}

function checkInstalledVersion(file) {
  const { module, version } = readVersions(file);
  const go = spawnSync('go', ['env', 'GOPATH'], { encoding: 'utf8' });
  if (go.status !== 0) return { ok: false, detail: 'go env GOPATH 不可用，无法核对 swag 安装版本' };
  const gopath = (go.stdout || '').trim();
  const bin = path.join(gopath, 'bin', process.platform === 'win32' ? 'swag.exe' : 'swag');
  if (!fs.existsSync(bin)) return { ok: false, detail: `未找到 ${bin}，请先按 tool-versions.env 安装 ${module}@${version}` };
  const info = spawnSync('go', ['version', '-m', bin], { encoding: 'utf8' });
  if (info.status !== 0) return { ok: false, detail: `读取 ${bin} 的构建信息失败` };
  const line = (info.stdout || '').split('\n').find(l => /\bmod\s/.test(l)) || '';
  const got = line.trim().split(/\s+/)[2] || '';
  if (got !== version || !(info.stdout || '').includes(module.replace(/\/cmd\/swag$/, ''))) {
    return { ok: false, detail: `swag 安装 ref 与 tool-versions.env 不一致：期望 ${module}@${version}，实得 ${got || '未知'}` };
  }
  return { ok: true, version };
}

const opts = parseArgs(process.argv.slice(2));
const contractPath = path.join(opts.root, CONTRACT_REL);
if (!fs.existsSync(contractPath)) fail('CONTRACT_MISSING', `已提交契约不存在：${contractPath}`);
const committed = fs.readFileSync(contractPath);
try { JSON.parse(committed.toString('utf8')); }
catch (e) { fail('CONTRACT_INVALID', `已提交契约不是合法 JSON：${e.message}`); }

if (opts.skipVersion !== true) {
  if (!fs.existsSync(opts.versionsFile)) fail('TOOL_VERSION_MISMATCH', `仓库级工具版本文件不存在：${opts.versionsFile}`);
  const v = checkInstalledVersion(opts.versionsFile);
  if (!v.ok) fail('TOOL_VERSION_MISMATCH', v.detail);
}

const manifestData = manifest();
if (!manifestData.baseline) fail('BAD_ARGS', 'contract-structures.json 缺少 minimumEntries 基线');

const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'blogx-contract-freshness-'));
const outputDir = path.join(tmpDir, 'generated');
fs.mkdirSync(outputDir, { recursive: true });
const generatedPath = path.join(outputDir, 'swagger.json');

// 默认走 `make openapi`：Makefile 的 openapi target 读 SWAG_OUTPUT_DIR，
// 与人工重新生成走的是同一条生成命令，只是产物落到临时目录。
const replacements = { __OUTPUT_DIR__: outputDir, __ROOT__: opts.root, __SOURCE__: contractPath };
const argv = opts.generator.map(a => replacements[a] ?? a);
const cmd = argv[0];
const args = argv.slice(1);
const gen = spawnSync(cmd, args, {
  cwd: path.join(opts.root, 'backend'),
  env: { ...process.env, SWAG_OUTPUT_DIR: outputDir, BLOGX_CONTRACT_OUTPUT_DIR: outputDir },
  encoding: 'utf8',
  maxBuffer: 20 * 1024 * 1024,
});

function cleanup() { fs.rmSync(tmpDir, { recursive: true, force: true }); }

if (gen.error || gen.status !== 0) {
  const tail = ((gen.stderr || '') + (gen.stdout || '')).split('\n').slice(-6).join('\n');
  cleanup();
  fail('COMMAND_FAILED', `生成命令失败（${cmd} ${args.join(' ')}，exit ${gen.status ?? 'spawn error'}）：\n${tail}`);
}
if (!fs.existsSync(generatedPath)) {
  cleanup();
  fail('SILENT_SKIP', '生成命令退出码 0，但临时目录里没有 swagger.json——生成器静默跳过整份写入（假绿 / 门禁撒谎）。');
}

const generated = fs.readFileSync(generatedPath);
let generatedDoc;
try { generatedDoc = JSON.parse(generated.toString('utf8')); }
catch (e) {
  cleanup();
  fail('CONTRACT_INVALID', `重新生成的契约不是合法 JSON：${e.message}`);
}

// ① 条目数量不下降：相对仓库级清单登记的基线，而不是相对自己算出来的数
//    （"生成结果自己跟自己比"是永真通过，抓不出整份退化）。
const after = counts(generatedDoc);
for (const key of ['paths', 'operations', 'schemas']) {
  if (after[key] < manifestData.baseline[key]) {
    cleanup();
    fail('ENTRY_COUNT_DROPPED', `${key} 条目数量低于基线：基线 ${manifestData.baseline[key]} → 重新生成 ${after[key]}。`);
  }
}

// ② 本次新增结构确实在场（先于逐字比对：结构缺席是比"内容不同"更具体的原因）。
for (const s of manifestData.schemas) {
  if (!(s in (generatedDoc.components?.schemas || {}))) {
    cleanup();
    fail('ADDED_STRUCTURE_MISSING', `本次新增 schema 不在场：${s}。`);
  }
}
for (const { path: p, method } of manifestData.paths) {
  if (!(generatedDoc.paths?.[p]?.[method])) {
    cleanup();
    fail('ADDED_STRUCTURE_MISSING', `本次新增端点不在场：${method.toUpperCase()} ${p}。`);
  }
}

// ③ 逐字比对：重新生成的产物必须与已提交契约同一个字节序列。
if (!generated.equals(committed)) {
  const head = buf => JSON.stringify(buf.toString('utf8').slice(0, 160));
  cleanup();
  fail('STALE', `重新生成的契约与已提交产物不逐字一致（生成 ${head(generated)} / 已提交 ${head(committed)}）。\n  请在本地执行 cd backend && make openapi，并把产物与两端 openapi.d.ts 同批提交。`);
}

cleanup();
report('OK', `✓ 契约未过期：逐字一致，条目数量 ${JSON.stringify(after)} 未低于基线，${manifestData.schemas.length + manifestData.paths.length} 项新增结构在场。`);
