build {
  sources = ["source.amazon-ebs.this"]

  provisioner "shell" {
    inline = [
      "sudo systemctl enable --now snap.amazon-ssm-agent.amazon-ssm-agent.service",
      "sudo -E bash -c 'export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get install -y mysql-client && apt-get clean && cloud-init clean'",
    ]
  }

  post-processor "manifest" {
    output     = "manifest.json"
    strip_path = true
  }
}
