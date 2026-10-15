import React, { useEffect, useRef, useState } from "react";
import Link from "@docusaurus/Link";
import useIsBrowser from "@docusaurus/useIsBrowser";
import { useHistory, useLocation } from "@docusaurus/router";
import { useStorageSlot } from "@docusaurus/theme-common";
import CodeBlock from "@theme/CodeBlock";
import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import Layout from "../FileBrowser/Layout";
import type { DocLink, FileBrowserOptions } from "../FileBrowser/types";
import {
  effectLabel,
  inputLabel,
  outcomeLabel,
  policySidebarLinks,
  policyViewUrl,
  resolvePolicyView,
  views,
} from "./policy-view.mjs";
import styles from "./styles.module.css";

interface PolicyCase {
  name: string;
  request: Record<string, unknown>;
  status: string;
  hooks?: unknown[];
}
interface InputField {
  path: string;
  type: string;
}
interface PolicyData {
  name: string;
  title: string;
  description: string;
  tags: string[];
  inputKind: string;
  action: string;
  variants: {
    engine: "cel" | "starlark";
    path: string;
    policy: string;
    githubUrl: string;
  }[];
  configuration: string;
  contract: string;
  inputReference: string;
  command: string;
  docs: DocLink[];
  caseLabels: Record<string, string>;
  cases: PolicyCase[];
  caseFields: InputField[][];
}

/** Copy the selected manifest and announce clipboard success or the manual-copy fallback. */
function CopyPolicy({ text }: { text: string }) {
  const [status, setStatus] = useState("");
  const timer = useRef<ReturnType<typeof setTimeout>>();
  useEffect(() => () => clearTimeout(timer.current), []);
  /** Replace any previous notification and clear the result after four seconds. */
  async function copy() {
    clearTimeout(timer.current);
    try {
      await navigator.clipboard.writeText(text);
      setStatus("Copied!");
    } catch {
      setStatus("Copy unavailable. Select and copy the policy code manually.");
    }
    timer.current = setTimeout(() => setStatus(""), 4000);
  }
  return (
    <div className={styles.copyControl}>
      <button type="button" className={styles.primary} onClick={copy}>
        Copy policy
      </button>
      <span role="status" className={styles.copyStatus}>
        {status}
      </span>
    </div>
  );
}

/** Inspect observed fixture fields and show an expression for the selected policy engine. */
function FieldExplorer({
  fields,
  engine,
}: {
  fields: InputField[];
  engine: string;
}) {
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState("");
  const visible = fields.filter((field) =>
    field.path.toLowerCase().includes(search.toLowerCase()),
  );
  const field = visible.find((item) => item.path === selected) ?? visible[0];
  return (
    <div className={styles.explorer}>
      <label className={styles.casePicker}>
        Find a field
        <input
          type="search"
          value={search}
          placeholder="e.g. pull_request or resource_changes"
          onChange={(event) => setSearch(event.target.value)}
        />
      </label>
      <label className={styles.fieldPicker}>
        Observed fields
        <select
          size={6}
          value={field?.path ?? ""}
          onChange={(event) => setSelected(event.target.value)}
        >
          {visible.map((item) => (
            <option key={item.path} value={item.path}>
              {item.path} · {item.type}
            </option>
          ))}
        </select>
      </label>
      {field ? (
        <>
          <p>
            JSON type: <code>{field.type}</code>
          </p>
          <CodeBlock
            language={engine === "cel" ? "text" : "python"}
            title={`${engine === "cel" ? "CEL" : "Starlark"} field expression`}
          >
            {engine === "cel" ? field.path : `ctx.${field.path}`}
          </CodeBlock>
        </>
      ) : (
        <p role="status">No matching fields in this example.</p>
      )}
      <p className={styles.note}>
        Fields and array indices belong to this example, not every event. A
        missing field is not necessarily unsupported. Guard optional values
        before accessing them.
      </p>
    </div>
  );
}

