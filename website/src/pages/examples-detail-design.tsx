import React, { useEffect, useRef, useState } from "react";
import Layout from "@theme/Layout";
import Head from "@docusaurus/Head";
import Link from "@docusaurus/Link";
import CodeBlock from "@theme/CodeBlock";
import { useHistory, useLocation } from "@docusaurus/router";
import {
  RiArrowLeftLine, RiArrowRightLine, RiArrowRightUpLine, RiCheckLine,
  RiCodeSSlashLine, RiFileTextLine, RiFolderLine, RiGitBranchLine,
  RiGithubFill, RiPlayFill, RiRestartLine, RiSearchLine, RiTerminalBoxLine,
  RiFileCopyLine, RiFullscreenLine, RiShareLine, RiDownloadLine,
} from "react-icons/ri";
import CastPlayer from "../components/CastPlayer";
import FileViewer from "../components/FileBrowser/FileViewer";
import type { ExamplesTree, FileNode, TreeNode } from "../components/FileBrowser/types";
import treeData from "@generated/file-browser/examples/file-browser-tree-examples.json";
import styles from "./examples-detail-design.module.css";

// Isolated design review. Production example routes and shared UI are unchanged.
const tree = treeData as ExamplesTree;
const example = [...tree.examples, ...tree.featured].find((item) => item.name === "quick-start-simple")!;
const route = "/examples-detail-design";
const rootPath = `${example.name}/`;
function collectFiles(nodes: TreeNode[]): FileNode[] {
  return nodes.flatMap((node) => node.type === "file" ? [node] : collectFiles(node.children));
}
const files = collectFiles(example.root.children);
const initialFile = files.find((file) => file.name === "atmos.yaml")!;
const relative = (file: FileNode) => file.path.slice(rootPath.length);

function CopyButton({ text, label = "Copy command", icon }: { text: string | (() => string); label?: string; icon?: React.ReactNode }) {
  const [status, setStatus] = useState("");
  const timer = useRef<ReturnType<typeof setTimeout>>();
  useEffect(() => () => clearTimeout(timer.current), []);
  async function copy() {
    try {
      await navigator.clipboard.writeText(typeof text === "function" ? text() : text);
      setStatus("Copied");
    } catch {
      setStatus("Copy unavailable");
    }
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setStatus(""), 2000);
  }
  return <button className={styles.copy} type="button" onClick={copy} aria-label={status || label} title={status || label}>
    {status === "Copied" ? <RiCheckLine /> : icon || <RiFileCopyLine />}
    <span className={styles.srOnly} role="status">{status}</span>
  </button>;
}

function Demo() {
  const [play, setPlay] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const [full, setFull] = useState(false);
  useEffect(() => {
    const update = () => setFull(document.fullscreenElement === container.current);
    document.addEventListener("fullscreenchange", update);
    return () => document.removeEventListener("fullscreenchange", update);
  }, []);
  async function expand() {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else if (container.current?.requestFullscreen) await container.current.requestFullscreen();
      else setFull((value) => !value);
    } catch { setFull((value) => !value); }
  }
  return <div ref={container} className={`${styles.demo} ${full ? styles.expanded : ""}`}>
    <div className={styles.demoToolbar}>
      <span><RiTerminalBoxLine /> quick-start-simple</span>
      <span className={styles.demoActions}>
        <CopyButton text={() => `${window.location.origin}${route}#demo`} label="Copy demo link" icon={<RiShareLine />} />
        <a href={example.cast!.file} download aria-label="Download recording" title="Download recording"><RiDownloadLine /></a>
        <button type="button" onClick={expand} aria-label={full ? "Exit expanded demo" : "Expand demo"}><RiFullscreenLine /></button>
      </span>
    </div>
    <CastPlayer key={play ? "play" : "poster"} src={example.cast!.file!}
      title="Quick Start Simple" static={!play} autoplay={play} loop={false}
      controls={play} scrubber showCommand={false} className={styles.player} />
    {!play && <div className={styles.demoFooter}>
      <button type="button" className={styles.watch} onClick={() => setPlay(true)}><RiPlayFill /> Watch walkthrough</button>
      <span>Recorded from this example</span>
    </div>}
    {play && <button type="button" className={styles.resetDemo} onClick={() => setPlay(false)}><RiRestartLine /> Back to preview</button>}
  </div>;
}

