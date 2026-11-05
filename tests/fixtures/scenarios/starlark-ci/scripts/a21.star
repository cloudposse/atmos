# Carried extras: defer ordering after fail(), digest.sha256, exec.which miss, fs.resolve with ~.
defer(lambda: print("deferred ran"))
print("sha256(abc)=" + digest.sha256("abc"))
print("which miss: %r" % (exec.which("definitely-not-a-binary"),))
print("resolve: " + fs.resolve("~/x"))
fail("boom after defer")
