import React, { useEffect, useState } from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import styles from "./media-kit.module.css";

const mediaKitDownloadDir = "/downloads/media-kit";

function mediaKitFiles(baseName, svgHref) {
  return [
    { label: `${baseName}.svg`, type: "SVG", href: svgHref },
    {
      label: `${baseName}.png`,
      type: "PNG",
      href: `${mediaKitDownloadDir}/${baseName}.png`,
    },
    {
      label: `${baseName}.jpg`,
      type: "JPG",
      href: `${mediaKitDownloadDir}/${baseName}.jpg`,
    },
  ];
}

const assetGroups = [
  {
    title: "Atmos Mark",
    variant: "Color",
    kind: "mark",
    usage: "Use the full-color mark where the brand icon can stand on its own.",
    preview: "/img/atmos-logo.svg",
    previewTone: "light",
    files: mediaKitFiles("atmos-mark-color", "/img/atmos-logo.svg"),
  },
  {
    title: "Atmos Mark",
    variant: "Monochrome",
    kind: "mark",
    usage:
      "Use the monochrome mark for restrained placements and dark UI surfaces.",
    preview: "/img/atmos-logo-bw.svg",
    previewTone: "dark",
    files: mediaKitFiles("atmos-mark-monochrome", "/img/atmos-logo-bw.svg"),
  },
  {
    title: "Atmos Wordmark",
    variant: "Light",
    kind: "wordmark",
    usage:
      "Use on dark backgrounds when the full Atmos name should be visible.",
    preview: "/img/atmos-docs-logo-light.svg",
    previewTone: "dark",
    files: mediaKitFiles(
      "atmos-wordmark-light",
      "/img/atmos-docs-logo-light.svg",
    ),
  },
  {
    title: "Atmos Wordmark",
    variant: "Dark",
    kind: "wordmark",
    usage:
      "Use on light backgrounds when the full Atmos name should be visible.",
    preview: "/img/atmos-docs-logo-dark.svg",
    previewTone: "light",
    files: mediaKitFiles(
      "atmos-wordmark-dark",
      "/img/atmos-docs-logo-dark.svg",
    ),
  },
];

const animatedWordmarks = [
  {
    title: "Atmos Logo Gradient",
    variant: "On dark",
    tone: "dark",
    usage:
      "The animated gradient Atmos lockup for dark header and sidebar placements.",
    preview: "/img/atmos-logo-gradient.svg",
    files: mediaKitFiles("atmos-logo-gradient", "/img/atmos-logo-gradient.svg"),
  },
  {
    title: "Atmos Logo Gradient",
    variant: "On light",
    tone: "light",
    usage:
      "The same animated lockup tuned for white and light neutral backgrounds.",
    preview: "/img/atmos-logo-gradient-on-light.svg",
    files: mediaKitFiles(
      "atmos-logo-gradient-on-light",
      "/img/atmos-logo-gradient-on-light.svg",
    ),
  },
  {
    title: "Atmos CI Gradient",
    variant: "CI on dark",
    tone: "dark",
    usage:
      "The animated Atmos CI lockup for dark native CI summaries, CI docs, and CI-related placements.",
    preview: "/img/atmos-ci-gradient.svg",
    files: mediaKitFiles("atmos-ci-gradient", "/img/atmos-ci-gradient.svg"),
  },
  {
    title: "Atmos CI Gradient",
    variant: "CI on light",
    tone: "light",
    usage:
      "The animated Atmos CI lockup tuned for white and light neutral CI placements.",
    preview: "/img/atmos-ci-gradient-on-light.svg",
    files: mediaKitFiles(
      "atmos-ci-gradient-on-light",
      "/img/atmos-ci-gradient-on-light.svg",
    ),
  },
  {
    title: "Atmos AI Gradient",
    variant: "AI on dark",
    tone: "dark",
    usage:
      "The animated Atmos AI lockup for dark AI features, AI docs, and assistant-related placements.",
    preview: "/img/atmos-ai-gradient.svg",
    files: mediaKitFiles("atmos-ai-gradient", "/img/atmos-ai-gradient.svg"),
  },
  {
    title: "Atmos AI Gradient",
    variant: "AI on light",
    tone: "light",
    usage:
      "The animated Atmos AI lockup tuned for white and light neutral AI placements.",
    preview: "/img/atmos-ai-gradient-on-light.svg",
    files: mediaKitFiles(
      "atmos-ai-gradient-on-light",
      "/img/atmos-ai-gradient-on-light.svg",
    ),
  },
];

