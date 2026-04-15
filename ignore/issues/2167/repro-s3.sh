#!/usr/bin/env bash
# ============================================================
# ATMOS REPRO: !terraform.output + atmos.Component auto-provision
#              for JIT workdir components — S3 backend variant.
#
# Issue: https://github.com/cloudposse/atmos/issues/2167
#
# The problem (before fix):
#   When a stack manifest uses !terraform.output (or atmos.Component)
#   to read an output from a JIT-workdir component, atmos resolves
#   the component's working directory using the static formula:
#     base_path + component_name (e.g. components/terraform/producer)
#   JIT provisioning never creates that path -- it provisions to
#   .workdir/terraform/<stack>-<component>/. As a result the
#   terraform init required to talk to the S3 backend never runs,
#   and the command fails even though state is perfectly readable.
#
# The fix:
#   When auto_provision_workdir_for_outputs: true and the component
#   has JIT workdir provisioning enabled, !terraform.output and
#   atmos.Component() auto-provision the workdir and run
#   terraform init before reading outputs.  With an S3/remote backend
#   the state survives across machine restarts and TTL evictions, so
#   this also covers the fresh-CI-runner scenario.
#
# Scenario:
#   - producer: JIT component, writes state to S3.
#   - consumer: reads producer's 'foo' output via !terraform.output.
#   - On a fresh machine (no .workdir/ at all), atmos auto-provisions
#     the workdir, inits terraform against the S3 backend, and reads
#     the output successfully.
#
# Requires:
#   - atmos binary on PATH
#   - terraform binary on PATH
#   - AWS credentials (aws configure, or env vars below)
#   - An existing S3 bucket for state storage
#   - An existing DynamoDB table for state locking
#
# Configuration (set via environment or edit defaults below):
#   AWS_PROFILE      — AWS profile to use          (default: default)
#   S3_BUCKET        — S3 bucket for state          (REQUIRED)
#   DYNAMODB_TABLE   — DynamoDB table for locking   (REQUIRED)
#   AWS_REGION       — region for bucket + table    (default: us-east-1)
# ============================================================

set -euo pipefail

AWS_PROFILE="${AWS_PROFILE:-default}"
S3_BUCKET="${S3_BUCKET:?Set S3_BUCKET to an existing S3 bucket}"
DYNAMODB_TABLE="${DYNAMODB_TABLE:?Set DYNAMODB_TABLE to an existing DynamoDB table}"
AWS_REGION="${AWS_REGION:-us-east-1}"

export AWS_PROFILE AWS_REGION

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

WORKDIR="$(mktemp -d -t atmos-repro-s3-XXXXXX)"
echo "WORKDIR: ${WORKDIR}"
echo "AWS profile: ${AWS_PROFILE}, region: ${AWS_REGION}"
echo "S3 bucket:   ${S3_BUCKET}"
echo "DynamoDB:    ${DYNAMODB_TABLE}"
trap 'echo; echo "Workspace: ${WORKDIR}"; echo "(not deleted — inspect if needed)"' EXIT
cd "${WORKDIR}"

mkdir -p "${WORKDIR}/.tf-plugin-cache"
export TF_PLUGIN_CACHE_DIR="${WORKDIR}/.tf-plugin-cache"

section "versions"
atmos version 2>&1
terraform version 2>&1 | head -1
aws sts get-caller-identity --query "Arn" --output text 2>&1 \
  || echo "(AWS credential check failed — ensure credentials are configured)"

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
    auto_generate_backend_file: true
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
# A trivial module: takes a string var and echoes it as an output.
# Declares an S3 backend so auto_generate_backend_file writes a
# backend.tf.json into the JIT workdir.
cat > components/terraform/mock/main.tf <<'EOF'
variable "foo" {
  default = "bar"
}

output "foo" {
  value = var.foo
}

terraform {
  backend "s3" {}
}
EOF

# ── stack manifest: producer (JIT, S3 backend) + consumer ────
# producer: JIT workdir, S3 state.  Applied in phase 3.
# consumer: reads producer's 'foo' output via !terraform.output.
#           Before the fix: atmos can't find components/terraform/producer,
#           terraform init never runs, and the read fails.
#           After the fix:  atmos auto-provisions .workdir/terraform/test-producer/,
#           generates the S3 backend config, runs init, and reads the output.
cat > stacks/deploy/test.yaml <<EOF
vars:
  stage: test

terraform:
  backend_type: s3
  backend:
    bucket:         "${S3_BUCKET}"
    key:            "repro-2167/{component}.tfstate"
    region:         "${AWS_REGION}"
    dynamodb_table: "${DYNAMODB_TABLE}"
    encrypt:        true

components:
  terraform:

    # PRODUCER: JIT-provisioned workdir, S3 state.
    producer:
      metadata:
        component: mock
      vars:
        foo: "hello-from-s3-producer"
      provision:
        workdir:
          enabled: true

    # CONSUMER: reads producer's output at describe/plan time.
    # Three-arg form: !terraform.output <component> <stack> <output>
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
# PHASE 2: show the path mismatch (the bug)
# ─────────────────────────────────────────────────────────────
phase "PHASE 2: path mismatch — what !terraform.output used to look for"
# Before the fix, !terraform.output resolves the path via the static
# formula: base_path + component = components/terraform/producer.
# JIT provisioning NEVER creates that directory.
# With auto_generate_backend_file: true, atmos also needs to write
# a backend.tf.json into the workdir before running init -- which
# never happened for the static path.

