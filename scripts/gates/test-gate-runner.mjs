#!/usr/bin/env node
// 门禁 runner 的行为用例。
//
// 接缝只有两个，都是外部可观察的：
//   1. `node scripts/gates/...` 的命令行（list / run / 单门禁 / 安装步骤 / 版本读取）；
//   2. 现有 `bash scripts/check.sh <all|backend|frontend|gates>` 与
//      `bash scripts/verify-gates.sh` 的外部行为（退出码 + 是否拒绝整体跳过）。
//
// 不读任何实现源码文本：清单从 `--list --json` 的输出看，版本从
// `scripts/tool-versions.env`（单一来源本身）看，失败原因从子进程输出看。
import { spawnSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(here, '..', '..');
const RUNNER = path.join(here, 'run.mjs');
const INSTALLER = path.join(here, 'install.mjs');
const VERSIONS_FILE = path.join(ROOT, 'scripts', 'tool-versions.env');

// Git Bash：WSL 的 bash 没有 go / node，且不认 C:/ 盘符路径。
const GIT_BASH = 'C:\\Users\\Jen\\scoop\\apps\\git\\current\\bin\\bash.exe';
const BASH = process.env.BLOGX_BASH || (fs.existsSync(GIT_BASH) ? GIT_BASH : 'bash');

let pass = 0;
let fail = 0;

function ok(name) {
  console.log(`✓ ${name}`);
  pass += 1;
}

function bad(name, detail) {
  console.log(`✗ ${name}`);
  if (detail) console.log(String(detail).split('\n').slice(0, 20).map(l => `    ${l}`).join('\n'));
  fail += 1;
}

function cli(args, opts = {}) {
  return spawnSync(process.execPath, [opts.script || RUNNER, ...args], {
    cwd: opts.cwd || ROOT,
    encoding: 'utf8',
    env: { ...process.env, ...(opts.env || {}) },
    timeout: opts.timeout || 180000,
  });
}

function out(r) {
  return `${r.stdout || ''}${r.stderr || ''}`;
}

function readVersionFile(file) {
  const text = fs.readFileSync(file, 'utf8');
  const get = key => text.match(new RegExp(`^\\s*${key}\\s*=\\s*(\\S+)\\s*$`, 'm'))?.[1];
  return { module: get('SWAG_MODULE'), version: get('SWAG_VERSION') };
}

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'blogx-gate-runner-test-'));

function tempRegistry(mutate) {
  const base = JSON.parse(fs.readFileSync(path.join(ROOT, 'scripts', 'gates', 'gates.json'), 'utf8'));
  mutate(base);
  const file = path.join(tmp, `gates-${Math.random().toString(36).slice(2)}.json`);
  fs.writeFileSync(file, JSON.stringify(base, null, 2));
  return file;
}

