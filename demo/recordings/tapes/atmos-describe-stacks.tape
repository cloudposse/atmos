Source demo/defaults.tape
Source demo/recordings/styles/defaults.tape
Source demo/recordings/styles/quick-start-advanced.tape
Source demo/recordings/styles/less.tape

Type "# Let's review the production VPC configuration in the us-east-2 region..." Sleep 500ms Enter
Type "atmos describe stacks --components=vpc --stack=plat-ue2-prod --sections=vars" Sleep 500ms Enter
#Down 5 Sleep 500ms
#Down 5 Sleep 500ms
#Ctrl+C
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

Hide
Ctrl+C
Show
