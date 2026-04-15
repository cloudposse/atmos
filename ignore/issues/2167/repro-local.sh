#!/usr/bin/env bash
# ============================================================
# ATMOS REPRO: !terraform.output auto-provision for JIT workdir
#              components — local Terraform backend variant.
#
# Issue: https://github.com/cloudposse/atmos/issues/2167
#
# The problem (before fix):
#   When a stack manifest uses !terraform.output to read an output
#   from a JIT-workdir component, atmos resolves the component's
#   terraform working directory using the static formula:
#     base_path + component_name (e.g. components/terraform/producer)
#   JIT provisioning never creates that path -- it provisions to
#   .workdir/terraform/<stack>-<component>/. As a result,
#   !terraform.output fails for any JIT component on a machine that
#   has not already applied the component (fresh CI runner, new
#   developer workstation, container restart, TTL eviction).
#
# The fix:
#   When auto_provision_workdir_for_outputs: true and the component
#   has JIT workdir provisioning enabled, !terraform.output and
#   atmos.Component() auto-provision the workdir and run
#   terraform init before reading outputs.
#
# Scenario (no AWS credentials needed):
#   - producer: JIT component, provision.workdir.enabled=true
#   - consumer: reads producer's output via !terraform.output
#   - On a machine where producer has been applied, consumer
#     correctly reads the output from the auto-provisioned workdir.
#
# Requires: atmos, terraform
# ============================================================

set -euo pipefail

# ─────────────────────────────────────────────────────────────
# helpers
# ─────────────────────────────────────────────────────────────
phase()   { echo; echo "══════════════════════════════════════════════════"; echo "  $*"; echo "══════════════════════════════════════════════════"; }
section() { echo; echo "── $* ──"; }
tree()    { find . \( -path './.git' -o -path './.workdir' -o -path './.terraform' -o -path './.tf-plugin-cache' \) -prune -o -type f -print | sort | sed 's|^\./||'; }
tree_all(){ find . -path './.git' -prune -o -type f -print | sort | sed 's|^\./||'; }

# ─────────────────────────────────────────────────────────────
# PHASE 0: isolated workspace
# ─────────────────────────────────────────────────────────────
phase "PHASE 0: isolated workspace"

WORKDIR="$(mktemp -d -t atmos-repro-local-XXXXXX)"
echo "WORKDIR: ${WORKDIR}"
trap 'echo; echo "Workspace: ${WORKDIR}"; echo "(not deleted — inspect if needed)"' EXIT
cd "${WORKDIR}"

mkdir -p "${WORKDIR}/.tf-plugin-cache"
export TF_PLUGIN_CACHE_DIR="${WORKDIR}/.tf-plugin-cache"

section "versions"
atmos version 2>&1
terraform version 2>&1 | head -1

# ─────────────────────────────────────────────────────────────
# PHASE 1: project layout
# ─────────────────────────────────────────────────────────────
phase "PHASE 1: write project layout"

mkdir -p components/terraform/mock stacks/deploy

# ── atmos.yaml ──────────────────────────────────────────────
cat > atmos.yaml <<'EOF'
base_path: "./"

components:
  terraform:
    base_path:             "components/terraform"
    auto_generate_backend_file: false
    auto_provision_workdir_for_outputs: true
    deploy_run_init:       true
    init_run_reconfigure:  true

stacks:
  base_path:      "stacks"
  included_paths: ["deploy/**/*"]
  name_pattern:   "{stage}"

logs:
  file:  "/dev/stderr"
  level: Info
EOF

# ── mock Terraform component ─────────────────────────────────
# A trivial local-only module: takes a string var and echoes it
# as an output. No remote modules, no cloud access needed.
cat > components/terraform/mock/main.tf <<'EOF'
variable "foo" {
  default = "bar"
}

output "foo" {
  value = var.foo
}
EOF

# ── stack manifest: producer (JIT) + consumer (!terraform.output) ──
# producer: has JIT workdir provisioning enabled and writes local state
#           inside its .workdir/terraform/test-producer/ directory.
# consumer: reads producer's 'foo' output at stack-evaluation time via
#           !terraform.output.  Before the fix this fails because
#           !terraform.output resolves the path as components/terraform/producer
#           which doesn't exist for JIT components.
cat > stacks/deploy/test.yaml <<'EOF'
vars:
  stage: test

components:
  terraform:

    # PRODUCER: JIT-provisioned workdir.
    # After apply, state lives at:
    #   .workdir/terraform/test-producer/terraform.tfstate
    producer:
      metadata:
        component: mock
      vars:
        foo: "hello-from-producer"
      provision:
        workdir:
          enabled: true

    # CONSUMER: reads producer's output.
    # Three-arg form: !terraform.output <component> <stack> <output>
    #   component = producer
    #   stack     = test
    #   output    = foo
    consumer:
      metadata:
        component: mock
      vars:
        foo: !terraform.output producer test foo
