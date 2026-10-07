#!/usr/bin/env atmos
def work(name):
    exec.run(["sleep", "1"])
    t = exec.run(["date", "+%s"], output = "capture").stdout.strip()
    ui.info("task %s finished at %s" % (name, t))
    return name

t0 = exec.run(["date", "+%s"], output = "capture").stdout.strip()
ui.info("start %s" % t0)
res = steps.parallel(tasks = [steps.task(name = "t%d" % i, function = work, args = ["t%d" % i]) for i in range(4)])
t1 = exec.run(["date", "+%s"], output = "capture").stdout.strip()
ui.success("done %s" % t1)
