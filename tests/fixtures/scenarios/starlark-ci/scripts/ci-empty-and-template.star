# An empty literal still counts as passed, so it is mutually exclusive with template=.
ci.summary("", template = "report.md")
print("must not run")
