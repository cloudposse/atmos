# tui scene - Interactive Atmos TUI
Output flagship-tui.gif
Output flagship-tui.mp4

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
Type "# Launch the Atmos TUI"
Enter
Sleep 1s

Type "atmos"
Sleep 500ms
Enter
Sleep 2s

# Navigate in the TUI - show off the interface
# Move down in Commands panel
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms

# Capture screenshot showing the TUI
Screenshot flagship-tui.png

# Switch to Stacks panel with Tab
Tab
Sleep 500ms

# Navigate stacks
Down
Sleep 400ms
Down
Sleep 400ms

# Switch to Components panel
Tab
Sleep 500ms

# Navigate components
Down
Sleep 400ms

# Go back to Commands
Tab
Sleep 500ms

# Navigate to a command and execute
Down
Sleep 400ms
Down
Sleep 400ms

# Execute command with Enter
Enter
Sleep 3s

# Exit the TUI
Escape
Sleep 500ms

Type "# The TUI provides an intuitive way to explore and run Atmos commands"
Enter
Sleep 2s
