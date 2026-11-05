# Fix: Keep example previews loaded while offscreen

**Date:** 2026-10-16

## Summary

Retain each example preview after its first viewport intersection and pause playback while it is offscreen.

## Context

Scrolling an example card out of view unmounted its cast player. Scrolling back fetched the same recording again with `cache: "no-store"` and reset playback. Changing static rendering to pause would also refetch the recording.

## Changes

- Track whether each preview has entered the viewport separately from its current visibility.
- Keep its player mounted after first entry and pass visibility through a dedicated `paused` prop.
- Suspend animation frames and loop timers without changing the loaded recording, playback position, or static rendering mode.
- Preserve the fallback behavior when IntersectionObserver is unavailable.

## Validation

- All 62 existing CastPlayer and FileBrowser Node tests passed.
- `pnpm build` completed successfully, including prebuild tests and production client/server generation. Existing unrelated webpack and broken-anchor warnings remain.
- Applicable pre-commit hooks passed.
- Independent Chromium verification confirmed one cast request across viewport exit/reentry, retained player and terminal DOM nodes, stopped animation frames while hidden, preserved content, and resumed playback on return.
- A pending three-second loop timer was cancelled on viewport exit and did not fire during more than 3.5 seconds hidden; no browser errors occurred.
- React checklist reviewed: stable component identity, independent fetch and playback dependencies, observer cleanup, and unchanged hidden-preview accessibility semantics.

## Follow-ups

None.
