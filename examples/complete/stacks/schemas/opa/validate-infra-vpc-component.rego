package validate

default allow = true

# In production, don't allow mapping public IPs on launch
deny_map_public_ip_on_launch_in_prod {
    input.vars.stage == "prod"
    input.vars.map_public_ip_on_launch == true
}

# 'atmos' looks for the 'allow' output from all OPA policies
allow = false {
    deny_map_public_ip_on_launch_in_prod
}
