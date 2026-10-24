// 按环境 profile 启动 Nuxt（读者站）
// 用法：node scripts/run-env.mjs <profile> [nuxt-command] [...args]
// profile 对应 env/<profile>.env，具体读取逻辑在 nuxt.config.ts 里。
import { spawn } from "node:child_process";

const [profile, command = "dev", ...args] = process.argv.slice(2);

if (!profile) {
  console.error("Usage: node scripts/run-env.mjs <profile> [nuxt-command] [...args]");
  process.exit(1);
}

const runner = process.platform === "win32" ? "pnpm.cmd" : "pnpm";
const nodeOptions = [
  process.env.NODE_OPTIONS,
  // Nuxt/Nitro 4.4.x 打包的错误模板会抛 DEP0155 上游告警。
  // 只在 build 时抑制，dev 仍保留告警信号。
  command === "build" ? "--disable-warning=DEP0155" : "",
]
  .filter(Boolean)
  .join(" ");

const child = spawn(runner, ["exec", "nuxt", command, ...args], {
  cwd: process.cwd(),
  stdio: "inherit",
  shell: process.platform === "win32",
  env: {
    ...process.env,
    BLOGX_WEB_ENV_PROFILE: profile,
    ...(nodeOptions ? { NODE_OPTIONS: nodeOptions } : {}),
  },
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 0);
});
