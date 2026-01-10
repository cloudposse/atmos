# use-version scene - Switch Atmos versions on the fly
Output use-version.gif
Output use-version.mp4
Output use-version.svg

# VHS Configuration
Source _theme.tape
Set Width 1400
Set Height 800

# Enable TrueColor for proper color rendering in VHS
Env COLORTERM "truecolor"

# Fix prompt (VHS runs with --no-rcs)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# Switch Atmos versions on the fly"
Enter
Sleep 1s

Type "# Current version"
Enter
Sleep 300ms

Type "atmos version"
Sleep 1s
Enter
Sleep 2s

# Capture screenshot
Screenshot use-version.png

Type "# Run with a different version (auto-installs if needed)"
Enter
Sleep 300ms

Type "atmos --use-version 1.160.0 version"
Sleep 2s
Enter
Sleep 5s

Type "# Back to current version"
Enter
Sleep 300ms

Type "atmos version"
Sleep 1s
Enter
Sleep 2s

Type "# Perfect for testing compatibility!"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
