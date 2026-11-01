// 自检专用：把 fixture/ 目录当成一个小站点跑起来（只在测试/自检里用，不用于生产测量）。
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const FIXTURE_ROOT = fileURLToPath(new URL("../fixture/", import.meta.url));
const MIME = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".woff2": "font/woff2",
};

export async function startFixtureSite({ host = "127.0.0.1", port = 0 } = {}) {
  const server = createServer(async (req, res) => {
    const requestUrl = new URL(req.url || "/", `http://${host}`);
    if (requestUrl.pathname === "/api/fixture") {
      res.writeHead(200, {
        "content-type": MIME[".json"],
        "cache-control": "public, max-age=60",
      });
      res.end(JSON.stringify({ fixture: true }));
      return;
    }
    const relative = fixturePathOf(requestUrl.pathname);
    try {
      const body = await readFile(path.join(FIXTURE_ROOT, relative));
      const extension = path.extname(relative).toLowerCase();
      res.writeHead(200, {
        "content-type": MIME[extension] || "application/octet-stream",
        "cache-control": "public, max-age=60",
      });
      res.end(body);
    } catch {
      res.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
      res.end("not found");
    }
  });

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, host, resolve);
  });
  const address = server.address();
  return {
    base: `http://${host}:${address.port}`,
    close: () => new Promise((resolve, reject) => server.close(error => (error ? reject(error) : resolve()))),
  };
}

function fixturePathOf(pathname) {
  if (pathname === "/" || pathname === "") return "index.html";
  if (pathname.startsWith("/article/")) return "article.html";
  return pathname.replace(/^[/\\]+/, "").replace(/\.\./g, "");
}
