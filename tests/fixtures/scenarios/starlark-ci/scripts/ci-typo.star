# The template reads .Stack, the data supplies Stak: the render must fail and name the key.
ci.summary(template = "report.md", data = {"Title": "Typo", "Stak": "dev", "Items": []})
print("must not run")
