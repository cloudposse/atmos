import React, { useEffect, useState } from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import styles from "./media-kit.module.css";

const assetGroups = [
  {
    title: "Atmos Mark",
    variant: "Color",
    kind: "mark",
    usage: "Use the full-color mark where the brand icon can stand on its own.",
    preview: "/img/atmos-logo.svg",
    previewTone: "light",
    files: [
      { label: "atmos-logo.svg", type: "SVG", href: "/img/atmos-logo.svg" },
      {
        label: "atmos-logo.png - 111 x 128",
        type: "PNG",
        href: "/img/atmos-logo.png",
      },
    ],
  },
  {
    title: "Atmos Mark",
    variant: "Monochrome",
    kind: "mark",
    usage:
      "Use the monochrome mark for restrained placements and dark UI surfaces.",
    preview: "/img/atmos-logo-bw.svg",
    previewTone: "dark",
    files: [
      {
        label: "atmos-logo-bw.svg",
        type: "SVG",
        href: "/img/atmos-logo-bw.svg",
      },
    ],
  },
  {
    title: "Atmos Wordmark",
    variant: "Light",
    kind: "wordmark",
    usage:
      "Use on dark backgrounds when the full Atmos name should be visible.",
    preview: "/img/atmos-docs-logo-light.svg",
    previewTone: "dark",
    files: [
      {
        label: "atmos-docs-logo-light.svg",
        type: "SVG",
        href: "/img/atmos-docs-logo-light.svg",
      },
    ],
  },
  {
    title: "Atmos Wordmark",
    variant: "Dark",
    kind: "wordmark",
    usage:
      "Use on light backgrounds when the full Atmos name should be visible.",
    preview: "/img/atmos-docs-logo-dark.svg",
    previewTone: "light",
    files: [
      {
        label: "atmos-docs-logo-dark.svg",
        type: "SVG",
        href: "/img/atmos-docs-logo-dark.svg",
      },
    ],
  },
];

const animatedMarks = [
  {
    variant: "On dark",
    tone: "dark",
    usage:
      "The animated mark on a dark surface, matching the app and docs header.",
  },
  {
    variant: "On light",
    tone: "light",
    usage:
      "The animated mark on a light surface, using the monochrome mark with cool green, blue, and purple motion from the site UI.",
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
    files: [
      {
        label: "atmos-logo-gradient.svg",
        type: "SVG",
        href: "/img/atmos-logo-gradient.svg",
      },
    ],
  },
  {
    title: "Atmos Logo Gradient",
    variant: "On light",
    tone: "light",
    usage:
      "The same animated lockup tuned for white and light neutral backgrounds.",
    preview: "/img/atmos-logo-gradient-on-light.svg",
    files: [
      {
        label: "atmos-logo-gradient-on-light.svg",
        type: "SVG",
        href: "/img/atmos-logo-gradient-on-light.svg",
      },
    ],
  },
  {
    title: "Atmos CI Gradient",
    variant: "CI on dark",
    tone: "dark",
    usage:
      "The animated Atmos CI lockup for dark native CI summaries, CI docs, and CI-related placements.",
    preview: "/img/atmos-ci-gradient.svg",
    files: [
      {
        label: "atmos-ci-gradient.svg",
        type: "SVG",
        href: "/img/atmos-ci-gradient.svg",
      },
    ],
  },
  {
    title: "Atmos CI Gradient",
    variant: "CI on light",
    tone: "light",
    usage:
      "The animated Atmos CI lockup tuned for white and light neutral CI placements.",
    preview: "/img/atmos-ci-gradient-on-light.svg",
    files: [
      {
        label: "atmos-ci-gradient-on-light.svg",
        type: "SVG",
        href: "/img/atmos-ci-gradient-on-light.svg",
      },
    ],
  },
  {
    title: "Atmos AI Gradient",
    variant: "AI on dark",
    tone: "dark",
    usage:
      "The animated Atmos AI lockup for dark AI features, AI docs, and assistant-related placements.",
    preview: "/img/atmos-ai-gradient.svg",
    files: [
      {
        label: "atmos-ai-gradient.svg",
        type: "SVG",
        href: "/img/atmos-ai-gradient.svg",
      },
    ],
  },
  {
    title: "Atmos AI Gradient",
    variant: "AI on light",
    tone: "light",
    usage:
      "The animated Atmos AI lockup tuned for white and light neutral AI placements.",
    preview: "/img/atmos-ai-gradient-on-light.svg",
    files: [
      {
        label: "atmos-ai-gradient-on-light.svg",
        type: "SVG",
        href: "/img/atmos-ai-gradient-on-light.svg",
      },
    ],
  },
];

