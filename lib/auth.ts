import { createHmac, timingSafeEqual } from "crypto";
import { cookies } from "next/headers";

export const PIN_COOKIE = "fleet_unlock";

export function dashboardPin(): string | undefined {
  const pin = process.env.DASHBOARD_PIN?.trim();
  return pin || undefined;
}

export function pinRequired(): boolean {
  return Boolean(dashboardPin());
}

export function signPin(pin: string): string {
  const secret = process.env.FLEET_TOKEN || "fleet-local-dev";
  return createHmac("sha256", secret).update(pin).digest("hex");
}

export function pinMatches(pin: string): boolean {
  const expected = dashboardPin();
  if (!expected) return true;
  const a = Buffer.from(pin);
  const b = Buffer.from(expected);
  if (a.length !== b.length) return false;
  return timingSafeEqual(a, b);
}

export async function isUnlocked(): Promise<boolean> {
  const expected = dashboardPin();
  if (!expected) return true;
  const jar = await cookies();
  const value = jar.get(PIN_COOKIE)?.value;
  if (!value) return false;
  const want = signPin(expected);
  const a = Buffer.from(value);
  const b = Buffer.from(want);
  if (a.length !== b.length) return false;
  return timingSafeEqual(a, b);
}

export function agentTokenOk(token: string | undefined, fleetToken: string): boolean {
  if (!token) return false;
  const a = Buffer.from(token);
  const b = Buffer.from(fleetToken);
  if (a.length !== b.length) return false;
  return timingSafeEqual(a, b);
}
