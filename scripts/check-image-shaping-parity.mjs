// 「两端 imageShaping.ts 逐字节一致」同步门禁（#200，承接 #195 的跨包共享交代）。
//
// 为什么必须有它（#176 的教训）：后台（frontend_admin）**不得相对路径 import 读者站
// 模块** —— 那样后台生产构建会失败（后台必须自洽，有自己的 workspace 与 lockfile）。
// 于是三种封面上传与头像定形要共用同一套定形实现，只能靠在两端各放一份**逐字一致**
// 的文件。只靠纪律的话，必然出现"改了读者站忘记改后台"：后台静默停在旧尺寸/旧裁切，
// 而且两端各自的测试都是绿的（每份文件内部自洽）。
//
// 判定是**逐字节**的，不是"解析后比语义"：
//   - 语义比对会放过注释、顺序、空白差异，而任何一种差异都意味着有人只改了一端；
//   - 只比文件大小会漏掉"改了一个字符、长度不变"的事故（最常见的一种）。
//
// 差异时报出**是哪一份漂移**：字节差异本身分不出谁对谁错，所以除了偏移/行号，
// 还报两份各多少字节、哪一侧多出内容。权威方向是 #195 定下的：读者站那份是原文，
// 改完逐字复制到后台。
//
// canary 由两处保证它会红：
//   1. frontend/test/utils/image-shaping-parity.test.ts：喂合成的不一致内容断言失败；
//   2. scripts/verify-gates.sh：真实仓库里**故意只改后台副本的一个常量**，断言门禁变红。
//
// 用法：node scripts/check-image-shaping-parity.mjs [--report]
//   --report：只打印两份文件大小/首行差异（排查用，不做判定）
// 退出码：0 = 逐字节一致；1 = 不一致或文件缺失
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

/** 两端各自那份定形工具（相对仓库根）。 */
export const IMAGE_SHAPING_FILES = {
  reader: "frontend/app/utils/imageShaping.ts",
  admin: "frontend_admin/app/utils/imageShaping.ts",
};

/** 从工作目录向上找到仓库根（含 scripts/ 与两端 imageShaping 的目录）。 */
export function findRepoRoot(fromDir = process.cwd()) {
  let dir = resolve(fromDir);
  for (;;) {
    const scriptPath = join(dir, "scripts", "check-image-shaping-parity.mjs");
    if (existsSync(join(dir, IMAGE_SHAPING_FILES.reader)) && existsSync(scriptPath)) return dir;
    const parent = dirname(dir);
    if (parent === dir) return resolve(fromDir);
    dir = parent;
  }
}

/** 把字符串 / Buffer 统一成 Buffer（比较与偏移都按原始字节）。 */
function toBuffer(content) {
  return Buffer.isBuffer(content) ? content : Buffer.from(String(content), "utf8");
}

/** 字节偏移对应的行号（1-based）：数它前面的换行符。 */
function lineAt(buffer, offset) {
  let line = 1;
  const end = Math.min(offset, buffer.length);
  for (let index = 0; index < end; index += 1) {
    if (buffer[index] === 0x0a) line += 1;
  }
  return line;
}

/**
 * 找第一个不同的字节。
 *
 * @returns {null | { offset: number, leftLine: number, rightLine: number,
 *   leftSize: number, rightSize: number, leftByte: number | null, rightByte: number | null }}
 */
export function findFirstDifference(left, right) {
  const leftBuffer = toBuffer(left);
  const rightBuffer = toBuffer(right);
  const shared = Math.min(leftBuffer.length, rightBuffer.length);

  let offset = 0;
  while (offset < shared && leftBuffer[offset] === rightBuffer[offset]) offset += 1;

  if (offset === leftBuffer.length && offset === rightBuffer.length) return null;

  return {
    offset,
    leftLine: lineAt(leftBuffer, offset),
    rightLine: lineAt(rightBuffer, offset),
    leftSize: leftBuffer.length,
    rightSize: rightBuffer.length,
    leftByte: offset < leftBuffer.length ? leftBuffer[offset] : null,
    rightByte: offset < rightBuffer.length ? rightBuffer[offset] : null,
  };
}

/** 把一个字节表示成可读形式（`0x5b '['` / `（无此字节）`）。 */
function describeByte(byte) {
  if (byte === null) return "（该文件在此处已结束）";
  const char = byte >= 0x20 && byte <= 0x7e ? String.fromCharCode(byte) : "·";
  return `0x${byte.toString(16).padStart(2, "0")} '${char}'`;
}

