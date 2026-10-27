# Declarative Starlark cast recorder

## Problem and outcome

Recording a command currently duplicates its text in simulated typing and execution steps. Timing, environment, validation, fixture setup, and cleanup obscure the story. Interactive recordings additionally require long lists of session actions.

Expose `cast.recorder(...)` to existing Starlark script steps and standalone scripts. Scripts declare actions once and choose generation or read-only validation. Preserve YAML casts and their recording engine.

## Interface

The recorder supports `run`, `comment`, `simulate`, `prompt`, `pause`, `cd`, `screenshot`, `step`, `setup`, `hidden`, and `cleanup`. `rec.terminal` groups `write`, `send`, `key`, `wait`, `pause`, `hide`, `show`, and `screenshot` actions in a persistent PTY until `close`. A subsequent terminal action starts another session. Ordinary actions require an explicit close; finalization closes the final session automatically.

`run` combines simulated typing with execution and accepts `display`, literal or `cast.regex` expectations/rejections, and expected exit status. PTY input does not claim to provide per-command exit statuses. Waits consume output between checkpoints.

Setup, hidden, and cleanup callbacks receive a builder. They declare actions and must not perform direct I/O. Cleanup is armed before setup and runs with a bounded grace period after cancellation. Calls to `generate` and `validate` finalize the builder. Constructor options retain cast step settings; `output` also accepts a string cast path. `policy(text)` checks raw recorded output. Optional literal `replacements` normalize machine-specific output before policy validation.

## Architecture

`pkg/castplan` owns the language-independent declaration and fingerprint. The Starlark binding builds it and delegates to an optional `automation.CastLibrary` host interface. The existing step host executes registered steps and uses the existing asciicast recorder, PTY implementation, simulated typing, and renderers.

Metadata uses a versioned marker namespace distinct from screenshot paths. A fingerprint covers the declaration, including hidden setup/cleanup; replacement source paths are excluded because they vary by checkout. Markers carry action identifiers and statuses, never command strings or environment values. Command output assertions exclude simulated command text. Visible terminal waits are checked against output between checkpoints; hidden waits are generation-time checks.

Generation stages the cast, runs cleanup, normalizes output, validates metadata and assertions, and calls the policy before committing. Failure preserves the previous recording. Rendering happens after validation. Read-only validation does not run declared actions, cleanup, rendering, or screenshot generation. Ordinary Starlark code outside the builder remains ordinary executable code.

## Compatibility and boundaries

Existing YAML commands and casts remain supported. No command discovery, new background scheduler, shell integration protocol, or filesystem-write builtin is introduced. Recording remains exclusive and is rejected in parallel script tasks. Workflow-owned background services retain their existing lifecycle. Legacy casts need regeneration before declaration validation; playback and rendering remain compatible.

## Acceptance

Representative migrations cover masked environment output, interactive project initialization, a container workflow, and a CLI screengrab. Tests cover syntax, action order, assertions, marker compatibility, PTY waits, cleanup, failure preservation, normalization across event boundaries, and read-only validation. Required Go checks and the website build must pass; generated recordings receive content and visual review.
