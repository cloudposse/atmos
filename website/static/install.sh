#!/usr/bin/env bash
set -euo pipefail

# Default method to auto
method="${1:-auto}"
installed_atmos_path=""

# Function to check if command exists
command_exists() {
  command -v "$@" >/dev/null 2>&1
}

# Function to detect package manager
detect_package_manager() {
    if command -v brew &> /dev/null; then
        echo "brew"
    elif command -v apt-get &> /dev/null; then
        echo "deb"
    elif command -v apt &> /dev/null; then
        echo "alpine"
    elif command -v yum &> /dev/null; then
        echo "rpm"
		elif command -v nix-env &> /dev/null; then
        echo "nix"
    else
        echo "none"
    fi
}

# Function for CloudSmith package registry installation
install_via_cloudsmith() {
		local package_manager=$(detect_package_manager)
    curl -1sLf "https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.${package_manager}.sh" | bash
		case $package_manager in
			alpine)
				echo "Using apk installation method..."
				apk add atmos
				;;
			deb)
				echo "Using apt package manager..."
				apt-get -y install atmos
				;;
			rpm)
				echo "Using yum installation method..."
				yum -y install atmos
				;;
			*)
				echo "Invalid method specified. Use 'alpine', 'deb', or 'rpm'."
				exit 1
				;;
		esac
}

# Function for binary download installation
install_via_binary_download() {
		# Check for curl
		if ! command_exists curl; then
			echo "curl is required but not installed. Please install curl and try again."
			exit 1
		fi
    if [ -n "${ATMOS_VERSION:-}" ]; then
      release="${ATMOS_VERSION#v}"
    else
      latest_url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/cloudposse/atmos/releases/latest)
      release="${latest_url##*/}"
      release="${release#v}"
    fi
    if [ -z "$release" ]; then
      echo "Unable to determine the latest Atmos release version." >&2
      exit 1
    fi

		os=$(uname -s | tr '[:upper:]' '[:lower:]')
		case "$os" in
			mingw*|msys*|cygwin*) os="windows" ;;
		esac

		arch=$(uname -m)
		case "$arch" in
			x86_64) arch="amd64" ;;
			aarch64|arm64) arch="arm64" ;;
			i386|i686) arch="386" ;;
		esac

    output="atmos"
    extension=""
    if [ "$os" = "windows" ]; then
      output="atmos.exe"
      extension=".exe"
    fi

    binary_url="https://github.com/cloudposse/atmos/releases/download/v${release}/atmos_${release}_${os}_${arch}${extension}"
    curl -fsSL "${binary_url}" -o "$output"
    checksums_url="https://github.com/cloudposse/atmos/releases/download/v${release}/atmos_${release}_SHA256SUMS"
    expected_sha="$(curl -fsSL "$checksums_url" | awk -v file="atmos_${release}_${os}_${arch}${extension}" '$2 == file {print $1; exit}')"
    if [ -z "$expected_sha" ]; then
      echo "Unable to find checksum for atmos_${release}_${os}_${arch}${extension}" >&2
      exit 1
    fi
    if command_exists sha256sum; then
      actual_sha="$(sha256sum "$output" | awk '{print $1}')"
    elif command_exists shasum; then
      actual_sha="$(shasum -a 256 "$output" | awk '{print $1}')"
    else
      echo "sha256sum or shasum is required to verify the downloaded Atmos binary." >&2
      exit 1
    fi
    if [ "$actual_sha" != "$expected_sha" ]; then
      echo "Checksum mismatch for $output" >&2
      exit 1
    fi
    chmod +x "$output"
    installed_atmos_path="./$output"
    echo "Atmos installed into $installed_atmos_path, make sure to move it into a directory in your PATH"
}

# Function to install via Homebrew
install_via_brew() {
	export HOMEBREW_NO_INSTALL_CLEANUP=1
	export HOMEBREW_NO_ENV_HINTS=1
	echo "Using Homebrew package manager..."
	brew install atmos
}

# Function to install via Homebrew
install_via_nix() {
	echo "Using Nix package manager..."
	nix-env -iA nixpkgs.atmos
}

# Main installation function
install_atmos() {
	# Installation flow
	case $method in
		auto)
			echo "Auto-detecting installation method..."
			package_manager=$(detect_package_manager)

			if [ "$package_manager" == "brew" ]; then
				install_via_brew
			elif [ "$package_manager" == "nix" ]; then
				install_via_nix
			elif [ "$package_manager" == "none" ]; then
				echo "No package manager detected. Installing via binary download..."
				install_via_binary_download
			else
				install_via_cloudsmith
			fi
			;;
		package)
			echo "Installing using package manager..."
			install_via_cloudsmith
			;;
		binary)
			echo "Installing directly from binary..."
			install_via_binary_download
			;;
		*)
			echo "Invalid method specified. Use 'auto', 'package', or 'binary'."
			exit 1
			;;
	esac
}

# Run the installation
install_atmos

# Check if atmos is installed properly
if [ -n "$installed_atmos_path" ]; then
	atmos="$(pwd)/${installed_atmos_path#./}"
else
	atmos=$(PATH=.:$PATH command -v atmos)
fi

verify_dir=$(mktemp -d 2>/dev/null || mktemp -d -t atmos-verify)
(cd "$verify_dir" && "$atmos" version)
rm -rf "$verify_dir"

echo "Atmos has been successfully installed to ${atmos}"
exit 0