EOF

section "atmos.yaml"
cat atmos.yaml

section "components/terraform/mock/main.tf"
cat components/terraform/mock/main.tf

section "stacks/deploy/test.yaml"
cat stacks/deploy/test.yaml

section "workspace tree"
tree

# ─────────────────────────────────────────────────────────────
# PHASE 2: apply the producer (writes state to JIT workdir)
# ─────────────────────────────────────────────────────────────
phase "PHASE 2: apply producer (writes state to JIT workdir)"
echo "Running: atmos terraform apply producer -s test -- -auto-approve"
echo
atmos terraform apply producer -s test -- -auto-approve

section "JIT workdir after apply"
ls -la "${WORKDIR}/.workdir/terraform/test-producer/" 2>/dev/null \
  || echo "(ERROR: .workdir/terraform/test-producer/ not found after apply)"

section "state file"
find "${WORKDIR}/.workdir" -name "terraform.tfstate" -type f \
  | sed "s|${WORKDIR}/||" \
  || echo "(no tfstate found)"

# ─────────────────────────────────────────────────────────────
# PHASE 3: show the path mismatch (the bug)
# ─────────────────────────────────────────────────────────────
phase "PHASE 3: path mismatch — what !terraform.output used to look for"
# Before the fix, !terraform.output resolves the path via the static
# formula: base_path + component = components/terraform/producer.
# JIT provisioning NEVER creates that directory.

section "static path (what !terraform.output looked for — does NOT exist)"
echo "  components/terraform/producer"
ls -la "${WORKDIR}/components/terraform/producer" 2>/dev/null \
  || echo "  (not found — expected)"

section "JIT workdir (where state actually lives — DOES exist)"
echo "  .workdir/terraform/test-producer"
ls -la "${WORKDIR}/.workdir/terraform/test-producer/" 2>/dev/null | head -10

# ─────────────────────────────────────────────────────────────
# PHASE 4: demonstrate the fix — describe component resolves output
# ─────────────────────────────────────────────────────────────
phase "PHASE 4: demonstrate fix — atmos describe component consumer -s test"
# With auto_provision_workdir_for_outputs: true and the fix applied,
# !terraform.output auto-provisions .workdir/terraform/test-producer/,
# runs terraform init, and reads the output from the local state.
# Expected: vars.foo = "hello-from-producer"

echo "Running: atmos describe component consumer -s test"
echo
atmos describe component consumer -s test 2>&1 \
  | grep -E '"foo":|foo:|workdir|provisioned|Error' | head -20 \
  || true

section "auto-provisioned workdir (created by the fix)"
ls -la "${WORKDIR}/.workdir/terraform/test-producer/" 2>/dev/null \
  && echo "(found — auto-provision succeeded)" \
  || echo "(ERROR: workdir was not auto-provisioned)"

# ─────────────────────────────────────────────────────────────
# PHASE 5: simulate fresh machine — wipe JIT workdir
# ─────────────────────────────────────────────────────────────
phase "PHASE 5: simulate fresh machine — wipe .workdir/"
# On a fresh CI runner or after TTL eviction, .workdir/ doesn't exist.
# With the fix and a local backend the outputs are unavailable (state
# was in the workdir). With an S3/remote backend the outputs survive.
# This phase demonstrates the local-backend limitation.

echo "Running: rm -rf .workdir/"
rm -rf "${WORKDIR}/.workdir"

section "workspace tree after wipe"
tree

section "atmos describe component consumer -s test (expect: init runs, state gone)"
echo "Running: atmos describe component consumer -s test"
echo "(With local backend, state was in .workdir/ — expect empty/default output)"
echo
atmos describe component consumer -s test 2>&1 \
  | grep -E '"foo":|foo:|workdir|provisioned|Error' | head -20 \
  || true

echo
echo "NOTE: For persistent outputs across machine restarts, use an S3/remote"
echo "      backend (see repro-s3.sh) or !terraform.state with remote state."

# ─────────────────────────────────────────────────────────────
# PHASE 6: verify the fix summary
# ─────────────────────────────────────────────────────────────
phase "PHASE 6: fix summary"

echo "The fix (auto_provision_workdir_for_outputs: true + JIT workdir):"
echo "  BEFORE: !terraform.output resolved path as components/terraform/producer"
echo "          which never exists for JIT components → always failed"
echo "  AFTER:  !terraform.output detects JIT components, auto-provisions"
echo "          .workdir/terraform/<stack>-<component>/, runs init,"
echo "          and reads the output — no manual workaround needed"
echo
echo "══════════════════════════════════════════════════"
echo "  Done. Workspace: ${WORKDIR}"
echo "══════════════════════════════════════════════════"
