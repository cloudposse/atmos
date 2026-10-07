import assert from "node:assert/strict";
import test from "node:test";
import {
  activeEntryPath,
  createNavigationStore,
  entryId,
  findEntry,
  historyStateKey,
  identifyItems,
  initialNavigation,
  transitionNavigation,
  validNavigation,
} from "./state.mjs";
import { filterItems, prepareItems } from "./navigation.mjs";

const link = (label, href, reference = false) => ({
  type: "link",
  label,
  href,
  docId: href,
  customProps: { yamlReference: reference },
});
const category = (label, href, items, reference = false) => ({
  type: "category",
  label,
  href,
  items,
  customProps: { yamlReference: reference },
});
const tree = identifyItems([
  category("Workflows", "/workflows", [link("steps", "/steps", true)]),
  category("Steps", "/steps", [link("retry", "/steps/retry")]),
  category("Stacks", "/stacks", [
    category("dependencies", "/stacks/dependencies", [
      link("components", "/stacks/dependencies/components"),
    ]),
    link("retry", "/stacks/components/terraform/retry"),
    category("terraform", "/stacks/components/terraform", [
      category("<name>", "/stacks/components/name", [
        category(
          "dependencies",
          "/stacks/dependencies",
          [link("components", "/stacks/dependencies/components", true)],
          true,
        ),
        link("retry", "/stacks/components/terraform/retry", true),
      ]),
    ]),
  ]),
]);
const named = tree[2].items[2].items[0];
const alias = named.items[0];
const workflow = () => initialNavigation(tree, "cli", "/workflows");

test("clicked shared categories and leaves stay in the selected branch", () => {
  const start = initialNavigation(tree, "cli", "/stacks/components/terraform");
  const next = transitionNavigation(start, tree, "cli", alias.href, {
    url: alias.href,
    entry: entryId(alias),
  });
  assert.equal(next.entry, entryId(alias));
  assert.equal(activeEntryPath(alias, alias.href, next.entry), alias.href);
  assert.equal(activeEntryPath(tree[2].items[0], alias.href, next.entry), "");
  assert.equal(activeEntryPath(named, alias.href, next.entry), alias.href);
  assert.equal(next.expanded[entryId(named)], true);
  const retry = named.items[1];
  const end = transitionNavigation(next, tree, "cli", retry.href, {
    url: retry.href,
    entry: entryId(retry),
  });
  assert.equal(end.entry, entryId(retry));
  assert.deepEqual(end.trail, []);
});

test("article links to children retain the nearest active branch", () => {
  const start = {
    ...initialNavigation(tree, "cli", alias.href),
    entry: entryId(alias),
  };
  const next = transitionNavigation(start, tree, "cli", alias.items[0].href);
  assert.equal(next.entry, entryId(alias.items[0]));
});

test("direct arrivals use the canonical entry, including fragments and queries", () => {
  const current = initialNavigation(
    tree,
    "cli",
    "/stacks/dependencies/?from=search#tools",
  );
  assert.equal(current.entry, entryId(tree[2].items[0]));
  assert.deepEqual(current.trail, []);
});

test("cross-section links remember the originating URL, expanded branches, and scroll", () => {
  const start = {
    ...workflow(),
    url: "/workflows?source=docs#example",
    scroll: { [entryId(tree[0])]: 214 },
    expanded: { example: false },
  };
  for (const intent of [
    undefined,
    { url: "/steps", entry: entryId(tree[0].items[0]) },
  ]) {
    const steps = transitionNavigation(start, tree, "cli", "/steps", intent);
    assert.equal(steps.section, entryId(tree[1]));
    assert.equal(steps.trail[0].url, start.url);
    assert.deepEqual(steps.trail[0].scroll, start.scroll);
    assert.deepEqual(steps.trail[0].expanded, start.expanded);
    const retry = transitionNavigation(steps, tree, "cli", "/steps/retry");
    assert.deepEqual(retry.trail, steps.trail);
  }
});

test("returning to a prior section unwinds the trail without a loop", () => {
  let state = transitionNavigation(workflow(), tree, "cli", "/steps");
  state = transitionNavigation(state, tree, "cli", "/stacks");
  assert.equal(state.trail.length, 2);
  state = transitionNavigation(state, tree, "cli", "/workflows");
  assert.deepEqual(state.trail, []);
});

test("identities survive filters, hidden entries, and duplicate labels", () => {
  const input = identifyItems([
    link("hidden", "/hidden"),
    category("group", "/group", [link("same", "/a"), link("same", "/b")]),
  ]);
  input[0].unlisted = true;
  const prepared = prepareItems(input, "/b");
  assert.equal(entryId(prepared[0]), entryId(input[1]));
  const filtered = filterItems(prepared, "same");
  assert.equal(entryId(filtered[0].items[1]), entryId(input[1].items[1]));
  assert.notEqual(entryId(filtered[0].items[0]), entryId(filtered[0].items[1]));
});

