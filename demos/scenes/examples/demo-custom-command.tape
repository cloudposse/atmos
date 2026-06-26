# Demo Custom Command Example
# Extend Atmos with custom commands
Output demo-custom-command.gif
Output demo-custom-command.mp4
Output demo-custom-command.svg

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
Type "# Demo Custom Commands: Extend Atmos CLI"
Enter
Sleep 1s

Type "# See available custom commands"
Enter
Sleep 500ms

Type "atmos --help"
Sleep 1s
Enter
Sleep 3s

Type "# Run a custom command"
Enter
Sleep 500ms

Type "atmos hello"
Sleep 1s
Enter
Sleep 2s

# Capture screenshot
Screenshot demo-custom-command.png

Type "# Custom commands integrate with your toolchain"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
