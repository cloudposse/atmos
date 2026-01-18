# Demo Library Example
# Distribute reusable components
Output demo-library.gif
Output demo-library.mp4
Output demo-library.svg

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
Type "# Demo Library: Reusable component distribution"
Enter
Sleep 1s

Type "# View available components in the library"
Enter
Sleep 500ms

Type "ls -la"
Sleep 1s
Enter
Sleep 2s

Type "# View a component's structure"
Enter
Sleep 500ms

Type "ls -la weather/"
Sleep 1s
Enter
Sleep 2s

# Capture screenshot
Screenshot demo-library.png

Type "# Components can be vendored into projects"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
