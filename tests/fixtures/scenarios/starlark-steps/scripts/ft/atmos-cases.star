# Dispatches one atmos.* wrapper case per invocation, because a Starlark argument error cannot be caught.
def show(r):
    print("exit_code=%s" % r.exit_code)
    print("stdout=%r" % r.stdout)
    print("stderr=%r" % r.stderr)

def list_components():
    r = atmos.list("components")
    print("type(data)=%s" % type(r.data))
    show(r)
    print("data=%r" % r.data)

def describe_vars():
    r = atmos.describe("component", "mock", flags = {"stack": "dev"})
    print("vars=%s" % json.encode(r.data["vars"]))

def describe_kwarg_stack():
    r = atmos.describe("component", "mock", stack = "dev")
    print(r.data["vars"])

def config_positional():
    r = atmos.config("get", "base_path")
    show(r)
    print("data=%r" % r.data)

def config_args():
    r = atmos.config("get", args = ["base_path"])
    show(r)
    print("data=%r" % r.data)

def config_stream():
    show(atmos.config("get", "base_path", output = "stream"))

def list_stacks_yaml():
    r = atmos.list("stacks", flags = {"format": "yaml"})
    print("stdout=%r" % r.stdout)
    print("data=%r" % r.data)

def list_stacks_default():
    r = atmos.list("stacks")
    print("stdout=%r" % r.stdout)
    print("data=%r" % r.data)

def run_version():
    show(atmos.run(["version"], output = "capture"))

def tf_plan_pos():
    show(atmos.terraform("plan", "mock", "dev", output = "capture"))

def tf_plan_kw():
    show(atmos.terraform("plan", component = "mock", stack = "dev", output = "capture"))

def tf_plan_detailed():
    show(atmos.terraform("plan", "mock", "dev", flags = {"detailed-exitcode": True}, output = "capture"))

def tf_plan_stack_flag():
    show(atmos.terraform("plan", "mock", "dev", flags = {"stack": "x"}, output = "capture"))

def run_timeout():
    show(atmos.run(["version"], timeout = "1s"))

def helm_list():
    show(atmos.helm("list"))

def nonexistent():
    show(atmos.nonexistent())

def toolchain_short():
    show(atmos.toolchain("install", "jq@1.7.1", output = "capture"))

def toolchain_full():
    show(atmos.toolchain("install", "jqlang/jq@1.7.1", output = "capture"))

def toolchain_kw():
    show(atmos.toolchain("install", tool = "jqlang/jq@1.7.1", output = "capture"))

def retry_conditions():
    steps.task(name = "t", function = list_components, retry = {"max_attempts": 2, "conditions": ["x"]})

def retry_delay():
    steps.task(name = "t", function = list_components, retry = {"max_attempts": 2, "delay": "1s"})

def exec_retry():
    show(exec.run(["false-cmd-that-does-not-exist"], retry = {"max_attempts": 2}))

def dir_atmos():
    print("dir(atmos)=%s" % dir(atmos))
    print("dir(steps)=%s" % dir(steps))

def exec_retry_counter():
    # Fails twice (counter file), succeeds on the third attempt; check=True so retry applies.
    r = exec.run(["sh", "-c", 'f=%s; n=$(cat $f 2>/dev/null || echo 0); n=$((n+1)); echo $n > $f; echo attempt $n; [ $n -ge 3 ]' % ctx.arguments["extra"]], retry = {"max_attempts": 3, "initial_delay": "50ms"}, output = "capture")
    show(r)

def exec_timeout():
    r = exec.run(["sleep", "5.31"], timeout = "1s", check = False, output = "capture")
    show(r)

def exec_retry_conditions():
    show(exec.run(["sh", "-c", "echo boom >&2; exit 3"], check = False, retry = {"max_attempts": 2, "conditions": ["boom"]}))

CASES = {
    "list-components": list_components,
    "describe-vars": describe_vars,
    "describe-kwarg-stack": describe_kwarg_stack,
    "config-positional": config_positional,
    "config-args": config_args,
    "config-stream": config_stream,
    "list-stacks-yaml": list_stacks_yaml,
    "list-stacks-default": list_stacks_default,
    "run-version": run_version,
    "tf-plan-pos": tf_plan_pos,
    "tf-plan-kw": tf_plan_kw,
    "tf-plan-detailed": tf_plan_detailed,
    "tf-plan-stack-flag": tf_plan_stack_flag,
    "run-timeout": run_timeout,
    "helm-list": helm_list,
    "nonexistent": nonexistent,
    "toolchain-short": toolchain_short,
    "toolchain-full": toolchain_full,
    "toolchain-kw": toolchain_kw,
    "retry-conditions": retry_conditions,
    "retry-delay": retry_delay,
    "exec-retry": exec_retry,
    "exec-retry-conditions": exec_retry_conditions,
    "exec-retry-counter": exec_retry_counter,
    "dir-atmos": dir_atmos,
    "exec-timeout": exec_timeout,
}

name = ctx.arguments["case"]
if name not in CASES:
    fail("unknown case %r; valid: %s" % (name, sorted(CASES.keys())))
CASES[name]()
