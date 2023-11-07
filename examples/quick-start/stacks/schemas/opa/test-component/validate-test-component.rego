# 'package atmos' is required in all `atmos` OPA policies
package atmos

errors["'service_1_name' variable length must be greater than 10 chars"] {
    count(input.vars.service_1_name) <= 10
}
