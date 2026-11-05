# Dispatches one retry.conditions case per invocation. Each attempt of the failing shell step prints
# wp5-attempt on stdout and flaky-failure on stderr, so the number of attempts is the number of
# wp5-attempt lines in the output.
def attempt(conditions):
    retry = {"max_attempts": 3, "initial_delay": "1ms", "backoff_strategy": "constant"}
    if conditions:
        retry["conditions"] = conditions
    steps.shell(
        command = "echo wp5-attempt; echo flaky-failure >&2; exit 1",
        output = "raw",
        retry = retry,
    )

CASES = {
    "no-conditions": [],
    "stderr-match": ["/flaky/"],
    "stdout-match": ["wp5-attempt"],
    "error-text-match": ["exit(ed)? .*1"],
    "never-matches": ["never-matches"],
    "invalid-pattern": ["(unclosed"],
}

name = ctx.arguments["case"]
if name not in CASES:
    fail("unknown case %r; valid: %s" % (name, sorted(CASES.keys())))
attempt(CASES[name])
