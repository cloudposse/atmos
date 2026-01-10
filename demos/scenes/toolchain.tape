# Toolchain scene - Native tool management
Output toolchain.gif
Output toolchain.mp4
Output toolchain.svg

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
Type "# Native toolchain management in Atmos"
Enter
Sleep 1s

Type "# List configured tools"
Enter
Sleep 300ms

Type "atmos toolchain list"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot toolchain.png

Type "# Install multiple tools at once"
Enter
Sleep 300ms

Type "atmos toolchain install opentofu helm k9s tflint"
Sleep 2s
Enter
Sleep 5s

Type "# Execute installed tools directly"
Enter
Sleep 300ms

Type "atmos toolchain exec opentofu -- --version"
Sleep 1s
Enter
Sleep 2s

Type "atmos toolchain exec helm -- version"
Sleep 1s
Enter
Sleep 2s

Type "atmos toolchain exec tflint -- --version"
Sleep 1s
Enter
Sleep 2s

Type "# Tools tracked in .tool-versions (asdf-compatible)"
Enter
Sleep 300ms

Type "cat .tool-versions"
Sleep 1s
Enter
Sleep 2s

Type "# No more manual tool installation!"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