// ── 清单：单一来源，列得出、每项可执行 ──────────────────────────────
{
  const r = cli(['--list', '--json']);
  if (r.status !== 0) {
    bad('--list --json 能列出全部门禁', out(r));
  } else {
    let gates;
    try { gates = JSON.parse(r.stdout); } catch (e) { gates = null; }
    if (!Array.isArray(gates) || gates.length === 0) {
      bad('--list --json 输出门禁数组', r.stdout);
    } else {
      const required = [
        'backend-test',
        'archcheck',
        'skills-format',
        'contract-freshness',
        'contract-freshness-cases',
        'frontend-typecheck',
        'contract-types',
        'route-contract',
        'admin-auth',
        'menu-pages',
        'frontend-lint',
        'coverage-threshold',
        'token-parity',
        'image-shaping-parity',
      ];
      const missing = required.filter(id => !gates.some(g => g.id === id));
      if (missing.length) bad(`清单包含全部门禁（缺 ${missing.join(', ')}）`, JSON.stringify(gates, null, 2));
      else ok('--list --json 输出门禁数组，含全部门禁');
      // 每条都必须有执行方式：steps（runner 代跑）或 section（由本地脚本 / CI job 执行）。
      const hasRun = g => (Array.isArray(g.steps) && g.steps.length > 0) || Number.isInteger(g.section);
      const shape = gates.filter(g => !g.label || !g.group || !hasRun(g));
      if (shape.length) bad('每道门禁都有 label、group 与执行方式（steps 或 section）', JSON.stringify(shape, null, 2));
      else ok('每道门禁都有 label、group 与执行方式（steps 或 section）');
      const uncovered = gates.filter(g => !Array.isArray(g.canaries) || g.canaries.length === 0);
      if (uncovered.length) bad('每道门禁都登记了对应的演练节号', coveredIds(uncovered));
      else ok('每道门禁都登记了对应的演练节号');
      // #287 AC1/AC4/AC6：清单正好 14 道仓库级门禁 —— 14 个演练节号里 13 个由
      // 清单门禁负责，第 14 个（生成式装饰参数校验）按 AC4 不进清单，必须由
      // notGates 显式豁免；既没人负责又没登记的节号照样算漏。
      // 14 这个数字来自 #309：运行时策略 skill 门禁（skills-format）与 archcheck
      // 共用第 3 节演练（它自带 --self-test，人造成违规用例就写在脚本里），
      // 因此门禁多一道、演练节号不变。
      const canaryFile = JSON.parse(fs.readFileSync(path.join(ROOT, 'scripts', 'gates', 'gates.json'), 'utf8'));
      const wanted = (canaryFile.canaries || []).map(c => c.id);
      const notGates = new Set(canaryFile.notGates || []);
      const covered = new Set(gates.flatMap(g => g.canaries || []));
      const unregistered = wanted.filter(id => !covered.has(id) && !notGates.has(id));
      const bothSides = [...notGates].filter(id => covered.has(id));
      if (gates.length !== 14 || wanted.length !== 14 || unregistered.length || bothSides.length) {
        bad(
          '清单正好 14 道门禁；14 个演练节号里 13 个有门禁负责、第 14 个按 notGates 显式豁免' +
          `（门禁 ${gates.length} 道，没人负责也没豁免的演练 ${unregistered.join(',') || '无'}，` +
          `既豁免又有门禁认领的 ${bothSides.join(',') || '无'}）`,
          JSON.stringify(gates.map(g => g.id), null, 2),
        );
      } else ok('清单正好 14 道门禁；14 个演练节号里 13 个有门禁负责、第 14 个按 notGates 显式豁免');
    }
  }

  const human = cli(['--list']);
  if (human.status !== 0) bad('--list 人能读的清单', out(human));
  else if (!['archcheck', 'contract-freshness', 'token-parity', 'image-shaping-parity'].every(id => human.stdout.includes(id))) {
    bad('--list 人能读的清单列出每道门禁', human.stdout);
  } else ok('--list 人能读的清单列出每道门禁');
}

function coveredIds(gates) {
  return JSON.stringify(gates.map(g => g.id));
}

// ── 清单只有一处：改这一处，消费方自动看到 ──────────────────────────
{
  const file = tempRegistry(data => {
    data.gates.push({
      id: 'demo-gate',
      label: '演示门禁（只在本用例的临时清单里）',
      group: 'demo',
      steps: [{ cwd: '.', command: ['node', '-e', 'process.exit(0)'] }],
      canaries: [99],
    });
  });
  const r = cli(['--list', '--json', '--registry', file]);
  let gates = [];
  try { gates = JSON.parse(r.stdout); } catch (e) { /* 下面统一报 */ }
  if (r.status !== 0 || !gates.some(g => g.id === 'demo-gate')) {
    bad('清单加一条后 --list 立刻看到它（不用改三处脚本）', out(r));
  } else ok('清单加一条后 --list 立刻看到它（不用改三处脚本）');

  const uncoveredFile = tempRegistry(data => {
    data.gates.push({
      id: 'demo-gate-without-canary',
      label: '没有演练的门禁',
      group: 'demo',
      steps: [{ cwd: '.', command: ['node', '-e', 'process.exit(0)'] }],
      canaries: [],
    });
  });
  const cov = cli(['--check-coverage', '--registry', uncoveredFile]);
  if (cov.status === 0 || !out(cov).includes('demo-gate-without-canary')) {
    bad('没有演练的门禁会让覆盖检查失败并点名', out(cov));
  } else ok('没有演练的门禁会让覆盖检查失败并点名');

  // 豁免只能显式：某节既没人负责又没登记 notGates 时，覆盖检查必须失败。
  // 否则「少登记一道门禁」会静默变成绿的那一类失效。
  const orphanFile = tempRegistry(data => {
    data.canaries.push({ id: 96, title: '既没门禁也没豁免的演练', affected: [] });
  });
  const orphan = cli(['--check-coverage', '--registry', orphanFile]);
  if (orphan.status === 0 || !out(orphan).includes('演练 96')) {
    bad('没人负责又没登记 notGates 的演练会让覆盖检查失败（豁免不是静默放过）', out(orphan));
  } else ok('没人负责又没登记 notGates 的演练会让覆盖检查失败（豁免不是静默放过）');

  const covOk = cli(['--check-coverage']);
  if (covOk.status !== 0) bad('当前清单的演练覆盖检查通过', out(covOk));
  else ok('当前清单的演练覆盖检查通过');
}