/** Present a selected fixture's expected decision and inputs without implying live execution. */
function TestCases({
  policy,
  fixture,
  engine,
  onSelect,
}: {
  policy: PolicyData;
  fixture?: PolicyCase;
  engine: string;
  onSelect: (name: string) => void;
}) {
  if (!fixture) return <p>No example cases are available for this policy.</p>;
  const fields = policy.caseFields[policy.cases.indexOf(fixture)] ?? [];
  const scenario = (item: PolicyCase) =>
    policy.caseLabels[item.name] ?? item.name;
  return (
    <>
      <div className={styles.sectionHeading}>
        <h2>Explore the decisions</h2>
        <span>TESTED FIXTURES</span>
      </div>
      <p className={styles.note}>
        These are expected evaluation results, not live runs. Both engines use
        the same fixtures. A nonmatching rule does not mean all policies passed;
        a required hook has not necessarily executed.
      </p>
      <div className={styles.caseContent}>
        <label className={styles.scenarioPicker}>
          Scenario
          <select
            value={fixture.name}
            onChange={(event) => onSelect(event.target.value)}
          >
            {policy.cases.map((item) => (
              <option key={item.name} value={item.name}>
                {scenario(item)} — {outcomeLabel(item)}
              </option>
            ))}
          </select>
        </label>
        <div className={styles.decision} role="status">
          <span className={styles.eyebrow}>EXPECTED DECISION</span>
          <h3>
            {scenario(fixture)} <span aria-hidden="true">→</span>{" "}
            {outcomeLabel(fixture)}
          </h3>
          <code>{fixture.status}</code>
        </div>
        <Tabs key={fixture.name} defaultValue="json">
          <TabItem value="json" label="Input JSON">
            <CodeBlock language="json" title="Policy request">
              {JSON.stringify(fixture.request, null, 2)}
            </CodeBlock>
          </TabItem>
          <TabItem value="fields" label="Field explorer">
            <FieldExplorer fields={fields} engine={engine} />
          </TabItem>
        </Tabs>
        {!!fixture.hooks?.length && (
          <CodeBlock language="json" title="Required hooks">
            {JSON.stringify(fixture.hooks, null, 2)}
          </CodeBlock>
        )}
      </div>
    </>
  );
}

