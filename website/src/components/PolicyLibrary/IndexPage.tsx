import React, {useState} from 'react';
import Link from '@docusaurus/Link';
import Layout from '../FileBrowser/Layout';
import type {FileBrowserOptions} from '../FileBrowser/types';
import styles from './styles.module.css';

interface PolicySummary {
  name: string;
  title: string;
  description: string;
  tags: string[];
}

export default function PolicyIndex({treeData, optionsData}: {
  treeData: {examples: PolicySummary[]; tags: string[]};
  optionsData: FileBrowserOptions;
}) {
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('');
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const entries = treeData.examples.filter(entry =>
    (!category || entry.tags.includes(category)) &&
    terms.every(term => [entry.name, entry.title, entry.description, ...entry.tags].join(' ').toLowerCase().includes(term)));
  return (
    <Layout options={optionsData} title="Policy Library" description={optionsData.description}>
      <article className={styles.page}>
        <h1>Policy Library</h1>
        <p className={styles.lead}>Copy a policy, adjust its parameters, and use it in your Atmos execution workflow.</p>
        <div className={styles.filters}>
          <label className={styles.search}>Search policies
            <input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search by name or purpose…" />
          </label>
          <label>Type
            <select aria-label="Policy type" value={category} onChange={event => setCategory(event.target.value)}>
              <option value="">All types</option>
              {treeData.tags.map(tag => <option key={tag}>{tag}</option>)}
            </select>
          </label>
        </div>
        <p className={styles.count} role="status">{entries.length} {entries.length === 1 ? 'policy' : 'policies'}</p>
        <ul className={styles.list}>
          {entries.map(entry => (
            <li key={entry.name}>
              <Link className={styles.row} to={`${optionsData.routeBasePath}/${entry.name}`}>
                <span><span className={styles.rowTitle}>{entry.title}</span><span className={styles.description}>{entry.description}</span></span>
                <span className={styles.badge}>{entry.tags[0]}</span>
                <span className={styles.arrow} aria-hidden="true">→</span>
              </Link>
            </li>
          ))}
        </ul>
        {entries.length === 0 && <p>No policies match. <button className={styles.reset} onClick={() => {setQuery(''); setCategory('');}}>Clear filters</button></p>}
      </article>
    </Layout>
  );
}
