# The child exercises one gated surface of each kind: a summary, a comment, and a check.
ci.summary("## child report\n")
c = ci.comment("child comment", key="ft-ci-child", target="commit")
ci.check("ft-ci/child", state="success", description="child done")
print("child done")
