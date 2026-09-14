// Coordinates in a 2 by 4 terminal cell, in Unicode Braille bit order.
export function brailleDots(character) {
  const code = character.codePointAt(0);
  if (character.length !== 1 || code < 0x2800 || code > 0x28ff) return null;
  const mask = code - 0x2800;
  return [
    [0.5, 0.5],
    [0.5, 1.5],
    [0.5, 2.5],
    [1.5, 0.5],
    [1.5, 1.5],
    [1.5, 2.5],
    [0.5, 3.5],
    [1.5, 3.5],
  ].filter((_, bit) => mask & (1 << bit));
}
