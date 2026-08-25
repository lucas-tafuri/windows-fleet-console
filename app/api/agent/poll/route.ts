import { NextRequest, NextResponse } from "next/server";
import { handleAgentPoll } from "@/lib/hub";
import type { Heartbeat } from "@/lib/types";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function POST(req: NextRequest) {
  const hb = (await req.json().catch(() => ({}))) as Heartbeat;
  const transport =
    req.headers.get("x-agent-transport") === "ws" ? "ws" : "poll";
  const result = await handleAgentPoll(hb, transport);
  if (!result.ok) {
    return NextResponse.json(result, { status: 401 });
  }
  return NextResponse.json(result);
}
