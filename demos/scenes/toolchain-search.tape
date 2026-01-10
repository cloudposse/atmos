# Toolchain Search scene - Find tools in the registry
Output toolchain-search.gif
Output toolchain-search.mp4
Output toolchain-search.svg

# VHS Configuration
Source _theme.tape
Set Width 1400
Set Height 800

# Fix prompt (VHS runs with --no-rcs)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos
Require jq

# Scene starts
Type "# Search for tools in the Atmos registry"
Enter
Sleep 1s

Type "# See how many tools are available"
Enter
Sleep 300ms

Type "atmos toolchain registry list aqua --limit 10000 --format json | jq length"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot toolchain-search.png

Type "# Find OpenTofu versions"
Enter
Sleep 300ms

Type "atmos toolchain search opentofu"
Sleep 2s
Enter
Sleep 3s

Type "# Search for Kubernetes tools"
Enter
Sleep 300ms

Type "atmos toolchain search helm"
Sleep 2s
Enter
Sleep 3s

Type "# Search is fuzzy - partial matches work"
Enter
Sleep 300ms

Type "atmos toolchain search k9"
Sleep 2s
Enter
Sleep 3s

Type "# Find linting tools"
Enter
Sleep 300ms

Type "atmos toolchain search tflint"
Sleep 2s
Enter
Sleep 3s

Type "# Discover the right tool for any job!"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
