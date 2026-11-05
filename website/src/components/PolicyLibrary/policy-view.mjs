export const views = ["policy", "examples", "reference"];
export const engines = ["cel", "starlark"];

/** Resolve URL selections, falling back to the saved engine, legacy examples anchor, and first fixture. */
export function resolvePolicyView(search, hash, cases, preference) {
  const params = new URLSearchParams(search);
  const requested = params.get("view");
  const view = views.includes(requested)
    ? requested
    : hash === "#examples"
      ? "examples"
      : "policy";
  const engine = [params.get("engine"), preference, "cel"].find((value) =>
    engines.includes(value),
  );
  const fixture =
    cases.find((item) => item.name === params.get("case")) ?? cases[0];
  return { view, engine, fixture };
}

/** Update selected URL parameters while preserving unrelated ones; nullish values remove parameters. */
export function policyViewUrl(pathname, search, changes) {
  const params = new URLSearchParams(search);
  for (const [key, value] of Object.entries(changes)) {
    if (value == null) params.delete(key);
    else params.set(key, value);
  }
  return `${pathname}?${params.toString()}`;
}

/** Carry view and engine across sidebar policy links, clearing fixture IDs belonging to the old policy. */
export function policySidebarLinks(items, search, selection) {
  return items.map((item) => {
    if (item.customProps?.policyLibraryEntry) {
      return {
        ...item,
        href: policyViewUrl(item.href, search, { ...selection, case: null }),
      };
    }
    return item.items
      ? { ...item, items: policySidebarLinks(item.items, search, selection) }
      : item;
  });
}

/** Describe a manifest action for catalog discovery, preserving unrecognized action names. */
export function effectLabel(action) {
  return (
    {
      deny: "Blocks",
      require_approval: "Requires approval",
      hooks: "Requires hooks",
      allow: "Allows",
    }[action] ?? action
  );
}

/** Name known catalog input kinds for readers, falling back to the original contract identifier. */
export function inputLabel(kind) {
  return (
    {
      "terraform.plan": "Terraform plan",
      operation: "Lifecycle event",
      "atmos.affected": "Affected changes",
      command: "CLI command",
      function: "Function call",
      source: "Source access",
      "component.command": "Component command",
    }[kind] ?? kind
  );
}

/** Label fixture expectations without equating nonmatches with allowance or masking denial with hooks. */
export function outcomeLabel(fixture) {
  // A denial or approval requirement must never be hidden by hook requirements.
  if (fixture.status === "deny") return "Blocked";
  if (fixture.status === "awaiting_approval") return "Approval required";
  if (fixture.status === "not_applicable") return "Rule does not match";
  if (fixture.hooks?.length) return "Hooks required";
  return fixture.status === "allow" ? "Allowed" : fixture.status;
}
