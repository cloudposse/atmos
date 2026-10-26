// Runs a clipboard write and reports the outcome as a single result, so a
// caller can never end up with "copied" and "failed" both true at once.
export async function performCopy(writeText, text) {
  try {
    await writeText(text);
    return 'copied';
  } catch {
    return 'failed';
  }
}
