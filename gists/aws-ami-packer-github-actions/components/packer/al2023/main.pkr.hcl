# Packer template for a hardened Amazon Linux 2023 AMI.
#
# This template is intentionally generic and fully parameterized: every
# environment-specific value (region, networking, KMS key, sharing targets,
# provisioner scripts) is a variable supplied by the Atmos stack. Nothing is
# hardcoded, so the same template works for any account.
#
# References:
#   - https://developer.hashicorp.com/packer/integrations/hashicorp/amazon
#   - https://developer.hashicorp.com/packer/integrations/hashicorp/amazon/latest/components/builder/ebs
#   - https://developer.hashicorp.com/packer/integrations/hashicorp/amazon/latest/components/data-source/ami
#   - https://developer.hashicorp.com/packer/integrations/hashicorp/amazon#authentication

packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1"
    }
  }
}

# ---------------------------------------------------------------------------
# Input variables (all supplied by the Atmos stack — see stacks/al2023.yaml).
# ---------------------------------------------------------------------------

variable "region" {
  type        = string
  description = "AWS region to build the AMI in."
}

variable "ami_name" {
  type        = string
  description = "Name of the resulting AMI."
}

variable "instance_type" {
  type        = string
  description = "EC2 instance type used to build the AMI."
}

variable "ssh_username" {
  type        = string
  description = "SSH username Packer uses to connect to the build instance."
  default     = "ec2-user"
}

# Source AMI lookup. We discover the latest matching base image by name pattern
# and owner instead of pinning a brittle AMI ID. `source_ami_name` is typically
# templated from an environment variable so CI can pin an exact base image.
variable "source_ami_name" {
  type        = string
  description = "Name (or name pattern, e.g. 'al2023-ami-2023.*-x86_64') of the base AMI to build from."
}

variable "source_ami_owner" {
  type        = string
  description = "Account ID that owns the base AMI. Amazon Linux is published by 137112412989."
  default     = "137112412989"
}

variable "source_ami_architecture" {
  type        = string
  description = "Architecture of the base AMI (x86_64 or arm64)."
  default     = "x86_64"
}

# Networking. Leave subnet/VPC empty to use the account's default VPC, or set
# them to build inside a specific private subnet.
variable "vpc_id" {
  type        = string
  description = "VPC to launch the build instance in. Empty string uses the default VPC."
  default     = ""
}

variable "subnet_id" {
  type        = string
  description = "Subnet to launch the build instance in. Empty string lets AWS choose."
  default     = ""
}

variable "associate_public_ip_address" {
  type        = bool
  description = "Whether to give the build instance a public IP (required when building in a public subnet without a NAT)."
  default     = true
}

variable "temporary_security_group_source_cidrs" {
  type        = list(string)
  description = "CIDRs allowed to SSH into the temporary build instance. Restrict this in production."
  default     = ["0.0.0.0/0"]
}

# Storage.
variable "volume_size" {
  type        = number
  description = "Root EBS volume size in GiB."
  default     = 20
}

variable "volume_type" {
  type        = string
  description = "Root EBS volume type."
  default     = "gp3"
}

variable "device_name" {
  type        = string
  description = "Root block device name."
  default     = "/dev/xvda"
}

# Encryption.
variable "encrypt_boot" {
  type        = bool
  description = "Whether to encrypt the resulting AMI's snapshots."
  default     = true
}

variable "kms_key_arn" {
  type        = string
  description = "KMS key ARN used to encrypt the AMI snapshots. Empty string uses the account's default EBS encryption key."
  default     = ""
}

# Authentication. In CI, credentials come from the OIDC-federated role, so
# `assume_role_arn` can be left empty. Set it to build via an explicit role.
variable "assume_role_arn" {
  type        = string
  description = "ARN of an IAM role to assume for the build. Empty string uses ambient credentials (e.g. the CI OIDC role)."
  default     = ""
}

variable "assume_role_session_name" {
  type        = string
  description = "Session name used when assuming the build role."
  default     = "atmos-packer"
}

variable "assume_role_duration_seconds" {
  type        = number
  description = "Duration of the assumed-role session, in seconds."
  default     = 3600
}

# Sharing. Who is allowed to launch the resulting AMI.
variable "ami_users" {
  type        = list(string)
  description = "Account IDs allowed to launch the resulting AMI. Sharing is usually done later via 'atmos ami share' after approval, so this defaults to empty."
  default     = []
}

variable "ami_org_arns" {
  type        = list(string)
  description = "AWS Organizations ARNs allowed to launch the resulting AMI."
  default     = []
}

variable "ami_ou_arns" {
  type        = list(string)
  description = "AWS Organizations OU ARNs allowed to launch the resulting AMI."
  default     = []
}

# Tags.
variable "ami_tags" {
  type        = map(string)
  description = "Tags applied to the AMI and its snapshots. Includes the governance tag (e.g. ScanStatus)."
  default     = {}
}

variable "run_tags" {
  type        = map(string)
  description = "Tags applied to the temporary build instance and its resources."
  default     = {}
}

