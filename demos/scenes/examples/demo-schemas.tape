# Demo Schemas Example
# Validate configuration against JSON Schema
Output demo-schemas.gif
Output demo-schemas.mp4
Output demo-schemas.svg

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
Type "# Demo Schemas: JSON Schema validation"
Enter
Sleep 1s

Type "# Validate YAML against JSON schemas"
Enter
Sleep 500ms

Type "atmos validate schema"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-schemas.png

Type "# Schemas catch configuration errors early"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
