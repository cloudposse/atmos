# Demo of Atmos

Output docs/demo.gif

Set Theme "Monokai Vivid"

Set FontFamily "FiraCode Nerd Font"
#Set FontFamily "Hack Nerd Font"
Set FontSize 14

Set TypingSpeed 40ms

Set Framerate 30

Set WindowBar Colorful

Set CursorBlink true

Set BorderRadius 8

Set Margin 20
Set MarginFill "#674EFF"

Set Padding 12

Set Width 1200
Set Height 600

Set PlaybackSpeed 1

Set Shell bash

Type "# First check you have atmos installed" Sleep 500ms Enter
Type "atmos version" Sleep 500ms Enter
Sleep 1

Type "# Now let's explore the quick start example" Sleep 500ms Enter
Type "cd examples/quick-start" Sleep 500ms Enter

Type "ls -al" Sleep 500ms Enter
Sleep 1

Type "# Let's see what components we have available!" Sleep 500ms Enter
Type "atmos list components" Sleep 500ms Enter
Sleep 500ms 

Type "# Let's see where they can be deployed" Sleep 500ms Enter
Type "atmos list stacks" Sleep 500ms Enter

Type "# Let's validate the stacks" Sleep 500ms Enter
Type "atmos validate stacks" Sleep 500ms Enter

Type "# Let's review the production VPC configuration in the ue2 region..." Sleep 500ms Enter
Type "atmos describe stacks --components=vpc --stack=plat-ue2-prod --sections=vars" Sleep 500ms Enter
Sleep 500ms

Type "# Or checkout all VPCs" Sleep 500ms Enter
Type "atmos describe stacks --components=vpc --sections=vars | less" Sleep 500ms Enter
Sleep 500ms

Down 25 Sleep 500ms
Down 25 Sleep 500ms
Down 25 Sleep 500ms
Down 25 Sleep 500ms

Type "q" Sleep 500ms
Sleep 1
Type "# atmos has a lot of documented commands" Sleep 500ms Enter
Type "atmos --help" Sleep 500ms Enter
Sleep 500ms
Type "# atmos has native workflows for terraform" Sleep 500ms Enter
Type "atmos terraform --help" Sleep 500ms Enter
Sleep 2s
Type "# check out the docs at https://atmos.tools/" Sleep 500ms Enter
Sleep 500ms
Type "# or join us in #atmos at https://slack.cloudposse.com!" Sleep 500ms Enter
Sleep 2s