const poweredBy = [
  {
    title: "Powered by atmos",
    variant: "Dark badge",
    tone: "light",
    usage:
      "Use on light surfaces when a project wants to show it runs on Atmos.",
    preview: "/img/powered-by-atmos-dark.svg",
    files: mediaKitFiles(
      "powered-by-atmos-dark",
      "/img/powered-by-atmos-dark.svg",
    ),
  },
  {
    title: "Powered by atmos",
    variant: "Light badge",
    tone: "dark",
    usage:
      "Use on dark surfaces when a project wants to show it runs on Atmos.",
    preview: "/img/powered-by-atmos-light.svg",
    files: mediaKitFiles(
      "powered-by-atmos-light",
      "/img/powered-by-atmos-light.svg",
    ),
  },
  {
    title: "Powered by atmos CI",
    variant: "CI gradient badge",
    tone: "dark",
    usage:
      "Use on dark surfaces for projects, examples, and CI summaries that specifically run on Atmos CI.",
    preview: "/img/powered-by-atmos-gradient.svg",
    files: mediaKitFiles(
      "powered-by-atmos-gradient",
      "/img/powered-by-atmos-gradient.svg",
    ),
  },
  {
    title: "Powered by atmos CI",
    variant: "CI gradient on light",
    tone: "light",
    usage:
      "The same Atmos CI badge tuned for white and light neutral surfaces.",
    preview: "/img/powered-by-atmos-gradient-on-light.svg",
    files: mediaKitFiles(
      "powered-by-atmos-gradient-on-light",
      "/img/powered-by-atmos-gradient-on-light.svg",
    ),
  },
];

// Atmos brand palette, sourced from the official logo assets.
const brandColors = [
  { name: "Atmos Green", hex: "#60e862" },
  { name: "Glow Green", hex: "#97f597" },
  { name: "Leaf Green", hex: "#42b14c" },
  { name: "Deep Green", hex: "#29a54c" },
  { name: "Forest", hex: "#093309" },
  { name: "Ink", hex: "#231f20" },
  { name: "White", hex: "#ffffff" },
];

// Animated accent palette, sourced from the live web treatment.
const accentColors = [
  { name: "Primary", hex: "#3578e5" },
  { name: "Iris", hex: "#583ce7" },
  { name: "Sky", hex: "#2ec8ff" },
  { name: "Mint", hex: "#23d5ab" },
  { name: "Glow Green", hex: "#97f597" },
];

// Dark surfaces used behind the badges and the animated web treatment.
const surfaceColors = [
  { name: "Deep Navy", hex: "#0d141d" },
  { name: "Midnight", hex: "#050914" },
];

const guidelines = {
  do: [
    "Use the official logo files from this page.",
    "Maintain adequate clear space around the mark.",
    "Use the light wordmark on dark backgrounds.",
    "Use the dark wordmark on light backgrounds.",
    "Scale the logo proportionally.",
  ],
  dont: [
    "Alter or distort the logo colors or proportions.",
    "Add effects such as shadows or outlines (the animated gradient is the only approved effect).",
    "Place the logo on busy or low-contrast backgrounds.",
    "Recreate the logo using other typefaces or icons.",
    "Use the logo to imply endorsement without permission.",
  ],
};

const facts = [
  { label: "Product", value: "Atmos" },
  { label: "Category", value: "DevOps and cloud automation" },
  { label: "Platform", value: "CLI, Go library, and Terraform provider" },
  { label: "Project type", value: "Open source" },
  {
    label: "License",
    value: "Apache-2.0",
    href: "https://github.com/cloudposse/atmos/blob/main/LICENSE",
  },
  { label: "Since", value: "2020" },
  { label: "Maintainer", value: "Cloud Posse", href: "https://cloudposse.com" },
  { label: "Website", value: "atmos.tools", href: "https://atmos.tools" },
  {
    label: "GitHub",
    value: "cloudposse/atmos",
    href: "https://github.com/cloudposse/atmos",
  },
  {
    label: "Community",
    value: "SweetOps Slack",
    href: "https://slack.cloudposse.com",
  },
];

