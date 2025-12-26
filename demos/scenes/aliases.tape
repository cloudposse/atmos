# aliases scene - Use shorthand aliases for common commands
Output aliases.gif
Output aliases.mp4
Output aliases.svg

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
Type "# Use shorthand aliases for faster workflows"
Enter
Sleep 1s

Type "# View available aliases in atmos.yaml"
Enter
Sleep 2s

Type "grep -A 10 'aliases:' atmos.yaml"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot aliases.png

Type "# Use 'ls' instead of 'list stacks'"
Enter
Sleep 2s

Type "atmos ls"
Sleep 2s
Enter
Sleep 3s

Type "# Use 'dc' instead of 'describe component'"
Enter
Sleep 1s

Type "atmos dc vpc -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 3s

Type "# Work faster with less typing"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