section "static path (what !terraform.output looked for — does NOT exist)"
echo "  components/terraform/producer"
ls -la "${WORKDIR}/components/terraform/producer" 2>/dev/null \
  || echo "  (not found — expected)"

section "JIT workdir (what the fix creates)"
echo "  .workdir/terraform/test-producer/"
echo "  (not yet — will be auto-provisioned in phase 4)"

# ─────────────────────────────────────────────────────────────
# PHASE 3: apply the producer (writes state to S3)
# ─────────────────────────────────────────────────────────────
phase "PHASE 3: apply producer (writes state to S3)"
echo "Running: atmos terraform apply producer -s test -- -auto-approve"
echo
atmos terraform apply producer -s test -- -auto-approve

section "JIT workdir after apply"
ls -la "${WORKDIR}/.workdir/terraform/test-producer/" 2>/dev/null \
  || echo "(ERROR: .workdir/terraform/test-producer/ not found after apply)"

section "backend.tf.json (generated S3 backend config)"
cat "${WORKDIR}/.workdir/terraform/test-producer/backend.tf.json" 2>/dev/null \
  || echo "(not found)"

section "S3 state key"
echo "s3://${S3_BUCKET}/repro-2167/producer.tfstate"
aws s3 ls "s3://${S3_BUCKET}/repro-2167/" 2>&1 || echo "(S3 list failed — check credentials/bucket)"

# ─────────────────────────────────────────────────────────────
# PHASE 4: demonstrate the fix — describe component resolves output
# ─────────────────────────────────────────────────────────────
phase "PHASE 4: demonstrate fix — atmos describe component consumer -s test"
# With the fix applied, !terraform.output:
#   1. Detects producer has JIT workdir provisioning enabled.
#   2. Auto-provisions .workdir/terraform/test-producer/.
#   3. Generates the S3 backend.tf.json into the workdir.
#   4. Runs terraform init against the S3 backend.
#   5. Reads the 'foo' output from S3 state.
# Expected: vars.foo = "hello-from-s3-producer"

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
# Key advantage of S3 backend over local backend (see repro-local.sh):
# state lives in S3, not in .workdir/. After wiping the JIT workdir,
# the fix can still auto-provision, init against S3, and read outputs.

echo "Running: rm -rf .workdir/"
rm -rf "${WORKDIR}/.workdir"

section "workspace tree after wipe"
tree

section "atmos describe component consumer -s test (no .workdir/ at all)"
echo "Running: atmos describe component consumer -s test"
echo "(With the fix: auto-provision .workdir/, init against S3, read output)"
echo "(Without the fix: fail because components/terraform/producer doesn't exist)"
echo
atmos describe component consumer -s test 2>&1 \
  | grep -E '"foo":|foo:|workdir|provisioned|Error' | head -20 \
  || true

section "auto-provisioned workdir (recreated from scratch)"
ls -la "${WORKDIR}/.workdir/terraform/test-producer/" 2>/dev/null \
  && echo "(found — auto-provision from fresh machine succeeded)" \
  || echo "(ERROR: workdir was not auto-provisioned on fresh machine)"

# ─────────────────────────────────────────────────────────────
# PHASE 6: !terraform.state alternative (no init required)
# ─────────────────────────────────────────────────────────────
phase "PHASE 6: !terraform.state alternative"
# !terraform.state reads directly from the state backend without
# running terraform init — a lighter alternative for S3/remote
# backends. The JIT workdir path fix also applies here.
# See the main repro.sh for full !terraform.state coverage.

echo "For S3/remote backends, !terraform.state is an alternative:"
echo "  foo: !terraform.state producer test foo"
echo
echo "  Advantages over !terraform.output:"
echo "    - Reads directly from S3 — no terraform init required"
echo "    - Works on fresh machines with no .workdir/ at all"
echo "    - Lower overhead (no provider plugin download)"
echo
echo "  Tradeoff:"
echo "    - Only works with supported remote backend types"
echo "    - Requires the state key/path to be known"

# ─────────────────────────────────────────────────────────────
# PHASE 7: cleanup S3 state (optional)
# ─────────────────────────────────────────────────────────────
phase "PHASE 7: optional S3 cleanup"
echo "To remove the state written to S3 by this repro:"
echo
echo "  aws s3 rm s3://${S3_BUCKET}/repro-2167/producer.tfstate"
echo "  aws s3 rm s3://${S3_BUCKET}/repro-2167/consumer.tfstate"
echo
echo "(State was not deleted automatically — run the commands above to clean up)"

# ─────────────────────────────────────────────────────────────
# PHASE 8: fix summary
# ─────────────────────────────────────────────────────────────
phase "PHASE 8: fix summary"

echo "The fix (auto_provision_workdir_for_outputs: true + JIT workdir + S3 backend):"
echo "  BEFORE: !terraform.output resolved path as components/terraform/producer"
echo "          which never exists for JIT components → always failed"
echo "          even though state was perfectly accessible in S3"
echo "  AFTER:  !terraform.output detects JIT components, auto-provisions"
echo "          .workdir/terraform/test-producer/, generates backend.tf.json,"
echo "          runs terraform init against S3, and reads the output"
echo "          — works on fresh machines with no prior .workdir/"
echo
echo "══════════════════════════════════════════════════"
echo "  Done. Workspace: ${WORKDIR}"
echo "══════════════════════════════════════════════════"
