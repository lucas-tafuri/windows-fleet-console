import { NextResponse } from "next/server";
import { getSnapshot } from "@/lib/hub";
import { isUnlocked } from "@/lib/auth";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET() {
  const unlocked = await isUnlocked();
  const snapshot = await getSnapshot({ unlocked });
  if (snapshot.pinRequired && !unlocked) {
    return NextResponse.json({
      machines: [],
      jobs: [],
      software: [],
      softwareStatus: {},
      demoActive: snapshot.demoActive,
      pinRequired: true,
      unlocked: false,
      unprotected: false,
      serverTime: snapshot.serverTime,
    });
  }
  return NextResponse.json(snapshot);
}
