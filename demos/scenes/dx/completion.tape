# completion scene - Demonstrate shell tab completion
Output dx-completion.gif
Output dx-completion.mp4

# VHS Configuration
Source ../../scenes/_theme.tape
Set Width 1400
Set Height 800

# Fix prompt and source completion (hidden setup)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Type "source <(atmos completion bash)"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts - show how to enable
Type "# Enable tab completion for atmos"
Enter
Sleep 500ms

Type "# Add this to your shell profile (~/.bashrc or ~/.zshrc)"
Enter
Sleep 300ms

Type "echo 'source <(atmos completion bash)'"
Sleep 500ms
Enter
Sleep 1s

Type "# Now let's see it in action!"
Enter
Sleep 500ms

# Demo tab completion - type partial command, hit Tab
Type "atmos ter"
Sleep 300ms
Tab
Sleep 500ms

Type " pla"
Sleep 300ms
Tab
Sleep 500ms

Type " --"
Sleep 300ms
Tab
Tab
Sleep 1s

# Clear and show another example
Ctrl+C
Sleep 300ms

Type "atmos desc"
Sleep 300ms
Tab
Sleep 500ms

Type " comp"
Sleep 300ms
Tab
Sleep 500ms

Type " vpc -s "
Sleep 300ms
Tab
Tab
Sleep 1s

# Capture screenshot
Screenshot dx-completion.png

Sleep 1s
