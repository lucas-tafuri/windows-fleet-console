export type MachineStatus =
  | "online"
  | "under_load"
  | "frozen"
  | "offline"
  | "limited";

export type Transport = "ws" | "poll" | "demo";

export type MappedDrive = {
  letter: string;
  path: string;
};

export type Machine = {
  id: string;
  hostname: string;
  user: string;
  os: string;
  demo: boolean;
  lastSeen: number;
  cpu: number | null;
  memory: number | null;
  frozenHint: boolean;
  metricsLimited: boolean;
  lastInputAgeMs: number | null;
  mappedDrives: MappedDrive[];
  transport: Transport;
};

export type JobKind =
  | "install"
  | "uninstall"
  | "check"
  | "map_drive"
  | "unmap_drive"
  | "clean_downloads"
  | "empty_recycle"
  | "launch"
  | "self_update";

export type JobResultStatus = "queued" | "running" | "ok" | "error";

export type JobResult = {
  machineId: string;
  hostname: string;
  status: JobResultStatus;
  via?: string;
  message: string;
  output?: string;
  startedAt?: number;
  finishedAt?: number;
};

export type JobPayload = {
  package?: string;
  letter?: string;
  unc?: string;
  username?: string;
  password?: string;
  target?: string;
  args?: string;
  repo?: string;
  branch?: string;
};

export type Job = {
  id: string;
  kind: JobKind;
  createdAt: number;
  payload: JobPayload;
  machineIds: string[];
  results: Record<string, JobResult>;
  status: "queued" | "running" | "done";
};

export type AssignedJob = {
  id: string;
  kind: JobKind;
  payload: JobPayload;
};

export type Heartbeat = {
  token?: string;
  machineId?: string;
  hostname: string;
  user?: string;
  os?: string;
  cpu?: number | null;
  memory?: number | null;
  frozen?: boolean;
  metricsLimited?: boolean;
  lastInputAgeMs?: number | null;
  mappedDrives?: MappedDrive[];
  results?: AgentJobResult[];
};

export type AgentJobResult = {
  jobId: string;
  status: "ok" | "error";
  via?: string;
  message: string;
  output?: string;
};

export type CatalogApp = {
  id: string;
  name: string;
  match: string;
  wingetId?: string;
};

export type SoftwareInstall = {
  installed: boolean;
  lastChecked: number;
  via?: string;
  detail?: string;
};

export type SoftwareStatusMap = Record<string, Record<string, SoftwareInstall>>;

export type MachineView = Machine & {
  status: MachineStatus;
  lastSeenLabel: string;
};

export type JoinStatus = "pending" | "approved" | "denied";

export type JoinRequest = {
  id: string;
  hostname: string;
  user: string;
  os: string;
  ip: string;
  createdAt: number;
  status: JoinStatus;
  decidedAt?: number;
};

export type MapPrefs = {
  letter: string;
  unc: string;
  username: string;
  password: string;
  uncHistory: string[];
};

export type FleetSnapshot = {
  machines: MachineView[];
  jobs: Job[];
  software: CatalogApp[];
  softwareStatus: SoftwareStatusMap;
  pendingJoins: JoinRequest[];
  mapPrefs: MapPrefs;
  demoActive: boolean;
  pinRequired: boolean;
  unlocked: boolean;
  unprotected: boolean;
  serverTime: number;
};

export type StoreData = {
  fleetToken: string;
  machines: Record<string, Machine>;
  jobs: Job[];
  software: CatalogApp[];
  softwareStatus: SoftwareStatusMap;
  joins: JoinRequest[];
  mapPrefs: MapPrefs;
};
