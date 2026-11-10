import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

import {
  INK_FLUID_DYE,
  INK_FLUID_GRID,
  INK_FLUID_SHADERS,
  compileInkProgram,
  makeInkSwap,
  makeInkTarget,
  readInkColor,
  releaseInkTargets,
  type InkFluidQuality,
  type InkPrograms,
  type InkRgb,
  type InkSwap,
  type InkTarget,
} from "~/utils/inkFluid";

export interface InkFluidOptions {
  /** 墨的最终不透明度，0–1。 */
  intensity: number;
  /** 次级反白占比，0–1；其余是主墨。 */
  tint: number;
  /** 快速划动推流体的力度。 */
  force: number;
  /** 指针静止时保留的缓慢墨羽。 */
  ambient: boolean;
  /** 是否跟随指针。 */
  interactive: boolean;
  /** 模拟分辨率；首屏默认 medium。 */
  quality: InkFluidQuality;
}

interface InkFluidController {
  setReduced: (value: boolean) => void;
  dispose: () => void;
}

const NOOP_CONTROLLER: InkFluidController = {
  setReduced: () => {},
  dispose: () => {},
};

/**
 * 把 ink-fluid 的稳定流体模拟挂到一块 canvas 上。
 *
 * 组件只负责 `<canvas>` 与铺满父级的根节点；这里负责 WebGL2 资源、指针采样、
 * RAF 循环、尺寸变化、主题色重读与「减少动态」的落位帧。没有 WebGL2 或浮点
 * color buffer 时只标记 `data-ink-fallback`，不让首屏因为特效失败而报错。
 *
 * 父元素（首屏 `.home-cover`）是指针事件源：画布自己是 `pointer-events: none`，
 * 所以按钮、轮播、文字照常收到点击，墨流只借用冒泡上来的 pointermove。
 */
export function useInkFluid(
  canvas: Ref<HTMLCanvasElement | null>,
  root: Ref<HTMLElement | null>,
  options: () => InkFluidOptions,
  reduced: Ref<boolean>,
) {
  let controller: InkFluidController | null = null;

  onMounted(() => {
    const canvasEl = canvas.value;
    const rootEl = root.value;
    if (!canvasEl || !rootEl) return;
    controller = createInkFluid(canvasEl, rootEl, options, reduced.value);
  });

  watch(reduced, (value) => {
    controller?.setReduced(value);
  });

  onBeforeUnmount(() => {
    controller?.dispose();
    controller = null;
  });
}

