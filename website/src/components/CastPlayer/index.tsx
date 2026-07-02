import React, { useEffect, useMemo, useRef, useState } from "react";
import { RiPauseFill, RiPlayFill } from "react-icons/ri";
import styles from "./styles.module.css";
import { applyIdleSkip } from "./playback.mjs";

type CastEvent = [number, string, string];

type CastHeader = {
  command?: string;
};

type Props = {
  src: string;
  command?: string;
  title?: string;
  chrome?: boolean;
  controls?: boolean;
  scrubber?: boolean;
  autoplay?: boolean;
  loop?: boolean;
  loopDelay?: number;
  speed?: number;
  idleSkip?: boolean;
  thumbnail?: boolean;
  showCommand?: boolean;
  prompt?: string;
  typeRate?: number;
  enterDelay?: number;
  exitDelay?: number;
};

export default function CastPlayer({
  src,
  command,
  title = "Atmos",
  chrome = false,
  controls,
  scrubber = true,
  autoplay = true,
  loop = true,
  loopDelay = 5,
  speed = 1,
  idleSkip = true,
  thumbnail = false,
  showCommand = true,
  prompt = "\x1b[1;38;2;0;95;135m>\x1b[0m ",
  typeRate = 0.035,
  enterDelay = 0.5,
  exitDelay = 0.6,
}: Props) {
  const [events, setEvents] = useState<CastEvent[]>([]);
  const [content, setContent] = useState("");
  const [playing, setPlaying] = useState(autoplay);
  const [position, setPosition] = useState(0);
  const animationFrame = useRef<number | undefined>();
  const loopTimer = useRef<number | undefined>();
  const renderedEventTime = useRef<number>(-1);
  const [seekVersion, setSeekVersion] = useState(0);
  const screenRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(src, { cache: "no-store" })
      .then((response) => {
        if (!response.ok) {
          throw new Error(`Failed to load cast: ${response.status}`);
        }
        return response.text();
      })
      .then((text) => {
        if (cancelled) return;
        const rows = text.trim().split("\n");
        let header: CastHeader;
        try {
          header = JSON.parse(rows[0] || "{}") as CastHeader;
        } catch {
          throw new Error("Malformed cast header");
        }
        const lines = rows.slice(1);
        const parsed = lines.reduce<CastEvent[]>((events, line) => {
          if (!line.trim()) return events;
          let event: CastEvent;
          try {
            event = JSON.parse(line) as CastEvent;
          } catch {
            throw new Error("Malformed cast event");
          }
          const normalized: CastEvent = [
            event[0],
            event[1],
            normalizeTerminalText(event[2]),
          ];
          if ((normalized[1] === "o" || normalized[1] === "e") && normalized[2] !== "") {
            events.push(normalized);
          }
          return events;
        }, []);
        const withIntro = addCommandIntro(parsed, {
          command: command ?? header.command,
          enabled: showCommand,
          prompt,
          typeRate,
          enterDelay,
          exitDelay,
        });
        const playbackEvents = applyIdleSkip(withIntro, idleSkip ? 1.5 : 0);
        const initialPosition = 0;
        setEvents(playbackEvents);
        setContent(renderAt(playbackEvents, initialPosition));
        setPosition(initialPosition);
        renderedEventTime.current = latestEventTimeAt(
          playbackEvents,
          initialPosition,
        );
        setSeekVersion((version) => version + 1);
      })
      .catch(() => {
        if (cancelled) return;
        setEvents([]);
        setContent("");
        setPosition(0);
        renderedEventTime.current = -1;
        setSeekVersion((version) => version + 1);
      });
    return () => {
      cancelled = true;
    };
  }, [
    src,
    command,
    showCommand,
    prompt,
    typeRate,
    enterDelay,
    exitDelay,
    idleSkip,
    autoplay,
  ]);

  const duration = useMemo(() => events.at(-1)?.[0] ?? 0, [events]);

  useEffect(() => {
    if (!screenRef.current) return;
    screenRef.current.scrollTop = screenRef.current.scrollHeight;
  }, [content]);

  useEffect(() => {
    window.cancelAnimationFrame(animationFrame.current ?? 0);
    window.clearTimeout(loopTimer.current);
    if (!playing || events.length === 0) return;

    if (duration <= 0 || position >= duration) {
      handlePlaybackEnd(
        events,
        loop,
        loopDelay,
        setContent,
        setPosition,
        setPlaying,
        setSeekVersion,
        renderedEventTime,
        loopTimer,
      );
      return;
    }

    const startedAt =
      performance.now() - (position * 1000) / Math.max(speed, 0.1);
    const tick = (now: number) => {
      const nextPosition = Math.min(
        duration,
        ((now - startedAt) / 1000) * Math.max(speed, 0.1),
      );
      setPosition(nextPosition);

      const eventTime = latestEventTimeAt(events, nextPosition);
      if (eventTime !== renderedEventTime.current) {
        renderedEventTime.current = eventTime;
        setContent(renderAt(events, nextPosition));
      }

      if (nextPosition >= duration) {
        handlePlaybackEnd(
          events,
          loop,
          loopDelay,
          setContent,
          setPosition,
          setPlaying,
          setSeekVersion,
          renderedEventTime,
          loopTimer,
        );
        return;
      }
      animationFrame.current = window.requestAnimationFrame(tick);
    };
    animationFrame.current = window.requestAnimationFrame(tick);

    return () => {
      window.cancelAnimationFrame(animationFrame.current ?? 0);
      window.clearTimeout(loopTimer.current);
    };
  }, [playing, seekVersion, events, duration, speed, loop, loopDelay]);

  const togglePlayback = () => {
    if (playing) {
      setPlaying(false);
      return;
    }
    if (duration > 0 && position >= duration) {
      setContent(renderAt(events, 0));
      setPosition(0);
      renderedEventTime.current = latestEventTimeAt(events, 0);
      setSeekVersion((version) => version + 1);
    }
    setPlaying(true);
  };

  const terminal = (
    <pre className={styles.screen} ref={screenRef}>
      {renderAnsi(content || " ")}
    </pre>
  );
  const rootClassName = [
    chrome ? styles.window : styles.plain,
    thumbnail ? styles.thumbnail : "",
  ]
    .filter(Boolean)
    .join(" ");
  const showControls = controls ?? !thumbnail;

  return (
    <div className={rootClassName}>
      {chrome && (
        <div className={styles.titlebar}>
          <span className={styles.dots} aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span className={styles.title}>{title}</span>
        </div>
      )}
      {terminal}
      {showControls && (
        <div className={styles.controls}>
          <button
            type="button"
            className={styles.playButton}
            aria-label={playing ? "Pause cast" : "Play cast"}
            onClick={togglePlayback}
          >
            {playing ? (
              <RiPauseFill aria-hidden="true" />
            ) : (
              <RiPlayFill aria-hidden="true" />
            )}
          </button>
          {scrubber && (
            <input
              aria-label="Cast position"
              type="range"
              min={0}
              max={duration || 0}
              step={0.01}
              value={position}
              onChange={(event) => {
                const next = Number(event.currentTarget.value);
                setPosition(next);
                setContent(renderAt(events, next));
                renderedEventTime.current = latestEventTimeAt(events, next);
                setSeekVersion((version) => version + 1);
              }}
            />
          )}
          <span>
            {formatTime(position)} / {formatTime(duration)}
          </span>
        </div>
      )}
    </div>
  );
}

