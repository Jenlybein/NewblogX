/**
 * 首屏「指针墨流」（ink fluid）的 WebGL 底座。
 *
 * 参考 `WheatXie/uiarc-pro-reverse` 的 `ink-fluid`：GPU 上跑一小步 Stable Fluids
 * （splat → curl → vorticity → divergence → pressure → gradient → advect），
 * 鼠标划过时把速度与染料一起推出去，墨迹在深墨底上晕开、慢慢褪色。
 *
 * 这里只放**纯数据与无状态辅助函数**：shader 源、render target 的创建/释放、
 * 颜色解析与程序编译。生命周期（RAF、pointer、resize、reduce-motion）在
 * `composables/useInkFluid.ts`，组件只负责挂画布。
 *
 * 首屏永远是深墨面，所以颜色不跟主题的 `--color-ink` 走：主墨读
 * `--color-on-inverse`（反白），次级反白读 `--color-on-inverse-muted`。
 * 两枚都是中性 token，不引入第五个色相。两者都经 canvas 解析成 0..1 通道，
 * 不在这里写死任何颜色值。
 */

/** 全屏三角形顶点着色器：所有 pass 共用。 */
export const INK_FLUID_VERTEX = `#version 300 es
in vec2 aPos; uniform vec2 uTexel;
out vec2 vUv; out vec2 vL; out vec2 vR; out vec2 vT; out vec2 vB;
void main() {
  vUv = aPos * 0.5 + 0.5;
  vL = vUv - vec2(uTexel.x, 0.0); vR = vUv + vec2(uTexel.x, 0.0);
  vT = vUv + vec2(0.0, uTexel.y); vB = vUv - vec2(0.0, uTexel.y);
  gl_Position = vec4(aPos, 0.0, 1.0);
}`;

/** 每个 fragment pass 共用的精度声明、varying 与唯一输出。 */
const FRAG = `#version 300 es
precision highp float; precision highp sampler2D;
in vec2 vUv; in vec2 vL; in vec2 vR; in vec2 vT; in vec2 vB; out vec4 o;
`;

