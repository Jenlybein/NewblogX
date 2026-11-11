// 两端 MetricCard 同源门禁（票 354）。
//
// 读者站的个人概览与后台的站点概览显示同一类数字（篇数、次数、阅读量），
// 两处各写一份组件就会出现"同一个数字在两页字号不同"这种没人会主动去比对的漂移。
// 所以两份必须**逐字节相同**：读者站那份是原文，后台那份是逐字复制。
//
// 与 check-image-shaping-parity.mjs 同一套判据（导出纯函数，便于用合成数据
// 断言"门禁会红"），只是对象换成一张卡片。
//
// 退出码：0 = 逐字节一致；1 = 漂移或文件缺失。
import { readFileSync, existsSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

/** 一对副本（相对仓库根）。 */
export const METRIC_CARD_FILES = {
  reader: 'frontend/app/components/studio/MetricCard.vue',
  admin: 'frontend_admin/app/components/media/MetricCard.vue',
};

/**
 * 判定一对副本是否逐字节一致（纯函数）。
 *
 * 判据只有一条：两份内容完全相同。不做"忽略空白"之类的宽松比较 ——
 * 宽松比较会让"只在一侧改了个换行"这种事永远发现不了。
 */
export function evaluateMetricCardParity({ left, right }) {
  if (typeof left !== 'string' || typeof right !== 'string') {
    return { ok: false, reason: 'MISSING_CONTENT' };
  }
  if (left.length === 0 || right.length === 0) {
    return { ok: false, reason: 'EMPTY_CONTENT' };
  }
  if (left !== right) {
    // 找到第一处差异，报错时能直接定位，而不是只说"不一样"。
    const limit = Math.min(left.length, right.length);
    let at = 0;
    while (at < limit && left[at] === right[at]) at += 1;
    return { ok: false, reason: 'DRIFT', offset: at, leftBytes: left.length, rightBytes: right.length };
  }
  return { ok: true, bytes: left.length };
}

/** 从仓库根读两份内容并判定。 */
export function checkMetricCardParity(root) {
  const leftPath = join(root, METRIC_CARD_FILES.reader);
  const rightPath = join(root, METRIC_CARD_FILES.admin);
  for (const file of [leftPath, rightPath]) {
    if (!existsSync(file)) {
      return { ok: false, reason: 'FILE_MISSING', file };
    }
  }
  return evaluateMetricCardParity({
    left: readFileSync(leftPath, 'utf8'),
    right: readFileSync(rightPath, 'utf8'),
  });
}

/** CLI 入口。 */
function main() {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
  const result = checkMetricCardParity(root);
  if (!result.ok) {
    console.error('✗ 两端 MetricCard 同源门禁失败');
    if (result.reason === 'FILE_MISSING') {
      console.error(`  找不到 ${result.file}`);
    } else if (result.reason === 'DRIFT') {
      console.error(`  两份内容不一致：第 ${result.offset} 个字符起分叉（${result.leftBytes} vs ${result.rightBytes} 字节）`);
      console.error('  修法：把读者站那份逐字复制到后台（两份必须完全相同）。');
    } else {
      console.error(`  原因：${result.reason}`);
    }
    process.exitCode = 1;
    return;
  }
  console.log(`✓ 两端 MetricCard 同源门禁通过（${result.bytes} 字节 × 2 份，逐字节一致）`);
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
  main();
}
