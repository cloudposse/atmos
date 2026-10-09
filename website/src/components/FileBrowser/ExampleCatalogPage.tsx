import React, { useEffect, useRef, useState } from "react";
import Layout from "@theme/Layout";
import Head from "@docusaurus/Head";
import Link from "@docusaurus/Link";
import { useLocation } from "@docusaurus/router";
import useIsBrowser from "@docusaurus/useIsBrowser";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  RiArrowRightLine,
  RiSearchLine,
  RiStarLine,
  RiGridLine,
  RiPauseLine,
  RiPlayLine,
  RiFolderLine,
} from "react-icons/ri";
import CastPlayer from "../CastPlayer";
import type { ExampleProject, ExamplesTree, FileBrowserOptions } from "./types";
import styles from "./example-catalog.module.css";

const layouts = [
  { id: "catalog", label: "01 Catalog" },
  { id: "gallery", label: "02 Gallery" },
  { id: "explorer", label: "03 Explorer" },
] as const;
const markdownComponents = {
  a: ({ children }: { children?: React.ReactNode }) => <>{children}</>,
  p: ({ children }: { children?: React.ReactNode }) => <>{children}</>,
};

function usePlaybackPreference() {
  // Default to still frames during SSR and until the browser preference is known.
  const [reducedMotion, setReducedMotion] = useState(true);
  const [hidden, setHidden] = useState(false);
  useEffect(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const updateMotion = () => setReducedMotion(media.matches);
    const updateVisibility = () => setHidden(document.hidden);
    updateMotion();
    updateVisibility();
    media.addEventListener("change", updateMotion);
    document.addEventListener("visibilitychange", updateVisibility);
    return () => {
      media.removeEventListener("change", updateMotion);
      document.removeEventListener("visibilitychange", updateVisibility);
    };
  }, []);
  return { reducedMotion, hidden };
}