export const INK_FLUID_SHADERS = {
  splat: `${FRAG}uniform sampler2D uTarget; uniform float uAspect; uniform vec3 uColor; uniform vec2 uPoint; uniform float uRadius;
void main() { vec2 p = vUv - uPoint; p.x *= uAspect; vec3 s = exp(-dot(p, p) / uRadius) * uColor; o = vec4(texture(uTarget, vUv).xyz + s, 1.0); }`,
  advect: `${FRAG}uniform sampler2D uVelocity; uniform sampler2D uSource; uniform vec2 uVelTexel; uniform float uDt; uniform float uDissipation; uniform float uTintFade;
void main() {
  vec2 coord = vUv - uDt * texture(uVelocity, vUv).xy * uVelTexel;
  vec4 c = texture(uSource, coord) / (1.0 + uDissipation * uDt);
  // 次级反白随扩散逐渐交还给主墨：新笔迹略亮，旧的淡成同一种灰。
  float moved = c.g * uTintFade * uDt;
  o = vec4(c.r + moved * 0.85, c.g - moved, c.b, c.a);
}`,
  divergence: `${FRAG}uniform sampler2D uVelocity;
void main() {
  float L = texture(uVelocity, vL).x, R = texture(uVelocity, vR).x, T = texture(uVelocity, vT).y, B = texture(uVelocity, vB).y;
  vec2 C = texture(uVelocity, vUv).xy;
  if (vL.x < 0.0) L = -C.x; if (vR.x > 1.0) R = -C.x; if (vT.y > 1.0) T = -C.y; if (vB.y < 0.0) B = -C.y;
  o = vec4(0.5 * (R - L + T - B), 0.0, 0.0, 1.0);
}`,
  curl: `${FRAG}uniform sampler2D uVelocity;
void main() { o = vec4(0.5 * (texture(uVelocity, vR).y - texture(uVelocity, vL).y - texture(uVelocity, vT).x + texture(uVelocity, vB).x), 0.0, 0.0, 1.0); }`,
  vorticity: `${FRAG}uniform sampler2D uVelocity; uniform sampler2D uCurl; uniform float uCurlStrength; uniform float uDt;
void main() {
  float L = texture(uCurl, vL).x, R = texture(uCurl, vR).x, T = texture(uCurl, vT).x, B = texture(uCurl, vB).x, C = texture(uCurl, vUv).x;
  vec2 f = 0.5 * vec2(abs(T) - abs(B), abs(R) - abs(L));
  f /= length(f) + 0.0001; f *= uCurlStrength * C; f.y *= -1.0;
  vec2 v = texture(uVelocity, vUv).xy + f * uDt;
  o = vec4(clamp(v, -1000.0, 1000.0), 0.0, 1.0);
}`,
  pressure: `${FRAG}uniform sampler2D uPressure; uniform sampler2D uDivergence;
void main() {
  float L = texture(uPressure, vL).x, R = texture(uPressure, vR).x, T = texture(uPressure, vT).x, B = texture(uPressure, vB).x;
  o = vec4((L + R + B + T - texture(uDivergence, vUv).x) * 0.25, 0.0, 0.0, 1.0);
}`,
  gradient: `${FRAG}uniform sampler2D uPressure; uniform sampler2D uVelocity;
void main() {
  float L = texture(uPressure, vL).x, R = texture(uPressure, vR).x, T = texture(uPressure, vT).x, B = texture(uPressure, vB).x;
  o = vec4(texture(uVelocity, vUv).xy - vec2(R - L, T - B), 0.0, 1.0);
}`,
  scale: `${FRAG}uniform sampler2D uTexture; uniform float uValue; void main() { o = uValue * texture(uTexture, vUv); }`,
  display: `${FRAG}uniform sampler2D uDye; uniform vec3 uInk; uniform vec3 uAccent; uniform float uStrength; uniform float uShade; uniform vec2 uRes;
float hash(vec2 p) { return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453); }
void main() {
  vec3 d = texture(uDye, vUv).rgb;
  float L = dot(texture(uDye, vL).rg, vec2(1.0)), R = dot(texture(uDye, vR).rg, vec2(1.0));
  float T = dot(texture(uDye, vT).rg, vec2(1.0)), B = dot(texture(uDye, vB).rg, vec2(1.0));
  vec3 n = normalize(vec3(R - L, T - B, 0.35));
  float light = clamp(dot(n, normalize(vec3(-0.4, 0.5, 0.75))), 0.0, 1.0);
  float an = (1.0 - exp(-max(d.r, 0.0) * 1.3)) * uStrength;
  // 次级反白比主墨略重一点，新笔迹先亮一档再褪回主墨。
  float aa = min(1.0, (1.0 - exp(-max(d.g, 0.0) * 1.3)) * uStrength * 1.8);
  float a = clamp(an + aa * (1.0 - an), 0.0, 1.0);
  vec3 col = (uInk * an + uAccent * aa) / max(an + aa, 0.0001);
  col = mix(col, col * (0.82 + 0.3 * light), uShade);
  a += (hash(gl_FragCoord.xy) - 0.5) / 255.0;
  a = clamp(a, 0.0, 1.0);
  o = vec4(col * a, a);
}`,
} as const;

export type InkFluidQuality = "low" | "medium" | "high";

export const INK_FLUID_GRID: Record<InkFluidQuality, number> = { low: 96, medium: 128, high: 192 };
export const INK_FLUID_DYE: Record<InkFluidQuality, number> = { low: 256, medium: 512, high: 768 };

export type InkRgb = [number, number, number];

export interface InkTarget {
  tex: WebGLTexture;
  fbo: WebGLFramebuffer;
  w: number;
  h: number;
}

export interface InkSwap {
  read: InkTarget;
  write: InkTarget;
  w: number;
  h: number;
  swap: () => void;
}

export interface InkProgram {
  p: WebGLProgram;
  u: (name: string) => WebGLUniformLocation | null;
}

export type InkPrograms = Record<keyof typeof INK_FLUID_SHADERS, InkProgram>;

