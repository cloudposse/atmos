# Terminal Themes Package

This package provides terminal theme support for Atmos markdown rendering.

## Attribution

The `themes.json` file contains terminal color themes sourced from:
- **Source**: https://github.com/charmbracelet/vhs
- **License**: MIT License
- **Copyright**: (c) 2022 Charmbracelet, Inc

Each individual theme in the collection maintains its original creator's attribution in the `meta.credits` field. See `LICENSE-THEMES` for full attribution details.

## Files

- `theme.go` - Core theme structures and management
- `registry.go` - Theme registry for loading and accessing themes
- `converter.go` - Converts terminal themes to glamour styles
- `themes.json` - Embedded JSON file with 349 terminal themes
- `LICENSE-THEMES` - Full attribution and license information

## Usage

```go
// Load a theme by name
registry, err := theme.NewRegistry()
if err != nil {
    return err
}

// Get a specific theme
dracula, exists := registry.Get("dracula")

// Convert to glamour style
style, err := theme.ConvertToGlamourStyle(dracula)
```

## Adding New Themes

New themes can be added to `themes.json`. Each theme should include:
- Standard terminal colors (black, red, green, yellow, blue, magenta, cyan, white, and bright variants)
- Background and foreground colors
- Metadata including `isDark` flag and credits

The "default" theme is the first entry and is optimized for Atmos output.
