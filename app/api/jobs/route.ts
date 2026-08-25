import { NextRequest, NextResponse } from "next/server";
import { createJob } from "@/lib/hub";
import { isUnlocked, pinRequired } from "@/lib/auth";
import { getStore } from "@/lib/store";
import type { JobKind, JobPayload } from "@/lib/types";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const KINDS = new Set<JobKind>([
  "install",
  "uninstall",
  "check",
  "map_drive",
  "unmap_drive",
  "clean_downloads",
  "empty_recycle",
  "launch",
  "self_update",
]);

export async function GET() {
  if (pinRequired() && !(await isUnlocked())) {
    return NextResponse.json({ error: "PIN required" }, { status: 401 });
  }
  const store = await getStore();
  return NextResponse.json({ jobs: store.jobs.slice(-80).reverse() });
}

export async function POST(req: NextRequest) {
  if (pinRequired() && !(await isUnlocked())) {
    return NextResponse.json({ error: "PIN required" }, { status: 401 });
  }
  const body = (await req.json().catch(() => ({}))) as {
    kind?: JobKind;
    payload?: JobPayload;
    machineIds?: string[];
  };
  if (!body.kind || !KINDS.has(body.kind)) {
    return NextResponse.json({ error: "Unknown job kind" }, { status: 400 });
  }
  if (!body.machineIds?.length) {
    return NextResponse.json({ error: "Select at least one machine" }, { status: 400 });
  }
  try {
    const job = await createJob({
      kind: body.kind,
      payload: body.payload || {},
      machineIds: body.machineIds,
    });
    return NextResponse.json({ job });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : "Failed to create job" },
      { status: 400 }
    );
  }
}
