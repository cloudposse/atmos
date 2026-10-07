#!/usr/bin/env atmos
# Field test: exec.run behaviors. First arg selects the case; second is a counter file path.
case = ctx.args[0]
counter = ctx.args[1] if len(ctx.args) > 1 else "/tmp/ft-counter"

def retry_script():
    return ["sh", "-c", 'f="$1"; n=$(cat "$f" 2>/dev/null || echo 0); n=$((n+1)); echo $n > "$f"; echo "attempt $n boom-transient" >&2; if [ "$n" -ge 3 ]; then echo "{\\"ok\\": true, \\"n\\": $n}"; exit 0; fi; exit 1', "sh", counter]

def run(case):
    if case == "capture":
        r = exec.run(["echo", "hello"], output = "capture")
        print("stdout=%r stderr=%r code=%d" % (r.stdout, r.stderr, r.exit_code))
    elif case == "stream":
        r = exec.run(["echo", "streamed"])
        print("after: stdout=%r" % r.stdout)
    elif case == "nocheck":
        r = exec.run(["sh", "-c", "echo out; echo err >&2; exit 7"], check = False, output = "capture")
        print("code=%d stdout=%r stderr=%r" % (r.exit_code, r.stdout, r.stderr))
    elif case == "check3":
        exec.run(["sh", "-c", "echo some-stdout; echo some-stderr >&2; exit 3"])
    elif case == "timeout":
        exec.run(["sleep", "5"], timeout = "1s")
        print("should not reach")
    elif case == "timeout_nocheck":
        r = exec.run(["sleep", "5"], timeout = "1s", check = False)
        print("reached after timeout with check=False code=%d" % r.exit_code)
    elif case == "retry":
        r = exec.run(retry_script(), output = "capture", retry = {"max_attempts": 3, "initial_delay": "100ms"})
        print("final code=%d stdout=%s stderr=%r" % (r.exit_code, r.stdout.strip(), r.stderr))
        print("data=" + json.encode(r.data))
    elif case == "retry_cond_match":
        r = exec.run(retry_script(), output = "capture", retry = {"max_attempts": 3, "initial_delay": "100ms", "conditions": ["boom-transient"]})
        print("final code=%d" % r.exit_code)
    elif case == "retry_cond_nomatch":
        r = exec.run(retry_script(), output = "capture", retry = {"max_attempts": 3, "initial_delay": "100ms", "conditions": ["^never-matches$"]})
        print("final code=%d" % r.exit_code)
    elif case == "retry_exhaust":
        r = exec.run(retry_script(), output = "capture", retry = {"max_attempts": 2, "initial_delay": "100ms"})
        print("final code=%d" % r.exit_code)
    elif case == "retry_timeout":
        r = exec.run(retry_script(), output = "capture", timeout = "300ms", retry = {"max_attempts": 5, "initial_delay": "1s"})
        print("final code=%d" % r.exit_code)
    elif case == "retry_nocheck":
        r = exec.run(retry_script(), output = "capture", check = False, retry = {"max_attempts": 3, "initial_delay": "100ms"})
        print("final code=%d (counter shows attempts)" % r.exit_code)
    elif case == "json":
        r = exec.run(["echo", '{"a": [1, 2.5, null], "b": "x"}'], output = "capture")
        print(json.encode(r.data))
        print(r.data["a"])
    elif case == "notjson":
        r = exec.run(["echo", "not json"], output = "capture")
        print("stdout ok: " + r.stdout.strip())
        print("now data:")
        print(r.data)
    elif case == "emptyjson":
        r = exec.run(["true"], output = "capture")
        print("now data:")
        print(r.data)
    elif case == "encode":
        r = exec.run(["echo", '{"a":1}'], output = "capture")
        print(json.encode(r))
    elif case == "mutate":
        r = exec.run(["echo", '{"a":1}'], output = "capture")
        r.data["a"] = 2
    elif case == "string":
        exec.run("ls -l")
    elif case == "missing":
        exec.run(["missing-binary-xyz"])
    elif case == "missing_nocheck":
        r = exec.run(["missing-binary-xyz"], check = False)
        print("reached")
    elif case == "badcwd":
        exec.run(["echo", "x"], working_directory = "/nonexistent/dir")
    elif case == "badoutput":
        exec.run(["echo", "x"], output = "nope")
    elif case == "emptylist":
        exec.run([])
    elif case == "badtimeout":
        exec.run(["echo", "x"], timeout = "banana")
    elif case == "negtimeout":
        exec.run(["echo", "x"], timeout = "-1s")
    elif case == "badretry":
        exec.run(["echo", "x"], retry = {"max_attempts": 0})
    elif case == "env":
        r = exec.run(["sh", "-c", "echo FOO=$FOO HOME=$HOME"], env = {"FOO": "bar"}, output = "capture")
        print(r.stdout.strip())
    elif case == "signal":
        exec.run(["sh", "-c", "kill -9 $$"])
    else:
        fail("unknown case " + case)


run(case)
