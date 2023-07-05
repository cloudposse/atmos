#!/bin/bash

function set-eks-cluster() {
	if (($# == 0)); then
		cat >&2 <<'EOF'
Usage:
  set-eks-cluster <stack> <role>

  set-eks-cluster tenant1-ue1-dev
  set-eks-cluster tenant2-uw2-prod admin
  set-eks-cluster tenant2-ue2-prod reader
  set-eks-cluster off

EOF
		return 1
	fi
	if [[ $1 == "off" ]]; then
		if [[ -n $KUBECONFIG ]] && [[ -f $KUBECONFIG ]]; then
			rm -f "$KUBECONFIG"
		fi
		return 0
	fi
	local stack="$1"
	local role="$2"
	if [[ -z $role ]]; then
		role="admin"
	fi

	KUBECONFIG="$(atmos set-eks-cluster eks/cluster -s "$stack" -r "$role")"
	export KUBECONFIG
}
