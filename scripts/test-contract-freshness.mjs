#!/usr/bin/env node
// 契约未过期判定的行为用例（跨 CLI 接缝，不读实现源码）。
//
// 接缝：node scripts/check-contract-freshness.mjs --root <仓库> [--generator <argv...>]
// 可观察行为：退出码 + stdout 里的 [reason] 标记 + 临时工作区里的产物。
//
// 用例只用「可控生成器」把同一份契约喂进来，验证的是判定的外部行为：
// 真生成 / 静默跳过 / 命令失败 / 陈旧 / 条目数下降 / 新增结构缺席。
// 生成器的实现细节（swag、make）不是本用例的断言对象，由 grep-real 与 canary 覆盖。
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const checker = path.join(here, 'check-contract-freshness.mjs');
const repoRoot = path.resolve(here, '..');
const contractRel = 'backend/docs/swagger.json';
const stub = path.join(here, 'fixtures', 'contract-generator-stub.mjs');

// 每个用例一份独立沙箱：真实契约逐字复制进去，模拟"已提交产物"。
function sandbox(mutate) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'blogx-contract-freshness-'));
  const out = path.join(dir, 'work');
  fs.mkdirSync(path.join(out, 'backend', 'docs'), { recursive: true });
  const committed = fs.readFileSync(path.join(repoRoot, contractRel));
  const target = path.join(out, contractRel);
  let doc = committed;
  if (mutate) {
    const parsed = JSON.parse(committed.toString('utf8'));
    mutate(parsed);
    doc = Buffer.from(JSON.stringify(parsed, null, 4));
  }
  fs.writeFileSync(target, doc);
  return { dir, out, target };
}

function run(args, env = {}) {
  const r = spawnSync(process.execPath, [checker, ...args], {
    cwd: repoRoot,
    env: { ...process.env, ...env },
    encoding: 'utf8',
    maxBuffer: 20 * 1024 * 1024,
  });
  return {
    status: r.status,
    stdout: r.stdout || '',
    stderr: r.stderr || '',
    reason: (r.stdout || '').match(/\[reason\]\s*([A-Z_]+)/)?.[1] ?? null,
  };
}

// 生成器用 node 直接调用（不依赖 make / shell），-o <dir> 与 swag CLI 同形。
// --skip-version-check 把"本机装没装 swag"这个环境事实从判定行为里剥开；
// 版本核对另有独立用例。
function generatorArgs(root, { withVersionCheck = false } = {}) {
  return [
    ...(withVersionCheck ? [] : ['--skip-version-check']),
    '--generator', process.execPath, stub, '-o', '__OUTPUT_DIR__',
    '--root', root,
  ];
}

let passed = 0;
const results = [];
function test(name, fn) {
  try {
    fn();
    passed++;
    results.push(`ok  ${name}`);
  } catch (e) {
    results.push(`FAIL ${name}: ${e.message}`);
  }
}

const cases = [
  {
    name: '真生成且逐字一致 → 通过，且不改动已提交产物',
    mode: 'copy',
    want: { status: 0, reason: 'OK' },
    check: (r, s) => {
      assert.equal(r.status, 0, r.stdout + r.stderr);
      assert.equal(r.reason, 'OK', r.stdout);
      // 判定在临时目录生成，绝不原地改写被检查的文件。
      assert.equal(fs.readFileSync(s.target, 'utf8'), fs.readFileSync(path.join(repoRoot, contractRel), 'utf8'));
    },
  },
  {
    name: '生成器静默跳过（exit 0 但不写产物）→ 判为 SILENT_SKIP',
    mode: 'silent-skip',
    want: { status: 1, reason: 'SILENT_SKIP' },
  },
  {
    name: '生成命令失败（exit 非 0）→ 判为 COMMAND_FAILED，且与静默跳过原因不同',
    mode: 'command-failed',
    want: { status: 1, reason: 'COMMAND_FAILED' },
  },
  {
    name: '生成产物与已提交契约不逐字一致 → 判为 STALE',
    mode: 'stale',
    want: { status: 1, reason: 'STALE' },
  },
  {
    name: '生成产物条目数量下降 → 判为 ENTRY_COUNT_DROPPED',
    mode: 'drop-entry',
    want: { status: 1, reason: 'ENTRY_COUNT_DROPPED' },
  },
  {
    name: '本次新增结构不在场 → 判为 ADDED_STRUCTURE_MISSING',
    mode: 'drop-added',
    want: { status: 1, reason: 'ADDED_STRUCTURE_MISSING' },
  },
];

for (const c of cases) {
  test(c.name, () => {
    const s = sandbox();
    try {
      const r = run(generatorArgs(s.out), {
        STUB_MODE: c.mode,
        STUB_SOURCE: s.target,
      });
      assert.equal(r.status, c.want.status, `${r.stdout}\n${r.stderr}`);
      assert.equal(r.reason, c.want.reason, `${r.stdout}\n${r.stderr}`);
      if (c.check) c.check(r, s);
    } finally {
      fs.rmSync(s.dir, { recursive: true, force: true });
    }
  });
}

// 「故意写歪必须变红」演练：把已提交契约本身写歪（删掉一个新增结构），
// 判定必须红，且原因不是"静默跳过"。
test('已提交契约写歪（新增结构不在场）→ 必须变红', () => {
  const s = sandbox(doc => {
    delete doc.components.schemas['myblogx_internal_chat_domain.MessageListData'];
    doc.components.schemas['__miswritten__'] = { type: 'object' };
  });
  try {
    const r = run(generatorArgs(s.out), { STUB_MODE: 'copy', STUB_SOURCE: s.target });
    assert.equal(r.status, 1, r.stdout + r.stderr);
    assert.equal(r.reason, 'ADDED_STRUCTURE_MISSING', r.stdout);
  } finally {
    fs.rmSync(s.dir, { recursive: true, force: true });
  }
});

// 工具版本单一来源：把安装 ref 写歪，判定必须变红；换回真来源必须绿。
// 验的是判定的外部行为（CLI 退出码 + 原因），不读实现源码文本。
test('工具版本单一来源：写歪安装 ref → 判红，恢复 → 判绿', () => {
  const envFile = path.join(repoRoot, 'scripts', 'tool-versions.env');
  assert.ok(fs.existsSync(envFile), '缺少 scripts/tool-versions.env');
  const tampered = path.join(os.tmpdir(), `blogx-tool-versions-${process.pid}.env`);
  fs.writeFileSync(tampered, 'SWAG_MODULE=github.com/swaggo/swag/v2/cmd/swag\nSWAG_VERSION=v9.9.9-canary\n');
  try {
    const s = sandbox();
    try {
      const bad = run([...generatorArgs(s.out, { withVersionCheck: true }), '--versions-file', tampered], { STUB_MODE: 'copy', STUB_SOURCE: s.target });
      assert.equal(bad.status, 1, bad.stdout + bad.stderr);
      assert.equal(bad.reason, 'TOOL_VERSION_MISMATCH', bad.stdout);
      const good = run([...generatorArgs(s.out, { withVersionCheck: true }), '--versions-file', envFile], { STUB_MODE: 'copy', STUB_SOURCE: s.target });
      assert.equal(good.status, 0, good.stdout + good.stderr);
      assert.equal(good.reason, 'OK', good.stdout);
    } finally {
      fs.rmSync(s.dir, { recursive: true, force: true });
    }
  } finally {
    fs.rmSync(tampered, { force: true });
  }
});

console.log(results.join('\n'));
console.log(`\n${passed}/${results.length} 通过`);
if (passed !== results.length) process.exit(1);
