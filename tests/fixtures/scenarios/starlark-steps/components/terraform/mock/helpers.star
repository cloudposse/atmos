# Decoy for the starlark-inherit hook: an inline script runs with the component directory as its
# working directory, so load("helpers.star") must find this file and not scripts/hooks/helpers.star.
def greet(name):
    return "hello from the component working directory, " + name
