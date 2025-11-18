# Atmos Demo Videos

This directory contains VHS scenes for generating demonstration videos and screenshots of Atmos commands.

## Quick Start

```bash
# From repository root
make director
./build/director catalog

# Or from demos directory
cd demos
make help            # Show all available targets
make install         # Install dependencies
make validate        # Validate all scenes
make render          # Render all scenes
make catalog         # List all scenes
make new NAME=my-scene  # Create new scene
```

## Directory Structure

```
demos/
├── scenes.yaml           # Scene index (defines all demos)
├── defaults.yaml         # Global VHS and optimization settings
├── scenes/               # VHS tape files
│   ├── terraform-plan-basic.tape
│   ├── describe-stacks.tape
│   ├── workflow-deploy.tape
│   └── demo-stacks/      # Demo stacks subfolder
│       ├── describe.tape
│       ├── component.tape
│       ├── validate.tape
│       ├── terraform-plan.tape
│       └── list.tape
├── fixtures/             # Test configurations for scenes
│   ├── basic-config/
│   └── auth-config/
├── .cache/               # Generated outputs (gitignored)
│   ├── metadata.json
│   ├── *.gif
│   └── *.png
└── README.md             # This file
```

## Scene Catalog

### Core Commands

| Scene | Description | Demo |
|-------|-------------|------|
| terraform-plan-basic | Basic terraform plan workflow | [▶️](https://cloudposse.github.io/atmos/demos/terraform-plan-basic.gif) |
| describe-stacks | List all stacks with descriptions | [▶️](https://cloudposse.github.io/atmos/demos/describe-stacks.gif) |
| workflow-deploy | Run a workflow to deploy infrastructure | [▶️](https://cloudposse.github.io/atmos/demos/workflow-deploy.gif) |

### Demo Stacks (Homepage Examples)

| Scene | Description | Demo |
|-------|-------------|------|
| demo-stacks-describe | Describe stacks in demo-stacks example | [▶️](https://cloudposse.github.io/atmos/demos/demo-stacks-describe.gif) |
| demo-stacks-component | Describe component configuration across environments | [▶️](https://cloudposse.github.io/atmos/demos/demo-stacks-component.gif) |
| demo-stacks-validate | Validate stack and component configurations | [▶️](https://cloudposse.github.io/atmos/demos/demo-stacks-validate.gif) |
| demo-stacks-terraform-plan | Run terraform plan with demo-stacks | [▶️](https://cloudposse.github.io/atmos/demos/demo-stacks-terraform-plan.gif) |
| demo-stacks-list | List stacks and components | [▶️](https://cloudposse.github.io/atmos/demos/demo-stacks-list.gif) |

## Creating a New Scene

1. **Create scene entry in `scenes.yaml`:**
   ```yaml
   - name: "my-feature"
     enabled: true
     description: "Demonstrates my awesome feature"
     tape: "scenes/my-feature.tape"
     requires:
       - atmos
     outputs:
       - gif
       - png
   ```

2. **Create VHS tape file in `scenes/my-feature.tape`:**
   ```tape
   Output my-feature.gif

   Set Theme "Catppuccin Mocha"
   Set Width 1400
   Set Height 800
   Set FontSize 14

   Require atmos

   Type "atmos my-command"
   Sleep 500ms
   Enter
   Sleep 2s

   Screenshot my-feature.png
   ```

3. **Test locally:**
   ```bash
   ./build/director render my-feature
   ```

4. **Commit only the tape file:**
   ```bash
   git add demos/scenes.yaml demos/scenes/my-feature.tape
   git commit -m "feat: add my-feature demo"
   ```

CI will automatically generate and deploy the GIF to GitHub Pages.

## How It Works

### Distributed Cache

The director tool implements a distributed cache to avoid regenerating unchanged demos:

1. Fetches `metadata.json` from GitHub Pages (contains SHA256 hashes of tape files)
2. Compares local tape file SHA256 with cached metadata
3. Only regenerates GIFs if tape file changed or doesn't exist
4. Saves generated assets to `.cache/` with updated metadata

This means:
- ✅ Fresh clones only generate new/changed scenes
- ✅ Contributors don't regenerate everything
- ✅ Fast iteration during development

### Storage

- **Source files** (`.tape`) committed to main repo
- **Generated assets** (`.gif`, `.png`) deployed to gh-pages branch
- **gh-pages branch** uses orphan mode (no history bloat)
- **Accessible at** `https://cloudposse.github.io/atmos/demos/{scene}.gif`

## Dependencies

- [VHS](https://github.com/charmbracelet/vhs) - Terminal recorder
- [gifsicle](https://www.lcdf.org/gifsicle/) - GIF optimizer
- [Freeze](https://github.com/charmbracelet/freeze) - Static screenshots (optional)
- director - Our orchestration tool (`tools/director`)

## Best Practices

1. **Keep scenes short** - 30-60 seconds max
2. **Use realistic examples** - Mirror actual user workflows
3. **Optimize GIFs** - Target < 500 KB per demo
4. **Check dependencies** - Use `Require` in tape files
5. **Test before committing** - Run `director render` locally
6. **Use fixtures** - Create reusable test configs in `fixtures/`

## Troubleshooting

**VHS not found:**
```bash
brew install vhs
```

**gifsicle not found:**
```bash
brew install gifsicle
```

**Scene not rendering:**
```bash
# Check dependencies
./build/director validate

# Force regenerate
./build/director render --force my-scene

# Run VHS directly for debugging
vhs demos/scenes/my-scene.tape
```

**Stale cache:**
```bash
# Clear local cache
rm -rf demos/.cache/*

# Regenerate all
./build/director render --force
```

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines on adding new demos.

For blog posts featuring new functionality, please include a demo video when possible.
