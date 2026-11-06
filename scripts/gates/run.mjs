#!/usr/bin/env node
// 门禁 runner：本地门禁、持续集成、门禁自检三处共用的唯一入口。
//
// 它只做四件事：
//   1. 读清单（scripts/gates/gates.json —— 加一道门禁只改那一处）；
//   2. 按 id / 分组 / 演练节号选择要跑的项（默认全跑）；
//   3. 执行清单里**有 steps** 的命令并把退出码如实汇总；只有 section 的门禁
//      由本地脚本 / CI job 执行，runner 不代跑，但会点名它是谁、第几节证明它会红；
//   4. 输出 `--list --json` 供测试、CI 与门禁自检消费。
//
// 它**不重写任何判据**：契约未过期仍是 scripts/check-contract-freshness.mjs
// 那一份实现（清单里的 contract-freshness 就是调用它），棘轮阈值仍只从
// scripts/arch-baseline.env 读，工具版本仍只从 scripts/tool-versions.env 读。
//
// 「整体跳过」没有任何入口：--skip / --skip-all 直接拒绝，BLOGX_SKIP_GATES
// 之类的环境变量同样拒绝；--only / --affected 选不中任何一项时按失败处理，
// 不存在「零项通过即绿」的路径。选中的全是「只有演练节号」的门禁时同样按失败
// 处理 —— 那等于这次没有执行任何门禁。
//
// 清单外的演练必须显式登记：`notGates` 列出「按 #287 AC4 不进清单」的节号
// （目前只有第 14 节：生成式装饰参数校验，由 check.sh / ci.yml 直接执行）。
// 没登记又没人负责的演练会让 --check-coverage 失败 —— 豁免不是「缺了就放过」。
import { spawnSync } from 'node:child_process';
import * as path from 'node:path';
import {
  DEFAULT_REGISTRY, ROOT, affectedCanaries, canaryIds, coverageProblems, executableGates,
  gateIds, gatesForCanaries, gatesInGroup, groups, hasSteps, loadRegistry,
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
  else if (arg === '--sections') opts.mode = 'sections';
  else if (arg === '--groups') opts.mode = 'groups';
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
  const executable = executableGates(registry.gates).length;
  const notGates = [...(registry.notGates || [])].sort((a, b) => a - b);
  console.log(
    `✓ ${registry.gates.length} 道仓库级门禁与 ${registry.canaries.length} 个演练节号对得上` +
    `（runner 直接执行 ${executable} 道；其余 ${registry.gates.length - executable} 道由本地脚本 / CI job 执行，靠对应演练证明会红）`,
  );
  if (notGates.length) {
    console.log(
      `  ${notGates.map(id => `${id}/${registry.canaries.length}`).join(',')} 不在清单里（notGates 显式登记）：` +
      '它们是参数生成脚本，按 #287 AC4 不进清单，由本地脚本 / CI job 直接执行，' +
      '自检对应节号仍证明它会红。',
    );
  }
  process.exit(0);
}

if (opts.mode === 'list') {
  const total = registry.canaries.length;
  if (opts.json) {
    console.log(JSON.stringify(registry.gates, null, 2));
    process.exit(0);
  }
  console.log(`门禁清单（${registry.file}）：${registry.gates.length} 道仓库级门禁`);
  for (const gate of registry.gates) {
    const canaries = (gate.canaries || []).map(id => `${id}/${total}`).join(', ');
    console.log(`  ${gate.id.padEnd(26)} [${gate.group}] ${gate.label}（演练 ${canaries}）`);
    if (hasSteps(gate)) {
      for (const step of gate.steps) {
        console.log(`      ${step.cwd === '.' ? '' : `(${step.cwd}) `}${step.command.join(' ')}`);
      }
    } else {
      console.log(`      ↳ runner 不代跑（演练 ${gate.section}/${total}）：${gate.executedBy || '真门禁在本地脚本 / CI job 里'}`);
    }
  }
  const notGates = [...(registry.notGates || [])].sort((a, b) => a - b);
  if (notGates.length) {
    console.log(
      `清单外（notGates 显式登记，按 #287 AC4 不进清单）：${notGates.map(id => `${id}/${total}`).join(',')}` +
      ' —— 由 scripts/check.sh / .github/workflows/ci.yml 直接执行',
    );
  }
  console.log(`分组：${groups(registry).join(' / ')}；演练节号：${canaryIds(registry).join(',')}`);
  process.exit(0);
}

if (opts.mode === 'sections') {
  // 门禁自检的节号从清单派生：清单里每个演练节号都必须有门禁负责，
  // 自检脚本里的 `===== n/14` 标记必须与这份序列逐项一致（多一节少一节都算漂移）。
  const ids = canaryIds(registry);
  if (opts.json) console.log(JSON.stringify(ids));
  else console.log(ids.join(' '));
  process.exit(0);
}

if (opts.mode === 'groups') {
  // 分组名也只在清单里定义：本地门禁据此核对「清单里的分组我是不是都跑了」，
  // 不再在 check.sh 里另抄一份分组清单（另抄一份就会两边一起忘）。
  const names = groups(registry);
  if (opts.json) console.log(JSON.stringify(names));
  else console.log(names.join(' '));
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
    console.log(`${canary.id}/${registry.canaries.length} ${canary.title}`);
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
  else console.log(ids.map(id => `${id}/${registry.canaries.length}`).join(' '));
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
// 只有 steps 的门禁由 runner 代跑；只有 section 的门禁由本地脚本 / CI job 执行，
// 这里点名但不代跑 —— 代跑等于去跑门禁自检的注入，结果没有意义。
const runnable = executableGates(selected);
const delegated = selected.filter(g => !hasSteps(g));
if (runnable.length === 0) {
  die(
    `选中的 ${selected.length} 道门禁（${selected.map(g => g.id).join(', ')}）都由本地脚本 / CI job 执行，` +
    'runner 没有可跑的步骤。要证明它们会红用 `bash scripts/verify-gates.sh --only ' +
    selected.map(g => g.section ?? g.canaries.join(',')).join(',') + '`；要跑真门禁用 `bash scripts/check.sh`。',
  );
}
for (const gate of delegated) {
  console.log(`· ${gate.id}：runner 不代跑（由${gate.executedBy || '本地脚本 / CI job'}）；演练 ${gate.section}/${registry.canaries.length} 证明它会红`);
}

let executed = 0;
for (const gate of runnable) {
  console.log('');
  console.log(`──── ${gate.label}（${gate.id}）────`);
  executed += 1;
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
console.log(
  `✓ 门禁通过（runner 执行 ${executed} 道：${runnable.map(g => g.id).join(', ')}` +
  `${delegated.length ? `；另有 ${delegated.length} 道由本地脚本 / CI job 执行，见上面的点名` : ''}）`,
);
