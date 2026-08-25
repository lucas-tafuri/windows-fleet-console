import { createServer } from "http";
import { parse } from "url";
import dgram from "dgram";
import os from "os";
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
  console.log(`PrettyDamnFleet on http://${hostname}:${port}`);
  startDiscovery();
});

function lanIPv4() {
  const out = [];
  for (const list of Object.values(os.networkInterfaces())) {
    for (const a of list || []) {
      const v4 = a.family === "IPv4" || a.family === 4;
      if (v4 && !a.internal && a.address) out.push(a.address);
    }
  }
  return out;
}

function same24(a, b) {
  const pa = String(a).split(".");
  const pb = String(b).split(".");
  return pa.length === 4 && pb.length === 4 && pa[0] === pb[0] && pa[1] === pb[1] && pa[2] === pb[2];
}

function discoverUrl(remote) {
  const pub = process.env.FLEET_PUBLIC_URL?.replace(/\/$/, "");
  if (pub) return pub;
  const addrs = lanIPv4();
  const match = addrs.find((ip) => same24(ip, remote)) || addrs[0];
  return `http://${match || "127.0.0.1"}:${port}`;
}

function startDiscovery() {
  const discPort = parseInt(process.env.FLEET_DISCOVER_PORT || "43124", 10);
  const sock = dgram.createSocket({ type: "udp4", reuseAddr: true });
  sock.on("error", (err) => {
    console.error("discover:", err.message);
  });
  sock.on("message", (msg, rinfo) => {
    const text = String(msg).trim();
    if (text !== "FLEETDISC") return;
    const reply = Buffer.from("FLEETHTTP " + discoverUrl(rinfo.address));
    sock.send(reply, rinfo.port, rinfo.address);
  });
  sock.bind(discPort, hostname, () => {
    try {
      sock.setBroadcast(true);
    } catch {
      /* ignore */
    }
    console.log(`Fleet discovery on udp/${discPort}`);
  });
}
