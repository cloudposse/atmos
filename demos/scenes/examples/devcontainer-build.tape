# Devcontainer Build Example
# Custom Dockerfile with devcontainers
Output devcontainer-build.gif
Output devcontainer-build.mp4
Output devcontainer-build.svg

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
Type "# Devcontainer Build: Custom Dockerfile"
Enter
Sleep 1s

Type "# List available devcontainers"
Enter
Sleep 500ms

Type "atmos devcontainer list"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot devcontainer-build.png

Type "# Custom Dockerfiles extend base images"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
