import assert from "node:assert/strict";
import test from "node:test";
import { brailleDots } from "./braille.mjs";

test("Braille follows Unicode bit ordering, including the bottom two dots", () => {
  const positions = [
    [0.5, 0.5],
    [0.5, 1.5],
    [0.5, 2.5],
    [1.5, 0.5],
    [1.5, 1.5],
    [1.5, 2.5],
    [0.5, 3.5],
    [1.5, 3.5],
  ];
  positions.forEach((position, bit) =>
    assert.deepEqual(brailleDots(String.fromCodePoint(0x2800 + (1 << bit))), [
      position,
    ]),
  );
  assert.deepEqual(brailleDots("⠀"), []);
  assert.deepEqual(brailleDots("⣿"), positions);
  assert.equal(brailleDots("x"), null);
  assert.equal(brailleDots(""), null);
});

test("every shared Dot spinner frame has seven dots inside one cell", () => {
  for (const frame of "⣾⣽⣻⢿⡿⣟⣯⣷") assert.equal(brailleDots(frame).length, 7);
});
