export const DEFAULT_MAX_CAST_SPEED = 0.6;
export const MIN_CAST_SPEED = 0.1;

export function parseCast(text) {
  const rows = text.trim().split("\n");
  let header;
  try {
    header = JSON.parse(rows[0] || "{}");
  } catch {
    throw new Error("Malformed cast header");
  }
  let absoluteTime = 0;
  const events = rows.slice(1).reduce((parsed, line) => {
    if (!line.trim() || line.startsWith("#")) return parsed;
    let event;
    try {
      event = JSON.parse(line);
    } catch {
      throw new Error("Malformed cast event");
    }
    if (!Array.isArray(event) || event.length !== 3) return parsed;
    const stream = event[1];
    const time = Number(event[0]);
    if (!Number.isFinite(time)) return parsed;
    const eventTime = header.version === 3 ? absoluteTime + time : time;
    absoluteTime = header.version === 3 ? eventTime : absoluteTime;
    if (stream !== "o" && stream !== "e") return parsed;
    const data = typeof event[2] === "string" ? event[2] : "";
    if (data === "") return parsed;
    parsed.push([eventTime, stream, data]);
    return parsed;
  }, []);
  return { header, events };
}

export function resolvePlaybackSpeed(
  speed = DEFAULT_MAX_CAST_SPEED,
  maxScrollRate = DEFAULT_MAX_CAST_SPEED,
) {
  const requestedSpeed = Number.isFinite(speed) ? speed : DEFAULT_MAX_CAST_SPEED;
  const maxSpeed = Number.isFinite(maxScrollRate)
    ? maxScrollRate
    : DEFAULT_MAX_CAST_SPEED;
  return Math.max(MIN_CAST_SPEED, Math.min(requestedSpeed, maxSpeed));
}

export function applyIdleSkip(events, maxGap) {
  if (maxGap <= 0 || events.length === 0) {
    return events;
  }
  let previous = 0;
  let offset = 0;
  return events.map((event, index) => {
    const gap = index === 0 ? event[0] : event[0] - previous;
    if (gap > maxGap) {
      offset += gap - maxGap;
    }
    previous = event[0];
    return [Math.max(0, event[0] - offset), event[1], event[2]];
  });
}
