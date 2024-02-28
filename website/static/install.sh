#!/usr/bin/env bash

# Default method to auto
method="${1:-auto}"

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
    latest_release=$(curl -s https://api.github.com/repos/cloudposse/atmos/releases/latest | grep 'tag_name' | cut -d '"' -f 4  | tr -d v)
		os=$(uname -s| tr '[:upper:]' '[:lower:]')
		arch=$(uname -m)
    binary_url="https://github.com/cloudposse/atmos/releases/download/v${latest_release}/atmos_${latest_release}_${os}_${arch}"
    curl -fsSL "${binary_url}" -o atmos
    chmod +x atmos
		echo "Atmos installed into "./atmos", make sure to move it into a directory in your PATH"
}

# Function to install via Homebrew
install_via_brew() {
	export HOMEBREW_NO_INSTALL_CLEANUP=1
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
atmos=$(PATH=.:$PATH command -v atmos)
$atmos version

echo "Atmos has been successfully installed to ${atmos}"
exit 0
