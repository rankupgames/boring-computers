#!/usr/bin/env bash
set -euo pipefail

target_alias="${1:-}"
if [[ -z "${target_alias}" ]]; then
	echo "usage: BORING_CONTROL_TOKEN=<injected> $0 <worker-ssh-alias> < sudo-password" >&2
	exit 2
fi

control_token="${BORING_CONTROL_TOKEN:-}"
IFS= read -r sudo_password
if [[ -z "${control_token}" || -z "${sudo_password}" ]]; then
	echo "injected control token or sudo credential is empty" >&2
	exit 1
fi
if [[ "${control_token}" == *$'\n'* || "${control_token}" == *$'\r'* || "${sudo_password}" == *$'\n'* || "${sudo_password}" == *$'\r'* ]]; then
	echo "injected credentials must be single-line values" >&2
	exit 1
fi

printf '%s\n%s\n' "${sudo_password}" "${control_token}" | \
	ssh -o ClearAllForwardings=yes -o ExitOnForwardFailure=yes "${target_alias}" \
	'sudo -S -p "" /bin/sh -eu -c '\''umask 077; install -d -m0700 /etc/boring; IFS= read -r control_token; printf %s "$control_token" > /etc/boring/boring_token; chmod 0600 /etc/boring/boring_token; unset control_token'\'''

unset control_token sudo_password BORING_CONTROL_TOKEN
echo "installed protected boringd control credential"
