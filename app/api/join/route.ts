import { NextRequest, NextResponse } from "next/server";
import { createJoinRequest, publicJoinView } from "@/lib/hub";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

function clientIp(req: NextRequest) {
  const forwarded = req.headers.get("x-forwarded-for");
  if (forwarded) return forwarded.split(",")[0]?.trim() || "";
  return req.headers.get("x-real-ip") || "";
}

export async function POST(req: NextRequest) {
  const body = (await req.json().catch(() => ({}))) as {
    hostname?: string;
    user?: string;
    os?: string;
  };
  try {
    const join = await createJoinRequest({
      hostname: body.hostname || "",
      user: body.user || "",
      os: body.os || "Windows",
      ip: clientIp(req),
    });
    return NextResponse.json(publicJoinView(join));
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : "Join failed" },
      { status: 400 }
    );
  }
}
