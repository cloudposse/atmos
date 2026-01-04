# list-components scene - Explore component metadata
Output list-components.gif
Output list-components.mp4
Output list-components.svg

# VHS Configuration
Source ../../scenes/_theme.tape
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
Type "# Explore your infrastructure with rich metadata"
Enter
Sleep 1s

Type "atmos list components"
Sleep 2s
Enter
Sleep 5s

# Capture screenshot
Screenshot list-components.png

Enter
Type "# Filter components with simplified YQ expressions"
Enter
Sleep 1s

Type@150ms `atmos list components --query 'select(.kind == "terraform")'`
Sleep 2s
Enter
Sleep 5s

Enter
Type "# Filter locked components"
Enter
Sleep 1s

Type@150ms `atmos list components --query 'select(.locked == true)'`
Sleep 2s
Enter
Sleep 5s

Enter
Type "# Combine filters with stack patterns"
Enter
Sleep 1s

Type@150ms `atmos list components --query 'select(.stack | test("prod"))' --stack 'plat-*'`
Sleep 2s
Enter
Sleep 5s

# Hold final frame
Sleep 10s
