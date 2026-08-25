import { NextRequest, NextResponse } from "next/server";
import { getEnrollInfo } from "@/lib/hub";
import { isUnlocked, pinRequired } from "@/lib/auth";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET(req: NextRequest) {
  if (pinRequired() && !(await isUnlocked())) {
    return NextResponse.json({ error: "PIN required" }, { status: 401 });
  }
  const host = req.headers.get("x-forwarded-host") || req.headers.get("host");
  const info = await getEnrollInfo(host);
  return NextResponse.json(info);
}
