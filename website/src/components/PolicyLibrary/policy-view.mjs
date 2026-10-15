export const views = ["policy", "examples", "reference"];
export const engines = ["cel", "starlark"];

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

export function policyViewUrl(pathname, search, changes) {
  const params = new URLSearchParams(search);
  for (const [key, value] of Object.entries(changes)) {
    if (value == null) params.delete(key);
    else params.set(key, value);
  }
  return `${pathname}?${params.toString()}`;
}

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

export function inputLabel(kind) {
  return (
    {
      "terraform.plan": "Terraform plan",
      operation: "Lifecycle event",
      "atmos.affected": "Affected changes",
      command: "CLI command",
      function: "YAML function",
      "component.command": "Component command",
    }[kind] ?? kind
  );
}

export function outcomeLabel(fixture) {
  // A denial or approval requirement must never be hidden by hook requirements.
  if (fixture.status === "deny") return "Blocked";
  if (fixture.status === "awaiting_approval") return "Approval required";
  if (fixture.status === "not_applicable") return "Rule does not match";
  if (fixture.hooks?.length) return "Hooks required";
  return fixture.status === "allow" ? "Allowed" : fixture.status;
}