// ── 执行：单门禁 / 分组 / 失败即非零 ────────────────────────────────
{
  const file = tempRegistry(data => {
    data.gates.push(
      {
        id: 'demo-ok',
        label: '演示：通过的门禁',
        group: 'demo',
        steps: [{ cwd: '.', command: ['node', '-e', 'console.log("demo-ok ran")'] }],
        canaries: [98],
      },
      {
        id: 'demo-fail',
        label: '演示：失败的门禁',
        group: 'demo',
        steps: [{ cwd: '.', command: ['node', '-e', 'process.exit(7)'] }],
        canaries: [97],
      },
    );
  });

  const good = cli(['demo-ok', '--registry', file]);
  if (good.status !== 0 || !out(good).includes('演示：通过的门禁')) {
    bad('单门禁能跑通并打印它的名字', out(good));
  } else ok('单门禁能跑通并打印它的名字');

  const badRun = cli(['demo-fail', '--registry', file]);
  if (badRun.status === 0) bad('门禁失败时 runner 返回非零');
  else ok('门禁失败时 runner 返回非零');

  const group = cli(['--group', 'demo', '--registry', file]);
  if (group.status === 0 || !out(group).includes('演示：失败的门禁')) {
    bad('按分组跑会逐个执行，任一失败即非零', out(group));
  } else ok('按分组跑会逐个执行，任一失败即非零');

  const unknown = cli(['no-such-gate']);
  if (unknown.status === 0) bad('未知门禁名返回非零而不是静默跳过');
  else ok('未知门禁名返回非零而不是静默跳过');

  const unknownGroup = cli(['--group', 'no-such-group']);
  if (unknownGroup.status === 0) bad('未知分组返回非零而不是静默跳过');
  else ok('未知分组返回非零而不是静默跳过');
}

// ── 判定：与真门禁跨同一个接口（#273 那一份），不另写第二份 ────────
{
  const clean = cli(['contract-freshness']);
  if (clean.status !== 0 || !out(clean).includes('[reason] OK')) {
    bad('契约未过期门禁（真跑）通过并给出同一份 [reason] 标记', out(clean));
  } else ok('契约未过期门禁（真跑）通过并给出同一份 [reason] 标记');

  const skipScript = path.join(tmp, 'swag-silent-skip.sh');
  fs.writeFileSync(skipScript, '#!/bin/sh\nexit 0\n');
  const silent = cli(['contract-freshness'], { env: { SWAG: `sh ${skipScript}` } });
  if (silent.status === 0 || !out(silent).includes('[reason] SILENT_SKIP')) {
    bad('注入「静默跳过」时同一份判定报 SILENT_SKIP（不是另写一份）', out(silent));
  } else ok('注入「静默跳过」时同一份判定报 SILENT_SKIP（不是另写一份）');

  const tampered = path.join(tmp, 'tool-versions-canary.env');
  const original = readVersionFile(VERSIONS_FILE);
  fs.writeFileSync(
    tampered,
    `SWAG_MODULE=${original.module}\nSWAG_VERSION=v9.9.9-canary\n`,
  );
  const mismatch = cli(['contract-freshness', '--', '--versions-file', tampered]);
  if (mismatch.status === 0 || !out(mismatch).includes('[reason] TOOL_VERSION_MISMATCH')) {
    bad('runner 把参数原样交给同一份判定（写歪版本 → TOOL_VERSION_MISMATCH）', out(mismatch));
  } else ok('runner 把参数原样交给同一份判定（写歪版本 → TOOL_VERSION_MISMATCH）');
}

