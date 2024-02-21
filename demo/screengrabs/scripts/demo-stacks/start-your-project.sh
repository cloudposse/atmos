#!/bin/bash

relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc

comment ""
comment "Here's a simple way to organize environemnts."
comment ""
run tree -I '*.tf' .

newline 2
comment ""
comment "Customize how to organize environments in the atmos.yml file."
comment "Pay special attention to the name_pattern, which is how atmos where to"
comment "find the stacks."
comment ""
run cat atmos.yaml
newline 2