type Segment = {
  text: string;
  style?: React.CSSProperties;
  cursor?: boolean;
};

const CURSOR_MARKER = "\uE000";

const ANSI_COLORS: Record<number, string> = {
  30: "#111111",
  31: "#ff5f57",
  32: "#28c840",
  33: "#ffbd2e",
  34: "#6c73ff",
  35: "#d670d6",
  36: "#00d1b2",
  37: "#f5f5f5",
  90: "#666a73",
  91: "#ff8f87",
  92: "#6ee787",
  93: "#ffd866",
  94: "#9aa2ff",
  95: "#f0a6f0",
  96: "#55e7d0",
  97: "#ffffff",
};

const ANSI_BACKGROUNDS: Record<number, string> = {
  40: "#111111",
  41: "#ff5f57",
  42: "#28c840",
  43: "#ffbd2e",
  44: "#6c73ff",
  45: "#d670d6",
  46: "#00d1b2",
  47: "#f5f5f5",
  100: "#3a3f4b",
  101: "#ff8f87",
  102: "#6ee787",
  103: "#ffd866",
  104: "#848cff",
  105: "#f0a6f0",
  106: "#55e7d0",
  107: "#ffffff",
};

const ANSI_256_COLORS = [
  "#000000",
  "#800000",
  "#008000",
  "#808000",
  "#000080",
  "#800080",
  "#008080",
  "#c0c0c0",
  "#808080",
  "#ff0000",
  "#00ff00",
  "#ffff00",
  "#0000ff",
  "#ff00ff",
  "#00ffff",
  "#ffffff",
];

