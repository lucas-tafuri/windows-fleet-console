import { NextRequest, NextResponse } from "next/server";
import { addCatalogApp, removeCatalogApp } from "@/lib/hub";
import { isUnlocked, pinRequired } from "@/lib/auth";
import { getStore } from "@/lib/store";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET() {
  if (pinRequired() && !(await isUnlocked())) {
    return NextResponse.json({ error: "PIN required" }, { status: 401 });
  }
  const store = await getStore();
  return NextResponse.json({
    software: store.software || [],
    softwareStatus: store.softwareStatus || {},
  });
}

export async function POST(req: NextRequest) {
  if (pinRequired() && !(await isUnlocked())) {
    return NextResponse.json({ error: "PIN required" }, { status: 401 });
  }
  const body = (await req.json().catch(() => ({}))) as {
    name?: string;
    match?: string;
    wingetId?: string;
  };
  try {
    const item = await addCatalogApp({
      name: body.name || "",
      match: body.match,
      wingetId: body.wingetId,
    });
    return NextResponse.json({ item });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : "Could not add software" },
      { status: 400 }
    );
  }
}

export async function DELETE(req: NextRequest) {
  if (pinRequired() && !(await isUnlocked())) {
    return NextResponse.json({ error: "PIN required" }, { status: 401 });
  }
  const body = (await req.json().catch(() => ({}))) as { id?: string };
  if (!body.id) {
    return NextResponse.json({ error: "id required" }, { status: 400 });
  }
  await removeCatalogApp(body.id);
  return NextResponse.json({ ok: true });
}
