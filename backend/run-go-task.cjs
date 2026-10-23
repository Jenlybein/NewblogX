const { spawnSync } = require("node:child_process");
const path = require("node:path");
const os = require("node:os");

const task = process.argv[2];
const extraArgs = process.argv.slice(3).filter((arg, index) => {
  if (index === 0 && arg === "--") {
    return false;
  }
  return true;
});

const commands = {
  dev: ["run", "./cmd/server"],
  build: ["build", "-o", "./bin/server", "./cmd/server"],
  test: ["test", "./..."],
};

if (!commands[task]) {
  console.error(`unsupported go task: ${task}`);
  process.exit(1);
}

const localAppData = process.env.LOCALAPPDATA || process.env.LocalAppData;
const fallbackCacheDir = localAppData
  ? path.join(localAppData, "go-build")
  : path.join(os.tmpdir(), "go-build");

const result = spawnSync("go", [...commands[task], ...extraArgs], {
  stdio: "inherit",
  shell: true,
  env: {
    ...process.env,
    GOCACHE: process.env.GOCACHE || fallbackCacheDir,
  },
});

if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}

process.exit(result.status ?? 0);
