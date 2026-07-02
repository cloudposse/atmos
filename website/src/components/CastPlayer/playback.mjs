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