function absoluteAssetUrl(href) {
  if (typeof window === "undefined") {
    return href;
  }

  return new URL(href, window.location.origin).href;
}

function useCopyFeedback() {
  const [copied, setCopied] = useState(null);

  function markCopied(kind) {
    setCopied(kind);
    window.setTimeout(() => setCopied(null), 1600);
  }

  return [copied, markCopied];
}

function PreviewActions({ href, label }) {
  const [copied, markCopied] = useCopyFeedback();

  async function copyLink() {
    await navigator.clipboard.writeText(absoluteAssetUrl(href));
    markCopied("link");
  }

  async function copyImage() {
    const url = absoluteAssetUrl(href);

    try {
      if (!window.ClipboardItem || !navigator.clipboard?.write) {
        throw new Error("Image clipboard is not available");
      }

      const response = await fetch(url);
      const blob = await response.blob();
      await navigator.clipboard.write([
        new ClipboardItem({ [blob.type || "image/svg+xml"]: blob }),
      ]);
      markCopied("image");
    } catch {
      await navigator.clipboard.writeText(url);
      markCopied("link");
    }
  }

  return (
    <div className={styles.previewActions} aria-label={`${label} actions`}>
      <button type="button" onClick={copyImage}>
        {copied === "image" ? "Copied" : "Copy image"}
      </button>
      <button type="button" onClick={copyLink}>
        {copied === "link" ? "Copied" : "Copy link"}
      </button>
    </div>
  );
}

function AssetPreview({ href, label, tone, className = "", children }) {
  return (
    <div className={`${styles.preview} ${styles[tone]} ${className}`}>
      {children}
      {href ? <PreviewActions href={href} label={label} /> : null}
    </div>
  );
}

function AssetCard({ asset }) {
  return (
    <article className={styles.assetCard}>
      <AssetPreview
        href={asset.preview}
        label={`${asset.title} ${asset.variant}`}
        tone={asset.previewTone}
        className={asset.kind === "wordmark" ? styles.wordmark : ""}
      >
        <img src={asset.preview} alt={`${asset.title} ${asset.variant}`} />
      </AssetPreview>
      <div className={styles.assetBody}>
        <div>
          <p className={styles.assetKicker}>{asset.variant}</p>
          <h2>{asset.title}</h2>
          <p>{asset.usage}</p>
        </div>
        <div className={styles.fileList}>
          {asset.files.map((file) => (
            <a
              key={file.href}
              className={styles.fileLink}
              href={file.href}
              download
            >
              <span>{file.label}</span>
              <strong>{file.type}</strong>
            </a>
          ))}
        </div>
      </div>
    </article>
  );
}

function AnimatedWordmarkCard({ lockup }) {
  return (
    <article className={styles.assetCard}>
      <AssetPreview
        href={lockup.preview}
        label={`${lockup.title}, ${lockup.variant}`}
        tone={lockup.tone}
      >
        <img
          className={styles.animatedWordmarkPreview}
          src={lockup.preview}
          alt={`${lockup.title}, ${lockup.variant}`}
        />
      </AssetPreview>
      <div className={styles.assetBody}>
        <div>
          <p className={styles.assetKicker}>{lockup.variant}</p>
          <h2>{lockup.title}</h2>
          <p>{lockup.usage}</p>
        </div>
        <div className={styles.fileList}>
          {lockup.files.map((file) => (
            <a
              key={file.href}
              className={styles.fileLink}
              href={file.href}
              download
            >
              <span>{file.label}</span>
              <strong>{file.type}</strong>
            </a>
          ))}
        </div>
      </div>
    </article>
  );
}

function PoweredByCard({ badge }) {
  return (
    <article className={styles.assetCard}>
      <AssetPreview
        href={badge.preview}
        label={`${badge.title} ${badge.variant}`}
        tone={badge.tone}
        className={styles.badgePreview}
      >
        <img src={badge.preview} alt={`${badge.title}, ${badge.variant}`} />
      </AssetPreview>
      <div className={styles.assetBody}>
        <div>
          <p className={styles.assetKicker}>{badge.variant}</p>
          <h2>{badge.title}</h2>
          <p>{badge.usage}</p>
        </div>
        <div className={styles.fileList}>
          {badge.files.map((file) => (
            <a
              key={file.href}
              className={styles.fileLink}
              href={file.href}
              download
            >
              <span>{file.label}</span>
              <strong>{file.type}</strong>
            </a>
          ))}
        </div>
      </div>
    </article>
  );
}