// ── 工具版本与重复安装步骤：单一入口，版本不写第二遍 ────────────────
{
  const expected = readVersionFile(VERSIONS_FILE);
  const v = cli(['versions', '--json']);
  let got = null;
  try { got = JSON.parse(v.stdout); } catch (e) { /* 下面统一报 */ }
  if (v.status !== 0 || !got || got.swag?.module !== expected.module || got.swag?.version !== expected.version) {
    bad('versions --json 从 tool-versions.env 读出安装 ref', out(v));
  } else ok('versions --json 从 tool-versions.env 读出安装 ref');

  const dry = cli(['swag', '--dry-run'], { script: INSTALLER });
  const want = `go install ${expected.module}@${expected.version}`;
  if (dry.status !== 0 || !out(dry).includes(want)) {
    bad(`安装 swag 的步骤从单一版本来源推出（期望含 ${want}）`, out(dry));
  } else ok(`安装 swag 的步骤从单一版本来源推出（期望含 ${want}）`);

  const tampered = path.join(tmp, 'tool-versions-other.env');
  fs.writeFileSync(tampered, `SWAG_MODULE=${expected.module}\nSWAG_VERSION=v1.2.3-canary\n`);
  const dryTampered = cli(['swag', '--dry-run', '--versions-file', tampered], { script: INSTALLER });
  if (dryTampered.status !== 0 || !out(dryTampered).includes(`@v1.2.3-canary`)) {
    bad('换一份版本文件，安装步骤跟着变（证明没有第二份字面版本）', out(dryTampered));
  } else ok('换一份版本文件，安装步骤跟着变（证明没有第二份字面版本）');

  const dryPnpm = cli(['pnpm', '--dry-run', '--dir', 'frontend_admin'], { script: INSTALLER });
  if (dryPnpm.status !== 0 || !out(dryPnpm).includes('pnpm install --frozen-lockfile') || !out(dryPnpm).includes('frontend_admin')) {
    bad('pnpm 安装步骤只有这一处定义（含目录）', out(dryPnpm));
  } else ok('pnpm 安装步骤只有这一处定义（含目录）');

  const dryMod = cli(['go-mod', '--dry-run', '--dir', 'backend'], { script: INSTALLER });
  if (dryMod.status !== 0 || !out(dryMod).includes('go mod download') || !out(dryMod).includes('backend')) {
    bad('go mod download 步骤只有这一处定义（含目录）', out(dryMod));
  } else ok('go mod download 步骤只有这一处定义（含目录）');
}

// ── 不许整体跳过：任何整体跳过口径都必须不可用 ─────────────────────
{
  const skipArgs = [['--skip-all'], ['--skip'], ['--only'], ['--skip', 'all']];
  let bad_ = 0;
  for (const args of skipArgs) {
    const r = cli(args);
    if (r.status === 0) {
      bad(`整体跳过参数 ${args.join(' ')} 必须被拒绝`, out(r));
      bad_ += 1;
    }
  }
  if (!bad_) ok('整体跳过参数（--skip-all / --skip）被拒绝');

  const env = cli(['--list'], { env: { BLOGX_SKIP_GATES: '1' } });
  if (env.status === 0) bad('BLOGX_SKIP_GATES=1 必须被拒绝而不是静默跳过', out(env));
  else ok('BLOGX_SKIP_GATES=1 必须被拒绝而不是静默跳过');

  const vs = spawnSync(BASH, ['scripts/verify-gates.sh', '--skip-all'], {
    cwd: ROOT, encoding: 'utf8', env: { ...process.env, BLOGX_SKIP_GATES: '' }, timeout: 120000,
  });
  const vsOut = `${vs.stdout || ''}${vs.stderr || ''}`;
  if (vs.status === 0 || /通过：\d+/.test(vsOut)) {
    bad('门禁自检拒绝整体跳过参数（不得跑完或静默通过）', vsOut);
  } else ok('门禁自检拒绝整体跳过参数（不得跑完或静默通过）');
}

console.log('');
console.log(`通过：${pass}  失败：${fail}`);
if (fail !== 0) {
  console.log('✗ 门禁 runner 行为用例未通过');
  process.exit(1);
}
console.log('✓ 门禁 runner 行为用例全部通过');
