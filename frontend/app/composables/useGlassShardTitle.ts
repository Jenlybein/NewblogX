import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

import { useMotionPreference } from "~/composables/useMotionPreference";
import { GLASS_SHARDS, type GlassShardDef } from "~/utils/glassShards";

/**
 * 「碎玻璃大标题」的动效接线（首屏封面的动效例外，见组件注释）。
 *
 * 还原对象：lanrhyme.top 首页 h1 的 ParticleText 组件。这里只做三件事：
 *   1. 指针视差：鼠标位置归一化后做 0.1 阻尼插值，每块碎片按 speed/层深写 transform；
 *   2. 点击碎裂：碎片朝各自象限弹开 1 秒，同时文字乱码化 → 逐字解码回原文；
 *   3. 省电：IntersectionObserver 离屏停 rAF、`visibilitychange` 隐藏停 rAF、
 *      「减少动态」下只渲染一帧静帧（不抖动、不乱码、不弹开）。
 *
 * 数值全部是原组件的实测值（15/40px 基础幅度、5°/15° 旋转、sin/cos 各 3px 抖动、
 * 炸开 30px…）。它们是这个效果的身份，不做"项目动效预算"式的收缩；
 * 代价由离屏停帧和 reduce 归零兜住，而不是靠调小位移。
 */

/** 乱码解码用字符集：与原站同一个集合。 */
export const GLASS_SCRAMBLE_CHARS = "!<>-_\\/[]{}—=+*^?#01";

/** 乱码刷新间隔（毫秒）。原站 40ms。 */
const SCRAMBLE_INTERVAL_MS = 40;
/** 解码总步数：20 步 × 20ms ≈ 400ms 从乱码回到正常字。 */
const DECODE_STEPS = 20;
const DECODE_INTERVAL_MS = 20;
/** 碎裂状态持续时间（毫秒）。到点后碎片归位并开始解码。 */
const BURST_DURATION_MS = 1000;
/** reduce 下的淡光时长（进 200ms、停留一点、出交给过渡）。 */
const REDUCED_FLASH_MS = 260;
/** 空闲判定：指针停下多久之后把 rAF 限到 30Hz 并摘掉 will-change。 */
const IDLE_AFTER_MS = 1500;

/** 视差阻尼：指针值 → 渲染值，每帧靠拢 10%。 */
const POINTER_LERP = 0.1;
/** 按下/弹开的阻尼：每帧靠拢 15%。 */
const SPRING_LERP = 0.15;

/** 底片 / 浮层的基础位移倍数与旋转角度（原站实测）。 */
const BASE_TRAVEL_Z1 = 15;
const BASE_TRAVEL_Z2 = 40;
const BASE_ROTATE_Z1 = 5;
const BASE_ROTATE_Z2 = 15;

/*
 * 静止时的「呼吸」不再由 JS 每帧写 transform（见 loop()）：九块碎片的 3px 慢漂移
 * 交给 CSS keyframes（`.glass-title.is-idle .glass-title__shard-inner`），跑在合成
 * 线程上。JS 只在交互/炸开/回弹期间跑 rAF，停手 1.5s 后彻底停帧。
 */
/** 按下时整体幅度收敛比例（0.8 = 只剩 20%）。 */
const PRESS_DAMPING = 0.8;
/** 浮层碎片在悬停时放大到 1.05。 */
const OVERLAY_HOVER_SCALE = 1.05;
/** 按下时整体缩小 0.05。 */
const PRESS_SCALE = 0.05;
/** 炸开位移：横纵 30px、纵深 z×20px、旋转 15°。 */
const BURST_TRAVEL_PX = 30;
const BURST_DEPTH_PX = 20;
const BURST_ROTATE_DEG = 15;

/**
 * 原站标题字号（clamp(48px, 8.5vw, 150px) 的上限）。
 *
 * 截图里的幅度都是"150px 字号下的绝对值"，直接搬到首屏 clamp(2rem, 9vw, 6rem)
 * （最大 96px）就相对过头了：字小一半、位移没变，同一个指针位置会把字拆得更散。
 * 所以所有位移 / 旋转都按 `实际字号 / 150` 等比缩放，视觉强度与字号保持一致。
 */
const REFERENCE_FONT_SIZE_PX = 150;
/** 缩放钳位：太小没有"玻璃感"，太大又会散架。 */
const MIN_UNIT_SCALE = 0.55;
const MAX_UNIT_SCALE = 1.6;

