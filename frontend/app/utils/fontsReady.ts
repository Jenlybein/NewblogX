/**
 * 「字体就绪后再量一次」的唯一入口（目录树圆点与标题大纲轨道共用）。
 *
 * 为什么需要这次重测：首测往往落在 webfont 替换之前，行高会在字体到货那一刻变。
 * 依赖布局坐标的组件（圆点的行中心、轨道每行的 y）必须再量一遍，否则会停在
 * 旧行高上，直到下一次别的原因触发重测才突然跳过去。
 *
 * 为什么把 SSR 守卫收进来：`document` 在服务端不存在，而这类重测在 SSR 里既无
 * 意义也会直接抛错。守卫放在这里，调用点就不必各写一遍——此前两处各有一份，
 * 其中一处正好漏了守卫。差异不该靠"记得写"来维持。
 *
 * 回调最多跑一次，失败（字体加载被取消等）静默吞掉：重测是锦上添花，
 * 不该把 `onMounted` 变成一条可能炸掉的 Promise 链。
 */
export function whenFontsReady(run: () => void): void {
  if (typeof document === "undefined") {
    return;
  }

  const fonts = document.fonts;
  if (!fonts?.ready) {
    return;
  }

  void fonts.ready.then(run).catch(() => undefined);
}