const poweredBy = [
  { variant: "Dark", tone: "dark", stage: "dark" },
  { variant: "Light", tone: "light", stage: "light" },
  { variant: "Animated", tone: "dark", stage: "dark", animated: true },
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
  { label: "Maintainer", value: "Cloud Posse" },
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

function AssetCard({ asset }) {
  return (
    <article className={styles.assetCard}>
      <div
        className={`${styles.preview} ${styles[asset.previewTone]} ${
          asset.kind === "wordmark" ? styles.wordmark : ""
        }`}
      >
        <img src={asset.preview} alt={`${asset.title} ${asset.variant}`} />
      </div>
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

function AnimatedCard({ mark }) {
  return (
    <article className={styles.assetCard}>
      <div className={`${styles.preview} ${styles[mark.tone]}`}>
        <span className={styles.animatedMark}>
          <img
            src="/img/atmos-logo-bw.svg"
            alt={`Animated Atmos mark, ${mark.variant}`}
          />
        </span>
      </div>
      <div className={styles.assetBody}>
        <div>
          <p className={styles.assetKicker}>{mark.variant}</p>
          <h2>Animated Mark</h2>
          <p>{mark.usage}</p>
        </div>
        <div className={styles.fileList}>
          <a className={styles.fileLink} href="/img/atmos-logo-bw.svg" download>
            <span>atmos-logo-bw.svg</span>
            <strong>SVG</strong>
          </a>
        </div>
      </div>
    </article>
  );
}

function AnimatedWordmarkCard({ lockup }) {
  return (
    <article className={styles.assetCard}>
      <div className={`${styles.preview} ${styles[lockup.tone]}`}>
        <img
          className={styles.animatedWordmarkPreview}
          src={lockup.preview}
          alt={`${lockup.title}, ${lockup.variant}`}
        />
      </div>
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

function PoweredByBadge({ tone, animated }) {
  return (
    <span
      className={`${styles.badge} ${styles[tone]} ${animated ? styles.badgeAnimated : ""}`}
    >
      {animated ? (
        <span className={styles.animatedMark} style={{ width: 26, height: 26 }}>
          <img className={styles.badgeMark} src="/img/atmos-logo-bw.svg" alt="" />
        </span>
      ) : (
        <img className={styles.badgeMark} src="/img/atmos-logo.svg" alt="" />
      )}
      <span className={styles.badgeText}>
        <span className={styles.badgePre}>Powered by</span>
        <span className={styles.badgeName}>Atmos</span>
      </span>
    </span>
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
            <p>Light and dark SVGs, plus available PNG exports.</p>
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
            {animatedMarks.map((mark) => (
              <AnimatedCard key={mark.variant} mark={mark} />
            ))}
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
            Show that your project runs on Atmos. Use the dark badge on light
            surfaces, the light badge on dark surfaces, and the animated badge
            anywhere it can shine.
          </p>
          <div className={styles.badgeGrid}>
            {poweredBy.map((badge) => (
              <div
                key={badge.variant}
                className={`${styles.badgeStage} ${styles[badge.stage]}`}
              >
                <PoweredByBadge tone={badge.tone} animated={badge.animated} />
              </div>
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
