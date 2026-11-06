#!/usr/bin/env node
// 门禁 runner：本地门禁、持续集成、门禁自检三处共用的唯一入口。
//
// 它只做四件事：
//   1. 读清单（scripts/gates/gates.json —— 加一道门禁只改那一处）；
//   2. 按 id / 分组 / 演练节号选择要跑的项（默认全跑）；
//   3. 执行清单里的命令并把退出码如实汇总；
//   4. 输出 `--list --json` 供测试、CI 与门禁自检消费。
//
// 它**不重写任何判据**：契约未过期仍是 scripts/check-contract-freshness.mjs
// 那一份实现（清单里的 contract-freshness 就是调用它），棘轮阈值仍只从
// scripts/arch-baseline.env 读，工具版本仍只从 scripts/tool-versions.env 读。
//
// 「整体跳过」没有任何入口：--skip / --skip-all 直接拒绝，BLOGX_SKIP_GATES
// 之类的环境变量同样拒绝；--only / --affected 选不中任何一项时按失败处理，
// 不存在「零项通过即绿」的路径。
import { spawnSync } from 'node:child_process';
import * as path from 'node:path';
import {
  DEFAULT_REGISTRY, ROOT, affectedCanaries, canaryIds, coverageProblems, gateIds,
  gatesForCanaries, gatesInGroup, groups, loadRegistry,
} from './registry.mjs';
import { DEFAULT_VERSIONS_FILE, readToolVersions, stepCommand } from './tools.mjs';

function die(message) {
  console.error(`✗ ${message}`);
  process.exit(2);
}

const SKIP_ENV = ['BLOGX_SKIP_GATES', 'SKIP_GATES', 'BLOGX_SKIP_ALL_GATES', 'SKIP_ALL_GATES'];
for (const name of SKIP_ENV) {
  if ((process.env[name] || '').trim() !== '') {
    die(`门禁不许整体跳过：检测到 ${name}。要只跑一部分请用 --affected <改动路径> 或 --only <演练节号>，且必须至少选中一项。`);
  }
}

const argv = process.argv.slice(2);
const opts = {
  mode: null, json: false, registry: DEFAULT_REGISTRY, versionsFile: DEFAULT_VERSIONS_FILE,
  group: null, only: null, gateIds: [], passthrough: [], affectedPaths: [],
};

for (let i = 0; i < argv.length; i++) {
  const arg = argv[i];
  if (arg === '--') { opts.passthrough = argv.slice(i + 1); break; }
  if (/^--?skip/i.test(arg)) {
    die(`门禁不许整体跳过：不接受 ${arg}。要只跑一部分请用 --affected <改动路径>，且必须至少选中一项。`);
  }
  if (arg === '--list') opts.mode = 'list';
  else if (arg === '--check-coverage') opts.mode = 'coverage';
  else if (arg === '--list-canaries') opts.mode = 'canaries';
  else if (arg === '--affected') {
    opts.mode = 'affected';
    while (i + 1 < argv.length && !argv[i + 1].startsWith('--')) opts.affectedPaths.push(argv[++i]);
  } else if (arg === 'versions') opts.mode = 'versions';
  else if (arg === '--json') opts.json = true;
  else if (arg === '--registry') {
    if (!argv[i + 1]) die('--registry 需要一个清单文件');
    opts.registry = path.resolve(argv[++i]);
  } else if (arg === '--versions-file') {
    if (!argv[i + 1]) die('--versions-file 需要一个版本文件');
    opts.versionsFile = path.resolve(argv[++i]);
  } else if (arg === '--group') {
    if (!argv[i + 1]) die('--group 需要一个分组名');
    opts.group = argv[++i];
  } else if (arg === '--only') {
    if (!argv[i + 1] || argv[i + 1].startsWith('-')) {
      die('--only 至少需要一个演练节号（不许整体跳过，也不许零项通过）');
    }
    opts.only = argv[++i].split(',').map(s => Number(s.trim())).filter(n => Number.isInteger(n));
    if (opts.only.length === 0) die('--only 至少需要一个演练节号（不许整体跳过，也不许零项通过）');
  } else if (arg.startsWith('-')) die(`未知参数 ${arg}`);
  else opts.gateIds.push(arg);
}

const registry = loadRegistry(opts.registry);

if (opts.mode === 'coverage') {
  const problems = coverageProblems(registry);
  if (problems.length) {
    for (const p of problems) console.error(`✗ ${p}`);
    console.error(`✗ 门禁清单与演练清单不一致（${registry.file}）`);
    process.exit(1);
  }
  console.log(`✓ ${registry.gates.length} 道门禁都有对应的演练节号，${registry.canaries.length} 道演练都有对应门禁`);
  process.exit(0);
}

