package validate

default allow := false

allow {
    input.vars.tenant == "tenant1"
}
