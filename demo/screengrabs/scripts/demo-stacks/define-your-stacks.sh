#!/bin/bash

relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc

prompt
comment "Here's how simple it is to organize your environments."
comment "(but it's entirely configurable)."
prompt
run tree stacks

newline 2
prompt
comment "Start by defining the baseline configuration for myapp."
comment "This is the configuration that you want to import anytime you deploy your component."
prompt
run cat stacks/catalog/myapp.yaml

newline 2
prompt
comment "Then we define each environment importing that baseline configuration."
comment "Here's how the configuration for the 'dev' environment looks like..."
prompt
run cat stacks/deploy/dev.yaml

newline 2
prompt
comment "Define the configuration for the 'staging' environment."
prompt
run cat stacks/deploy/staging.yaml

newline 2
prompt
comment "Finally, define the configuration for the 'prod' environment..."
prompt
run cat stacks/deploy/prod.yaml
