import React, { useState } from "react";
import Link from "@docusaurus/Link";
import Layout from "../FileBrowser/Layout";
import type { FileBrowserOptions } from "../FileBrowser/types";
import { effectLabel } from "./policy-view.mjs";
import styles from "./styles.module.css";

interface PolicySummary {
  name: string;
  title: string;
  description: string;
  tags: string[];
  action: string;
}

export default function PolicyIndex({
  treeData,
  optionsData,
}: {
  treeData: { examples: PolicySummary[]; tags: string[] };
  optionsData: FileBrowserOptions;
}) {
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("");
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const matches = treeData.examples.filter((entry) =>
    terms.every((term) =>
      [
        entry.name,
        entry.title,
        entry.description,
        effectLabel(entry.action),
        ...entry.tags,
      ]
        .join(" ")
        .toLowerCase()
        .includes(term),
    ),
  );
  const entries = matches.filter(
    (entry) => !category || entry.tags.includes(category),
  );
  const categories = ["", ...treeData.tags];
  const count = (tag: string) =>
    matches.filter((entry) => !tag || entry.tags.includes(tag)).length;
  const reset = () => {
    setQuery("");
    setCategory("");
  };
  return (
    <Layout
      options={optionsData}
      title="Policy Library"
      description={optionsData.description}
    >
      <article className={styles.page}>
        <header className={styles.hero}>
          <div className={styles.eyebrow}>THE ATMOS LIBRARY</div>
          <h1>Policy Library</h1>
          <p className={styles.lead}>
            Find a guardrail for your workflow. Explore its behavior, choose an
            engine, and make it yours.
          </p>
          <Link to="/policies/configuration">Get started with policies →</Link>
        </header>
        <div className={styles.toolbar}>
          <label className={styles.search}>
            Search policies
            <input
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search policies, tools, or workflows…"
            />
          </label>
          <div
            className={styles.categories}
            role="group"
            aria-label="Policy categories"
          >
            {categories.map((tag) => (
              <button
                type="button"
                key={tag}
                aria-pressed={category === tag}
                onClick={() => setCategory(tag)}
              >
                {tag || "All policies"} <span>{count(tag)}</span>
              </button>
            ))}
          </div>
          <label className={styles.mobileCategory}>
            Category
            <select
              value={category}
              onChange={(event) => setCategory(event.target.value)}
            >
              {categories.map((tag) => (
                <option key={tag} value={tag}>
                  {tag || "All policies"} ({count(tag)})
                </option>
              ))}
            </select>
          </label>
        </div>
        <div className={styles.resultsHeader}>
          <h2>{category || "All policies"}</h2>
          <span className={styles.count} role="status">
            {entries.length} {entries.length === 1 ? "policy" : "policies"}
          </span>
          {(query || category) && (
            <button type="button" className={styles.reset} onClick={reset}>
              Clear filters ×
            </button>
          )}
        </div>
        <ul className={styles.list}>
          {entries.map((entry) => (
            <li key={entry.name}>
              <Link
                className={styles.row}
                to={`${optionsData.routeBasePath}/${entry.name}`}
              >
                <span>
                  <span className={styles.rowTitle}>{entry.title}</span>
                  <span className={styles.description}>
                    {entry.description}
                  </span>
                  <span className={styles.rowTags}>
                    {entry.tags.map((tag) => (
                      <span key={tag}>{tag}</span>
                    ))}
                  </span>
                </span>
                <span className={styles.badge}>
                  {effectLabel(entry.action)}
                </span>
                <span className={styles.arrow} aria-hidden="true">
                  →
                </span>
              </Link>
            </li>
          ))}
        </ul>
        {entries.length === 0 && (
          <div className={styles.empty}>
            <h2>No policies found</h2>
            <p>Try another search or choose a different category.</p>
            <button type="button" className={styles.secondary} onClick={reset}>
              Clear filters
            </button>
          </div>
        )}
      </article>
    </Layout>
  );
}
