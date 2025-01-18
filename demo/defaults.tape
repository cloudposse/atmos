# VHS Defaults for Recordings and Screenshots

#Set Theme "Monokai Vivid"

# It's not possible to combine named themes, with explicitly defined themes
Set Theme { "name": "Monokai Vivid", "black": "#121212", "red": "#fa2934", "green": "#98e123", "yellow": "#fff30a", "blue": "#0443ff", "magenta": "#f800f8", "cyan": "#01b6ed", "white": "#ffffff", "brightBlack": "#838383", "brightRed": "#f6669d", "brightGreen": "#b1e05f", "brightYellow": "#fff26d", "brightBlue": "#0443ff", "brightMagenta": "#f200f6", "brightCyan": "#51ceff", "brightWhite": "#ffffff", "background": "#121212", "foreground": "#f9f9f9", "cursor": "#b3b0d6", "selection": "#ffffff"}

Set FontFamily "FiraCode Nerd Font"
#Set FontFamily "Hack Nerd Font"
Set FontSize 14

Set TypingSpeed 20ms

Set WindowBar Colorful

Set BorderRadius 8

Set Margin 0

Set Padding 10

# !! WARNING !!
# Large aspect ratios require more memory to process (and a factor of the framerate)
# Free-tier GitHub Action runners do not have sufficient memory to process long recordings
# at a high framerate before crashing. Therefore, for automation purposes we need to keep these small.

# Use standard aspect ratios
Set Width 1280
Set Height 720

Set Shell "bash"
