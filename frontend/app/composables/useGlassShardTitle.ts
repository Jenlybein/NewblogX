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

/** 视差阻尼：指针值 → 渲染值，每帧靠拢 10%。 */
const POINTER_LERP = 0.1;
/** 按下/弹开的阻尼：每帧靠拢 15%。 */
const SPRING_LERP = 0.15;

/** 底片 / 浮层的基础位移倍数与旋转角度（原站实测）。 */
const BASE_TRAVEL_Z1 = 15;
const BASE_TRAVEL_Z2 = 40;
const BASE_ROTATE_Z1 = 5;
const BASE_ROTATE_Z2 = 15;

/** 静止时的呼吸抖动幅度（px）与相位差。 */
const IDLE_WOBBLE_PX = 3;
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
    if (prefersReducedMotion.value) return;
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

  function renderFrame(time: number) {
    pressAmount += ((pressTarget === 1 ? 1 : 0) - pressAmount) * SPRING_LERP;
    smoothedX += (pointerX - smoothedX) * POINTER_LERP;
    smoothedY += (pointerY - smoothedY) * POINTER_LERP;

    const seconds = time * 0.001;
    // 「减少动态」下只画一帧：呼吸抖动与悬停放大一起归零，否则静帧会停在一个歪的相位上。
    const idleMotion = prefersReducedMotion.value ? 0 : 1;

    for (const shard of shards) {
      const { def } = shard;
      const layered = def.z === 1;
      const travel = (layered ? BASE_TRAVEL_Z1 : BASE_TRAVEL_Z2) * (1 - pressAmount * PRESS_DAMPING);
      const wobbleX =
        Math.sin(seconds + def.id) * IDLE_WOBBLE_PX * idleMotion * unitScale * (1 - pressAmount * 0.5);
      const wobbleY =
        Math.cos(seconds + def.id * 1.5) * IDLE_WOBBLE_PX * idleMotion * unitScale * (1 - pressAmount * 0.5);

      const parallaxX = smoothedX * travel * unitScale * def.speed * 20 + wobbleX;
      const parallaxY = smoothedY * travel * unitScale * def.speed * 20 + wobbleY;
      const parallaxRy = smoothedX * (layered ? BASE_ROTATE_Z1 : BASE_ROTATE_Z2) * unitScale;
      const parallaxRx = -smoothedY * (layered ? BASE_ROTATE_Z1 : BASE_ROTATE_Z2) * unitScale;

      const exploded = burstPhase === 1;
      const burstTravel = BURST_TRAVEL_PX * unitScale;
      const burstRotate = BURST_ROTATE_DEG * unitScale;
      shard.px += ((exploded ? def.bx * burstTravel : 0) - shard.px) * SPRING_LERP;
      shard.py += ((exploded ? def.by * burstTravel : 0) - shard.py) * SPRING_LERP;
      shard.pz += ((exploded ? def.z * BURST_DEPTH_PX : 0) - shard.pz) * SPRING_LERP;
      shard.prx += ((exploded ? def.by * burstRotate : 0) - shard.prx) * SPRING_LERP;
      shard.pry += ((exploded ? def.bx * burstRotate : 0) - shard.pry) * SPRING_LERP;

      shard.tx = parallaxX + shard.px;
      shard.ty = parallaxY + shard.py;
      shard.rx = parallaxRx + shard.prx;
      shard.ry = parallaxRy + shard.pry;

      const scaleTarget = prefersReducedMotion.value
        ? 1
        : (hovering && !layered ? OVERLAY_HOVER_SCALE : 1) - pressAmount * PRESS_SCALE;
      shard.scale += (scaleTarget - shard.scale) * SPRING_LERP;

      shard.node.style.transform =
        `translate3d(${shard.tx.toFixed(2)}px, ${shard.ty.toFixed(2)}px, ${(def.z * 10 + shard.pz).toFixed(2)}px) ` +
        `rotateX(${shard.rx.toFixed(2)}deg) rotateY(${shard.ry.toFixed(2)}deg) scale(${shard.scale.toFixed(3)})`;
    }
  }

  function loop(time: number) {
    renderFrame(time);
    rafId = window.requestAnimationFrame(loop);
  }

  function startLoop() {
    if (rafId || prefersReducedMotion.value) return;
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
  }

  function handlePointerLeave() {
    pointerX = 0;
    pointerY = 0;
    hovering = false;
    pressTarget = 0;
  }

  function handlePointerDown() {
    pressTarget = 1;
  }

  function handlePointerUp() {
    pressTarget = 0;
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