function createInkFluid(
  canvas: HTMLCanvasElement,
  root: HTMLElement,
  options: () => InkFluidOptions,
  initialReduced: boolean,
): InkFluidController {
  const gl = canvas.getContext("webgl2", {
    alpha: true,
    premultipliedAlpha: true,
    antialias: false,
    depth: false,
    stencil: false,
    preserveDrawingBuffer: false,
  });
  if (!gl || !gl.getExtension("EXT_color_buffer_float")) {
    root.dataset.inkFallback = "1";
    return NOOP_CONTROLLER;
  }

  let programs: InkPrograms;
  try {
    programs = Object.fromEntries(
      Object.entries(INK_FLUID_SHADERS).map(([name, source]) => [
        name,
        compileInkProgram(gl, source),
      ]),
    ) as InkPrograms;
  } catch {
    root.dataset.inkFallback = "1";
    return NOOP_CONTROLLER;
  }

  const buffer = gl.createBuffer();
  if (!buffer) {
    root.dataset.inkFallback = "1";
    return NOOP_CONTROLLER;
  }
  gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  gl.enableVertexAttribArray(0);
  gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);

  let width = 0;
  let height = 0;
  let built = false;
  let velocity!: InkSwap;
  let dye!: InkSwap;
  let pressure!: InkSwap;
  let divergence!: InkTarget;
  let curl!: InkTarget;

  const resize = () => {
    const rect = root.getBoundingClientRect();
    const ratio = Math.min(window.devicePixelRatio || 1, 1.5);
    width = Math.max(1, Math.round(rect.width * ratio));
    height = Math.max(1, Math.round(rect.height * ratio));
    canvas.width = width;
    canvas.height = height;

    const aspect = width / height;
    const fit = (size: number): [number, number] =>
      aspect >= 1 ? [Math.round(size * aspect), size] : [size, Math.round(size / aspect)];
    const [gridW, gridH] = fit(INK_FLUID_GRID[options().quality]);
    const [dyeW, dyeH] = fit(INK_FLUID_DYE[options().quality]);

    if (built) {
      releaseInkTargets(
        gl,
        velocity.read,
        velocity.write,
        dye.read,
        dye.write,
        pressure.read,
        pressure.write,
        divergence,
        curl,
      );
    }
    velocity = makeInkSwap(gl, gridW, gridH, gl.RG16F, gl.RG);
    dye = makeInkSwap(gl, dyeW, dyeH, gl.RGBA16F, gl.RGBA);
    pressure = makeInkSwap(gl, gridW, gridH, gl.R16F, gl.RED);
    divergence = makeInkTarget(gl, gridW, gridH, gl.R16F, gl.RED);
    curl = makeInkTarget(gl, gridW, gridH, gl.R16F, gl.RED);
    built = true;
  };

  const draw = (target: InkTarget | null) => {
    if (target) {
      gl.viewport(0, 0, target.w, target.h);
      gl.bindFramebuffer(gl.FRAMEBUFFER, target.fbo);
    } else {
      gl.viewport(0, 0, width, height);
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    }
    gl.drawArrays(gl.TRIANGLES, 0, 3);
  };

  const withTexel = (program: InkPrograms[keyof InkPrograms], texel: [number, number]) => {
    gl.useProgram(program.p);
    gl.uniform2f(program.u("uTexel"), texel[0], texel[1]);
    return program;
  };

  const bind = (
    program: InkPrograms[keyof InkPrograms],
    name: string,
    unit: number,
    target: InkTarget,
  ) => {
    gl.activeTexture(gl.TEXTURE0 + unit);
    gl.bindTexture(gl.TEXTURE_2D, target.tex);
    gl.uniform1i(program.u(name), unit);
  };

  /** 一笔：把速度与染料同时从点 (x,y) 向外推。 */
  const splat = (
    x: number,
    y: number,
    dx: number,
    dy: number,
    neutral: number,
    accentShare: number,
    radius: number,
  ) => {
    const program = withTexel(programs.splat, [1 / velocity.w, 1 / velocity.h]);
    gl.uniform1f(program.u("uAspect"), width / height);
    gl.uniform2f(program.u("uPoint"), x, y);
    gl.uniform1f(program.u("uRadius"), radius / 100);

    bind(program, "uTarget", 0, velocity.read);
    gl.uniform3f(program.u("uColor"), dx, dy, 0);
    draw(velocity.write);
    velocity.swap();

    bind(program, "uTarget", 0, dye.read);
    gl.uniform3f(program.u("uColor"), neutral, accentShare, 0);
    draw(dye.write);
    dye.swap();
  };

  /** 一步 Stable Fluids：curl → vorticity → divergence → pressure → gradient → advect。 */
  const step = (dt: number) => {
    const texel: [number, number] = [1 / velocity.w, 1 / velocity.h];

    let program = withTexel(programs.curl, texel);
    bind(program, "uVelocity", 0, velocity.read);
    draw(curl);

    program = withTexel(programs.vorticity, texel);
    bind(program, "uVelocity", 0, velocity.read);
    bind(program, "uCurl", 1, curl);
    gl.uniform1f(program.u("uCurlStrength"), 14);
    gl.uniform1f(program.u("uDt"), dt);
    draw(velocity.write);
    velocity.swap();

    program = withTexel(programs.divergence, texel);
    bind(program, "uVelocity", 0, velocity.read);
    draw(divergence);

    program = withTexel(programs.scale, texel);
    bind(program, "uTexture", 0, pressure.read);
    gl.uniform1f(program.u("uValue"), 0.8);
    draw(pressure.write);
    pressure.swap();

    program = withTexel(programs.pressure, texel);
    bind(program, "uDivergence", 1, divergence);
    for (let iteration = 0; iteration < 20; iteration += 1) {
      bind(program, "uPressure", 0, pressure.read);
      draw(pressure.write);
      pressure.swap();
    }

    program = withTexel(programs.gradient, texel);
    bind(program, "uPressure", 0, pressure.read);
    bind(program, "uVelocity", 1, velocity.read);
    draw(velocity.write);
    velocity.swap();

    program = withTexel(programs.advect, texel);
    gl.uniform2f(program.u("uVelTexel"), texel[0], texel[1]);
    gl.uniform1f(program.u("uDt"), dt);
    bind(program, "uVelocity", 0, velocity.read);
    bind(program, "uSource", 1, velocity.read);
    gl.uniform1f(program.u("uDissipation"), 0.3);
    gl.uniform1f(program.u("uTintFade"), 0);
    draw(velocity.write);
    velocity.swap();

    program = withTexel(programs.advect, [1 / dye.w, 1 / dye.h]);
    gl.uniform2f(program.u("uVelTexel"), texel[0], texel[1]);
    gl.uniform1f(program.u("uDt"), dt);
    bind(program, "uVelocity", 0, velocity.read);
    bind(program, "uSource", 1, dye.read);
    gl.uniform1f(program.u("uDissipation"), 1.1);
    gl.uniform1f(program.u("uTintFade"), 0.22);
    draw(dye.write);
    dye.swap();
  };

  /**
   * 首屏是深墨面：墨流只用两档中性反白（主墨 + 次级反白），不引入第五个色相。
   *
   * 不读 `--color-ink`：亮色主题下它是深石墨，画在深墨底上等于消失。
   * 主题切换只影响两枚 token 的取值，MutationObserver 会在下一帧重读。
   */
  let ink: InkRgb = [1, 1, 1];
  let accent: InkRgb = [1, 1, 1];

  const readTheme = () => {
    const computed = getComputedStyle(document.documentElement);
    ink = readInkColor(computed.getPropertyValue("--color-on-inverse").trim(), ink);
    accent = readInkColor(computed.getPropertyValue("--color-on-inverse-muted").trim(), accent);
  };
  readTheme();

  const show = () => {
    const program = withTexel(programs.display, [1 / dye.w, 1 / dye.h]);
    bind(program, "uDye", 0, dye.read);
    gl.uniform3f(program.u("uInk"), ink[0], ink[1], ink[2]);
    gl.uniform3f(program.u("uAccent"), accent[0], accent[1], accent[2]);
    gl.uniform1f(program.u("uStrength"), Math.min(1, options().intensity * 0.44));
    gl.uniform1f(program.u("uShade"), 0.8);
    gl.uniform2f(program.u("uRes"), width, height);
    draw(null);
  };

  let reducedFlag = initialReduced;
  const pointer = { x: 0, y: 0, dx: 0, dy: 0, moved: false, last: 0, has: false };
  const interaction = root.parentElement ?? root;

  const onMove = (event: PointerEvent) => {
    if (!options().interactive || reducedFlag) return;
    const rect = root.getBoundingClientRect();
    const x = (event.clientX - rect.left) / rect.width;
    const y = 1 - (event.clientY - rect.top) / rect.height;
    if (pointer.has) {
      pointer.dx += (x - pointer.x) * (rect.width / rect.height);
      pointer.dy += y - pointer.y;
    }
    pointer.x = x;
    pointer.y = y;
    pointer.has = true;
    pointer.moved = true;
    pointer.last = performance.now();
    // 默认不跑 ambient 时，指针是唯一的启动信号；静止后 tick 会自己停。
    start();
  };

  const onLeave = () => {
    pointer.has = false;
  };

  interaction.addEventListener("pointermove", onMove, { passive: true });
  interaction.addEventListener("pointerleave", onLeave);
  interaction.addEventListener("pointerdown", onMove);

  let seed = 100 * Math.random();
  let hue = 0;
  let frame = 0;
  let running = false;
  let visible = true;
  let clock = performance.now();

  /** 没人碰时让墨面继续缓慢呼吸的羽流。 */
  const plume = (time: number, dt: number) => {
    const turn = (time / 1000) * 0.16 + seed;
    splat(
      0.5 + 0.34 * Math.sin(1.3 * turn) * Math.cos(0.7 * turn),
      0.5 + 0.28 * Math.sin(0.9 * turn + 1.7),
      60 * Math.cos(1.1 * turn) * dt * 9,
      60 * Math.sin(1.7 * turn) * dt * 9,
      2 * dt * (1 - 0.85 * options().tint),
      2 * dt * options().tint * 0.85,
      0.45,
    );
  };

  const tick = (now: number) => {
    frame = 0;
    const dt = Math.min(0.033, (now - clock) / 1000);
    clock = now;

    if (pointer.moved) {
      const length = Math.hypot(pointer.dx, pointer.dy);
      const push = 5200 * (0.4 + 1.2 * options().force);
      const radius = Math.min(0.32, 5 * length);
      hue = (hue + 3 * length) % 1;
      const share = options().tint;
      splat(
        pointer.x,
        pointer.y,
        pointer.dx * push,
        pointer.dy * push,
        radius * (1 - share),
        radius * share * (0.75 + 0.25 * Math.sin(hue * Math.PI * 2)),
        0.22,
      );
      pointer.dx = 0;
      pointer.dy = 0;
      pointer.moved = false;
    }

    if (options().ambient && now - pointer.last > 1200) plume(now, dt);
    step(dt);
    show();

    // 没有 ambient 时，指针停 2.5s 后就不再占着 RAF；最后一帧留在画布上。
    if (!options().ambient && !pointer.moved && now - pointer.last > 2500) {
      running = false;
      frame = 0;
      return;
    }
    if (running) frame = requestAnimationFrame(tick);
  };

  const start = () => {
    // ambient 关掉时，只有指针移动过才需要跑；首屏不应该无条件占 GPU。
    const wants = options().ambient || pointer.moved;
    const live = visible && !document.hidden && !reducedFlag && wants;
    if (live && !running) {
      running = true;
      clock = performance.now();
      frame = requestAnimationFrame(tick);
    } else if (!live && running) {
      running = false;
      cancelAnimationFrame(frame);
      frame = 0;
    }
  };

  /** 减少动态：固定种子跑一帧静帧，之后不再动。 */
  const still = () => {
    seed = 3.2;
    for (let i = 0; i < 140; i += 1) {
      plume(16.7 * i * 2.4, 1 / 60);
      step(1 / 60);
    }
    show();
  };

  try {
    resize();
  } catch {
    root.dataset.inkFallback = "1";
    return NOOP_CONTROLLER;
  }
  if (reducedFlag) still();
  else start();

  const intersection = new IntersectionObserver((entries) => {
    visible = entries[0]?.isIntersecting ?? true;
    start();
  });
  intersection.observe(root);

  const onVisibility = () => start();
  document.addEventListener("visibilitychange", onVisibility);

  let pending = 0;
  const watcher = new ResizeObserver(() => {
    window.clearTimeout(pending);
    pending = window.setTimeout(() => {
      const rect = root.getBoundingClientRect();
      const ratio = Math.min(window.devicePixelRatio || 1, 1.5);
      if (
        Math.max(1, Math.round(rect.width * ratio)) !== width ||
        Math.max(1, Math.round(rect.height * ratio)) !== height
      ) {
        resize();
        if (reducedFlag) still();
        else show();
      }
    }, 120);
  });
  watcher.observe(root);

  const theme = new MutationObserver(() => {
    readTheme();
    if (!running) show();
  });
  theme.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme", "data-accent", "class"],
  });

  const setReduced = (value: boolean) => {
    reducedFlag = value;
    if (value) {
      if (running) {
        running = false;
        cancelAnimationFrame(frame);
        frame = 0;
      }
      still();
      return;
    }
    start();
  };

  let disposed = false;
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    running = false;
    cancelAnimationFrame(frame);
    intersection.disconnect();
    watcher.disconnect();
    theme.disconnect();
    window.clearTimeout(pending);
    document.removeEventListener("visibilitychange", onVisibility);
    interaction.removeEventListener("pointermove", onMove);
    interaction.removeEventListener("pointerleave", onLeave);
    interaction.removeEventListener("pointerdown", onMove);
    if (built) {
      releaseInkTargets(
        gl,
        velocity.read,
        velocity.write,
        dye.read,
        dye.write,
        pressure.read,
        pressure.write,
        divergence,
        curl,
      );
    }
    Object.values(programs).forEach((program) => gl.deleteProgram(program.p));
    gl.deleteBuffer(buffer);
  };

  return { setReduced, dispose };
}
