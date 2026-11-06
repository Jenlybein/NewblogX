// 门禁清单的读取、校验与「按改动范围挑演练」的唯一实现。
//
// 清单本体是 scripts/gates/gates.json（数据，不是代码）：加一道门禁只改那一处。
// 本模块不重复登记任何命令 —— 它只加载 + 校验 + 派生，因此不会出现
// 「本地一份、CI 一份、自检又一份」。
import * as fs from 'node:fs';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

export const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
export const DEFAULT_REGISTRY = path.join(ROOT, 'scripts', 'gates', 'gates.json');

// loadRegistry 读清单并校验形状。形状错了就抛，绝不"读不到就算没有门禁"。
export function loadRegistry(file = DEFAULT_REGISTRY) {
  const abs = path.resolve(file);
  if (!fs.existsSync(abs)) throw new Error(`门禁清单不存在：${abs}`);
  let data;
  try {
    data = JSON.parse(fs.readFileSync(abs, 'utf8'));
  } catch (e) {
    throw new Error(`门禁清单不是合法 JSON：${abs}（${e.message}）`);
  }
  const gates = Array.isArray(data.gates) ? data.gates : [];
  const canaries = Array.isArray(data.canaries) ? data.canaries : [];
  if (gates.length === 0) throw new Error(`门禁清单为空：${abs}（门禁不许整体消失）`);

  const seenGate = new Set();
  for (const gate of gates) {
    if (!gate.id) throw new Error(`门禁条目缺 id：${JSON.stringify(gate)}`);
    if (seenGate.has(gate.id)) throw new Error(`门禁 id 重复：${gate.id}`);
    seenGate.add(gate.id);
    if (!gate.label) throw new Error(`门禁 ${gate.id} 缺 label`);
    if (!gate.group) throw new Error(`门禁 ${gate.id} 缺 group`);
    if (!hasSteps(gate) && !hasSection(gate)) {
      throw new Error(`门禁 ${gate.id} 没有执行方式（要么给 steps，要么给 section 指向门禁自检第 N 节）`);
    }
    if (hasSteps(gate)) {
      for (const step of gate.steps) {
        if (!Array.isArray(step.command) || step.command.length === 0) {
          throw new Error(`门禁 ${gate.id} 的某个步骤没有 command`);
        }
      }
    }
    if (hasSection(gate) && !Number.isInteger(gate.section)) {
      throw new Error(`门禁 ${gate.id} 的 section 必须是整型演练节号`);
    }
  }

  const seenCanary = new Set();
  for (const canary of canaries) {
    if (!Number.isInteger(canary.id)) throw new Error(`演练条目缺整型 id：${JSON.stringify(canary)}`);
    if (seenCanary.has(canary.id)) throw new Error(`演练 id 重复：${canary.id}`);
    seenCanary.add(canary.id);
  }

  const notGates = Array.isArray(data.notGates) ? data.notGates : [];
  for (const id of notGates) {
    if (!Number.isInteger(id)) throw new Error(`notGates 只能列演练节号（整型），收到 ${JSON.stringify(id)}`);
    if (!seenCanary.has(id)) throw new Error(`notGates 里的演练 ${id} 不存在于 canaries（豁免不存在的演练等于没豁免）`);
  }

  return { file: abs, gates, canaries, notGates };
}

// hasSteps 判断这道门禁的命令是否可由 runner 直接执行。
export function hasSteps(gate) {
  return Array.isArray(gate.steps) && gate.steps.length > 0;
}

// hasSection 判断这道门禁是否只靠门禁自检的某一节证明（真门禁在别处执行）。
export function hasSection(gate) {
  return gate.section !== undefined && gate.section !== null;
}

// executableGates 是「runner 能代跑」的那部分：有 steps 的。
export function executableGates(gates) {
  return gates.filter(hasSteps);
}