export interface UseGlassShardTitleOptions {
  /** 读取当前标题文字（乱码要用最新值，所以传 getter 而不是字符串）。 */
  text: () => string;
  /** 点击时是否合成碎裂音效，默认关（博客首屏不该突然发声）。 */
  sound?: boolean;
}

interface ShardRuntime {
  node: HTMLElement;
  def: GlassShardDef;
  /** 当前渲染位移 / 旋转 / 缩放。 */
  tx: number;
  ty: number;
  rx: number;
  ry: number;
  scale: number;
  /** 炸开的偏移，弹开时向目标靠拢、松开时回零。 */
  px: number;
  py: number;
  pz: number;
  prx: number;
  pry: number;
}

function randomScrambleChar(): string {
  return GLASS_SCRAMBLE_CHARS[Math.floor(Math.random() * GLASS_SCRAMBLE_CHARS.length)]!;
}

/**
 * 合成碎裂音（无音频文件，全在 Web Audio 里现做）。
 *
 * 只在调用方显式打开 `sound` 时才会走到这里 —— 博客首屏点一下突然出声太突兀，
 * 默认静音。链路与原站一致：随机白噪走 bandpass 3k→8k，再叠一记 100→20Hz 方波。
 */
function playShatterSound() {
  try {
    const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!Ctor) return;
    const ctx = new Ctor();
    if (ctx.state === "suspended") void ctx.resume();

    const seconds = 0.4;
    const frames = Math.round(ctx.sampleRate * seconds);
    const buffer = ctx.createBuffer(1, frames, ctx.sampleRate);
    const data = buffer.getChannelData(0);
    for (let index = 0; index < frames; index += 1) {
      // 每 8 帧空 2 帧，给噪声一点"颗粒"感而不是连续白噪
      data[index] = (Math.random() * 2 - 1) * (index % 8 < 2 ? 0 : 1);
    }

    const noise = ctx.createBufferSource();
    noise.buffer = buffer;

    const bandpass = ctx.createBiquadFilter();
    bandpass.type = "bandpass";
    bandpass.frequency.setValueAtTime(3000, ctx.currentTime);
    bandpass.frequency.exponentialRampToValueAtTime(8000, ctx.currentTime + 0.3);

    const noiseGain = ctx.createGain();
    noiseGain.gain.setValueAtTime(0.35, ctx.currentTime);
    noiseGain.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.3);

    const oscillator = ctx.createOscillator();
    oscillator.type = "square";
    oscillator.frequency.setValueAtTime(100, ctx.currentTime);
    oscillator.frequency.exponentialRampToValueAtTime(20, ctx.currentTime + 0.3);

    const oscillatorGain = ctx.createGain();
    oscillatorGain.gain.setValueAtTime(0.25, ctx.currentTime);
    oscillatorGain.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.3);

    noise.connect(bandpass).connect(noiseGain).connect(ctx.destination);
    oscillator.connect(oscillatorGain).connect(ctx.destination);

    noise.start();
    oscillator.start();
    oscillator.stop(ctx.currentTime + 0.3);
    window.setTimeout(() => {
      void ctx.close().catch(() => {});
    }, 800);
  } catch {
    // 音效只是彩蛋：浏览器不给 AudioContext 就静默跳过
  }
}

/**
 * @param root 标题容器（组件根节点）；碎片与文字副本由模板渲染，这里只负责动。
 */