function mockHistory() {
  let index = 0;
  let counter = 0;
  const entries = [{ pathname: "/workflows", key: "0" }];
  const listeners = new Set();
  const history = {
    get location() {
      return entries[index];
    },
    listen: (fn) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    push(url, state, action = "PUSH") {
      const parsed = new URL(url, "https://atmos.tools");
      const next = {
        pathname: parsed.pathname,
        search: parsed.search,
        hash: parsed.hash,
        key: String(++counter),
        state,
      };
      entries.splice(++index, entries.length, next);
      listeners.forEach((fn) => fn(next, action));
    },
    go(delta) {
      index += delta;
      listeners.forEach((fn) => fn(entries[index], "POP"));
    },
  };
  return history;
}

test("browser Back/Forward restores each history entry and sidebar back restores its source page", () => {
  const history = mockHistory();
  const persisted = [];
  const store = createNavigationStore(history, {
    write: (state) => persisted.push(state),
  });
  const stop = store.subscribe(() => {});
  store.register(tree, "cli");
  store.update({ scroll: { [entryId(tree[0])]: 125 } });
  const source = store.getSnapshot();
  history.push("/steps");
  const steps = store.getSnapshot();
  history.push("/steps/retry");
  history.go(-1);
  assert.deepEqual(store.getSnapshot(), steps);
  history.go(-1);
  assert.deepEqual(store.getSnapshot(), source);
  history.go(1);
  store.back();
  assert.equal(history.location.pathname, "/workflows");
  assert.deepEqual(store.getSnapshot(), source);
  assert.ok(persisted.length > 0);
  stop();
});

test("same-URL alias selection works without creating a route transition", () => {
  const history = mockHistory();
  const store = createNavigationStore(history);
  store.subscribe(() => {});
  store.register(tree, "cli");
  history.push("/stacks/dependencies");
  store.intend(alias.href, entryId(alias));
  assert.equal(store.getSnapshot().entry, entryId(alias));
});

test("new stores restore serializable history state and reject stale or foreign snapshots", () => {
  const history = mockHistory();
  const source = { ...workflow(), scroll: { [entryId(tree[0])]: 80 } };
  history.location.state = {
    [historyStateKey]: source,
    unrelated: "preserved",
  };
  const store = createNavigationStore(history);
  store.register(tree, "cli");
  assert.deepEqual(store.getSnapshot(), source);
  assert.equal(history.location.state.unrelated, "preserved");
  assert.equal(
    validNavigation(
      { ...source, entry: "/missing" },
      tree,
      "cli",
      "/workflows",
    ),
    false,
  );
  assert.equal(validNavigation(source, tree, "docs", "/workflows"), false);
  assert.equal(validNavigation(source, tree, "cli", "/steps"), false);
  assert.equal(
    validNavigation(
      {
        ...source,
        trail: [{ section: entryId(tree[1]), url: "//external.test" }],
      },
      tree,
      "cli",
      "/workflows",
    ),
    false,
  );
});

test("choosing from the section index clears the return trail", () => {
  const history = mockHistory();
  const store = createNavigationStore(history);
  store.subscribe(() => {});
  store.register(tree, "cli");
  history.push("/steps");
  store.update({ section: null, trail: [] });
  store.update({
    section: entryId(tree[2]),
    entry: entryId(tree[2]),
    trail: [],
  });
  history.push("/stacks");
  assert.deepEqual(store.getSnapshot().trail, []);
});

test("history snapshots survive leaving Reference for a different sidebar", () => {
  const history = mockHistory();
  const store = createNavigationStore(history);
  store.subscribe(() => {});
  store.register(tree, "cli");
  history.push("/steps");
  const referenceState = store.getSnapshot();
  const learn = identifyItems([
    category("Learn", "/learn", [link("Install", "/install")]),
  ]);
  history.push("/learn");
  store.register(learn, "docs");
  store.update({ scroll: { [entryId(learn[0])]: 40 } });
  const learnState = store.getSnapshot();
  history.go(-1);
  store.register(tree, "cli");
  assert.deepEqual(store.getSnapshot(), referenceState);
  history.go(1);
  store.register(learn, "docs");
  assert.deepEqual(store.getSnapshot(), learnState);
});

test("scroll snapshots are saved without rerendering menu subscribers", () => {
  const history = mockHistory();
  const store = createNavigationStore(history);
  let renders = 0;
  store.subscribe(() => renders++);
  store.register(tree, "cli");
  const before = renders;
  store.update({ scroll: { [entryId(tree[0])]: 240 } }, false);
  assert.equal(renders, before);
  history.push("/steps");
  history.go(-1);
  assert.equal(store.getSnapshot().scroll[entryId(tree[0])], 240);
});

test("native anchor navigation preserves the selected alias and collapsed branches", () => {
  const history = mockHistory();
  const store = createNavigationStore(history);
  store.subscribe(() => {});
  store.register(tree, "cli");
  history.push("/stacks/dependencies");
  store.intend(alias.href, entryId(alias));
  store.update({ expanded: { [entryId(alias)]: false } });
  const before = store.getSnapshot();
  history.push("/stacks/dependencies#tools", undefined, "POP");
  assert.deepEqual(store.getSnapshot(), {
    ...before,
    url: "/stacks/dependencies#tools",
  });
});