function renderAnsi(input: string) {
  return parseAnsi(input).map((segment, index) =>
    segment.cursor ? (
      <span key={index} className={styles.cursor} aria-hidden="true" />
    ) : (
      <span key={index} style={segment.style}>
        {segment.text}
      </span>
    ),
  );
}

function parseAnsi(input: string): Segment[] {
  const segments: Segment[] = [];
  const sgr = /\x1b\[([0-9;]*)m/g;
  let lastIndex = 0;
  let style: React.CSSProperties = {};
  let match: RegExpExecArray | null;
  const pushText = (text: string) => {
    if (text === "") return;
    const parts = text.split(CURSOR_MARKER);
    parts.forEach((part, index) => {
      if (part !== "") {
        segments.push({ text: part, style: { ...style } });
      }
      if (index < parts.length - 1) {
        segments.push({ text: "", cursor: true });
      }
    });
  };

  while ((match = sgr.exec(input)) !== null) {
    if (match.index > lastIndex) {
      pushText(input.slice(lastIndex, match.index));
    }
    style = applySgr(style, match[1]);
    lastIndex = sgr.lastIndex;
  }
  if (lastIndex < input.length) {
    pushText(input.slice(lastIndex));
  }
  return segments;
}

function applySgr(
  current: React.CSSProperties,
  sequence: string,
): React.CSSProperties {
  const codes =
    sequence === "" ? [0] : sequence.split(";").map((code) => Number(code));
  let next = { ...current };
  for (let i = 0; i < codes.length; i += 1) {
    const code = codes[i];
    if (code === 0) {
      next = {};
    } else if (code === 1) {
      next.fontWeight = 700;
    } else if (code === 22) {
      delete next.fontWeight;
    } else if (code === 39) {
      delete next.color;
    } else if (code === 49) {
      delete next.backgroundColor;
    } else if (ANSI_COLORS[code]) {
      next.color = ANSI_COLORS[code];
    } else if (ANSI_BACKGROUNDS[code]) {
      next.backgroundColor = ANSI_BACKGROUNDS[code];
    } else if (
      (code === 38 || code === 48) &&
      codes[i + 1] === 5 &&
      Number.isFinite(codes[i + 2])
    ) {
      const color = color256(codes[i + 2]);
      if (code === 38) {
        next.color = color;
      } else {
        next.backgroundColor = color;
      }
      i += 2;
    } else if (
      (code === 38 || code === 48) &&
      codes[i + 1] === 2 &&
      Number.isFinite(codes[i + 2]) &&
      Number.isFinite(codes[i + 3]) &&
      Number.isFinite(codes[i + 4])
    ) {
      const color = `rgb(${clampColor(codes[i + 2])}, ${clampColor(codes[i + 3])}, ${clampColor(codes[i + 4])})`;
      if (code === 38) {
        next.color = color;
      } else {
        next.backgroundColor = color;
      }
      i += 4;
    }
  }
  return next;
}

function color256(code: number) {
  if (code < 16) {
    return ANSI_256_COLORS[code] ?? "#ffffff";
  }
  if (code >= 16 && code <= 231) {
    const n = code - 16;
    const r = Math.floor(n / 36);
    const g = Math.floor((n % 36) / 6);
    const b = n % 6;
    return `rgb(${cubeColor(r)}, ${cubeColor(g)}, ${cubeColor(b)})`;
  }
  if (code >= 232 && code <= 255) {
    const gray = 8 + (code - 232) * 10;
    return `rgb(${gray}, ${gray}, ${gray})`;
  }
  return "#ffffff";
}

function cubeColor(value: number) {
  return value === 0 ? 0 : 55 + value * 40;
}

function clampColor(value: number) {
  return Math.max(0, Math.min(255, value));
}

function renderAt(events: CastEvent[], seconds: number) {
  return replayTerminal(
    events
      .filter((event) => event[0] <= seconds)
      .map((event) => event[2])
      .join(""),
  );
}

function latestEventTimeAt(events: CastEvent[], seconds: number) {
  let latest = -1;
  for (const event of events) {
    if (event[0] > seconds) {
      break;
    }
    latest = event[0];
  }
  return latest;
}

function handlePlaybackEnd(
  events: CastEvent[],
  loop: boolean,
  loopDelay: number,
  setContent: React.Dispatch<React.SetStateAction<string>>,
  setPosition: React.Dispatch<React.SetStateAction<number>>,
  setPlaying: React.Dispatch<React.SetStateAction<boolean>>,
  setSeekVersion: React.Dispatch<React.SetStateAction<number>>,
  renderedEventTime: React.MutableRefObject<number>,
  loopTimer: React.MutableRefObject<number | undefined>,
) {
  if (!loop) {
    setPlaying(false);
    return;
  }

  loopTimer.current = window.setTimeout(() => {
    setContent(renderAt(events, 0));
    setPosition(0);
    renderedEventTime.current = latestEventTimeAt(events, 0);
    setSeekVersion((version) => version + 1);
  }, loopDelay * 1000);
}

function addCommandIntro(
  events: CastEvent[],
  options: {
    command?: string;
    enabled: boolean;
    prompt: string;
    typeRate: number;
    enterDelay: number;
    exitDelay: number;
  },
): CastEvent[] {
  const command = options.command?.trim();
  if (!options.enabled || !command) {
    return events;
  }
  if (eventsStartWithPrompt(events)) {
    return events;
  }

  const intro: CastEvent[] = [[0, "o", options.prompt]];
  let cursor = Math.max(options.typeRate, 0.001);
  for (const char of command) {
    intro.push([cursor, "o", char]);
    cursor += Math.max(options.typeRate, 0.001);
  }
  cursor += Math.max(options.enterDelay, 0);
  intro.push([cursor, "o", "\n"]);
  const offset = cursor + 0.05;
  const shiftedEvents = events.map(
    (event) => [event[0] + offset, event[1], event[2]] as CastEvent,
  );
  const finalText = events.map((event) => event[2]).join("");
  const finalPrompt =
    finalText.endsWith("\n") || finalText === ""
      ? options.prompt
      : `\n${options.prompt}`;

  return [
    ...intro,
    ...shiftedEvents,
    [
      lastEventTime(events) + offset + Math.max(options.exitDelay, 0),
      "o",
      finalPrompt,
    ],
  ];
}

function lastEventTime(events: CastEvent[]) {
  return events.at(-1)?.[0] ?? 0;
}

function eventsStartWithPrompt(events: CastEvent[]) {
  const firstText = events
    .filter((event) => event[1] === "o" || event[1] === "e")
    .map((event) => event[2])
    .join("")
    .trimStart();
  return stripAnsi(firstText).startsWith("> ");
}

function stripAnsi(input: string) {
  return input.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "");
}

