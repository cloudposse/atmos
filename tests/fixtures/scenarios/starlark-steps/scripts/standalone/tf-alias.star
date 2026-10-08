#!/usr/bin/env atmos
# atmos.tf takes the same component= and stack= keywords as atmos.terraform. The command may fail
# when Terraform is not installed; the point is that the call is accepted and builds an argv.
result = atmos.tf("plan", component = "mock", stack = "dev", check = False, output = "capture")
print("tf alias accepted keywords")
