import { createServer } from "http";
import { parse } from "url";
import next from "next";
import { WebSocketServer } from "ws";

const port = parseInt(process.env.PORT || "43123", 10);
const hostname = process.env.HOST || "0.0.0.0";
const dev = process.env.NODE_ENV !== "production";

const app = next({ dev, hostname, port });
const handle = app.getRequestHandler();

function origin() {
  return `http://127.0.0.1:${port}`;
}

await app.prepare();

const server = createServer((req, res) => {
  const parsed = parse(req.url || "/", true);
  handle(req, res, parsed);
});

let upgradeHandler = null;
if (typeof app.getUpgradeHandler === "function") {
  upgradeHandler = app.getUpgradeHandler();
}

const wss = new WebSocketServer({ noServer: true });
const uiClients = new Set();

wss.on("connection", (ws, req) => {
  const { pathname } = parse(req.url || "/", true);
  if (pathname === "/api/ui/ws") {
    ws._fleetCookie = req.headers.cookie;
    uiClients.add(ws);
    ws.on("close", () => uiClients.delete(ws));
    pushSnapshot(ws, req.headers.cookie).catch(() => {});
    return;
  }

  if (pathname === "/api/agent/ws") {
    ws.on("message", async (raw) => {
      let msg;
      try {
        msg = JSON.parse(String(raw));
      } catch {
        ws.send(JSON.stringify({ ok: false, error: "Invalid JSON" }));
        return;
      }
      try {
        const res = await fetch(`${origin()}/api/agent/poll`, {
          method: "POST",
          headers: {
            "content-type": "application/json",
            "x-agent-transport": "ws",
          },
          body: JSON.stringify(msg),
        });
        const json = await res.json();
        ws.send(JSON.stringify(json));
      } catch (err) {
        ws.send(
          JSON.stringify({
            ok: false,
            error: err instanceof Error ? err.message : "poll failed",
          })
        );
      }
    });
  }
});

server.on("upgrade", (req, socket, head) => {
  const { pathname } = parse(req.url || "/", true);
  if (pathname === "/api/agent/ws" || pathname === "/api/ui/ws") {
    wss.handleUpgrade(req, socket, head, (ws) => {
      wss.emit("connection", ws, req);
    });
    return;
  }
  if (upgradeHandler) {
    upgradeHandler(req, socket, head);
    return;
  }
  socket.destroy();
});

async function pushSnapshot(ws, cookie) {
  const res = await fetch(`${origin()}/api/fleet`, {
    headers: cookie ? { cookie } : {},
  });
  const json = await res.json();
  if (ws.readyState === 1) ws.send(JSON.stringify(json));
}

setInterval(() => {
  for (const ws of uiClients) {
    const cookie = ws._fleetCookie;
    pushSnapshot(ws, cookie).catch(() => {});
  }
}, 2000);

server.listen(port, hostname, () => {
  console.log(`Fleet Console on http://${hostname}:${port}`);
});