function normalizeTerminalText(input: string) {
  return input.replace(/\r\n/g, "\n");
}

function replayTerminal(input: string) {
  const lines = [""];
  let row = 0;
  let carriageReturnPending = false;
  let cursorVisible = false;

  const currentLine = () => lines[row] ?? "";
  const setCurrentLine = (line: string) => {
    lines[row] = line;
  };
  const removeCursor = () => {
    setCurrentLine(currentLine().split(CURSOR_MARKER).join(""));
  };
  const syncCursor = () => {
    removeCursor();
    if (cursorVisible) {
      setCurrentLine(currentLine() + CURSOR_MARKER);
    }
  };
  const append = (text: string) => {
    if (carriageReturnPending) {
      clearLine();
      carriageReturnPending = false;
    }
    removeCursor();
    setCurrentLine(currentLine() + text);
    if (cursorVisible) {
      setCurrentLine(currentLine() + CURSOR_MARKER);
    }
  };
  const clearLine = () => {
    setCurrentLine("");
    if (cursorVisible) {
      setCurrentLine(CURSOR_MARKER);
    }
  };
  const moveToNextLine = () => {
    removeCursor();
    row += 1;
    if (lines[row] === undefined) {
      lines[row] = "";
    }
    syncCursor();
  };

  for (let index = 0; index < input.length; ) {
    const char = input[index];

    if (char === "\x1b") {
      const parsed = parseEscape(input, index);
      if (!parsed) {
        index += 1;
        continue;
      }
      if (parsed.kind === "sgr") {
        append(parsed.sequence);
      } else if (parsed.kind === "clearLine") {
        clearLine();
      } else if (parsed.kind === "cursorShow") {
        cursorVisible = true;
        syncCursor();
      } else if (parsed.kind === "cursorHide") {
        cursorVisible = false;
        removeCursor();
      }
      index = parsed.nextIndex;
      continue;
    }

    if (char === "\r") {
      carriageReturnPending = true;
      index += 1;
      continue;
    }

    if (char === "\n") {
      carriageReturnPending = false;
      moveToNextLine();
      index += 1;
      continue;
    }

    if (char === "\b") {
      removeCursor();
      setCurrentLine(removeLastVisibleChar(currentLine()));
      syncCursor();
      index += 1;
      continue;
    }

    append(char);
    index += 1;
  }

  return lines.join("\n");
}

