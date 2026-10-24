// 按环境 profile 启动 Nuxt（管理后台）
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
  // 同读者站：仅在 build 时抑制 Nitro 的 DEP0155 上游告警。
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
    BLOGX_ADMIN_ENV_PROFILE: profile,
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
