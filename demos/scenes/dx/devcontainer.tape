# devcontainer scene - Isolated development environments
Output dx-devcontainer.gif
Output dx-devcontainer.mp4

# VHS Configuration
Source ../../scenes/_theme.tape
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
Sleep 500ms

Type "# List available devcontainers"
Enter
Sleep 300ms

Type "atmos devcontainer list"
Sleep 500ms
Enter
Sleep 2s

# Capture screenshot
Screenshot dx-devcontainer.png

Type "# Start a devcontainer"
Enter
Sleep 300ms

Type "atmos devcontainer start default"
Sleep 500ms
Enter
Sleep 3s

Type "# Attach to the running container"
Enter
Sleep 300ms

Type "atmos devcontainer attach default"
Sleep 500ms
Enter
Sleep 2s

Type "# Or launch directly with shell"
Enter
Sleep 300ms

Type "atmos devcontainer shell default"
Sleep 500ms
Enter
Sleep 2s

Type "# Reproducible environments - no more 'works on my machine'"
Enter
Sleep 1s
