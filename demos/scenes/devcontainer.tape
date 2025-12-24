# devcontainer scene - Isolated development environments
Output devcontainer.gif
Output devcontainer.mp4
Output devcontainer.svg

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

# Scene starts
Type "# Isolated development environments with devcontainers"
Enter
Sleep 2s

Type "# List available devcontainers"
Enter
Sleep 300ms

Type "atmos devcontainer list"
Sleep 2s
Enter
Sleep 2s

# Capture screenshot
Screenshot devcontainer.png

Type "# Start a devcontainer"
Enter
Sleep 300ms

Type "atmos devcontainer start default"
Sleep 2s
Enter
Sleep 3s

Type "# Attach to the running container"
Enter
Sleep 300ms

Type "atmos devcontainer attach default"
Sleep 2s
Enter
Sleep 2s

Type "# Or launch directly with shell"
Enter
Sleep 300ms

Type "atmos devcontainer shell default"
Sleep 2s
Enter
Sleep 2s

Type "# Reproducible environments - no more 'works on my machine'"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
