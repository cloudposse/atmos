# completion scene - Demonstrate shell tab completion
Output completion.gif
Output completion.mp4
Output completion.svg

# VHS Configuration
Source _theme.tape
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
Sleep 2s

Type "# Add this to your shell profile (~/.bashrc or ~/.zshrc)"
Enter
Sleep 300ms

Type "echo 'source <(atmos completion bash)'"
Sleep 2s
Enter
Sleep 1s

Type "# Now let's see it in action!"
Enter
Sleep 2s

# Demo tab completion - type partial command, hit Tab
Type "atmos ter"
Sleep 300ms
Tab
Sleep 2s

Type " pla"
Sleep 300ms
Tab
Sleep 2s

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
Sleep 2s

Type " comp"
Sleep 300ms
Tab
Sleep 2s

Type " vpc -s "
Sleep 300ms
Tab
Tab
Sleep 1s

# Capture screenshot
Screenshot completion.png

Sleep 1s


# Hold final frame
Sleep 10s
