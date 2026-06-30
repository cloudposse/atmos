# Minimal Packer template for the "ship anything" demo. It is only listed by
# `atmos list components` here (not built), so it stays dependency-free.
packer {}

source "file" "ami" {
  content = "atmos-demo-ami"
  target  = "ami.txt"
}

build {
  sources = ["source.file.ami"]
}