if (opts.mode === 'list') {
  if (opts.json) {
    console.log(JSON.stringify(registry.gates, null, 2));
    process.exit(0);
  }
  console.log(`门禁清单（${registry.file}）：${registry.gates.length} 道`);
  for (const gate of registry.gates) {
    const canaries = (gate.canaries || []).map(id => `${id}/14`).join(', ');
    console.log(`  ${gate.id.padEnd(28)} [${gate.group}] ${gate.label}（演练 ${canaries}）`);
    for (const step of gate.steps) {
      console.log(`      ${step.cwd === '.' ? '' : `(${step.cwd}) `}${step.command.join(' ')}`);
    }
  }
  console.log(`分组：${groups(registry).join(' / ')}；演练节号：${canaryIds(registry).join(',')}`);
  process.exit(0);
}

if (opts.mode === 'canaries') {
  let selected = registry.canaries;
  if (opts.only) {
    const known = canaryIds(registry);
    const unknown = opts.only.filter(id => !known.includes(id));
    if (unknown.length) {
      die(`未知演练节号：${unknown.join(',')}（清单里的演练：${known.join(',')}）`);
    }
    selected = registry.canaries.filter(c => opts.only.includes(c.id));
    if (selected.length === 0) die('--only 没有选中任何演练（门禁自检不许零项通过）');
  }
  if (opts.json) {
    console.log(JSON.stringify(selected, null, 2));
    process.exit(0);
  }
  for (const canary of selected) {
    console.log(`${canary.id}/14 ${canary.title}`);
  }
  process.exit(0);
}

if (opts.mode === 'affected') {
  if (opts.affectedPaths.length === 0) die('--affected 至少需要一个改动路径');
  const ids = affectedCanaries(registry, opts.affectedPaths);
  if (ids.length === 0) {
    console.error(`✗ 改动路径没有命中任何演练：${opts.affectedPaths.join(' ')}`);
    console.error('  门禁自检不许整体跳过；若确实改了门禁自身，请直接全量跑 bash scripts/verify-gates.sh。');
    process.exit(1);
  }
  if (opts.json) console.log(JSON.stringify(ids));
  else console.log(ids.map(id => `${id}/14`).join(' '));
  process.exit(0);
}

if (opts.mode === 'versions') {
  const versions = readToolVersions(opts.versionsFile);
  if (opts.json) console.log(JSON.stringify(versions, null, 2));
  else console.log(`swag：${versions.swag.module}@${versions.swag.version}（来源 ${versions.file}）`);
  process.exit(0);
}

// ── 执行 ────────────────────────────────────────────────────────────
let selected;
if (opts.group) {
  selected = gatesInGroup(registry, opts.group);
  if (selected.length === 0) {
    die(`没有分组 ${opts.group} 的门禁（可用分组：${groups(registry).join(' / ')}）`);
  }
  if (opts.gateIds.length) die('--group 与单个门禁名不能同时使用');
} else if (opts.only) {
  const unknown = opts.only.filter(id => !canaryIds(registry).includes(id));
  if (unknown.length) die(`未知演练节号：${unknown.join(',')}（清单里的演练：${canaryIds(registry).join(',')}）`);
  selected = gatesForCanaries(registry, opts.only);
  if (selected.length === 0) die(`演练节号 ${opts.only.join(',')} 没有对应门禁`);
} else {
  if (opts.gateIds.length === 0) {
    die(`没有指定门禁。用 --list 看清单，或 --group <${groups(registry).join('|')}>；门禁不许什么都不跑就算通过`);
  }
  const known = gateIds(registry);
  const unknown = opts.gateIds.filter(id => !known.includes(id));
  if (unknown.length) die(`未知门禁：${unknown.join(',')}（清单里的门禁：${known.join(',')}）`);
  selected = registry.gates.filter(g => opts.gateIds.includes(g.id));
}

let failed = 0;
for (const gate of selected) {
  console.log('');
  console.log(`──── ${gate.label}（${gate.id}）────`);
  for (const step of gate.steps) {
    const command = stepCommand(step);
    const full = gate.passthrough && opts.passthrough.length
      ? [...command, ...opts.passthrough]
      : command;
    console.log(`  $ ${full.join(' ')}${step.cwd === '.' ? '' : `   (cwd: ${step.cwd})`}`);
    const r = spawnSync(full[0], full.slice(1), {
      cwd: path.resolve(ROOT, step.cwd || '.'),
      stdio: 'inherit',
      env: process.env,
      shell: false,
    });
    if (r.status !== 0) {
      console.error(`✗ ${gate.label}（${gate.id}）失败：退出码 ${r.status === null ? '被信号终止' : r.status}`);
      failed = 1;
      break;
    }
  }
  if (failed) break;
}

if (failed) {
  console.error('');
  console.error('✗ 门禁未通过');
  process.exit(1);
}
console.log('');
console.log(`✓ 门禁通过（${selected.length} 道：${selected.map(g => g.id).join(', ')}）`);