function Preview({
  example,
  animate,
}: {
  example: ExampleProject;
  animate: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    if (typeof IntersectionObserver === "undefined") {
      setVisible(true);
      return;
    }
    const observer = new IntersectionObserver(([entry]) =>
      setVisible(entry.isIntersecting),
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return (
    <div
      ref={ref}
      className={styles.preview}
      aria-hidden="true"
      data-preview={example.name}
    >
      {example.cast?.file ? (
        visible ? (
          <CastPlayer
            key={animate ? "animated" : "still"}
            src={example.cast.file}
            title={example.cast.title || example.title || example.name}
            chrome
            thumbnail
            controls={false}
            scrubber={false}
            showCommand={false}
            static={!animate}
            loopDelay={3}
            className={styles.player}
          />
        ) : (
          <div className={styles.previewPlaceholder}>
            <span>›_</span> Atmos example
          </div>
        )
      ) : (
        <div className={styles.filePreview}>
          <div>
            <RiFolderLine /> {example.name}
          </div>
          <pre>
            {example.root.children
              .slice(0, 4)
              .map(
                (file, index, files) =>
                  `${index === files.length - 1 ? "└─" : "├─"} ${file.name}${file.type === "directory" ? "/" : ""}`,
              )
              .join("\n")}
          </pre>
          <span>
            Explore project files <RiArrowRightLine />
          </span>
        </div>
      )}
    </div>
  );
}

function ExampleCard({
  example,
  animate,
  prominent = false,
}: {
  example: ExampleProject;
  animate: boolean;
  prominent?: boolean;
}) {
  const title = example.title || example.name;
  return (
    <article
      className={`${styles.card} ${prominent ? styles.prominent : ""}`}
      data-example={example.name}
    >
      <Link
        to={`/examples/${example.name}`}
        className={styles.cardLink}
        aria-label={`Open ${title}`}
      >
        <Preview example={example} animate={animate} />
        <div className={styles.cardBody}>
          <div className={styles.cardEyebrow}>
            <span>{example.tags[0] || "Example"}</span>
            {example.featured && (
              <span className={styles.featuredMark}>
                <RiStarLine /> Featured
              </span>
            )}
          </div>
          <h3>
            {title}
            <RiArrowRightLine className={styles.cardArrow} aria-hidden="true" />
          </h3>
          <div className={styles.description}>
            <Markdown
              components={markdownComponents}
              remarkPlugins={[remarkGfm]}
            >
              {example.description || "Explore this example project."}
            </Markdown>
          </div>
          <div className={styles.cardFooter}>
            <div className={styles.tags}>
              {example.tags.map((tag) => (
                <span key={tag}>{tag}</span>
              ))}
            </div>
            <span className={styles.demoLabel}>
              {example.cast?.file ? "Terminal demo" : "Source files"}
            </span>
          </div>
        </div>
      </Link>
    </article>
  );
}

export default function ExampleCatalogPage({
  tree,
  options,
  preview = false,
}: {
  tree: ExamplesTree;
  options: FileBrowserOptions;
  preview?: boolean;
}) {
  const featuredNames = new Set(tree.featured.map((example) => example.name));
  const examples = [
    ...tree.featured,
    ...tree.examples.filter((example) => !featuredNames.has(example.name)),
  ];
  const location = useLocation();
  const isBrowser = useIsBrowser();
  // Match the static catalog markup during hydration before applying URL state.
  const requestedLayout =
    preview && isBrowser
      ? new URLSearchParams(location.search).get("layout")
      : null;
  const layout =
    layouts.find(({ id }) => id === requestedLayout)?.id || "catalog";
  const [category, setCategory] = useState("All");
  const [query, setQuery] = useState("");
  const [paused, setPaused] = useState(false);
  const { reducedMotion, hidden } = usePlaybackPreference();
  const animate = !reducedMotion && !hidden && !paused;
  const normalizedQuery = query.trim().toLowerCase();
  const matchesQuery = (example: ExampleProject) =>
    [example.name, example.title, example.description, ...example.tags]
      .join(" ")
      .toLowerCase()
      .includes(normalizedQuery);
  const matchesCategory = (example: ExampleProject, value: string) =>
    value === "All" ||
    (value === "Featured"
      ? featuredNames.has(example.name)
      : example.tags.includes(value));
  const searchMatches = examples.filter(matchesQuery);
  const filtered = searchMatches.filter((example) =>
    matchesCategory(example, category),
  );
  const categories = ["All", "Featured", ...tree.tags];
  const filtering = category !== "All" || normalizedQuery !== "";
  const showFeatured = layout === "gallery" && !filtering;
  const featured = showFeatured ? tree.featured.slice(0, 2) : [];
  const displayed = filtered.filter(
    (example) => !featured.some((item) => item.name === example.name),
  );
  const reset = () => {
    setQuery("");
    setCategory("All");
  };

  return (
    <Layout
      title={preview ? "Examples · Design previews" : options.title}
      description={options.description}
    >
      {preview && (
        <Head>
          <meta name="robots" content="noindex, nofollow" />
        </Head>
      )}
      <main className={`${styles.page} ${styles[layout]}`} data-layout={layout}>
        {preview && (
          <div className={styles.reviewBar}>
            <span>Design preview</span>
            <nav aria-label="Layout previews">
              {layouts.map(({ id, label }) => (
                <Link
                  key={id}
                  to={`?layout=${id}`}
                  aria-current={id === layout ? "page" : undefined}
                >
                  {label}
                </Link>
              ))}
            </nav>
          </div>
        )}

        <header className={styles.hero}>
          <div>
            <div className={styles.eyebrow}>THE ATMOS LIBRARY</div>
            <h1>Start with an example.</h1>
            <p>
              Real configurations. Working infrastructure.
              <br />
              Find a starting point and make it yours.
            </p>
          </div>
          <Link className={styles.heroLink} to="/examples/quick-start-simple">
            New to Atmos? Start here <RiArrowRightLine />
          </Link>
        </header>

        <div className={styles.workspace}>
          <aside className={styles.sidebar} aria-label="Example categories">
            <div className={styles.sidebarHeading}>Browse examples</div>
            {categories.map((tag) => (
              <button
                key={tag}
                type="button"
                aria-pressed={category === tag}
                onClick={() => setCategory(tag)}
              >
                <span>
                  {tag === "All" ? (
                    <RiGridLine />
                  ) : tag === "Featured" ? (
                    <RiStarLine />
                  ) : null}
                  {tag === "All" ? "All examples" : tag}
                </span>
                <span>
                  {
                    searchMatches.filter((example) =>
                      matchesCategory(example, tag),
                    ).length
                  }
                </span>
              </button>
            ))}
            <p>
              See it in action.
              <br />
              Every demo runs real Atmos commands.
            </p>
          </aside>

          <div className={styles.results}>
            <div className={styles.toolbar}>
              <div className={styles.searchRow}>
                <div className={styles.searchBox}>
                  <RiSearchLine aria-hidden="true" />
                  <input
                    aria-label="Search examples"
                    type="search"
                    placeholder="Search examples, tools, or workflows…"
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                  />
                </div>
                <button
                  className={styles.motionButton}
                  type="button"
                  onClick={() => setPaused((value) => !value)}
                  aria-pressed={paused}
                  disabled={reducedMotion}
                  aria-label={
                    reducedMotion
                      ? "Previews paused for reduced motion"
                      : paused
                        ? "Play previews"
                        : "Pause previews"
                  }
                >
                  {paused || reducedMotion ? <RiPlayLine /> : <RiPauseLine />}
                  <span>
                    {paused || reducedMotion
                      ? "Previews paused"
                      : "Pause previews"}
                  </span>
                </button>
              </div>
              <div
                className={styles.categoryChips}
                aria-label="Filter examples by category"
              >
                {categories.map((tag) => (
                  <button
                    type="button"
                    key={tag}
                    aria-pressed={category === tag}
                    onClick={() => setCategory(tag)}
                  >
                    {tag === "Featured" && <RiStarLine />}
                    {tag}
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
                      {tag} (
                      {
                        searchMatches.filter((example) =>
                          matchesCategory(example, tag),
                        ).length
                      }
                      )
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <div className={styles.resultsHeader}>
              <h2>
                {category === "All" ? "All examples" : category}
                <span role="status" aria-live="polite" aria-atomic="true">
                  {filtered.length}{" "}
                  {filtered.length === 1 ? "example" : "examples"}
                </span>
              </h2>
              {filtering ? (
                <button type="button" onClick={reset} className={styles.reset}>
                  Clear filters <span aria-hidden="true">×</span>
                </button>
              ) : (
                <span className={styles.resultsHint}>
                  Explore the possibilities
                </span>
              )}
            </div>

            {showFeatured && (
              <section
                className={styles.featuredSection}
                aria-label="Featured examples"
              >
                <div className={styles.featuredHeading}>
                  <RiStarLine /> A great place to start
                </div>
                <div className={styles.featuredGrid}>
                  {featured.map((example) => (
                    <ExampleCard
                      key={example.name}
                      example={example}
                      animate={animate}
                      prominent
                    />
                  ))}
                </div>
                <h2 className={styles.moreHeading}>
                  Keep exploring<span>More ways to build with Atmos</span>
                </h2>
              </section>
            )}

            {filtered.length > 0 ? (
              <div className={styles.grid}>
                {displayed.map((example) => (
                  <ExampleCard
                    key={example.name}
                    example={example}
                    animate={animate}
                  />
                ))}
              </div>
            ) : (
              <div className={styles.empty}>
                <RiSearchLine aria-hidden="true" />
                <h3>No examples found</h3>
                <p>Try another search or choose a different category.</p>
                <button type="button" onClick={reset}>
                  Clear filters
                </button>
              </div>
            )}
          </div>
        </div>
      </main>
    </Layout>
  );
}
