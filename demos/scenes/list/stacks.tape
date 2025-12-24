# list-stacks scene - List and explore stacks
Output list-stacks.gif
Output list-stacks.mp4
Output list-stacks.svg

# VHS Configuration - Atmos theme (inlined to avoid Source path issues with filtered tapes)
Set Theme { "name": "Atmos", "black": "#1a1a1a", "red": "#f4005f", "green": "#98e024", "yellow": "#fa8419", "blue": "#9d65ff", "magenta": "#f4005f", "cyan": "#58d1eb", "white": "#c4c5b5", "brightBlack": "#625e4c", "brightRed": "#f4005f", "brightGreen": "#98e024", "brightYellow": "#e0d561", "brightBlue": "#9d65ff", "brightMagenta": "#f4005f", "brightCyan": "#58d1eb", "brightWhite": "#f6f6ef", "background": "#1a1a1a", "foreground": "#f6f6ef", "selection": "#555555", "cursor": "#5A56E0" }
Set FontFamily "FiraCode Nerd Font"
Set FontSize 14
Set LineHeight 1.0
Set Padding 10
Set Width 1400
Set Height 800
Set TypingSpeed 80ms

# Fix prompt (VHS runs with --no-rcs)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# Explore your infrastructure at a glance"
Enter
Sleep 2s

Type "atmos list stacks"
Sleep 2s
Enter
Sleep 5s

# Capture screenshot
Screenshot list-stacks.png

Enter
Type "# View stacks as a hierarchical tree"
Enter
Sleep 1s

Type "atmos list stacks --format tree"
Sleep 2s
Enter
Sleep 5s

Enter
Type "# Show import provenance to understand inheritance"
Enter
Sleep 1s

Type "atmos list stacks --format tree --provenance"
Sleep 2s
Enter
Sleep 5s

Enter
Type "# Filter stacks by component"
Enter
Sleep 1s

Type "atmos list stacks -c vpc"
Sleep 2s
Enter
Sleep 5s

Enter
Type "# Or use wildcards for multiple stacks"
Enter
Sleep 1s

Type "atmos list stacks -c vpc --format tree 'plat-prod-*'"
Sleep 2s
Enter
Sleep 5s

# Hold final frame
Sleep 10s
