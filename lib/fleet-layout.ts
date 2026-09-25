// Keep these dimensions in sync with the compact grid in globals.css.
export function fleetCapacity(width: number, height: number): number {
  const columns = Math.max(1, Math.floor((width + 12) / (240 + 12)));
  const rows = Math.max(1, Math.floor((height + 12) / (204 + 12)));
  return columns * rows;
}
