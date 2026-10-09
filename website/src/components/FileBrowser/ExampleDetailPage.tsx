import React, { useEffect, useMemo, useRef } from "react";
import Layout from "@theme/Layout";
import Head from "@docusaurus/Head";
import Link from "@docusaurus/Link";
import { useHistory, useLocation } from "@docusaurus/router";
import {
  RiArrowLeftLine,
  RiArrowRightLine,
  RiArrowRightUpLine,
  RiCodeSSlashLine,
  RiFileTextLine,
  RiGitBranchLine,
  RiGithubFill,
  RiTerminalBoxLine,
} from "react-icons/ri";
import FileViewer from "./FileViewer";
import ExampleDemo from "./ExampleDemo";
import ExampleSourceBrowser from "./ExampleSourceBrowser";
import {
  collectExampleFiles,
  exampleViewUrl,
  relativeExamplePath,
  resolveExampleView,
} from "./example-detail.mjs";
import type { ExampleProject, FileBrowserOptions, FileNode } from "./types";
import styles from "./example-detail.module.css";

interface Props {
  example: ExampleProject;
  options: FileBrowserOptions;
  /** Keep the review URL using the same component as published examples. */
  preview?: boolean;
}

export default function ExampleDetailPage({
  example,
  options,
  preview = false,
}: Props) {
  const location = useLocation();
  const history = useHistory();
  const files: FileNode[] = useMemo(
    () => collectExampleFiles(example.root.children),
    [example.root],
  );
  const { tab, selected } = resolveExampleView(
    location.search,
    files,
    example.name,
  );
  const route = preview
    ? "/examples-detail-design"
    : `${options.routeBasePath}/${example.name}`;
  const title = example.title || example.name;
  const readme = example.root.readme;
  const hasCast = !!example.cast?.file;
  const overviewUrl = exampleViewUrl(route, location.search, "overview");
  const tabs = useRef<HTMLDivElement>(null);
  const keyboardTab = useRef<string | null>(null);
  const sourceFiles = files
    .filter((file) => !file.name.startsWith(".") && file.path !== readme?.path)
    .sort((a, b) => a.path.split("/").length - b.path.split("/").length)
    .slice(0, 3);

  useEffect(() => {
    const target = keyboardTab.current;
    keyboardTab.current = null;
    const frame = requestAnimationFrame(() => {
      if (target)
        tabs.current
          ?.querySelector<HTMLButtonElement>(`#${target}-tab`)
          ?.focus();
      if (tab === "overview" && ["#readme", "#demo"].includes(location.hash)) {
        document
          .getElementById(location.hash.slice(1))
          ?.scrollIntoView({ block: "start" });
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [tab, location.key, location.hash]);

  function navigate(next: string, file: FileNode | undefined = selected) {
    history.push(
      exampleViewUrl(
        route,
        location.search,
        next,
        file && relativeExamplePath(file, example.name),
      ),
    );
  }
  function keyTabs(event: React.KeyboardEvent) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const next =
      event.key === "Home"
        ? "overview"
        : event.key === "End"
          ? "files"
          : tab === "overview"
            ? "files"
            : "overview";
    keyboardTab.current = next;
    navigate(next);
  }

  return (
    <Layout
      title={`${title} - ${options.title || "Examples"}`}
      description={example.description}
    >
      {preview && (
        <Head>
          <meta name="robots" content="noindex,nofollow" />
        </Head>
      )}
      <main className={styles.page}>
        <div className={styles.breadcrumb}>
          <Link to={options.routeBasePath}>
            <RiArrowLeftLine /> {options.title || "Examples"}
          </Link>
          <span>/</span>
          <span>{title}</span>
          {preview && (
            <span className={styles.previewLabel}>Design preview</span>
          )}
        </div>
        <header className={styles.hero}>
          <div className={styles.heroText}>
            <div className={styles.eyebrow}>
              <RiTerminalBoxLine /> EXAMPLE
            </div>
            <h1>{title}</h1>
            {example.description && <p>{example.description}</p>}
            <div className={styles.tags}>
              {example.tags.map((tag) => (
                <span key={tag}>{tag}</span>
              ))}
            </div>
          </div>
          <div className={styles.heroActions}>
            {readme ? (
              <Link className={styles.primary} to={`${overviewUrl}#readme`}>
                Read README <RiArrowRightLine />
              </Link>
            ) : (
              <button
                className={styles.primary}
                type="button"
                onClick={() => navigate("files")}
              >
                Browse files <RiArrowRightLine />
              </button>
            )}
            {example.root.githubUrl && (
              <a className={styles.secondary} href={example.root.githubUrl}>
                <RiGithubFill /> View on GitHub <RiArrowRightUpLine />
              </a>
            )}
          </div>
        </header>
        <div className={styles.tabbar}>
          <div
            role="tablist"
            aria-label="Example views"
            ref={tabs}
            onKeyDown={keyTabs}
          >
            <button
              role="tab"
              id="overview-tab"
              aria-controls="overview-panel"
              aria-selected={tab === "overview"}
              tabIndex={tab === "overview" ? 0 : -1}
              onClick={() => navigate("overview")}
            >
              Overview
            </button>
            <button
              role="tab"
              id="files-tab"
              aria-controls="files-panel"
              aria-selected={tab === "files"}
              tabIndex={tab === "files" ? 0 : -1}
              onClick={() => navigate("files")}
            >
              <RiCodeSSlashLine /> Files <span>{files.length}</span>
            </button>
          </div>
          <span className={styles.tabMeta}>
            <RiGitBranchLine /> {options.githubRepo.replace("/", " / ")}
          </span>
        </div>
        {tab === "files" ? (
          <section
            role="tabpanel"
            id="files-panel"
            aria-labelledby="files-tab"
            tabIndex={0}
          >
            <ExampleSourceBrowser
              example={example}
              files={files}
              selected={selected}
              routeBasePath={options.routeBasePath}
              onSelect={(file) => navigate("files", file)}
            />
          </section>
        ) : (
          <section
            role="tabpanel"
            id="overview-panel"
            aria-labelledby="overview-tab"
            tabIndex={0}
            className={styles.overview}
          >
            <div className={styles.content}>
              {hasCast && (
                <section id="demo">
                  <div className={styles.sectionHeading}>
                    <h2>See it in action</h2>
                    <span>RECORDED DEMO</span>
                  </div>
                  <ExampleDemo
                    cast={{ ...example.cast, file: example.cast!.file! }}
                    name={example.name}
                    options={options}
                  />
                </section>
              )}
              {readme ? (
                <section
                  id="readme"
                  className={styles.readme}
                  aria-label="README"
                >
                  <FileViewer
                    file={readme}
                    routeBasePath={options.routeBasePath}
                  />
                </section>
              ) : (
                <div className={styles.empty}>
                  <p>This example has no README.</p>
                  <button
                    className={styles.secondary}
                    type="button"
                    onClick={() => navigate("files")}
                  >
                    Browse source files <RiArrowRightLine />
                  </button>
                </div>
              )}
            </div>
            <aside className={styles.rail}>
              {(hasCast || readme) && (
                <nav className={styles.onPage} aria-label="On this page">
                  <h2>On this page</h2>
                  {hasCast && <a href="#demo">See it in action</a>}
                  {readme && <a href="#readme">README</a>}
                </nav>
              )}
              {files.length > 0 && (
                <div className={styles.keyFiles}>
                  <h2>Source files</h2>
                  {sourceFiles.map((file) => (
                    <button
                      key={file.path}
                      type="button"
                      title={relativeExamplePath(file, example.name)}
                      onClick={() => navigate("files", file)}
                    >
                      <span>
                        <RiFileTextLine />
                        <span>
                          <strong>{file.name}</strong>
                          <small>
                            {relativeExamplePath(file, example.name)}
                          </small>
                        </span>
                      </span>
                      <RiArrowRightLine />
                    </button>
                  ))}
                  <button
                    className={styles.browseAll}
                    type="button"
                    onClick={() => navigate("files")}
                  >
                    Browse all {files.length} files <RiArrowRightLine />
                  </button>
                </div>
              )}
              {example.docs.length > 0 && (
                <div className={styles.docs}>
                  <h2>Related documentation</h2>
                  {example.docs.map((doc) => (
                    <Link key={doc.url} to={doc.url}>
                      {doc.label}
                      <RiArrowRightUpLine />
                    </Link>
                  ))}
                </div>
              )}
            </aside>
          </section>
        )}
      </main>
    </Layout>
  );
}
