import { NextRequest, NextResponse } from "next/server";
import { decideJoin, getEnrollInfo, getJoin, publicJoinView } from "@/lib/hub";
import { isUnlocked, pinRequired } from "@/lib/auth";
import { getStore } from "@/lib/store";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

type Ctx = { params: Promise<{ id: string }> };

export async function GET(_req: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  const join = await getJoin(id);
  if (!join) {
    return NextResponse.json({ id, status: "expired" });
  }
  const store = await getStore();
  const host = _req.headers.get("x-forwarded-host") || _req.headers.get("host");
  const info = await getEnrollInfo(host);
  return NextResponse.json(
    publicJoinView(join, {
      token: join.status === "approved" ? store.fleetToken : undefined,
      server: join.status === "approved" ? info.serverUrl : undefined,
    })
  );
}

export async function POST(req: NextRequest, ctx: Ctx) {
  if (pinRequired() && !(await isUnlocked())) {
    return NextResponse.json({ error: "PIN required" }, { status: 401 });
  }
  const { id } = await ctx.params;
  const body = (await req.json().catch(() => ({}))) as { action?: string };
  if (body.action !== "approve" && body.action !== "deny") {
    return NextResponse.json({ error: "action must be approve or deny" }, { status: 400 });
  }
  try {
    const join = await decideJoin(id, body.action);
    return NextResponse.json({ ok: true, join: publicJoinView(join) });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : "Could not update join" },
      { status: 400 }
    );
  }
}
