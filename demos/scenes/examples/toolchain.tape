# Toolchain Example
# Configure tool registries and versions
Output toolchain.gif
Output toolchain.mp4
Output toolchain.svg

# VHS Configuration
Source "../_theme.tape"
Set Width 1400
Set Height 800

# Enable TrueColor for proper color rendering
Env COLORTERM "truecolor"

# Fix prompt (VHS runs with --no-rcs)
Env PS1 "\033[38;2;90;86;224m> \033[0m"

Hide
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# Toolchain: Manage tool versions"
Enter
Sleep 1s

Type "# List installed tools"
Enter
Sleep 500ms

Type "atmos toolchain list"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot toolchain.png

Type "# Toolchain integrates with custom commands"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
