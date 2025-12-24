# commands scene - Extend Atmos with your own commands
Output commands.gif
Output commands.mp4
Output commands.svg

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
Type "# Extend Atmos with your own commands"
Enter
Sleep 1s

Type "# View custom commands in atmos.yaml"
Enter
Sleep 2s

Type "grep -A 15 'commands:' atmos.yaml"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot commands.png

Type "# Run your custom command"
Enter
Sleep 2s

Type "atmos deploy-stack plat-dev-ue2"
Sleep 2s
Enter
Sleep 3s

Type "# Your workflows, your commands - no scripting"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
