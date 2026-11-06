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
    if (!Array.isArray(gate.steps) || gate.steps.length === 0) {
      throw new Error(`门禁 ${gate.id} 没有可执行步骤`);
    }
    for (const step of gate.steps) {
      if (!Array.isArray(step.command) || step.command.length === 0) {
        throw new Error(`门禁 ${gate.id} 的某个步骤没有 command`);
      }
    }
  }

  const seenCanary = new Set();
  for (const canary of canaries) {
    if (!Number.isInteger(canary.id)) throw new Error(`演练条目缺整型 id：${JSON.stringify(canary)}`);
    if (seenCanary.has(canary.id)) throw new Error(`演练 id 重复：${canary.id}`);
    seenCanary.add(canary.id);
  }

  return { file: abs, gates, canaries };
}

// coverageProblems 返回「清单里有门禁，但门禁自检没有对应演练」等问题。
// 空数组 = 每道门禁都被门禁自检演练过。
//
// 只查这一个方向：演练清单里还有一批节（后端单测、typecheck、lint、覆盖率门槛等）
// 不在本清单里 —— 它们不是仓库级判据，也不在「清单 / 版本 / 判定 / 安装」四件事内。
export function coverageProblems(registry) {
  const problems = [];
  const canaryIds = new Set(registry.canaries.map(c => c.id));
  for (const gate of registry.gates) {
    if (!Array.isArray(gate.canaries) || gate.canaries.length === 0) {
      problems.push(`门禁 ${gate.id} 没有登记演练节号（门禁自检会验不到它）`);
      continue;
    }
    for (const id of gate.canaries) {
      if (!canaryIds.has(id)) problems.push(`门禁 ${gate.id} 指向不存在的演练 ${id}`);
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
