import React, {useState} from 'react';
import Link from '@docusaurus/Link';
import CodeBlock from '@theme/CodeBlock';
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
interface PolicyData {
  name: string;
  title: string;
  description: string;
  tags: string[];
  policy: string;
  configuration: string;
  contract: string;
  command: string;
  githubUrl: string;
  docs: DocLink[];
  cases: PolicyCase[];
}

function TestCases({cases}: {cases: PolicyCase[]}) {
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
      <CodeBlock language="json" title="Policy request">{JSON.stringify(fixture.request, null, 2)}</CodeBlock>
      {fixture.hooks?.length ? <CodeBlock language="json" title="Required hooks">{JSON.stringify(fixture.hooks, null, 2)}</CodeBlock> : null}
      <p className={styles.note}>These are tested evaluation fixtures, not live runs. A nonmatching rule does not mean all policies passed; selecting a hook does not prove it executed.</p>
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
          <CodeBlock language="yaml" title={`policy.d/${policy.name}.yaml`}>{policy.policy.trimEnd()}</CodeBlock>
        </div>
        <h2 id="configure">Configure</h2>
        <div className="markdown"><Markdown remarkPlugins={[remarkGfm]}>{policy.configuration}</Markdown></div>
        <h2 id="run">Run</h2>
        <p>Save the policy above as <code>policy.d/{policy.name}.yaml</code>. {policy.command.includes('<component>') && <>Replace <code>&lt;component&gt;</code> and <code>&lt;stack&gt;</code> with your target.</>}</p>
        <CodeBlock language="shell">{policy.command}</CodeBlock>
        <p>Atmos discovers <code>policy.d/</code> in the invocation directory. Set <code>ATMOS_POLICY_DIR</code> to use a different policy directory. Plan and lifecycle policies support <Link to="/policies/configuration#instance-attachments">tag or label selectors</Link>; affected policies inspect before/after instances in their expressions.</p>
        <p>Review the rule and any required tools or callback settings before running. In CI, load policies from a protected, pinned revision—not the candidate PR. See <Link to="/policies/configuration#enforcement">enforcement configuration</Link>.</p>
        <details className={styles.contract}>
          <summary>Input and enforcement contract</summary>
          <div className="markdown"><Markdown remarkPlugins={[remarkGfm]}>{policy.contract}</Markdown></div>
        </details>
        <h2 id="examples">Examples</h2>
        <TestCases cases={policy.cases} />
        <footer className={styles.references}>
          <a href={policy.githubUrl}>Source on GitHub</a>
          <Link to={`${optionsData.routeBasePath}/${policy.name}/tests/cases.json`}>All test fixtures</Link>
          {policy.docs.map(doc => <Link key={doc.url} to={doc.url}>{doc.label}</Link>)}
        </footer>
      </article>
    </Layout>
  );
}
