import React, {useState} from 'react';
import Link from '@docusaurus/Link';
import CodeBlock from '@theme/CodeBlock';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import Layout from '../FileBrowser/Layout';
import type {DocLink, FileBrowserOptions} from '../FileBrowser/types';
import styles from './styles.module.css';

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
  variants: {
    engine: 'cel' | 'starlark';
    path: string;
    policy: string;
    githubUrl: string;
  }[];
  configuration: string;
  contract: string;
  inputReference: string;
  command: string;
  docs: DocLink[];
  cases: PolicyCase[];
  caseFields: InputField[][];
}

function FieldExplorer({fields}: {fields: InputField[]}) {
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState('');
  const visible = fields.filter(field => field.path.toLowerCase().includes(search.toLowerCase()));
  const field = visible.find(item => item.path === selected) ?? visible[0];
  return (
    <div className={styles.explorer}>
      <label className={styles.casePicker}>Find a field
        <input type="search" value={search} placeholder="e.g. pull_request or resource_changes" onChange={event => setSearch(event.target.value)} />
      </label>
      <label className={styles.fieldPicker}>Observed fields
        <select size={6} value={field?.path ?? ''} onChange={event => setSelected(event.target.value)}>
          {visible.map(item => <option key={item.path} value={item.path}>{item.path} · {item.type}</option>)}
        </select>
      </label>
      {field ? <>
        <p>JSON type: <code>{field.type}</code></p>
        <Tabs groupId="policy-engine" defaultValue="cel" lazy>
          <TabItem value="cel" label="CEL">
            <CodeBlock language="text" title="Field expression">{field.path}</CodeBlock>
          </TabItem>
          <TabItem value="starlark" label="Starlark">
            <CodeBlock language="python" title="Field expression">{`ctx.${field.path}`}</CodeBlock>
          </TabItem>
        </Tabs>
      </> : <p role="status">No matching fields in this example.</p>}
      <p className={styles.note}>Fields and array indices belong to this example, not every event. A missing field is not necessarily unsupported. Guard optional values before accessing them.</p>
    </div>
  );
}

function TestCases({cases, caseFields}: {cases: PolicyCase[]; caseFields: InputField[][]}) {
  const [selected, setSelected] = useState(0);
  const fixture = cases[selected];
  const outcome = fixture.hooks?.length ? 'Hooks required' : fixture.status === 'deny' ? 'Blocked' : fixture.status === 'not_applicable' ? 'Rule does not match' : fixture.status;
  return (
    <>
      <label className={styles.casePicker}>Example
        <select aria-label="Example" value={selected} onChange={event => setSelected(Number(event.target.value))}>
          {cases.map((item, index) => <option key={item.name} value={index}>{item.name}</option>)}
        </select>
      </label>
      <p role="status"><strong>{outcome}</strong> · <code>{fixture.status}</code></p>
      <Tabs defaultValue="fields">
        <TabItem value="fields" label="Field explorer">
          <FieldExplorer key={selected} fields={caseFields[selected]} />
        </TabItem>
        <TabItem value="json" label="Input JSON">
          <CodeBlock language="json" title="Policy request">{JSON.stringify(fixture.request, null, 2)}</CodeBlock>
        </TabItem>
      </Tabs>
      {fixture.hooks?.length ? <CodeBlock language="json" title="Required hooks">{JSON.stringify(fixture.hooks, null, 2)}</CodeBlock> : null}
      <p className={styles.note}>Both engine implementations run against these same evaluation fixtures; these are not live runs. A nonmatching rule does not mean all policies passed; selecting a hook does not prove it executed.</p>
    </>
  );
}

export default function PolicyPage({policyData: policy, optionsData}: {
  policyData: PolicyData;
  optionsData: FileBrowserOptions;
}) {
  return (
    <Layout options={optionsData} title={policy.title} description={policy.description}>
      <article className={styles.page}>
        <Link className={styles.back} to={optionsData.routeBasePath}>← Policy Library</Link>
        <h1>{policy.title}</h1>
        <p className={styles.lead}>{policy.description}</p>
        <div id="policy" className={styles.policyCode}>
          <Tabs groupId="policy-engine" defaultValue="cel" lazy>
            {policy.variants.map(variant => (
              <TabItem key={variant.engine} value={variant.engine} label={variant.engine === 'cel' ? 'CEL' : 'Starlark'}>
                <CodeBlock language="yaml" title={`policy.d/${policy.name}.yaml`}>{variant.policy.trimEnd()}</CodeBlock>
                <a href={variant.githubUrl}>View {variant.engine === 'cel' ? 'CEL' : 'Starlark'} source on GitHub</a>
              </TabItem>
            ))}
          </Tabs>
        </div>
        <p><a href="#examples">Explore this policy's input fields and example payloads ↓</a></p>
        <h2 id="configure">Configure</h2>
        <div className="markdown"><Markdown remarkPlugins={[remarkGfm]}>{policy.configuration}</Markdown></div>
        <h2 id="run">Run</h2>
        <p>Choose one engine tab and save its policy as <code>policy.d/{policy.name}.yaml</code>; do not install both implementations of the same policy. {policy.command.includes('<component>') && <>Replace <code>&lt;component&gt;</code> and <code>&lt;stack&gt;</code> with your target.</>}</p>
        <CodeBlock language="shell">{policy.command}</CodeBlock>
        <p>Atmos discovers <code>policy.d/</code> in the invocation directory. Set <code>ATMOS_POLICY_DIR</code> to use a different policy directory. Plan and lifecycle policies support <Link to="/policies/configuration#instance-attachments">tag or label selectors</Link>; affected policies inspect before/after instances in their expressions.</p>
        <p>Review the rule and any required tools or callback settings before running. In CI, load policies from a protected, pinned revision—not the candidate PR. See <Link to="/policies/configuration#enforcement">enforcement configuration</Link>.</p>
        <details className={styles.contract}>
          <summary>Input and enforcement contract</summary>
          <div className="markdown"><Markdown remarkPlugins={[remarkGfm]}>{policy.contract}</Markdown></div>
        </details>
        <h2 id="examples">Inputs and examples</h2>
        <p>Search the fields supplied to this policy and copy an expression in either engine. See the <Link to="/policies/inputs">input contracts</Link> for each policy kind.</p>
        {policy.tags.includes('GitHub Actions') && <p>GitHub owns the fields inside <code>ci.event.data</code>. Use the <Link to="/policies/ci#github-event-reference">event reference</Link> to look up their meaning, and <Link to="/cli/commands/ci/context"><code>atmos ci context</code></Link> to inspect your runner's actual payload. Triggering events are not live approval evidence.</p>}
        {policy.inputReference && <div className="markdown"><Markdown remarkPlugins={[remarkGfm]}>{policy.inputReference}</Markdown></div>}
        <TestCases cases={policy.cases} caseFields={policy.caseFields} />
        <footer className={styles.references}>
          <Link to={`${optionsData.routeBasePath}/${policy.name}/tests/cases.json`}>All test fixtures</Link>
          {policy.docs.map(doc => <Link key={doc.url} to={doc.url}>{doc.label}</Link>)}
        </footer>
      </article>
    </Layout>
  );
}
