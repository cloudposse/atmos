# Dispatches one steps.* case per invocation (a Starlark argument error cannot be caught).
def dump(r):
    print("value=%r" % r.value)
    print("values=%r" % r.values)
    print("metadata=%s" % json.encode(r.metadata))
    print("outputs=%s" % json.encode(r.outputs))
    print("skipped=%r error=%r" % (r.skipped, r.error))

def shell_capture():
    dump(steps.shell(command = "echo hi", output = "capture"))

def shell_stream():
    dump(steps.shell(command = "echo hi-stream"))

def run_shell():
    dump(steps.run("shell", command = "echo hi-run"))

def shell_failing():
    dump(steps.shell(command = "echo out; echo err >&2; exit 4"))

def confirm():
    dump(steps.confirm(prompt = "Proceed?", default = True))

def confirm_nodefault():
    dump(steps.confirm(prompt = "Proceed?"))

def input_default():
    dump(steps.input(prompt = "Name?", default = "api"))

def input_nodefault():
    dump(steps.input(prompt = "Name?"))

def choose():
    dump(steps.choose(prompt = "Env?", options = ["dev", "prod"], default = "dev"))

def choose_nodefault():
    dump(steps.choose(prompt = "Env?", options = ["dev", "prod"]))

def wait_all():
    dump(steps.wait_all())

def sleep():
    dump(steps.sleep(duration = "100ms"))

def sleep_timeout():
    dump(steps.sleep(timeout = "100ms"))

def sleep_bogus():
    dump(steps.sleep(bogus = "100ms"))

def env_then_exec():
    steps.env(vars = {"FT_X": "from-steps-env"})
    r = exec.run(["sh", "-c", "echo X=$FT_X"], output = "capture")
    print("exec.run sees: %r" % r.stdout)
    r2 = steps.shell(command = "echo X=$FT_X", output = "none")
    print("steps.shell sees: %r" % r2.value)

def join():
    dump(steps.join(options = ["a", "b"], separator = ","))

def http_local():
    # ctx.arguments["extra"] is the URL of a listener started by the caller.
    r = steps.http(url = ctx.arguments["extra"])
    print("status=%s" % r.metadata.get("status_code"))
    print("value=%r" % r.value[:60])

def webhook_alias():
    r = steps.webhook(url = ctx.arguments["extra"])
    print("status=%s" % r.metadata.get("status_code"))

def run_nonexistent():
    dump(steps.run("nonexistent"))

def parallel_prompt():
    def ask():
        return steps.input(prompt = "Name?", default = "x").value
    output = steps.parallel(functions = [ask, ask])

def nested_script():
    dump(steps.run("script", interpreter = "starlark", script = "print(1)"))

def nested_script_deep():
    dump(steps.script(interpreter = "starlark", script = 'print(steps.script(interpreter = "starlark", script = "print(2)").value)'))

def container():
    dump(steps.container(action = "run", with_ = {"image": "alpine:3.20", "command": "echo hi"}))

def unknown_field():
    dump(steps.shell(command = "echo x", bogus_field = 1))

def workflow_only():
    dump(steps.shell(command = "echo x", needs = ["a"]))

CASES = {
    "shell-capture": shell_capture,
    "shell-stream": shell_stream,
    "run-shell": run_shell,
    "shell-failing": shell_failing,
    "confirm": confirm,
    "confirm-nodefault": confirm_nodefault,
    "input-default": input_default,
    "input-nodefault": input_nodefault,
    "choose": choose,
    "choose-nodefault": choose_nodefault,
    "wait-all": wait_all,
    "sleep": sleep,
    "sleep-timeout": sleep_timeout,
    "sleep-bogus": sleep_bogus,
    "env-then-exec": env_then_exec,
    "join": join,
    "http-local": http_local,
    "webhook-alias": webhook_alias,
    "run-nonexistent": run_nonexistent,
    "parallel-prompt": parallel_prompt,
    "nested-script": nested_script,
    "nested-script-deep": nested_script_deep,
    "container": container,
    "unknown-field": unknown_field,
    "workflow-only": workflow_only,
}

name = ctx.arguments["case"]
if name not in CASES:
    fail("unknown case %r; valid: %s" % (name, sorted(CASES.keys())))
CASES[name]()
