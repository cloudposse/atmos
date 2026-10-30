# Internal harden-runner runtime

Use the [parent composite](../README.md) in normal workflows. This helper owns
policy loading before StepSecurity's pre-hook, DNS repair in the main step, and
DNS recovery in the post step. It uses only Node built-ins.

The diagnostic invokes it separately after recording the original failure. Its
pre-hook still loads the same policy; its main step repairs DNS after the proxy
agent is running, or retains host DNS after confirming current eBPF block
enforcement. Unit tests inject file and command operations, so they never alter
the developer machine's DNS or firewall.
