// 工具版本与公共安装步骤的唯一实现。
//
// 版本本体是 scripts/tool-versions.env（#273 建立的单一来源），本模块只读它，
// 不写第二份字面版本；棘轮阈值同理只读 scripts/arch-baseline.env。
//
// 消费方：scripts/gates/run.mjs（跑门禁）、scripts/gates/install.mjs（装工具）、
// .github/workflows/ci.yml（CI 只调 install.mjs），以及门禁自检的版本演练。
import * as fs from 'node:fs';
import * as path from 'node:path';
import { ROOT } from './registry.mjs';

export const DEFAULT_VERSIONS_FILE = path.join(ROOT, 'scripts', 'tool-versions.env');
export const DEFAULT_BASELINE_FILE = path.join(ROOT, 'scripts', 'arch-baseline.env');

function readEnvFile(file) {
  const abs = path.resolve(file);
  if (!fs.existsSync(abs)) throw new Error(`环境文件不存在：${abs}`);
  const values = {};
  for (const line of fs.readFileSync(abs, 'utf8').split(/\r?\n/)) {
    const m = line.match(/^\s*([A-Z0-9_]+)\s*=\s*(.*?)\s*$/);
    if (m && !line.trim().startsWith('#')) values[m[1]] = m[2];
  }
  return { file: abs, values };
}

// readToolVersions 返回 { file, swag: { module, version } }；缺键就抛，不猜默认值。
export function readToolVersions(file = DEFAULT_VERSIONS_FILE) {
  const { file: abs, values } = readEnvFile(file);
  const module = values.SWAG_MODULE;
  const version = values.SWAG_VERSION;
  if (!module || !version) {
    throw new Error(`${abs} 缺少 SWAG_MODULE / SWAG_VERSION（版本只从这一处读）`);
  }
  return { file: abs, swag: { module, version } };
}

// readArchBaseline 返回棘轮阈值（唯一来源 scripts/arch-baseline.env）。
export function readArchBaseline(file = DEFAULT_BASELINE_FILE) {
  const { file: abs, values } = readEnvFile(file);
  if (values.ARCH_MAX_VIOLATIONS === undefined) {
    throw new Error(`${abs} 缺少 ARCH_MAX_VIOLATIONS（棘轮基线只从这一处读）`);
  }
  return { file: abs, maxViolations: values.ARCH_MAX_VIOLATIONS };
}

// stepCommand 把清单里的占位符换成单一来源里的值。
export function stepCommand(step) {
  const baseline = readArchBaseline();
  return step.command.map(arg =>
    String(arg).replace(/__ARCH_MAX_VIOLATIONS__/g, baseline.maxViolations));
}

const PNPM_INSTALL = ['pnpm', 'install', '--frozen-lockfile'];
const GO_MOD_DOWNLOAD = ['go', 'mod', 'download'];

// installSteps 返回某个安装套件的步骤（相对仓库根的工作目录 + 命令）。
//   swag   装契约生成工具：版本只从 tool-versions.env 读
//   go-mod 把 Go 依赖下全（swag 的 --parseDependency 会与下载并发，见 ci.yml 注释）
//   pnpm   前端依赖（两端都不是 workspace，一次只装一个目录）
export function installSteps(suite, opts = {}) {
  const dir = opts.dir || '.';
  if (suite === 'swag') {
    const { swag } = readToolVersions(opts.versionsFile);
    return {
      suite,
      steps: [{ cwd: '.', command: ['go', 'install', `${swag.module}@${swag.version}`] }],
      // go install 落在 $(go env GOPATH)/bin，CI 上默认不在 PATH。
      afterInstall: 'gopath-bin',
    };
  }
  if (suite === 'go-mod') {
    return { suite, steps: [{ cwd: dir, command: [...GO_MOD_DOWNLOAD] }] };
  }
  if (suite === 'pnpm') {
    return { suite, steps: [{ cwd: dir, command: [...PNPM_INSTALL] }] };
  }
  throw new Error(`未知安装套件：${suite}（可用：swag / go-mod / pnpm）`);
}

export const INSTALL_SUITES = ['swag', 'go-mod', 'pnpm'];
