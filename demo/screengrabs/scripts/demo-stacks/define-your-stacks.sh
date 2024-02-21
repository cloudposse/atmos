relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc

comment ""
comment "Here's how simple it is to organize your environemnts."
comment "(but it's entirely configurable)."
comment ""
run tree stacks/

newline 2
comment ""
comment "Start by defining the baseline configuration for myapp."
comment "This is the configuration that you want to import anytime you deploy your component."
comment ""
run cat stacks/catalog/myapp.yaml

newline 2
comment ""
comment "Then we define each environment importing that baseline configuration."
comment "Here's the the configuration for the 'dev' environment looks like..."
comment ""
run cat stacks/deploy/dev.yaml

newline 2
comment ""
comment "Define the configuration for the 'staging' environment."
comment ""
run cat stacks/deploy/staging.yaml

newline 2
comment ""
comment "Finally, define the configuration for the 'prod' environment..."
comment ""
run cat stacks/deploy/prod.yaml