type ParsedEscape = {
  kind: "sgr" | "clearLine" | "cursorShow" | "cursorHide" | "ignore";
  sequence: string;
  nextIndex: number;
};

function parseEscape(input: string, start: number): ParsedEscape | null {
  const next = input[start + 1];
  if (next === "]") {
    const bel = input.indexOf("\x07", start + 2);
    const st = input.indexOf("\x1b\\", start + 2);
    const end = bel === -1 ? st : st === -1 ? bel : Math.min(bel, st);
    if (end === -1) {
      return null;
    }
    return {
      kind: "ignore",
      sequence: input.slice(start, end + (end === st ? 2 : 1)),
      nextIndex: end + (end === st ? 2 : 1),
    };
  }

  if (next !== "[") {
    return {
      kind: "ignore",
      sequence: input.slice(start, start + 2),
      nextIndex: start + 2,
    };
  }

  const match = /\x1b\[[0-?]*[ -/]*[@-~]/.exec(input.slice(start));
  if (!match || match.index !== 0) {
    return null;
  }

  const sequence = match[0];
  const final = sequence.at(-1);
  if (final === "m") {
    return { kind: "sgr", sequence, nextIndex: start + sequence.length };
  }
  if (sequence === "\x1b[?25h") {
    return { kind: "cursorShow", sequence, nextIndex: start + sequence.length };
  }
  if (sequence === "\x1b[?25l") {
    return { kind: "cursorHide", sequence, nextIndex: start + sequence.length };
  }
  if (final === "K") {
    return { kind: "clearLine", sequence, nextIndex: start + sequence.length };
  }
  return { kind: "ignore", sequence, nextIndex: start + sequence.length };
}

function removeLastVisibleChar(input: string) {
  const match = /\x1b\[[0-?]*[ -/]*[@-~]$/.exec(input);
  if (match) {
    return input.slice(0, match.index);
  }
  return input.slice(0, -1);
}

function formatTime(seconds: number) {
  const safe = Math.max(0, seconds);
  const mins = Math.floor(safe / 60)
    .toString()
    .padStart(2, "0");
  const secs = Math.floor(safe % 60)
    .toString()
    .padStart(2, "0");
  const tenths = Math.floor((safe % 1) * 10);
  return `${mins}:${secs}.${tenths}`;
}
