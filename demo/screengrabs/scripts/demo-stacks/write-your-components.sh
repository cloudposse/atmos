#!/bin/bash

relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc

echo "# Here's an example of how to organize components"

run tree components
newline 2
comment "Let's take a look at myapp terraform 'root' module..."
comment "This is a simple example of retrieving the weather."
comment "Taking a closer look at the main.tf, you'll notice it accepts a lot of parameters."
comment "This is a best practice for writing reusable components."
prompt
run cat components/terraform/myapp/main.tf

newline 2
comment "Then we define all the variables we plan to accept."
comment "Generally, we recommend avoiding defaults here and using baseline stack configurations."
prompt
run cat components/terraform/myapp/variables.tf

newline 2
comment "Then let's provide some outputs that can be used by other components."
prompt
run cat components/terraform/myapp/outputs.tf

newline 2
comment "It's a best practice to pin verions, so let's do that."
prompt
run cat components/terraform/myapp/versions.tf