export function useGlassShardTitle(root: Ref<HTMLElement | null>, options: UseGlassShardTitleOptions) {
  const { prefersReducedMotion } = useMotionPreference();

  let shards: ShardRuntime[] = [];
  let textNodes: HTMLElement[] = [];

  // 指针状态：目标值 / 阻尼后的渲染值 / 按下程度。
  let pointerX = 0;
  let pointerY = 0;
  let smoothedX = 0;
  let smoothedY = 0;
  let pressTarget = 0;
  let pressAmount = 0;
  let hovering = false;
  let burstPhase = 0;

  let rafId = 0;
  let burstTimer = 0;
  let scrambleTimer = 0;
  let decodeTimer = 0;
  let observer: IntersectionObserver | null = null;
  let inView = true;
  /** 位移 / 旋转的等比系数，随标题字号与视口大小重算（见 REFERENCE_FONT_SIZE_PX）。 */
  let unitScale = 1;

  function measureUnitScale() {
    const host = root.value;
    if (!host) return;
    const fontSize = Number.parseFloat(window.getComputedStyle(host).fontSize);
    if (!Number.isFinite(fontSize) || fontSize <= 0) return;
    unitScale = Math.min(MAX_UNIT_SCALE, Math.max(MIN_UNIT_SCALE, fontSize / REFERENCE_FONT_SIZE_PX));
  }

  function captureNodes() {
    const host = root.value;
    if (!host) {
      shards = [];
      textNodes = [];
      return;
    }
    // 模板按 GLASS_SHARDS 的顺序渲染，所以下标与常量一一对应。
    shards = Array.from(host.querySelectorAll<HTMLElement>("[data-glass-shard]")).map((node, index) => ({
      node,
      def: GLASS_SHARDS[index]!,
      tx: 0,
      ty: 0,
      rx: 0,
      ry: 0,
      scale: 1,
      px: 0,
      py: 0,
      pz: 0,
      prx: 0,
      pry: 0,
    }));
    textNodes = Array.from(host.querySelectorAll<HTMLElement>("[data-glass-text]"));
  }

  function writeText(text: string) {
    for (const node of textNodes) node.textContent = text;
  }

  function stopTextTimers() {
    if (scrambleTimer) window.clearInterval(scrambleTimer);
    if (decodeTimer) window.clearInterval(decodeTimer);
    scrambleTimer = 0;
    decodeTimer = 0;
  }

  /** 乱码阶段：每 40ms 把每个可见字符换成随机符号。 */
  function scramble() {
    stopTextTimers();
    if (prefersReducedMotion.value) return;
    scrambleTimer = window.setInterval(() => {
      writeText(
        options.text()
          .split("")
          .map((char) => (char === " " ? " " : randomScrambleChar()))
          .join(""),
      );
    }, SCRAMBLE_INTERVAL_MS);
  }

  /** 解码阶段：从左到右逐步还原，未轮到的位置仍是随机符号。 */
  function decode() {
    stopTextTimers();
    const source = options.text();
    if (prefersReducedMotion.value) {
      writeText(source);
      return;
    }
    let step = 0;
    decodeTimer = window.setInterval(() => {
      const reveal = step / DECODE_STEPS;
      writeText(
        source
          .split("")
          .map((char, index) => {
            if (char === " ") return " ";
            return index < reveal * source.length ? char : randomScrambleChar();
          })
          .join(""),
      );
      step += 1;
      if (step > DECODE_STEPS) {
        stopTextTimers();
        writeText(source);
      }
    }, DECODE_INTERVAL_MS);
  }

  function toggleHalo(active: boolean) {
    const halo = root.value?.querySelector(".glass-title__halo");
    halo?.classList.toggle("is-active", active);
  }

  /** 点击彩蛋：碎片炸开 1 秒 → 归位 + 解码。 */
  function burst() {
    if (prefersReducedMotion.value) {
      /*
       * 减少动态：不做位移、不跑乱码，但保留一次**纯淡光**。
       * 字标是首屏唯一可点的元素，此前在 reduce 下点击完全没有任何反馈
       * （只剩 crosshair 光标在承诺一个不会发生的交互）。
       */
      toggleHalo(true);
      window.clearTimeout(burstTimer);
      burstTimer = window.setTimeout(() => toggleHalo(false), REDUCED_FLASH_MS);
      return;
    }
    burstPhase = 1;
    toggleHalo(true);
    scramble();
    if (options.sound) playShatterSound();
    window.clearTimeout(burstTimer);
    burstTimer = window.setTimeout(() => {
      burstPhase = 0;
      toggleHalo(false);
      decode();
    }, BURST_DURATION_MS);
  }

  /** 上一帧的 rAF 时间戳，用来把"每帧接近 base"归一化成按真实 dt。 */
  let lastFrameAt = 0;
  /** 最后一次交互（指针移动/按下/离开）的时间，用于空闲限频。 */
  let lastInteractionAt = 0;


  /**
   * 把"每帧接近 base"换算成按真实 dt 接近 base。
   *
   * 原来的 `value += (target - value) * 0.1` 是每**帧**一次：60Hz 上约 0.3s 落位，
   * 120Hz 上只要 0.15s —— 同一个手势在高刷屏上快一倍、且松手不带速度。按 dt 归一化
   * 之后落位速度与刷新率无关（系数仍取原组件的 0.1 / 0.15，视觉强度不变）。
   */
  function smoothingAlpha(base: number, nowMs: number): number {
    if (!lastFrameAt) return base;
    const frames = Math.min(4, Math.max(0.25, (nowMs - lastFrameAt) / 16.6667));
    return 1 - Math.pow(1 - base, frames);
  }

  function renderFrame(time: number) {
    const pointerAlpha = smoothingAlpha(POINTER_LERP, time);
    const springAlpha = smoothingAlpha(SPRING_LERP, time);
    lastFrameAt = time;

    pressAmount += ((pressTarget === 1 ? 1 : 0) - pressAmount) * springAlpha;
    smoothedX += (pointerX - smoothedX) * pointerAlpha;
    smoothedY += (pointerY - smoothedY) * pointerAlpha;

    for (const shard of shards) {
      const { def } = shard;
      const layered = def.z === 1;
      const travel = (layered ? BASE_TRAVEL_Z1 : BASE_TRAVEL_Z2) * (1 - pressAmount * PRESS_DAMPING);

      const parallaxX = smoothedX * travel * unitScale * def.speed * 20;
      const parallaxY = smoothedY * travel * unitScale * def.speed * 20;
      const parallaxRy = smoothedX * (layered ? BASE_ROTATE_Z1 : BASE_ROTATE_Z2) * unitScale;
      const parallaxRx = -smoothedY * (layered ? BASE_ROTATE_Z1 : BASE_ROTATE_Z2) * unitScale;

      const exploded = burstPhase === 1;
      const burstTravel = BURST_TRAVEL_PX * unitScale;
      const burstRotate = BURST_ROTATE_DEG * unitScale;
      shard.px += ((exploded ? def.bx * burstTravel : 0) - shard.px) * springAlpha;
      shard.py += ((exploded ? def.by * burstTravel : 0) - shard.py) * springAlpha;
      shard.pz += ((exploded ? def.z * BURST_DEPTH_PX : 0) - shard.pz) * springAlpha;
      shard.prx += ((exploded ? def.by * burstRotate : 0) - shard.prx) * springAlpha;
      shard.pry += ((exploded ? def.bx * burstRotate : 0) - shard.pry) * springAlpha;

      shard.tx = parallaxX + shard.px;
      shard.ty = parallaxY + shard.py;
      shard.rx = parallaxRx + shard.prx;
      shard.ry = parallaxRy + shard.pry;

      const scaleTarget = prefersReducedMotion.value
        ? 1
        : (hovering && !layered ? OVERLAY_HOVER_SCALE : 1) - pressAmount * PRESS_SCALE;
      shard.scale += (scaleTarget - shard.scale) * springAlpha;

      shard.node.style.transform =
        `translate3d(${shard.tx.toFixed(2)}px, ${shard.ty.toFixed(2)}px, ${(def.z * 10 + shard.pz).toFixed(2)}px) ` +
        `rotateX(${shard.rx.toFixed(2)}deg) rotateY(${shard.ry.toFixed(2)}deg) scale(${shard.scale.toFixed(3)})`;
    }
  }

  /**
   * 这一帧真的还需要跑吗。
   *
   * 只保留「有东西正在动」的四种情形：指针还在靠拢目标、按住/回弹没停稳、碎片还在
   * 炸开或归位、以及交互后的收尾窗口。静止时一次都不写 —— 上一版是把 60Hz 的 rAF
   * 用「每 32ms 才写一次」去跳帧，得到的是 33.4ms / 50.1ms / 66.8ms 这种
   * 不均匀的帧距：慢速漂移最怕帧距不齐，比稳定 30fps 更容易看出抖。
   * 现在静止的漂移交给 CSS keyframes（合成线程，帧率稳定），JS 完全不参与。
   */
  function isSettled(time: number): boolean {
    if (burstPhase === 1) return false;
    if (pressTarget === 1 || pressAmount > 0.001) return false;
    if (hovering) return false;
    if (time - lastInteractionAt < IDLE_AFTER_MS) return false;
    if (Math.abs(smoothedX - pointerX) > 0.001 || Math.abs(smoothedY - pointerY) > 0.001) return false;
    return shards.every(
      (shard) =>
        Math.abs(shard.px) < 0.05 &&
        Math.abs(shard.py) < 0.05 &&
        Math.abs(shard.pz) < 0.05 &&
        Math.abs(shard.prx) < 0.05 &&
        Math.abs(shard.pry) < 0.05,
    );
  }

  /** 空闲态：`is-idle` 交给 CSS 呼吸，`is-live`（will-change）交还给浏览器。 */
  function enterIdle() {
    root.value?.classList.add("is-idle");
    root.value?.classList.remove("is-live");
  }

  function leaveIdle() {
    root.value?.classList.remove("is-idle");
    root.value?.classList.add("is-live");
  }

  function loop(time: number) {
    if (isSettled(time)) {
      renderFrame(time);
      stopLoop();
      enterIdle();
      return;
    }
    renderFrame(time);
    rafId = window.requestAnimationFrame(loop);
  }

  function startLoop() {
    if (rafId || prefersReducedMotion.value) return;
    leaveIdle();
    rafId = window.requestAnimationFrame(loop);
  }

  function stopLoop() {
    if (!rafId) return;
    window.cancelAnimationFrame(rafId);
    rafId = 0;
  }

  /** 动效开关的唯一入口：reduce / 离屏 / 页面隐藏 三种情况都停 rAF 并落一帧静帧。 */
  function syncMotion() {
    if (prefersReducedMotion.value || !inView || document.hidden) {
      stopLoop();
      renderFrame(performance.now());
      // 离屏/隐藏时同样把 will-change 交还浏览器（否则这块合成层会一直挂着）。
      root.value?.classList.remove("is-live");
      return;
    }
    startLoop();
  }

  function handlePointerMove(event: PointerEvent) {
    const host = root.value;
    if (!host) return;
    const rect = host.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const halfWidth = rect.width / 2;
    const halfHeight = rect.height / 2;
    const nx = (event.clientX - rect.left - halfWidth) / halfWidth;
    const ny = (event.clientY - rect.top - halfHeight) / halfHeight;
    pointerX = Math.max(-1, Math.min(1, nx));
    pointerY = Math.max(-1, Math.min(1, ny));
    hovering = true;
    markInteraction();
  }

  function handlePointerLeave() {
    pointerX = 0;
    pointerY = 0;
    hovering = false;
    pressTarget = 0;
  }

  /** 交互探针：记录时间并确保 will-change 只在真正要动的时候挂着。 */
  function markInteraction() {
    lastInteractionAt = performance.now();
    startLoop();
  }

  function handlePointerDown() {
    pressTarget = 1;
    markInteraction();
  }

  function handlePointerUp() {
    pressTarget = 0;
    markInteraction();
    burst();
  }

  function handlePointerCancel() {
    pressTarget = 0;
  }

  function handleVisibility() {
    syncMotion();
  }

  watch(
    prefersReducedMotion,
    (reduced) => {
      if (reduced) {
        stopTextTimers();
        writeText(options.text());
        burstPhase = 0;
        toggleHalo(false);
      }
      syncMotion();
    },
    { flush: "sync" },
  );

  onMounted(() => {
    captureNodes();
    measureUnitScale();
    const host = root.value;
    host?.addEventListener("pointermove", handlePointerMove);
    host?.addEventListener("pointerleave", handlePointerLeave);
    host?.addEventListener("pointerdown", handlePointerDown);
    host?.addEventListener("pointerup", handlePointerUp);
    host?.addEventListener("pointercancel", handlePointerCancel);
    document.addEventListener("visibilitychange", handleVisibility);
    // 标题字号是 clamp(vw)，窗口宽度一变幅度就得跟着重算（见 REFERENCE_FONT_SIZE_PX）。
    window.addEventListener("resize", measureUnitScale, { passive: true });

    // 离屏停帧：首页首屏只占 62vh，往下滑就会离开视口。
    if (host && typeof IntersectionObserver !== "undefined") {
      observer = new IntersectionObserver(
        ([entry]) => {
          inView = entry?.isIntersecting ?? true;
          syncMotion();
        },
        { threshold: 0.05 },
      );
      observer.observe(host);
    }

    syncMotion();
  });

  onBeforeUnmount(() => {
    stopLoop();
    stopTextTimers();
    window.clearTimeout(burstTimer);
    observer?.disconnect();
    observer = null;
    const host = root.value;
    host?.removeEventListener("pointermove", handlePointerMove);
    host?.removeEventListener("pointerleave", handlePointerLeave);
    host?.removeEventListener("pointerdown", handlePointerDown);
    host?.removeEventListener("pointerup", handlePointerUp);
    host?.removeEventListener("pointercancel", handlePointerCancel);
    window.removeEventListener("resize", measureUnitScale);
    document.removeEventListener("visibilitychange", handleVisibility);
  });

  return { burst };
}


