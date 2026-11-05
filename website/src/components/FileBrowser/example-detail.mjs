/** Collect readable and non-previewable files alike, preserving source tree order. */
export function collectExampleFiles(nodes) {
  return nodes.flatMap((node) =>
    node.type === "file" ? [node] : collectExampleFiles(node.children),
  );
}

/** Paths in the selector and search are relative to this example. */
export function relativeExamplePath(file, name) {
  return file.path.slice(name.length + 1);
}

/** Resolve only files in the published tree; malformed or stale URLs fall back safely. */
export function resolveExampleView(search, files, name) {
  const params = new URLSearchParams(search);
  const selected =
    files.find(
      (file) => relativeExamplePath(file, name) === params.get("file"),
    ) ||
    files.find((file) => file.path === `${name}/atmos.yaml`) ||
    files.find((file) => file.path === `${name}/README.md`) ||
    files[0];
  return {
    tab: params.get("tab") === "files" ? "files" : "overview",
    selected,
  };
}

/** Keep unrelated query parameters when changing views or selecting a file. */
export function exampleViewUrl(route, search, tab, file) {
  const params = new URLSearchParams(search);
  params.delete("tab");
  params.delete("file");
  if (tab === "files") {
    params.set("tab", "files");
    if (file) params.set("file", file);
  }
  const query = params.toString();
  return `${route}${query ? `?${query}` : ""}`;
}

export function filterExampleFiles(files, name, query) {
  const term = query.trim().toLowerCase();
  return files.filter((file) =>
    relativeExamplePath(file, name).toLowerCase().includes(term),
  );
}