function SourceTree({ nodes, selected, onSelect }: { nodes: TreeNode[]; selected: string; onSelect: (file: FileNode) => void }) {
  return <ul className={styles.tree}>
    {nodes.map((node) => <li key={node.path}>
      {node.type === "directory" ? <details open={selected.startsWith(`${node.path}/`) || node.name === "stacks" || node.path.includes("/stacks/")}>
        <summary><RiFolderLine /> {node.name}</summary>
        <SourceTree nodes={node.children} selected={selected} onSelect={onSelect} />
      </details> : <button type="button" aria-current={node.path === selected ? "true" : undefined}
        className={node.path === selected ? styles.selected : ""} onClick={() => onSelect(node)}>
        <RiFileTextLine /> {node.name}
      </button>}
    </li>)}
  </ul>;
}

function SourceBrowser({ selected, onSelect }: { selected: FileNode; onSelect: (file: FileNode) => void }) {
  const [search, setSearch] = useState("");
  const matches = files.filter((file) => relative(file).toLowerCase().includes(search.toLowerCase()));
  return <div className={styles.sourceBrowser}>
    <aside className={styles.sourceSidebar} aria-label="Example files">
      <label className={styles.fileSearch}><RiSearchLine /><input aria-label="Find a file" placeholder="Find a file…" value={search} onChange={(event) => setSearch(event.target.value)} /></label>
      <div className={styles.treeTitle}>QUICK-START-SIMPLE</div>
      {search ? <ul className={styles.tree}>{matches.map((file) => <li key={file.path}>
        <button type="button" className={file.path === selected.path ? styles.selected : ""} onClick={() => onSelect(file)}><RiFileTextLine /> {relative(file)}</button>
      </li>)}{!matches.length && <li className={styles.noFiles}>No matching files. <button onClick={() => setSearch("")} type="button">Clear search</button></li>}</ul> :
        <SourceTree nodes={example.root.children} selected={selected.path} onSelect={onSelect} />}
    </aside>
    <div className={styles.sourceMain}>
      <label className={styles.mobileFiles}>File
        <select value={selected.path} onChange={(event) => onSelect(files.find((file) => file.path === event.target.value)!)}>
          {files.map((file) => <option key={file.path} value={file.path}>{relative(file)}</option>)}
        </select>
      </label>
      <div className={styles.sourceToolbar}>
        <span><RiFileTextLine /> {relative(selected)}</span>
        <div><CopyButton text={selected.content || ""} label="Copy file contents" />
          {selected.githubUrl && <a href={selected.githubUrl} aria-label="View selected file on GitHub" title="View on GitHub"><RiGithubFill /></a>}
        </div>
      </div>
      <CodeBlock language={selected.language || "text"} showLineNumbers>{selected.content || "# Empty file"}</CodeBlock>
      <div className={styles.sourceFooter}><span>{selected.language || "Plain text"}</span><Link to={`/examples/${selected.path}`}>Open file page <RiArrowRightUpLine /></Link></div>
    </div>
  </div>;
}

