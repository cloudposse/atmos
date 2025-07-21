build {
  sources = ["source.amazon-ebs.this"]

  # SSM Agent is pre-installed on AL2023 AMIs but should be enabled explicitly as done above.
  # MySQL client on AL2023 is installed via dnf install mysql (the mysql package includes the CLI tools).
  # `cloud-init clean` ensures the image will boot as a new instance on next launch.
  # `dnf clean all` removes cached metadata and packages to reduce AMI size.
  provisioner "shell" {
    inline = [
      # Enable and start the SSM agent (already installed by default on AL2023)
      "sudo systemctl enable --now amazon-ssm-agent",

      # Install MySQL client (via dnf), clean metadata and cloud-init
      "sudo -E bash -c 'dnf install -y mysql && dnf clean all && cloud-init clean'"
    ]
  }

  # https://developer.hashicorp.com/packer/tutorials/docker-get-started/docker-get-started-post-processors
  # https://developer.hashicorp.com/packer/docs/post-processors
  # https://developer.hashicorp.com/packer/docs/post-processors/manifest
  post-processor "manifest" {
    output     = var.manifest_file_name
    strip_path = var.manifest_strip_path
  }
}
