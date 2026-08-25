import type { JobKind } from "./types";

export const JOB_LABEL: Record<JobKind, string> = {
  install: "Install",
  uninstall: "Uninstall",
  check: "Check installed",
  map_drive: "Map drive",
  unmap_drive: "Unmap drive",
  clean_downloads: "Clean Downloads",
  empty_recycle: "Empty Recycle Bin",
  launch: "Launch",
  self_update: "Update & restart",
};