// coverageProblems 返回「清单与演练对不上」的问题。空数组 = 对得上。
//
// 两个方向都查：
//   1. 每道门禁都有演练节号、节号存在、且有执行方式（steps 或 section）；
//   2. 每个演练节号都有门禁负责 —— 否则清单里少登记了一道门禁，
//      自检会在跑一节没有任何门禁对应的演练，等于演了个不存在的东西。
//
// 唯一的例外是 notGates：`#287 AC4` 只把「清单 / 版本 / 判定 / 重复安装」
// 四件收进清单，参数生成脚本不属于这四件。那种演练必须**显式**登记在
// notGates 里才豁免；没登记的一律照旧算「没人负责」—— 所以豁免不会
// 变成「少一道门禁也自动放行」。反过来，某节既被 notGates 豁免又有门禁
// 认领，说明登记自相矛盾，同样报出来。
export function coverageProblems(registry) {
  const problems = [];
  const canaryIds = new Set(registry.canaries.map(c => c.id));
  const notGates = new Set(registry.notGates || []);
  for (const gate of registry.gates) {
    if (!Array.isArray(gate.canaries) || gate.canaries.length === 0) {
      problems.push(`门禁 ${gate.id} 没有登记演练节号（门禁自检会验不到它）`);
      continue;
    }
    for (const id of gate.canaries) {
      if (!canaryIds.has(id)) problems.push(`门禁 ${gate.id} 指向不存在的演练 ${id}`);
      if (notGates.has(id)) problems.push(`门禁 ${gate.id} 认领了清单外演练 ${id}（它已登记在 notGates，两者只能其一）`);
    }
    if (!hasSteps(gate) && !hasSection(gate)) {
      problems.push(`门禁 ${gate.id} 没有执行方式（steps 或 section 至少要有一个）`);
    }
    if (hasSection(gate) && !gate.canaries.includes(gate.section)) {
      problems.push(`门禁 ${gate.id} 的 section=${gate.section} 不在它的 canaries 里（自检节号与门禁对不上）`);
    }
  }
  const covered = new Set();
  for (const gate of registry.gates) {
    for (const id of gate.canaries || []) covered.add(id);
  }
  for (const canary of registry.canaries) {
    if (!covered.has(canary.id) && !notGates.has(canary.id)) {
      problems.push(`演练 ${canary.id}（${canary.title}）没有门禁负责（清单里少了一道门禁；确属清单外请显式登记到 notGates）`);
    }
  }
  return problems;
}

// globMatches 支持 `**`（任意层级）、`*`（单层）与 `?`，路径统一用 `/`。
export function globMatches(pattern, target) {
  const p = String(pattern).replace(/\\/g, '/');
  const t = String(target).replace(/\\/g, '/');
  const tail = p.endsWith('/**');
  const body = tail ? p.slice(0, -3) : p;
  let re = '';
  for (let i = 0; i < body.length; i++) {
    const ch = body[i];
    if (ch === '*') {
      if (body[i + 1] === '*') { re += '.*'; i += 1; } else re += '[^/]*';
    } else if (ch === '?') re += '.';
    else if ('\\^$.|+()[]{}'.includes(ch)) re += `\\${ch}`;
    else re += ch;
  }
  if (tail) return new RegExp(`^${re}(/.*)?$`).test(t);
  return new RegExp(`^${re}$`).test(t);
}

// affectedCanaries 按改动路径挑出受影响的演练节号（升序、去重）。
// 只用于「按改动范围只跑受影响演练」，默认仍然是全跑。
export function affectedCanaries(registry, paths) {
  const hit = new Set();
  for (const canary of registry.canaries) {
    for (const pattern of canary.affected || []) {
      if (paths.some(p => globMatches(pattern, p))) { hit.add(canary.id); break; }
    }
  }
  return [...hit].sort((a, b) => a - b);
}

// gatesForCanaries 选出被这些演练节号覆盖的门禁。
export function gatesForCanaries(registry, canaryIds) {
  return registry.gates.filter(g =>
    Array.isArray(g.canaries) && g.canaries.some(id => canaryIds.includes(id)));
}

export function gatesInGroup(registry, group) {
  return registry.gates.filter(g => g.group === group);
}

export function gateIds(registry) {
  return registry.gates.map(g => g.id);
}

export function groups(registry) {
  return [...new Set(registry.gates.map(g => g.group))].sort();
}

export function canaryIds(registry) {
  return registry.canaries.map(c => c.id).sort((a, b) => a - b);
}
