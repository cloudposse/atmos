import React, { useState } from "react";
import Link from "@docusaurus/Link";
import CodeBlock from "@theme/CodeBlock";
import {
  RiArrowRightUpLine,
  RiFileTextLine,
  RiFolderLine,
  RiGithubFill,
  RiSearchLine,
} from "react-icons/ri";
import FileViewer from "./FileViewer";
import ExampleCopyButton from "./ExampleCopyButton";
import { isBinaryFile } from "./utils";
import { filterExampleFiles, relativeExamplePath } from "./example-detail.mjs";
import type { ExampleProject, FileNode, TreeNode } from "./types";
import styles from "./example-source.module.css";

function SourceTree({
  nodes,
  selected,
  onSelect,
}: {
  nodes: TreeNode[];
  selected: string;
  onSelect: (file: FileNode) => void;
}) {
  return (
    <ul className={styles.tree}>
      {nodes.map((node) => (
        <li key={node.path}>
          {node.type === "directory" ? (
            <details open={selected.startsWith(`${node.path}/`)}>
              <summary>
                <RiFolderLine /> {node.name}
              </summary>
              <SourceTree
                nodes={node.children}
                selected={selected}
                onSelect={onSelect}
              />
            </details>
          ) : (
            <button
              type="button"
              aria-current={node.path === selected ? "true" : undefined}
              className={node.path === selected ? styles.selected : ""}
              onClick={() => onSelect(node)}
            >
              <RiFileTextLine /> {node.name}
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}

interface Props {
  example: ExampleProject;
  files: FileNode[];
  selected?: FileNode;
  routeBasePath: string;
  onSelect: (file: FileNode) => void;
}

export default function ExampleSourceBrowser({
  example,
  files,
  selected,
  routeBasePath,
  onSelect,
}: Props) {
  const [search, setSearch] = useState("");
  const matches: FileNode[] = filterExampleFiles(files, example.name, search);
  if (!selected)
    return (
      <p className={styles.empty}>No files are available for this example.</p>
    );
  const relative = (file: FileNode) => relativeExamplePath(file, example.name);
  const previewable = selected.content !== null && !isBinaryFile(selected);
  return (
    <div className={styles.sourceBrowser}>
      <aside className={styles.sourceSidebar} aria-label="Example files">
        <label className={styles.fileSearch}>
          <RiSearchLine />
          <input
            aria-label="Find a file"
            placeholder="Find a file…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
        <div className={styles.treeTitle}>{example.name}</div>
        {search.trim() ? (
          <>
            <p className={styles.searchCount} role="status">
              {matches.length} {matches.length === 1 ? "file" : "files"}
            </p>
            <button
              className={styles.clearSearch}
              type="button"
              onClick={() => setSearch("")}
            >
              Clear search
            </button>
            <ul className={styles.tree}>
              {matches.map((file) => (
                <li key={file.path}>
                  <button
                    type="button"
                    aria-current={
                      file.path === selected.path ? "true" : undefined
                    }
                    className={
                      file.path === selected.path ? styles.selected : ""
                    }
                    onClick={() => onSelect(file)}
                  >
                    <RiFileTextLine /> {relative(file)}
                  </button>
                </li>
              ))}
            </ul>
            {!matches.length && (
              <p className={styles.empty}>No matching files.</p>
            )}
          </>
        ) : (
          <SourceTree
            nodes={example.root.children}
            selected={selected.path}
            onSelect={onSelect}
          />
        )}
      </aside>
      <div className={styles.sourceMain}>
        <label className={styles.mobileFiles}>
          File
          <select
            value={selected.path}
            onChange={(event) => {
              const file = files.find(
                (item) => item.path === event.target.value,
              );
              if (file) onSelect(file);
            }}
          >
            {files.map((file) => (
              <option key={file.path} value={file.path}>
                {relative(file)}
              </option>
            ))}
          </select>
        </label>
        <div className={styles.sourceToolbar}>
          <span>
            <RiFileTextLine /> {relative(selected)}
          </span>
          <div>
            {previewable && <ExampleCopyButton text={selected.content!} />}
            {selected.githubUrl && (
              <a
                href={selected.githubUrl}
                aria-label="View selected file on GitHub"
              >
                <RiGithubFill />
              </a>
            )}
          </div>
        </div>
        {previewable ? (
          <CodeBlock language={selected.language || "text"} showLineNumbers>
            {selected.content!}
          </CodeBlock>
        ) : (
          <FileViewer file={selected} routeBasePath={routeBasePath} />
        )}
        <div className={styles.sourceFooter}>
          <span>{selected.language || "Plain text"}</span>
          <Link to={`${routeBasePath}/${selected.path}`}>
            Open file page <RiArrowRightUpLine />
          </Link>
        </div>
      </div>
    </div>
  );
}
