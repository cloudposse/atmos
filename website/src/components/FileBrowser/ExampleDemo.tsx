import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  RiDownloadLine,
  RiFullscreenLine,
  RiPlayFill,
  RiRestartLine,
  RiTerminalBoxLine,
} from "react-icons/ri";
import CastPlayer from "../CastPlayer";
import CastShareLink from "../CastShareLink";
import CastProDownload from "../CastProDownload";
import type { ExampleCast, FileBrowserOptions } from "./types";
import styles from "./example-demo.module.css";

export default function ExampleDemo({
  cast,
  name,
  options,
}: {
  cast: ExampleCast & { file: string };
  name: string;
  options: FileBrowserOptions;
}) {
  const [play, setPlay] = useState(false);
  const [full, setFull] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const expandButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const update = () =>
      setFull(document.fullscreenElement === container.current);
    document.addEventListener("fullscreenchange", update);
    return () => document.removeEventListener("fullscreenchange", update);
  }, []);
  const collapse = useCallback(async () => {
    if (document.fullscreenElement) await document.exitFullscreen();
    setFull(false);
    expandButton.current?.focus();
  }, []);
  useEffect(() => {
    if (!full) return;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        void collapse();
      }
      if (event.key !== "Tab") return;
      const controls = container.current?.querySelectorAll<HTMLElement>(
        "button, a[href], input",
      );
      if (!controls?.length) return;
      const first = controls[0];
      const last = controls[controls.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    expandButton.current?.focus();
    return () => {
      document.body.style.overflow = overflow;
      document.removeEventListener("keydown", onKey);
    };
  }, [full, collapse]);
  async function expand() {
    if (full) {
      await collapse();
      return;
    }
    try {
      if (container.current?.requestFullscreen)
        await container.current.requestFullscreen();
      else setFull(true);
    } catch {
      // Embedded browsers can advertise fullscreen but deny the request.
      setFull(true);
    }
  }
  const [owner, repo] = options.githubRepo.split("/");
  // Recordings are stored in website/static, independent of the example source directory.
  const source = {
    owner,
    repo,
    gitRef: options.githubBranch || "main",
    path: `website/static${cast.file}`,
  };
  return (
    <>
      <div
        ref={container}
        role={full ? "dialog" : undefined}
        aria-modal={full ? true : undefined}
        aria-label={full ? `${name} recording` : undefined}
        className={`${styles.demo} ${full ? styles.expanded : ""}`}
      >
        <div className={styles.demoToolbar}>
          <span>
            <RiTerminalBoxLine /> {name}
          </span>
          <span className={styles.demoActions}>
            <a
              href={cast.file}
              download
              aria-label="Download recording"
              title="Download recording"
            >
              <RiDownloadLine />
            </a>
            <button
              ref={expandButton}
              type="button"
              onClick={expand}
              aria-label={full ? "Exit expanded demo" : "Expand demo"}
            >
              <RiFullscreenLine />
            </button>
          </span>
        </div>
        <CastPlayer
          key={play ? "play" : "poster"}
          src={cast.file}
          title={cast.title || name}
          static={!play}
          autoplay={play}
          loop={false}
          controls={play}
          scrubber
          showCommand={false}
          className={styles.player}
        />
        {!play ? (
          <div className={styles.demoFooter}>
            <button
              type="button"
              className={styles.watch}
              onClick={() => setPlay(true)}
            >
              <RiPlayFill /> Watch walkthrough
            </button>
            <span>Recorded from this example</span>
          </div>
        ) : (
          <button
            type="button"
            className={styles.resetDemo}
            onClick={() => setPlay(false)}
          >
            <RiRestartLine /> Back to preview
          </button>
        )}
      </div>
      <div className={styles.recordingActions}>
        <span>Real commands. Real output.</span>
        <div>
          <CastShareLink {...source} />
          <CastProDownload {...source} />
        </div>
      </div>
    </>
  );
}
