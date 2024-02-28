#!/bin/bash

relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc

prompt
comment "Here's a simple way to organize environments."
prompt
run tree -I '*.tf' .

newline 2
prompt
comment "Customize how to organize environments in the atmos.yaml file."
comment "Pay special attention to the name_pattern, which is how Atmos knows"
comment "where to find the stacks."
prompt
run cat atmos.yaml
newline 2