# Provisioning. The ordered list of shell scripts to run inside the image,
# supplied by the stack so the build steps are configuration, not code.
variable "provisioner_shell_scripts" {
  type        = list(string)
  description = "Ordered list of shell scripts (relative to the component dir) to run during the build."
  default     = []
}

variable "provisioner_env_vars" {
  type        = list(string)
  description = "Environment variables (KEY=value) made available to every provisioner script. Used to pass feature toggles such as ENABLE_FIREWALL=true."
  default     = []
}

# Build behavior.
variable "skip_create_ami" {
  type        = bool
  description = "If true, run provisioners but do not register an AMI. Useful for dry-run validation."
  default     = false
}

variable "force_deregister" {
  type        = bool
  description = "Deregister an existing AMI with the same name before building."
  default     = false
}

variable "force_delete_snapshot" {
  type        = bool
  description = "Delete snapshots of any AMI deregistered by force_deregister."
  default     = false
}

variable "manifest_file_name" {
  type        = string
  description = "Path to the build manifest produced by the manifest post-processor. 'atmos ami get-ami-id' reads this."
  default     = "manifest.json"
}

# ---------------------------------------------------------------------------
# Locals.
# ---------------------------------------------------------------------------

locals {
  # Treat empty strings as "unset" so AWS defaults apply.
  kms_key_id      = var.kms_key_arn != "" ? var.kms_key_arn : null
  assume_role_arn = var.assume_role_arn != "" ? var.assume_role_arn : null
  vpc_id          = var.vpc_id != "" ? var.vpc_id : null
  subnet_id       = var.subnet_id != "" ? var.subnet_id : null
}

# ---------------------------------------------------------------------------
# Source AMI lookup — find the most recent base image matching the name.
# ---------------------------------------------------------------------------

data "amazon-ami" "base" {
  filters = {
    name                = var.source_ami_name
    architecture        = var.source_ami_architecture
    root-device-type    = "ebs"
    virtualization-type = "hvm"
  }
  owners      = [var.source_ami_owner]
  most_recent = true
  region      = var.region

  assume_role {
    role_arn         = local.assume_role_arn
    session_name     = var.assume_role_session_name
    duration_seconds = var.assume_role_duration_seconds
  }
}

# ---------------------------------------------------------------------------
# Builder.
# ---------------------------------------------------------------------------

source "amazon-ebs" "al2023" {
  ami_name      = var.ami_name
  source_ami    = data.amazon-ami.base.id
  instance_type = var.instance_type
  region        = var.region
  ssh_username  = var.ssh_username

  vpc_id                      = local.vpc_id
  subnet_id                   = local.subnet_id
  associate_public_ip_address = var.associate_public_ip_address

  temporary_security_group_source_cidrs = var.temporary_security_group_source_cidrs

  # Sharing — usually empty here; sharing happens post-approval via 'atmos ami share'.
  ami_users    = var.ami_users
  ami_org_arns = var.ami_org_arns
  ami_ou_arns  = var.ami_ou_arns

  # Encryption.
  encrypt_boot = var.encrypt_boot
  kms_key_id   = local.kms_key_id

  ami_block_device_mappings {
    device_name           = var.device_name
    volume_size           = var.volume_size
    volume_type           = var.volume_type
    delete_on_termination = true
    encrypted             = var.encrypt_boot
  }

  assume_role {
    role_arn         = local.assume_role_arn
    session_name     = var.assume_role_session_name
    duration_seconds = var.assume_role_duration_seconds
  }

  aws_polling {
    delay_seconds = 5
    max_attempts  = 100
  }

  skip_create_ami       = var.skip_create_ami
  force_deregister      = var.force_deregister
  force_delete_snapshot = var.force_delete_snapshot

  tags     = var.ami_tags
  run_tags = var.run_tags
}

# ---------------------------------------------------------------------------
# Build — run the ordered provisioner scripts, then emit a manifest.
# ---------------------------------------------------------------------------

build {
  sources = ["source.amazon-ebs.al2023"]

  # Run each stack-provided script in order. Scripts are toggle-driven via
  # `provisioner_env_vars`, so optional steps (firewall, scan agent) can be
  # enabled per-stack without editing this template.
  provisioner "shell" {
    scripts          = var.provisioner_shell_scripts
    environment_vars = var.provisioner_env_vars
    # Run each script as root and pass the toggle vars as sudo command-line
    # assignments. This works under AL2023's default `NOPASSWD:ALL` sudoers,
    # where `sudo -E` is denied; command-line `VAR=value` assignments survive
    # env_reset, so the scripts reliably see ENABLE_FIREWALL, etc.
    execute_command = "chmod +x '{{ .Path }}'; sudo {{ .Vars }} '{{ .Path }}'"
  }

  # Machine-readable build output. 'atmos ami get-ami-id' parses this to find
  # the AMI ID of the most recent build.
  post-processor "manifest" {
    output     = var.manifest_file_name
    strip_path = true
  }
}
