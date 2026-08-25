import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET() {
  return NextResponse.json({
    ok: true,
    service: "prettydamnfleet",
    port: parseInt(process.env.PORT || "43123", 10),
  });
}
