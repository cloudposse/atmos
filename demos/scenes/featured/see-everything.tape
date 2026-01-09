# Featured: See Everything
# Story: No more ls terraform/ or hunting through directories
Output featured-see-everything.gif
Output featured-see-everything.mp4
Output featured-see-everything.svg

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

# Scene starts
Type "# Before: ls terraform/, cat varfiles, grep to find what changed"
Enter
Sleep 2s

Type "# After: See everything at a glance"
Enter
Sleep 2s

# Show stacks
Type "atmos list stacks --format tree"
Sleep 1.5s
Enter
Sleep 4s

# Capture screenshot
Screenshot featured-see-everything.png

# Show components
Type "# What components are available?"
Enter
Sleep 1s

Type "atmos list components"
Sleep 1.5s
Enter
Sleep 3s

# Show vars across environments
Type "# Compare vars across all environments"
Enter
Sleep 1s

Type "atmos list vars vpc"
Sleep 1.5s
Enter
Sleep 4s

# Show what changed
Type "# What's affected by my changes?"
Enter
Sleep 1s

Type "atmos describe affected --format table"
Sleep 1.5s
Enter
Sleep 4s

Type "# No more hunting through directories"
Enter
Sleep 2s

# Hold final frame
Sleep 8s
