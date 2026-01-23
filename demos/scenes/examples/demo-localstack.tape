# Demo LocalStack Example
# Test with LocalStack AWS emulator
Output demo-localstack.gif
Output demo-localstack.mp4
Output demo-localstack.svg

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
Type "# Demo LocalStack: Test with AWS emulator"
Enter
Sleep 1s

Type "# Describe component against LocalStack"
Enter
Sleep 500ms

Type "atmos describe component demo -s dev"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-localstack.png

Type "# LocalStack provides local AWS services"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
