import { NextRequest, NextResponse } from "next/server";
import { PIN_COOKIE, pinMatches, pinRequired, signPin } from "@/lib/auth";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function POST(req: NextRequest) {
  if (!pinRequired()) {
    return NextResponse.json({ ok: true, unlocked: true });
  }
  const body = (await req.json().catch(() => ({}))) as { pin?: string };
  if (!body.pin || !pinMatches(body.pin)) {
    return NextResponse.json({ ok: false, error: "Wrong PIN" }, { status: 401 });
  }
  const res = NextResponse.json({ ok: true, unlocked: true });
  res.cookies.set(PIN_COOKIE, signPin(body.pin), {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 14,
  });
  return res;
}
