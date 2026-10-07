#!/usr/bin/env atmos
dependencies.tools("jqlang/jq", "1.7.1")
r = exec.run(["jq", "--version"], output = "capture")
print(r.stdout.strip())
w = exec.run(["sh", "-c", "command -v jq"], output = "capture")
print(w.stdout.strip())
