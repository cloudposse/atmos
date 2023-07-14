# 'package atmos' is required in all `atmos` OPA policies
package atmos

# In production, don't allow mapping public IPs on launch
errors["'service_1_name' variable lenght must be greater than 10 chars"] {
    count(input.vars.service_1_name) <= 10
}
