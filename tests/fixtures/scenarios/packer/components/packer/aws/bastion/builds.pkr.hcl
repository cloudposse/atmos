build {
  sources = ["source.amazon-ebs.this"]

  provisioner "shell" {
    inline = [
      "sudo systemctl enable --now snap.amazon-ssm-agent.amazon-ssm-agent.service",
      "sudo -E bash -c 'export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get install -y mysql-client && apt-get clean && cloud-init clean'",
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