/** Render policy, fixture, and reference views from shared data and URL-backed selections. */
export default function PolicyPage({
  policyData: policy,
  optionsData,
}: {
  policyData: PolicyData;
  optionsData: FileBrowserOptions;
}) {
  const location = useLocation();
  const history = useHistory();
  const isBrowser = useIsBrowser();
  const [preference, preferenceSlot] = useStorageSlot(
    "docusaurus.tab.policy-engine",
  );
  // Static pages render the default view; apply URL state only after hydration.
  const { view, engine, fixture } = resolvePolicyView(
    isBrowser ? location.search : "",
    isBrowser ? location.hash : "",
    policy.cases,
    preference,
  );
  const variant = policy.variants.find((item) => item.engine === engine)!;
  const destination = `policy.d/${policy.name}.yaml`;
  const url = (changes: Record<string, string>) =>
    policyViewUrl(location.pathname, location.search, {
      view,
      engine,
      ...changes,
    });
  const caseUrl = (name: string) => url({ view: "examples", case: name });
  return (
    <Layout
      options={optionsData}
      sidebarTransform={(items) =>
        policySidebarLinks(items, isBrowser ? location.search : "", {
          view,
          engine,
        })
      }
      title={policy.title}
      description={policy.description}
    >
      <article className={styles.page}>
        <Link className={styles.back} to={optionsData.routeBasePath}>
          ← Policy Library
        </Link>
        <header className={styles.hero}>
          <div className={styles.eyebrow}>POLICY</div>
          <h1>{policy.title}</h1>
          <p className={styles.lead}>{policy.description}</p>
          <div className={styles.metadata}>
            <span>{inputLabel(policy.inputKind)}</span>
            <span>{effectLabel(policy.action)}</span>
            <span>CEL / Starlark</span>
          </div>
        </header>
        <div className={styles.policyToolbar}>
          <div
            className={styles.engines}
            role="group"
            aria-label="Policy engine"
          >
            {policy.variants.map((item) => (
              <button
                type="button"
                key={item.engine}
                aria-pressed={engine === item.engine}
                onClick={() => {
                  preferenceSlot.set(item.engine);
                  history.push(url({ engine: item.engine }));
                }}
              >
                {item.engine === "cel" ? "CEL" : "Starlark"}
              </button>
            ))}
          </div>
          <code className={styles.destination}>{destination}</code>
          <div className={styles.actions}>
            <CopyPolicy
              key={`${policy.name}-${engine}`}
              text={variant.policy}
            />
            <a className={styles.secondary} href={variant.githubUrl}>
              View source ↗
            </a>
          </div>
        </div>
        <nav className={styles.views} aria-label="Policy views">
          {views.map((item) => (
            <Link
              key={item}
              to={url({ view: item })}
              aria-current={view === item ? "page" : undefined}
            >
              {item.charAt(0).toUpperCase() + item.slice(1)}
              {item === "examples" && <span>{policy.cases.length}</span>}
            </Link>
          ))}
        </nav>
        <div className={styles.policyContent}>
          {view === "policy" && (
            <section aria-label="Policy">
              <div id="policy" className={styles.policyCode}>
                <CodeBlock language="yaml" title={destination}>
                  {variant.policy.trimEnd()}
                </CodeBlock>
              </div>
              <div className={styles.setup}>
                <section id="configure">
                  <h2>
                    <span>1</span> Configure
                  </h2>
                  <div className="markdown">
                    <Markdown remarkPlugins={[remarkGfm]}>
                      {policy.configuration}
                    </Markdown>
                  </div>
                </section>
                <section id="save">
                  <h2>
                    <span>2</span> Save
                  </h2>
                  <p>
                    Save the {engine === "cel" ? "CEL" : "Starlark"} policy as{" "}
                    <code>{destination}</code>. Install only one implementation
                    of each policy.
                  </p>
                  <p>
                    Atmos discovers <code>policy.d/</code> in the invocation
                    directory. Set <code>ATMOS_POLICY_DIR</code> to use another
                    directory.
                  </p>
                </section>
                <section id="run">
                  <h2>
                    <span>3</span> Run
                  </h2>
                  {policy.command.includes("<component>") && (
                    <p>
                      Replace <code>&lt;component&gt;</code> and{" "}
                      <code>&lt;stack&gt;</code> with your target.
                    </p>
                  )}
                  <CodeBlock language="shell">{policy.command}</CodeBlock>
                  <p className={styles.note}>
                    Review required tools and callback settings before running.
                    In CI, load policies from a protected, pinned revision. See{" "}
                    <Link to="/policies/configuration#enforcement">
                      enforcement configuration
                    </Link>
                    .
                  </p>
                </section>
              </div>
              <Link
                className={styles.exploreLink}
                to={url({ view: "examples" })}
              >
                Explore inputs and expected decisions →
              </Link>
            </section>
          )}
          {view === "examples" && (
            <section aria-label="Examples" id="examples">
              <TestCases
                key={policy.name}
                policy={policy}
                fixture={fixture}
                engine={engine}
                onSelect={(name) => history.push(caseUrl(name))}
              />
            </section>
          )}
          {view === "reference" && (
            <section aria-label="Reference">
              <h2>Input and enforcement contract</h2>
              <div className="markdown">
                <Markdown remarkPlugins={[remarkGfm]}>
                  {policy.contract}
                </Markdown>
              </div>
              <h2>Inputs and expressions</h2>
              <p>
                See the <Link to="/policies/inputs">input contracts</Link> for
                each policy kind. Plan, lifecycle, and component command
                policies support{" "}
                <Link to="/policies/configuration#instance-attachments">
                  tag or label selectors
                </Link>
                ; affected policies inspect before/after instances in their
                expressions.
              </p>
              {policy.tags.includes("GitHub Actions") && (
                <p>
                  GitHub owns the fields inside <code>ci.event.data</code>. Use
                  the{" "}
                  <Link to="/policies/ci#github-event-reference">
                    event reference
                  </Link>{" "}
                  to look up their meaning, and{" "}
                  <Link to="/cli/commands/ci/context">
                    <code>atmos ci context</code>
                  </Link>{" "}
                  to inspect your runner's actual payload. Triggering events are
                  not live approval evidence.
                </p>
              )}
              {policy.inputReference && (
                <div className="markdown">
                  <Markdown remarkPlugins={[remarkGfm]}>
                    {policy.inputReference}
                  </Markdown>
                </div>
              )}
              <footer className={styles.references}>
                <Link
                  to={`${optionsData.routeBasePath}/${policy.name}/tests/cases.json`}
                >
                  All test fixtures
                </Link>
                {policy.docs.map((doc) => (
                  <Link key={doc.url} to={doc.url}>
                    {doc.label}
                  </Link>
                ))}
              </footer>
            </section>
          )}
        </div>
      </article>
    </Layout>
  );
}