/**
 * 说清"哪一份漂移"：字节差异分不出对错，但多出内容的那一侧一定不是原文。
 *
 * @returns {string} 一句面向人的判定提示（含权威方向）
 */
export function describeDrift(difference, leftPath, rightPath) {
  const delta = difference.rightSize - difference.leftSize;
  const direction =
    delta > 0
      ? `后台那份比读者站多出 ${delta} 字节（只改了后台，或读者站还没同步）`
      : delta < 0
        ? `读者站那份比后台多出 ${-delta} 字节（只改了读者站，后台副本忘了同步）`
        : "两份长度相同，差异在内容里（只看大小判断不出是哪一份改了）";

  return (
    `漂移方向：${direction}。` +
    `权威方向：读者站那份（${leftPath}）是原文，改完必须逐字复制到 ${rightPath}。`
  );
}

/**
 * 判定两份内容是否逐字节一致（纯函数，便于用人造数据断言门禁会红）。
 *
 * @returns {{ ok: boolean, failures: {kind: string, message: string}[], difference: object | null }}
 */
export function evaluateImageShapingParity({ left, right, leftPath, rightPath }) {
  const difference = findFirstDifference(left, right);
  if (!difference) return { ok: true, failures: [], difference: null };

  const message =
    `${leftPath} 与 ${rightPath} 不是逐字节一致：` +
    `第 ${difference.offset} 字节起（前者第 ${difference.leftLine} 行 / 后者第 ${difference.rightLine} 行）` +
    `${leftPath} 是 ${describeByte(difference.leftByte)}，` +
    `${rightPath} 是 ${describeByte(difference.rightByte)}；` +
    `文件大小 ${difference.leftSize} / ${difference.rightSize} 字节。` +
    `${describeDrift(difference, leftPath, rightPath)}` +
    "两端 imageShaping 是同一份同源文件：改一端必须把同一份内容复制到另一端。";

  return {
    ok: false,
    failures: [{ kind: "byte-mismatch", message }],
    difference,
  };
}

/** 读两端真实文件并判定。任一文件缺失都算失败（不能当成「两端都空」通过）。 */
export function checkImageShapingParity({ repoRoot = findRepoRoot() } = {}) {
  const contents = {};
  const failures = [];

  for (const [side, relativePath] of Object.entries(IMAGE_SHAPING_FILES)) {
    const fullPath = join(repoRoot, relativePath);
    if (!existsSync(fullPath)) {
      failures.push({
        kind: "image-shaping-file-missing",
        message: `找不到 ${relativePath}（仓库根 ${repoRoot}）：两端 imageShaping.ts 必须都在。`,
      });
      continue;
    }
    contents[side] = readFileSync(fullPath);
  }

  if (failures.length > 0) {
    return { ok: false, failures, bytes: 0, files: {} };
  }

  const result = evaluateImageShapingParity({
    left: contents.reader,
    right: contents.admin,
    leftPath: IMAGE_SHAPING_FILES.reader,
    rightPath: IMAGE_SHAPING_FILES.admin,
  });

  return {
    ...result,
    bytes: contents.reader.length,
    files: { reader: contents.reader.length, admin: contents.admin.length },
  };
}

function runCli(argv) {
  const report = argv.includes("--report");
  const result = checkImageShapingParity({ repoRoot: findRepoRoot() });

  if (report) {
    console.log(`两端 imageShaping：${IMAGE_SHAPING_FILES.reader} / ${IMAGE_SHAPING_FILES.admin}`);
    console.log(`文件大小：${result.files.reader ?? 0} / ${result.files.admin ?? 0} 字节`);
    console.log(result.ok ? "逐字节一致" : result.failures.map((failure) => failure.message).join("\n"));
    return 0;
  }

  if (result.ok) {
    console.log(
      `✓ 两端定形工具同源门禁通过（${result.files.reader} 字节 × 2 份，逐字节一致）`,
    );
    return 0;
  }

  console.error("✗ 两端定形工具（imageShaping）同源门禁失败");
  for (const failure of result.failures) console.error(`  ${failure.message}`);
  return 1;
}

if (process.argv[1] && /check-image-shaping-parity\.mjs$/u.test(process.argv[1])) {
  process.exit(runCli(process.argv.slice(2)));
}
