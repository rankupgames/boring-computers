#!/usr/bin/env bash
set -euo pipefail

in_scope_reference="${1:-}"
out_of_scope_reference="${2:-}"
target_url="${3:-}"
if [[ -z "${in_scope_reference}" || -z "${out_of_scope_reference}" || -z "${target_url}" ]]; then
	echo "usage: $0 <in-scope-credential-reference> <out-of-scope-credential-reference> <synthetic-target-url>" >&2
	exit 2
fi
for reference in "${in_scope_reference}" "${out_of_scope_reference}"; do
	[[ "${reference}" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "invalid credential reference" >&2; exit 2; }
done

boring_url="${BORING_URL:-http://127.0.0.1:8080}"
boring_token_file="${BORING_TOKEN_FILE:-/etc/boring/boring_token}"
wispkey_bin="${WISPKEY_BIN:-wispkey}"
relay_bin="${WISPKEY_RELAY_BIN:-/usr/local/bin/wispkey-vsock-relay}"
loopback_port="${WISPKEY_LOOPBACK_PORT:-17700}"
chroot_base="${BORING_CHROOT_BASE:-/srv/jailer}"
jailer_uid="${BORING_JAILER_UID:-30000}"
jailer_gid="${BORING_JAILER_GID:-30000}"

for command in curl jq base64 sudo; do
	command -v "${command}" >/dev/null || { echo "missing required command: ${command}" >&2; exit 1; }
done
command -v "${wispkey_bin}" >/dev/null || { echo "wispkey command is unavailable" >&2; exit 1; }
[[ -x "${relay_bin}" ]] || { echo "vsock relay is unavailable" >&2; exit 1; }
sudo -n test -r "${boring_token_file}" || { echo "boring control token is not readable through sudo" >&2; exit 1; }

work="$(mktemp -d /tmp/boring-wispkey-verify.XXXXXX)"
chmod 0700 "${work}"
trap 'rm -rf -- "${work}"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
header_file="${work}/boring.headers"
boring_token="$(sudo -n cat "${boring_token_file}")"
[[ -n "${boring_token}" ]] || { echo "boring control token is empty" >&2; exit 1; }
printf 'Authorization: Bearer %s\nContent-Type: application/json\n' "${boring_token}" > "${header_file}"
unset boring_token
chmod 0600 "${header_file}"

machine_id=""
instance_name=""
wispkey_pid=""
relay_pid=""
tty_echo_disabled=0

api() {
	method="$1"
	path="$2"
	body="${3:-}"
	if [[ -n "${body}" ]]; then
		printf '%s' "${body}" | curl --fail-with-body --silent --show-error \
			--request "${method}" --header "@${header_file}" --data-binary @- "${boring_url}${path}"
	else
		curl --fail-with-body --silent --show-error \
			--request "${method}" --header "@${header_file}" "${boring_url}${path}"
	fi
}

guest_exec() {
	command="$1"
	payload="$(printf '%s' "${command}" | jq -Rsc '{command:.,timeout_seconds:60}')"
	unset command
	response="$(api POST "/v1/machines/${machine_id}/exec" "${payload}")"
	[[ "$(jq -r '.exit_code' <<<"${response}")" == "0" ]] || return 1
	jq -r '.output' <<<"${response}"
}

cleanup() {
	result=$?
	if [[ "${tty_echo_disabled}" == "1" && -n "${machine_id}" ]]; then
		guest_exec "stty echo" >/dev/null 2>&1 || true
	fi
	if [[ -n "${relay_pid}" ]]; then
		sudo -n kill "${relay_pid}" >/dev/null 2>&1 || true
	fi
	if [[ -n "${wispkey_pid}" ]]; then
		kill "${wispkey_pid}" >/dev/null 2>&1 || true
	fi
	if [[ -n "${instance_name}" ]]; then
		"${wispkey_bin}" instance revoke "${instance_name}" >/dev/null 2>&1 || true
	fi
	if [[ -n "${machine_id}" ]]; then
		api DELETE "/v1/machines/${machine_id}" >/dev/null 2>&1 || true
	fi
	rm -rf "${work}"
	exit "${result}"
}
trap cleanup EXIT INT TERM

machine="$(api POST /v1/machines '{"template":"unterm-builder","ttl_seconds":900,"net":false}')"
machine_id="$(jq -er '.id' <<<"${machine}")"
instance_name="bc-${machine_id}"

enrollment="$("${wispkey_bin}" --format json instance enroll "${instance_name}" \
	--description "Disposable Firecracker verification worker" \
	--credential "${in_scope_reference}")"
instance_id="$(jq -er '.id' <<<"${enrollment}")"
instance_secret="$(jq -er '.secret' <<<"${enrollment}")"
in_scope_token="$("${wispkey_bin}" --format json get "${in_scope_reference}" --show-token | jq -er '.credential.wisp_token')"
out_of_scope_token="$("${wispkey_bin}" --format json get "${out_of_scope_reference}" --show-token | jq -er '.credential.wisp_token')"

"${wispkey_bin}" serve --listen "tcp://127.0.0.1:${loopback_port}" --require-identity \
	>"${work}/wispkey.log" 2>&1 &
wispkey_pid=$!

vsock_port_path="${chroot_base}/firecracker/${machine_id}/root/run/vsock_7700"
sudo -n "${relay_bin}" --listen-unix "${vsock_port_path}" --upstream "127.0.0.1:${loopback_port}" \
	--socket-uid "${jailer_uid}" --socket-gid "${jailer_gid}" \
	>"${work}/relay.log" 2>&1 &
relay_pid=$!

for _ in {1..100}; do
	[[ -S "${vsock_port_path}" ]] && break
	kill -0 "${wispkey_pid}" 2>/dev/null || { echo "WispKey listener exited" >&2; exit 1; }
	sleep 0.05
done
[[ -S "${vsock_port_path}" ]] || { echo "Firecracker vsock relay did not bind" >&2; exit 1; }

guest_exec "stty -echo" >/dev/null
tty_echo_disabled=1

guest_request() {
	token="$1"
	show_denial="$2"
	command="export WISPKEY_INSTANCE_ID=\"\$(printf %s '$(printf '%s' "${instance_id}" | base64 | tr -d '\n')' | base64 -d)\"; export WISPKEY_INSTANCE_SECRET=\"\$(printf %s '$(printf '%s' "${instance_secret}" | base64 | tr -d '\n')' | base64 -d)\"; export WISPKEY_TOKEN=\"\$(printf %s '$(printf '%s' "${token}" | base64 | tr -d '\n')' | base64 -d)\"; export WISPKEY_TARGET_URL=\"\$(printf %s '$(printf '%s' "${target_url}" | base64 | tr -d '\n')' | base64 -d)\"; WISPKEY_SHOW_DENIAL=${show_denial} wispkey-vsock-request"
	guest_exec "${command}"
}

in_scope_result="$(guest_request "${in_scope_token}" 0)"
in_scope_status="$(jq -er '.status' <<<"${in_scope_result}")"
if [[ "${in_scope_status}" == "401" || "${in_scope_status}" == "403" ]]; then
	echo "in-scope request was rejected" >&2
	exit 1
fi

denied_result="$(guest_request "${out_of_scope_token}" 1)"
[[ "$(jq -er '.status' <<<"${denied_result}")" == "403" ]] || { echo "out-of-scope request did not fail closed" >&2; exit 1; }
access_request="$(jq -er '.access_request' <<<"${denied_result}")"

"${wispkey_bin}" instance approve "${access_request}" >/dev/null
approved_result="$(guest_request "${out_of_scope_token}" 0)"
approved_status="$(jq -er '.status' <<<"${approved_result}")"
if [[ "${approved_status}" == "401" || "${approved_status}" == "403" ]]; then
	echo "approved request remained blocked" >&2
	exit 1
fi

"${wispkey_bin}" instance revoke "${instance_name}" >/dev/null
revoked_result="$(guest_request "${in_scope_token}" 0)"
[[ "$(jq -er '.status' <<<"${revoked_result}")" == "401" ]] || { echo "revoked identity was not rejected" >&2; exit 1; }
instance_name=""

guest_exec "stty echo" >/dev/null
tty_echo_disabled=0
echo "WispKey Firecracker vsock verification passed"