/** 一张渲染纹理 + framebuffer，清成不透明白（和参考组件一致）。 */
export function makeInkTarget(
  gl: WebGL2RenderingContext,
  w: number,
  h: number,
  internal: number,
  format: number,
): InkTarget {
  const tex = gl.createTexture();
  const fbo = gl.createFramebuffer();
  if (!tex || !fbo) throw new Error("ink-fluid: target");
  gl.bindTexture(gl.TEXTURE_2D, tex);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  gl.texImage2D(gl.TEXTURE_2D, 0, internal, w, h, 0, format, gl.HALF_FLOAT, null);
  gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
  gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0);
  gl.viewport(0, 0, w, h);
  gl.clearColor(0, 0, 0, 1);
  gl.clear(gl.COLOR_BUFFER_BIT);
  return { tex, fbo, w, h };
}

/** 两张 target 来回 ping-pong，每一步模拟只写其中一张。 */
export function makeInkSwap(
  gl: WebGL2RenderingContext,
  w: number,
  h: number,
  internal: number,
  format: number,
): InkSwap {
  const pair: InkSwap = {
    read: makeInkTarget(gl, w, h, internal, format),
    write: makeInkTarget(gl, w, h, internal, format),
    w,
    h,
    swap: () => {},
  };
  pair.swap = () => {
    const next = pair.read;
    pair.read = pair.write;
    pair.write = next;
  };
  return pair;
}

export function releaseInkTargets(gl: WebGL2RenderingContext, ...targets: InkTarget[]): void {
  for (const target of targets) {
    gl.deleteTexture(target.tex);
    gl.deleteFramebuffer(target.fbo);
  }
}

/**
 * 让 2D canvas 解析一段 CSS 颜色，返回 0..1 通道。
 *
 * 不自己解析十六进制 / rgb：token 可能是三位十六进制、`rgb(...)` 或未来任何 color() 写法，
 * 交给浏览器最稳；解析失败就退回调用方给的保底色。
 */
export function readInkColor(css: string, fallback: InkRgb): InkRgb {
  if (!css) return fallback;
  const canvas = document.createElement("canvas");
  canvas.width = 1;
  canvas.height = 1;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) return fallback;
  ctx.fillStyle = css;
  ctx.fillRect(0, 0, 1, 1);
  const data = ctx.getImageData(0, 0, 1, 1).data;
  const [r, g, b] = [data[0] ?? 0, data[1] ?? 0, data[2] ?? 0];
  return [r / 255, g / 255, b / 255];
}

export function compileInkProgram(gl: WebGL2RenderingContext, fragment: string): InkProgram {
  const shader = (type: number, source: string): WebGLShader => {
    const handle = gl.createShader(type);
    if (!handle) throw new Error("ink-fluid: shader");
    gl.shaderSource(handle, source);
    gl.compileShader(handle);
    if (!gl.getShaderParameter(handle, gl.COMPILE_STATUS)) {
      const log = gl.getShaderInfoLog(handle) ?? "shader compile";
      gl.deleteShader(handle);
      throw new Error(log);
    }
    return handle;
  };
  const program = gl.createProgram();
  if (!program) throw new Error("ink-fluid: program");
  const vertex = shader(gl.VERTEX_SHADER, INK_FLUID_VERTEX);
  const pixel = shader(gl.FRAGMENT_SHADER, fragment);
  gl.attachShader(program, vertex);
  gl.attachShader(program, pixel);
  gl.bindAttribLocation(program, 0, "aPos");
  gl.linkProgram(program);
  gl.deleteShader(vertex);
  gl.deleteShader(pixel);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    const log = gl.getProgramInfoLog(program) ?? "program link";
    gl.deleteProgram(program);
    throw new Error(log);
  }
  const locations = new Map<string, WebGLUniformLocation | null>();
  return {
    p: program,
    u: (name) => {
      if (!locations.has(name)) locations.set(name, gl.getUniformLocation(program, name));
      return locations.get(name) ?? null;
    },
  };
}
