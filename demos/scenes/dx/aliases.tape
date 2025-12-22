# aliases scene - Use shorthand aliases for common commands
Output dx-aliases.gif
Output dx-aliases.mp4

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
Type "# Use shorthand aliases for faster workflows"
Enter
Sleep 1s

Type "# View available aliases in atmos.yaml"
Enter
Sleep 500ms

Type "grep -A 10 'aliases:' atmos.yaml"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot dx-aliases.png

Type "# Use 'tp' instead of 'terraform plan'"
Enter
Sleep 500ms

Type "atmos tp vpc -s plat-dev-ue2"
Sleep 500ms
Enter
Sleep 3s

Type "# Work faster with less typing"
Enter
Sleep 1s
