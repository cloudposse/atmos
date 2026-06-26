# PRD: VHS Demo Videos Infrastructure

**Status:** Approved
**Created:** 2025-01-17
**Owner:** CloudPosse Engineering

## Executive Summary

Create a comprehensive infrastructure for producing, managing, and distributing demonstration videos and screenshots of Atmos CLI commands using VHS (Charm Bracelet's terminal recorder) and a custom orchestration tool called `director`.

## Problem Statement

Atmos has powerful features but lacks visual demonstrations that help users:
- Understand how commands work in practice
- See expected output and behavior
- Learn workflows and best practices
- Troubleshoot issues by comparing with examples

Currently, we:
- Create occasional demos manually without consistency
- Have no systematic way to update demos when commands change
- Lack infrastructure for contributors to add demos easily
- Miss opportunities to showcase features in blog posts and documentation

## Goals

### Primary Goals
1. **Systematic demo production** - Every major command has a demonstration
2. **Contributor-friendly** - External contributors can add demos without CloudPosse AWS access
3. **Automated distribution** - Demos automatically deployed to accessible CDN
4. **Incremental updates** - Only regenerate changed demos (distributed caching)
5. **Scenario-based organization** - Mirror test fixture pattern for consistency

### Secondary Goals
1. **Blog post integration** - Feature announcements include demo videos
2. **Documentation enhancement** - Embed demos in Docusaurus pages
3. **Screenshot generation** - Both animated GIFs and static PNGs
4. **Quality enforcement** - CI checks for demo requirements

## Non-Goals

- Hosting long-form tutorial videos (use YouTube for that)
- Interactive demos (use Cloud Posse Refarch for that)
- Demos requiring production AWS resources (use mock data)
- Real-time screen recording (pre-scripted VHS tapes only)

## Solution Design

### Architecture

```
┌─────────────────┐
│ Contributor     │
│ - Edits .tape   │
│ - Commits       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ GitHub Actions  │
│ - Runs director │
│ - Generates GIF │
│ - Deploys       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ GitHub Pages    │
│ (orphan branch) │
│ - Hosts GIFs    │
│ - metadata.json │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Documentation   │
│ - Embeds GIFs   │
│ - README links  │
└─────────────────┘
```

### Components

#### 1. Director Tool (`tools/director`)

**Purpose:** Orchestrate VHS scene generation, validation, and deployment.

**Commands:**
```bash
director new <scene-name>    # Scaffold new scene
director render [scenes...]  # Generate GIFs (incremental)
director validate            # Check dependencies
director catalog             # List all scenes
director deploy              # Deploy to backend (future)
```

**Implementation:**
- Go tool in `tools/director/`
- Reads `demos/scenes.yaml` for scene index
- Checks dependencies before rendering
- Implements distributed caching via SHA256 comparison
- Pluggable backend support (gh-pages, S3)

#### 2. Scene Index (`demos/scenes.yaml`)

**Purpose:** Central registry of all demo scenes (similar to test-cases/*.yaml pattern).

**Schema:**
```yaml
version: "1.0"
scenes:
  - name: "terraform-plan-basic"
    enabled: true
    description: "Basic terraform plan workflow"
    tape: "scenes/terraform-plan-basic.tape"
    requires:
      - atmos
      - terraform
    outputs:
      - gif
      - png
      - mp4  # Optional: MP4 video format
    audio:  # Optional: Background audio for MP4 outputs
      source: "audio/background-music.mp3"  # Path relative to demos/
      volume: 0.3                            # Volume level (0.0-1.0), default 0.3
      fade_out: 2.0                          # Fade-out duration in seconds, default 2.0
      loop: true                             # Loop audio if shorter than video, default true
```

**Audio Configuration (Optional):**
- Only applies to MP4 outputs (GIF does not support audio)
- Requires FFmpeg to be installed (validated by `director validate`)
- Audio files placed in `demos/audio/` directory
- Volume range: 0.0 (muted) to 1.0 (full volume), default 0.3 (30%)
- Fade-out automatically applied in the final N seconds (default 2.0s)
- Loop enabled by default for seamless background music
- Cache tracks audio file changes and regenerates MP4 when audio changes

#### 3. VHS Tape Files (`demos/scenes/*.tape`)

**Purpose:** Scripts that define terminal sessions for VHS to record.

**Format:** Native VHS tape format (not YAML wrapper)

**Example:**
```tape
Output terraform-plan-basic.gif
Set Theme "Catppuccin Mocha"
Set Width 1400
Set Height 800

Require atmos
Require terraform

Type "atmos terraform plan vpc --stack dev"
Enter
Sleep 5s
Screenshot plan-output.png
```

#### 4. Distributed Cache

**Purpose:** Avoid regenerating unchanged demos across contributors and CI runs.

**Mechanism:**
1. `metadata.json` stored on gh-pages with SHA256 hashes of tape files
2. `director render` fetches metadata before generating
3. Compares local tape SHA256 with cached metadata
4. Only regenerates if tape changed or doesn't exist

**Benefits:**
- Fresh clone: Fetches all existing GIFs, generates only new/changed ones
- Contributor: Only regenerates their modified scene
- CI: Fast builds (90%+ cache hit rate expected)

**metadata.json structure:**
```json
{
  "terraform-plan-basic": {
    "tape_sha256": "abc123...",
    "generated_at": "2025-01-15T10:30:00Z",
    "vhs_version": "0.7.2",
    "outputs": {
      "gif": "terraform-plan-basic.gif",
      "png": "terraform-plan-basic.png"
    }
  }
}
```

#### 5. GitHub Pages Deployment

**Storage Strategy:**
- Use orphan `gh-pages` branch (no history)
- Deploy with `peaceiris/actions-gh-pages@v4` and `force_orphan: true`
- No clone bloat (separate branch, shallow clones possible)
- Free hosting with 100 GB/month bandwidth

**URL Structure:**
```
https://cloudposse.github.io/atmos/demos/{scene-name}.gif
https://cloudposse.github.io/atmos/demos/{scene-name}.png
https://cloudposse.github.io/atmos/demos/metadata.json
```

### Workflow

#### Contributor Workflow

```bash
# 1. Clone repo
git clone github.com/cloudposse/atmos
cd atmos

# 2. Create new scene
./build/director new my-feature
# Creates demos/scenes/my-feature.tape

# 3. Edit tape file
vim demos/scenes/my-feature.tape

# 4. Add to index
# Edit demos/scenes.yaml to add scene entry

# 5. Test locally
./build/director render my-feature
# Fetches existing demos from gh-pages
# Generates only my-feature.gif

# 6. Commit (tape file only)
git add demos/scenes/my-feature.tape demos/scenes.yaml
git commit -m "feat: add my-feature demo"
git push
```

#### CI/CD Workflow

```yaml
# .github/workflows/deploy-demos.yml
on:
  push:
    branches: [main]
    paths:
      - 'demos/scenes/**/*.tape'

jobs:
  deploy:
    steps:
      - install: vhs, gifsicle
      - run: director render --force
      - deploy: peaceiris/actions-gh-pages@v4
        with:
          force_orphan: true
          publish_dir: demos/.cache
```

## Technical Specifications

### Tools & Dependencies

| Tool | Purpose | Installation |
|------|---------|--------------|
| VHS | Terminal recorder | `brew install vhs` |
| gifsicle | GIF optimizer | `brew install gifsicle` |
| FFmpeg | Audio processing for MP4 | `brew install ffmpeg` |
| Freeze | Static screenshots (optional) | `brew install freeze` |
| director | Orchestration | `go build ./tools/director` |

### File Formats

**Primary:** GIF (animated)
- Works in GitHub README, docs, issues, PRs
- Universal browser support
- Target size: < 500 KB per demo

**Secondary:** PNG (screenshots)
- Static captures via VHS `Screenshot` command
- Lighter weight for simple demos
- Better quality for documentation

**Optional:** MP4 (video with audio)
- Supports background music via FFmpeg
- Useful for social media, presentations, and external documentation sites
- Cannot embed in GitHub README (markdown parser blocks GitHub Pages URLs)
- Audio features: volume control, automatic looping, fade-out effects
- Generated via post-processing after VHS render

### VHS Configuration

**Global Defaults** (`demos/defaults.yaml`):
```yaml
vhs:
  theme: "Catppuccin Mocha"
  font_size: 14
  width: 1400
  height: 800
  framerate: 20
  typing_speed: "50ms"
```

**Optimization:**
```yaml
optimization:
  gifsicle:
    enabled: true
    lossy: 80
    colors: 256
    optimize_level: 3
```

### Repository Structure

```
demos/
├── scenes.yaml              # Scene index
├── defaults.yaml            # Global settings
├── scenes/                  # VHS tape files
│   ├── terraform-plan-basic.tape
│   ├── describe-stacks.tape
│   └── workflow-deploy.tape
├── audio/                   # Background music for MP4 outputs
│   ├── .gitkeep
│   └── *.mp3               # Audio files referenced in scenes.yaml
├── fixtures/                # Test configurations
│   ├── basic-config/
│   └── auth-config/
├── .cache/                  # Generated outputs (gitignored)
│   ├── metadata.json
│   ├── *.gif
│   ├── *.png
│   └── *.mp4               # MP4 videos with audio
├── .gitignore
└── README.md

tools/director/
├── main.go
├── cmd/
│   ├── root.go
│   ├── new.go
│   ├── render.go
│   ├── validate.go
│   └── catalog.go
├── internal/
│   ├── scene/              # Scene data structures
│   └── vhs/                # VHS rendering and cache
└── go.mod

pkg/
├── vhs/                     # VHS command execution
│   └── vhs.go              # Wrapper for VHS CLI
└── ffmpeg/                  # FFmpeg command execution
    └── ffmpeg.go           # Audio processing for MP4
```

## Implementation Plan

### Phase 1: Foundation (Week 1-2)
- [x] Create `demos/` directory structure
- [x] Create `scenes.yaml` and `defaults.yaml`
- [x] Build `director` tool with basic commands
- [x] Create 3 pilot scenes
- [ ] Test local workflow

### Phase 2: Distributed Cache (Week 2-3)
- [ ] Implement SHA256 metadata tracking
- [ ] Add cache fetching from gh-pages
- [ ] Test incremental builds
- [ ] Optimize with gifsicle integration

### Phase 3: CI/CD (Week 3)
- [ ] Create `deploy-demos.yml` workflow
- [ ] Set up orphan gh-pages branch
- [ ] Test deployment pipeline
- [ ] Verify cache works in CI

### Phase 4: Documentation (Week 3-4)
- [ ] Create `<DemoGif>` Docusaurus component
- [ ] Integrate 5-10 demos into docs
- [ ] Update CLAUDE.md with requirements
- [ ] Write contributor guide

### Phase 5: Rollout (Week 4-6)
- [ ] Create 20+ demos for core commands
- [ ] Add PR check for demo requirements (soft enforcement)
- [ ] Announce to contributors
- [ ] Gather feedback and iterate

## Success Metrics

| Metric | Target | Timeline |
|--------|--------|----------|
| Scene coverage | 30+ scenes | 3 months |
| Repository size | < 300 MB | Ongoing |
| Cache hit rate | > 90% | After 3 months |
| Contributor demos | 5+ external | 6 months |
| Blog post adoption | 80% include demos | Ongoing |
| Demo creation time | < 30 minutes | Immediate |

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| **VHS requires graphical deps** | Contributors without ttyd/ffmpeg can't test | Document installation, provide pre-commit hook |
| **GIF file bloat** | Repo size grows | Enforce .gitignore, optimize with gifsicle, monitor size |
| **GitHub Pages bandwidth** | Hit 100 GB/month limit | Monitor usage, migrate to S3 if needed |
| **Stale demos** | Demos don't match current output | CI checks for outdated demos, periodic refresh |
| **Contributor confusion** | Hard to add demos | Clear documentation, simple workflow, helpful error messages |

## Alternative Approaches Considered

### 1. Commit GIFs to Main Repo
**Rejected:** Would bloat repo to 500 MB+ with 100 demos, slow clones

### 2. Use GitHub Releases for Storage
**Rejected:** Ties demos to releases, can't update independently, not embeddable

### 3. AWS S3 + CloudFront
**Rejected for now:** Requires AWS access for contributors, adds complexity. May revisit if GitHub Pages bandwidth becomes an issue.

### 4. Separate atmos-demos Repository
**Rejected:** Two repos to maintain, out-of-sync risk, extra contributor friction

## Open Questions

1. **Hard vs soft enforcement?** Start with soft (PR comments), move to hard (CI failure) after 3 months?
2. **All commands or subset?** Require demos for user-facing commands only, not internal/helper commands?
3. **Auto-generation?** Should VHS demo agent generate first drafts of tape files?
4. **Versioning?** Keep historical demos for old Atmos versions or only latest?

## References

- [VHS Documentation](https://github.com/charmbracelet/vhs)
- [Freeze Documentation](https://github.com/charmbracelet/freeze)
- [GitHub Pages Limits](https://docs.github.com/en/pages/getting-started-with-github-pages/about-github-pages#usage-limits)
- [peaceiris/actions-gh-pages](https://github.com/peaceiris/actions-gh-pages)

## Appendix: Example Scenes

### terraform-plan-basic.tape
```tape
Output terraform-plan-basic.gif
Set Theme "Catppuccin Mocha"
Set Width 1400
Set Height 800
Set FontSize 14

Require atmos
Require terraform

Type "export ATMOS_BASE_PATH=examples/complete"
Enter
Sleep 500ms

Type "atmos terraform plan vpc --stack plat-ue2-dev"
Sleep 500ms
Enter
Sleep 5s

Screenshot terraform-plan-basic.png
Sleep 1s
```

### scenes.yaml Entry
```yaml
- name: "terraform-plan-basic"
  enabled: true
  description: "Basic terraform plan workflow"
  tape: "scenes/terraform-plan-basic.tape"
  requires:
    - atmos
    - terraform
  outputs:
    - gif
    - png
```

### MP4 with Background Audio Example

```yaml
- name: "workflow-deploy-demo"
  enabled: true
  description: "Complete deployment workflow with background music"
  tape: "scenes/workflow-deploy.tape"
  requires:
    - atmos
    - terraform
  outputs:
    - gif       # For GitHub README
    - mp4       # For social media with audio
  audio:
    source: "audio/ambient-tech.mp3"
    volume: 0.25     # Subtle background music
    fade_out: 3.0    # 3-second fade out
    loop: true       # Loop if video is longer than audio
```

**Audio Processing Flow:**
1. VHS generates silent MP4 from tape file
2. FFmpeg merges background audio with video
3. Audio automatically loops if shorter than video duration
4. Fade-out effect applied in final N seconds
5. Original silent MP4 replaced with audio-merged version
6. Cache tracks both tape and audio file hashes for incremental builds
