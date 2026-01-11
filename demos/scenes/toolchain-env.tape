# Toolchain Environment scene - Shell integration
Output toolchain-env.gif
Output toolchain-env.mp4
Output toolchain-env.svg

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
Type "# Integrate toolchain with your shell"
Enter
Sleep 1s

Type "# Show path to a specific tool"
Enter
Sleep 300ms

Type "atmos toolchain which opentofu"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot toolchain-env.png

Type "# Get the full path for scripts"
Enter
Sleep 300ms

Type "atmos toolchain path opentofu"
Sleep 2s
Enter
Sleep 3s

Type "# Export environment for shell integration"
Enter
Sleep 300ms

Type "atmos toolchain env"
Sleep 2s
Enter
Sleep 3s

Type "# Add to your shell: eval \"$(atmos toolchain env)\""
Enter
Sleep 1s

Type "# Tools automatically available in PATH!"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
