#!/usr/bin/env node
// 安装步骤的唯一入口：CI 的每个 job 都调它，本地需要时也调它。
//
// 以前 `go install <module>@<version>` 在 ci.yml 里出现两遍、
// `pnpm install --frozen-lockfile` 出现八遍，版本与参数各写一份必然漂移。
// 现在版本只从 scripts/tool-versions.env 读、参数只在这里定义：
//
//   node scripts/gates/install.mjs swag                    # 契约生成工具（按单一来源的 ref）
//   node scripts/gates/install.mjs go-mod --dir backend    # 把 Go 依赖下全（见 ci.yml 的说明）
//   node scripts/gates/install.mjs pnpm --dir frontend     # 前端依赖（两端不是同一个 workspace）
//
// `--dry-run` 只打印将要执行的命令，供本地核对与行为用例使用。
import { spawnSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { ROOT } from './registry.mjs';
import { DEFAULT_VERSIONS_FILE, INSTALL_SUITES, installSteps } from './tools.mjs';

function die(message) {
  console.error(`✗ ${message}`);
  process.exit(2);
}

const argv = process.argv.slice(2);
let suite = null;
let dryRun = false;
let dir = null;
let versionsFile = DEFAULT_VERSIONS_FILE;

for (let i = 0; i < argv.length; i++) {
  const arg = argv[i];
  if (arg === '--dry-run') dryRun = true;
  else if (arg === '--dir') {
    if (!argv[i + 1]) die('--dir 需要一个目录');
    dir = argv[++i];
  } else if (arg === '--versions-file') {
    if (!argv[i + 1]) die('--versions-file 需要一个版本文件');
    versionsFile = path.resolve(argv[++i]);
  } else if (arg.startsWith('-')) die(`未知参数 ${arg}`);
  else if (suite) die(`只能指定一个安装套件（已有 ${suite}，又给了 ${arg}）`);
  else suite = arg;
}

// 不传 --dir 时用**当前工作目录**（CI 的 defaults.run.working-directory 就是
// 两端各自目录）；--dir 一律相对仓库根，避免"以为装在了 app 里、其实装在根上"。
if (dir === null) {
  const rel = path.relative(ROOT, process.cwd()).split(path.sep).join('/');
  dir = rel === '' ? '.' : rel;
}

if (!suite) die(`没有指定安装套件（可用：${INSTALL_SUITES.join(' / ')}）`);
if (!INSTALL_SUITES.includes(suite)) die(`未知安装套件 ${suite}（可用：${INSTALL_SUITES.join(' / ')}）`);

const plan = installSteps(suite, { dir, versionsFile });

for (const step of plan.steps) {
  const line = `$ ${step.command.join(' ')}${step.cwd === '.' ? '' : `   (cwd: ${step.cwd})`}`;
  if (dryRun) {
    console.log(line);
    continue;
  }
  console.log(line);
  const useShell = suite === 'pnpm' && process.platform === 'win32';
  const r = useShell
    ? spawnSync(step.command.join(' '), { cwd: path.resolve(ROOT, step.cwd), stdio: 'inherit', shell: true, env: process.env })
    : spawnSync(step.command[0], step.command.slice(1), {
      cwd: path.resolve(ROOT, step.cwd), stdio: 'inherit', env: process.env,
    });
  if (r.status !== 0) {
    console.error(`✗ 安装失败：${step.command.join(' ')}（退出码 ${r.status === null ? '被信号终止' : r.status}）`);
    process.exit(1);
  }

  if (plan.afterInstall === 'gopath-bin') {
    const gopath = spawnSync('go', ['env', 'GOPATH'], { encoding: 'utf8', env: process.env });
    if (gopath.status !== 0) die('go env GOPATH 不可用，无法把 swag 所在目录加入 PATH');
    const bin = path.join((gopath.stdout || '').trim(), 'bin');
    if (process.env.GITHUB_PATH) {
      fs.appendFileSync(process.env.GITHUB_PATH, `${bin}\n`);
      console.log(`  已把 ${bin} 加入 $GITHUB_PATH（CI）`);
    } else {
      console.log(`  提示：${bin} 需要在本机 PATH 上（CI 由 $GITHUB_PATH 注入）`);
    }
  }
}

if (!dryRun) console.log(`✓ 安装完成：${suite}${dir === '.' ? '' : `（${dir}）`}`);
