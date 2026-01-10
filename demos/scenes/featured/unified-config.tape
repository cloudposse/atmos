# Featured: One Config, Every Environment
# Story: No installing terraform, no tfenv, no version switching - just works
Output featured-unified-config.gif
Output featured-unified-config.mp4
Output featured-unified-config.svg

# VHS Configuration - Atmos theme (inlined)
Set Theme { "name": "Atmos", "black": "#1a1a1a", "red": "#f4005f", "green": "#98e024", "yellow": "#fa8419", "blue": "#9d65ff", "magenta": "#f4005f", "cyan": "#58d1eb", "white": "#c4c5b5", "brightBlack": "#625e4c", "brightRed": "#f4005f", "brightGreen": "#98e024", "brightYellow": "#e0d561", "brightBlue": "#9d65ff", "brightMagenta": "#f4005f", "brightCyan": "#58d1eb", "brightWhite": "#f6f6ef", "background": "#1a1a1a", "foreground": "#f6f6ef", "selection": "#555555", "cursor": "#5A56E0" }
Set FontFamily "FiraCode Nerd Font"
Set FontSize 14
Set LineHeight 1.0
Set Padding 10
Set Width 1400
Set Height 800
Set TypingSpeed 80ms

# Fix prompt
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts - The "Before" problem
Type "# Before: Manual tool management, version conflicts, env vars everywhere"
Enter
Sleep 2s

Type "# After: One command, every environment"
Enter
Sleep 2s

# Show the unified experience
Type "atmos terraform plan vpc -s plat-dev-ue2"
Sleep 1.5s
Enter
Sleep 4s

# Capture screenshot at plan output
Screenshot featured-unified-config.png

Type "# Same command, different environment - same tooling"
Enter
Sleep 1s

Type "atmos terraform plan vpc -s plat-prod-uw2"
Sleep 1.5s
Enter
Sleep 4s

Type "# OpenTofu installed automatically, no tfenv needed"
Enter
Sleep 1s

Type "atmos toolchain list"
Sleep 1.5s
Enter
Sleep 3s

Type "# Stack config drives everything - tools, vars, backends"
Enter
Sleep 1s

Type "ATMOS_FORCE_COLOR=true atmos describe component vpc -s plat-dev-ue2 --format yaml | head -20"
Sleep 1.5s
Enter
Sleep 4s

Type "# One config. Every environment. Just works."
Enter
Sleep 2s

# Hold final frame
Sleep 8s
