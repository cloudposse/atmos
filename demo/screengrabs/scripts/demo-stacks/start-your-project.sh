relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc

comment ""
comment "Here's how simple it is to organize your environemnts."
comment ""
run tree stacks/

newline 2
comment ""
comment "You can customize how you organize your environments in the atmos.yml file."
comment "Pay special attention to the name_pattern, which is how atmos know how to"
comment "find your stacks."
comment ""
run cat atmos.yml
newline 2