function useGithubStars(repo) {
  const [stars, setStars] = useState(null);

  useEffect(() => {
    let active = true;
    fetch(`https://api.github.com/repos/${repo}`)
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (active && data && typeof data.stargazers_count === "number") {
          setStars(data.stargazers_count);
        }
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [repo]);

  return stars;
}

function ColorSwatches({ colors }) {
  return (
    <div className={styles.colorGrid}>
      {colors.map((color) => (
        <div key={color.hex} className={styles.swatch}>
          <div
            className={styles.swatchChip}
            style={{ background: color.hex }}
          />
          <div className={styles.swatchMeta}>
            <div className={styles.swatchName}>{color.name}</div>
            <div className={styles.swatchHex}>{color.hex}</div>
          </div>
        </div>
      ))}
    </div>
  );
}

export default function MediaKitPage() {
  const stars = useGithubStars("cloudposse/atmos");

  return (
    <Layout
      title="Atmos media kit"
      description="Official Atmos brand assets, product description, and links."
    >
      <main className={styles.mediaKit}>
        <section className={styles.hero}>
          <p className={styles.eyebrow}>Media Kit</p>
          <h1>Atmos media kit</h1>
          <p className={styles.lede}>
            Official Atmos brand assets, product description, and links.
          </p>
          <p>
            Download the logo set, use individual SVG and PNG files, and
            reference approved language for describing Atmos.
          </p>
        </section>

        <section
          className={styles.downloadPanel}
          aria-labelledby="media-kit-download"
        >
          <div className={styles.downloadIcon} aria-hidden="true">
            ZIP
          </div>
          <div>
            <h2 id="media-kit-download">Media kit download</h2>
            <p>Light and dark SVGs, plus generated PNG and JPG exports.</p>
          </div>
          <a
            className="button button--primary"
            href="/downloads/atmos-media-kit.zip"
            download
          >
            Download ZIP
          </a>
        </section>

        <section className={styles.assetGrid} aria-label="Atmos brand assets">
          {assetGroups.map((asset) => (
            <AssetCard key={`${asset.title}-${asset.variant}`} asset={asset} />
          ))}
        </section>

        <section
          className={styles.aboutSection}
          aria-labelledby="animated-mark"
        >
          <h2 id="animated-mark">Animated web treatment</h2>
          <div className={styles.assetGrid}>
            {animatedWordmarks.map((lockup) => (
              <AnimatedWordmarkCard
                key={`${lockup.title}-${lockup.variant}`}
                lockup={lockup}
              />
            ))}
          </div>
        </section>

        <section className={styles.aboutSection} aria-labelledby="powered-by">
          <h2 id="powered-by">Powered by Atmos</h2>
          <p className={styles.sectionLede}>
            Show that your project runs on Atmos using real SVG badges from the
            media kit. Use the dark badge on light surfaces, the light badge on
            dark surfaces, and the CI gradient badge — in dark or light — for
            Atmos CI placements.
          </p>
          <div className={styles.assetGrid}>
            {poweredBy.map((badge) => (
              <PoweredByCard key={badge.variant} badge={badge} />
            ))}
          </div>
        </section>

        <section className={styles.aboutSection} aria-labelledby="brand-colors">
          <h2 id="brand-colors">Brand colors</h2>
          <p className={styles.sectionLede}>
            Core Atmos colors come from the official logo artwork and should be
            used for brand-forward placements.
          </p>
          <ColorSwatches colors={brandColors} />
        </section>

        <section
          className={styles.aboutSection}
          aria-labelledby="animated-accent-colors"
        >
          <h2 id="animated-accent-colors">Animated accent colors</h2>
          <p className={styles.sectionLede}>
            These colors support the animated web treatment used in the site
            header and live mark effects. They are cool motion accents, not the
            canonical brand palette.
          </p>
          <ColorSwatches colors={accentColors} />
        </section>

        <section className={styles.aboutSection} aria-labelledby="surface-colors">
          <h2 id="surface-colors">Surface colors</h2>
          <p className={styles.sectionLede}>
            The deep navy backgrounds used behind the dark badges and the
            animated web treatment. Pair them with the light wordmarks and the
            animated gradient for high-contrast, brand-forward dark surfaces.
          </p>
          <ColorSwatches colors={surfaceColors} />
        </section>

        <section
          className={styles.aboutSection}
          aria-labelledby="usage-guidelines"
        >
          <h2 id="usage-guidelines">Usage guidelines</h2>
          <div className={styles.guidelines}>
            <div className={`${styles.guideCol} ${styles.do}`}>
              <div className={`${styles.guideHead} ${styles.do}`}>Do</div>
              <ul className={styles.guideList}>
                {guidelines.do.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </div>
            <div className={`${styles.guideCol} ${styles.dont}`}>
              <div className={`${styles.guideHead} ${styles.dont}`}>Don't</div>
              <ul className={styles.guideList}>
                {guidelines.dont.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </div>
          </div>
        </section>

        <section className={styles.aboutSection} aria-labelledby="about-atmos">
          <h2 id="about-atmos">About Atmos</h2>
          <div className={styles.aboutPanel}>
            <div className={styles.copyBlock}>
              <h3>Description</h3>
              <p>
                Atmos is a universal tool for DevOps and cloud automation. It
                orchestrates infrastructure workflows across Terraform,
                OpenTofu, Packer, Helmfile, Ansible, Devcontainers, and related
                toolchains.
              </p>
              <p>
                Teams use Atmos to break infrastructure into reusable
                components, tie environments together with YAML stack
                configurations, reduce duplication, and run consistent workflows
                locally and in CI/CD.
              </p>
            </div>
            <div className={styles.copyBlock}>
              <h3>Short description</h3>
              <p>
                Atmos is an open source DevOps automation framework for
                orchestrating infrastructure workflows across cloud and platform
                engineering toolchains.
              </p>
            </div>
            <dl className={styles.factGrid}>
              {facts.map((fact) => (
                <div key={fact.label} className={styles.factItem}>
                  <dt>{fact.label}</dt>
                  <dd>
                    {fact.href ? (
                      <a href={fact.href} target="_blank" rel="noreferrer">
                        {fact.value}
                      </a>
                    ) : (
                      fact.value
                    )}
                  </dd>
                </div>
              ))}
              {stars !== null && (
                <div className={styles.factItem}>
                  <dt>GitHub stars</dt>
                  <dd>
                    <a
                      href="https://github.com/cloudposse/atmos/stargazers"
                      target="_blank"
                      rel="noreferrer"
                    >
                      {stars.toLocaleString()}
                    </a>
                  </dd>
                </div>
              )}
            </dl>
          </div>
        </section>

        <section
          className={styles.aboutSection}
          aria-labelledby="maintained-by"
        >
          <h2 id="maintained-by">Maintained by Cloud Posse</h2>
          <p className={styles.sectionLede}>
            Atmos is built and maintained by Cloud Posse, a DevOps accelerator
            for cloud-native startups and enterprises.
          </p>
          <a
            className={styles.maintainerLogo}
            href="https://cloudposse.com"
            target="_blank"
            rel="noreferrer"
            aria-label="Cloud Posse — visit cloudposse.com"
          >
            <img
              className={styles.logoForLightTheme}
              src="/img/cloudposse-logo-dark.svg"
              alt="Cloud Posse"
            />
            <img
              className={styles.logoForDarkTheme}
              src="/img/cloudposse-logo-light.svg"
              alt="Cloud Posse"
            />
          </a>
        </section>

        <section
          className={styles.linksSection}
          aria-labelledby="official-links"
        >
          <h2 id="official-links">Official links</h2>
          <div className={styles.linksGrid}>
            <Link to="/">Website</Link>
            <Link to="/intro">Docs</Link>
            <Link to="/changelog">Changelog</Link>
            <a
              href="https://github.com/cloudposse/atmos"
              target="_blank"
              rel="noreferrer"
            >
              GitHub
            </a>
            <a
              href="https://slack.cloudposse.com"
              target="_blank"
              rel="noreferrer"
            >
              Community
            </a>
          </div>
        </section>
      </main>
    </Layout>
  );
}
