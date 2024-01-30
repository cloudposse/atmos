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

Set Padding 10

Set Width 1400
Set Height 800

Set PlaybackSpeed 1

Set Shell bash

Type "# First check you have Atmos installed" Sleep 500ms Enter
Type "atmos version" Sleep 500ms Enter
Sleep 1s

Type "# Now let's explore the Quick Start example" Sleep 500ms Enter
Type "cd examples/quick-start" Sleep 500ms Enter

Type "ls -al" Sleep 500ms Enter
Sleep 1s

Type "# We will start by installing some 3rd-party components and other artifacts..." Sleep 500ms Enter
Type "atmos vendor pull" Sleep 500ms Enter
Sleep 7s

Type "# In Atmos you can easily explore components, stacks, and run commands..." Sleep 500ms Enter
Type "atmos" Sleep 500ms Enter
Sleep 2s
Down Sleep 500ms
Down Sleep 500ms
Up Sleep 500ms
Up Sleep 500ms
Up Sleep 500ms
Up Sleep 500ms
Up Sleep 1s
Right Sleep 1s
Down Sleep 500ms
Down Sleep 500ms
Down Sleep 500ms
Right Sleep 1s
Down Sleep 500ms
Up Sleep 1s
Enter
Sleep 1s

Type "# Let's see what components we have available!" Sleep 500ms Enter
Type "atmos list components" Sleep 500ms Enter
Sleep 1s

Type "# Let's see where they can be deployed" Sleep 500ms Enter
Type "atmos list stacks" Sleep 500ms Enter
Sleep 2s

Type "# Let's validate the stack configurations" Sleep 500ms Enter
Type "atmos validate stacks" Sleep 500ms Enter
Sleep 2s

Type "# Let's review the production VPC configuration in the us-east-2 region..." Sleep 500ms Enter
Type "atmos describe stacks --components=vpc --stack=plat-ue2-prod --sections=vars" Sleep 500ms Enter
Sleep 2s

Type "# Or checkout all VPCs" Sleep 500ms Enter
Type "atmos describe stacks --components=vpc --sections=vars | less" Sleep 500ms Enter
Sleep 1s

Down 25 Sleep 500ms
Down 25 Sleep 500ms
Down 25 Sleep 500ms
Down 25 Sleep 500ms

Type "q" Sleep 500ms
Sleep 1s

Type "# In Atmos you can easily explore workflows and execute workflow commands..." Sleep 500ms Enter
Type "atmos workflow" Sleep 500ms Enter
Sleep 2s
Down Sleep 500ms
Down Sleep 500ms
Down Sleep 500ms
Right Sleep 500ms
Down Sleep 500ms
Down Sleep 500ms
Down Sleep 500ms
Right Sleep 500ms
Down Sleep 500ms
Down Sleep 500ms
Up Sleep 500ms
Up Sleep 500ms
Enter
Sleep 3s

Type "# Atmos has a lot of documented commands" Sleep 500ms Enter
Type "atmos --help" Sleep 500ms Enter
Sleep 3s

Type "# Atmos has native workflows for Terraform" Sleep 500ms Enter
Type "atmos terraform --help" Sleep 500ms Enter
Sleep 4s

Type "# Check out the docs at https://atmos.tools/" Sleep 500ms Enter
Sleep 1s

Type "# or join us in #atmos at https://slack.cloudposse.com!" Sleep 500ms Enter
Sleep 3s
