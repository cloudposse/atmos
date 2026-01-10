# Featured: Tools That Just Work
# Story: No more tfenv, tofuenv, aqua, brew juggling - Atmos handles it
Output featured-tools-just-work.gif
Output featured-tools-just-work.mp4
Output featured-tools-just-work.svg

# VHS Configuration - Atmos theme (inlined)
Set Theme { "name": "Atmos", "black": "#1a1a1a", "red": "#f4005f", "green": "#98e024", "yellow": "#fa8419", "blue": "#9d65ff", "magenta": "#f4005f", "cyan": "#58d1eb", "white": "#c4c5b5", "brightBlack": "#625e4c", "brightRed": "#f4005f", "brightGreen": "#98e024", "brightYellow": "#e0d561", "brightBlue": "#9d65ff", "brightMagenta": "#f4005f", "brightCyan": "#58d1eb", "brightWhite": "#f6f6ef", "background": "#1a1a1a", "foreground": "#f6f6ef", "selection": "#555555", "cursor": "#5A56E0" }
Set FontFamily "FiraCode Nerd Font"
Set FontSize 14
Set LineHeight 1.0
Set Padding 10
Set Width 1200
Set Height 600
Set TypingSpeed 80ms

# Enable TrueColor for proper color rendering in VHS
Env COLORTERM "truecolor"

# Fix prompt
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# Before: tfenv, tofuenv, aqua, brew, asdf - tool sprawl"
Enter
Sleep 2s

Type "# After: Atmos handles it"
Enter
Sleep 2s

# Search for tools
Type "atmos toolchain search tofu"
Sleep 1.5s
Enter
Sleep 3s

Enter
Sleep 0.5s
# Install multiple tools
Type "# Install everything you need in one command"
Enter
Sleep 1s

Type "atmos toolchain install"
Sleep 1.5s
Enter
Sleep 5s

Enter
Sleep 0.5s
# Show what's installed
Type "atmos toolchain list"
Sleep 1.5s
Enter
Sleep 3s

# Capture screenshot showing toolchain list output
Screenshot featured-tools-just-work.png

Enter
Sleep 0.5s
# Show tools work automatically with terraform commands
Type "# Tools work automatically - no PATH juggling"
Enter
Sleep 1s

Type "atmos terraform plan vpc -s plat-dev-ue2"
Sleep 1.5s
Enter
Sleep 4s

# Clear screen after terraform plan
Ctrl+L
Sleep 0.5s
# Show where tools are installed
Type "# Tools installed and managed by atmos"
Enter
Sleep 1s

Type "atmos toolchain which opentofu/opentofu"
Sleep 1.5s
Enter
Sleep 2s

Enter
Sleep 0.5s
Type "# One tool to rule them all"
Enter
Sleep 2s

# Hold final frame
Sleep 8s