export default function ExampleDetailDesign() {
  const location = useLocation();
  const history = useHistory();
  const params = new URLSearchParams(location.search);
  const tab = params.get("tab") === "files" ? "files" : "overview";
  const selected = files.find((file) => relative(file) === params.get("file")) || initialFile;
  const tabs = useRef<HTMLDivElement>(null);
  const keyboardTab = useRef<string | null>(null);
  useEffect(() => {
    const target = keyboardTab.current;
    keyboardTab.current = null;
    const frame = requestAnimationFrame(() => {
      if (target) tabs.current?.querySelector<HTMLButtonElement>(`[id="${target}-tab"]`)?.focus();
      if (location.hash === "#readme") document.getElementById("readme")?.scrollIntoView({ block: "start" });
    });
    return () => cancelAnimationFrame(frame);
  }, [tab, location.hash]);
  function navigate(next: string, file = selected) {
    history.push(`${route}${next === "files" ? `?tab=files&file=${encodeURIComponent(relative(file))}` : ""}`);
  }
  function keyTabs(event: React.KeyboardEvent) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const next = event.key === "Home" ? "overview" : event.key === "End" ? "files" : tab === "overview" ? "files" : "overview";
    keyboardTab.current = next;
    navigate(next);
  }
  return <Layout title={`${example.title || example.name} · Design preview`} description={example.description}>
    <Head><meta name="robots" content="noindex,nofollow" /></Head>
    <main className={styles.page}>
      <div className={styles.breadcrumb}><Link to="/examples"><RiArrowLeftLine /> Examples</Link><span>/</span><span>{example.title || example.name}</span><span className={styles.previewLabel}>Design preview</span></div>
      <header className={styles.hero}>
        <div className={styles.heroText}>
          <div className={styles.eyebrow}><RiTerminalBoxLine /> EXAMPLE</div>
          <h1>{example.title || example.name}</h1>
          <p>{example.description}</p>
          <div className={styles.tags}>{example.tags.map(tag => <span key={tag}>{tag}</span>)}</div>
        </div>
        <div className={styles.heroActions}>
          <Link className={styles.primary} to={`${route}#readme`}>Read README <RiArrowRightLine /></Link>
          <a className={styles.secondary} href={example.root.githubUrl || "https://github.com/cloudposse/atmos/tree/main/examples/quick-start-simple"}><RiGithubFill /> View on GitHub <RiArrowRightUpLine /></a>
        </div>
      </header>
      <div className={styles.tabbar}>
        <div role="tablist" aria-label="Example views" ref={tabs} onKeyDown={keyTabs}>
          <button role="tab" id="overview-tab" aria-controls="overview-panel" aria-selected={tab === "overview"} tabIndex={tab === "overview" ? 0 : -1} onClick={() => navigate("overview")}>Overview</button>
          <button role="tab" id="files-tab" aria-controls="files-panel" aria-selected={tab === "files"} tabIndex={tab === "files" ? 0 : -1} onClick={() => navigate("files")}><RiCodeSSlashLine /> Files <span>{files.length}</span></button>
        </div>
        <span className={styles.tabMeta}><RiGitBranchLine /> cloudposse / atmos</span>
      </div>
      {tab === "files" ? <section role="tabpanel" id="files-panel" aria-labelledby="files-tab" className={styles.filesPanel}>
        <SourceBrowser selected={selected} onSelect={(file) => navigate("files", file)} />
      </section> : <section role="tabpanel" id="overview-panel" aria-labelledby="overview-tab" className={styles.overview}>
        <div className={styles.content}>
          <section id="demo" className={styles.section}>
            <div className={styles.sectionHeading}><h2>See it in action</h2><span>RECORDED DEMO</span></div>
            <Demo />
            <p className={styles.caption}>Real commands. Real output. Follow along at your own pace.</p>
          </section>
          {example.root.readme && <section id="readme" className={styles.readme} aria-label="README">
            <FileViewer file={example.root.readme} routeBasePath="/examples" />
          </section>}
        </div>
        <aside className={styles.rail}>
          <nav className={styles.onPage} aria-label="On this page"><h2>On this page</h2><a href="#demo">See it in action</a><a href="#readme">README</a></nav>
          <div className={styles.keyFiles}><h2>Source files</h2>
            {files.filter(file => !file.name.startsWith(".") && file.name !== "README.md").slice(0, 3).map(file => <button key={file.path} type="button" onClick={() => navigate("files", file)}><span><RiFileTextLine /><strong>{relative(file)}</strong></span><RiArrowRightLine /></button>)}
            <button className={styles.browseAll} type="button" onClick={() => navigate("files")}>Browse all {files.length} files <RiArrowRightLine /></button>
          </div>
          <div className={styles.docs}><h2>Go a little deeper</h2>{example.docs.slice(0, 3).map((doc) => <Link key={doc.url} to={doc.url}>{doc.label}<RiArrowRightUpLine /></Link>)}</div>
        </aside>
      </section>}
    </main>
  </Layout>;
}
