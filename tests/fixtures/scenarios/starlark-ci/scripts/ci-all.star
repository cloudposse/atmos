# Calls every ci.* function once. Locally (generic provider) each call renders on stderr and stdout
# carries only the lines printed here.
c = ci.context
print("context: provider=%s local=%s" % (c.provider, c.local))

b = ci.base()
print("base: %s" % ("none" if b == None else "resolved"))

ci.summary("## Plain summary")
ci.summary(template = "report.md", data = {"Title": "Rendered report", "Stack": "dev", "Items": ["alpha", "beta"]})
ci.output("answer", "42")
ci.env("FT_REGION", "us-east-1")
ci.path("/opt/ft/bin")
ci.mask("hunter2-secret-value")
ci.annotate("warning", "deprecated module", file = "main.tf", line = 3, end_line = 5, title = "Lint")

for target in ["auto", "pr", "commit"]:
    result = ci.comment("body for " + target, key = "ft-" + target, target = target)
    print("comment %s: id=%d target=%s" % (target, result.id, result.target))
rendered = ci.comment(template = "comment.md", data = {"Stack": "dev", "Adds": 3}, key = "ft-template")
print("comment template: id=%d" % rendered.id)

check = ci.check("ft-ci/all", state = "in_progress", description = "working")
check.update("success", description = "done")
print("check: name=%s state=%s id=%d" % (check.name, check.state, check.id))

def inside():
    print("inside group")
    return 7

print("group returned %d" % ci.group("Group title", inside))
ci.sarif("scripts/findings.sarif", category = "ft-ci")
print("ci-all done")
