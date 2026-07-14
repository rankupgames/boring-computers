#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
verify_script="${script_dir}/verify-wispkey-vsock.sh"
wispkey_bin="${WISPKEY_BIN:-/usr/local/bin/wispkey}"

for command in python3 od tr sudo; do
	command -v "${command}" >/dev/null || { echo "missing required command: ${command}" >&2; exit 1; }
done
[[ -x "${verify_script}" ]] || { echo "verification harness is unavailable" >&2; exit 1; }
[[ -x "${wispkey_bin}" ]] || { echo "wispkey command is unavailable" >&2; exit 1; }
sudo -n true || { echo "prime a short-lived sudo timestamp before running the proof" >&2; exit 1; }

work="$(mktemp -d /tmp/boring-wispkey-synthetic.XXXXXX)"
chmod 0700 "${work}"
target_pid=""

cleanup() {
	result=$?
	if [[ -n "${target_pid}" ]]; then
		kill "${target_pid}" >/dev/null 2>&1 || true
		wait "${target_pid}" >/dev/null 2>&1 || true
	fi
	unset WISPKEY_PASSWORD synthetic_in_scope synthetic_out_of_scope
	rm -rf -- "${work}"
	exit "${result}"
}
trap cleanup EXIT INT TERM

random_hex() {
	od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
}

export WISPKEY_VAULT_PATH="${work}/vault"
export WISPKEY_PASSWORD="$(random_hex)"
synthetic_in_scope="$(random_hex)"
synthetic_out_of_scope="$(random_hex)"

"${wispkey_bin}" init >/dev/null
printf '%s' "${synthetic_in_scope}" | "${wispkey_bin}" add synthetic-in-scope \
	--type bearer_token --description "Disposable in-scope Firecracker proof credential" \
	--hosts 127.0.0.1 --tags synthetic,firecracker-proof --value-file - >/dev/null
printf '%s' "${synthetic_out_of_scope}" | "${wispkey_bin}" add synthetic-out-of-scope \
	--type bearer_token --description "Disposable out-of-scope Firecracker proof credential" \
	--hosts 127.0.0.1 --tags synthetic,firecracker-proof --value-file - >/dev/null
unset synthetic_in_scope synthetic_out_of_scope

port_file="${work}/target.port"
python3 - "${port_file}" >"${work}/target.log" 2>&1 <<'PYTHON' &
import http.server
import sys


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(204)
        self.end_headers()

    def log_message(self, *_args):
        pass


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
with open(sys.argv[1], "w", encoding="ascii") as handle:
    handle.write(str(server.server_port))
    handle.flush()
server.serve_forever()
PYTHON
target_pid=$!

for _ in {1..100}; do
	[[ -s "${port_file}" ]] && break
	kill -0 "${target_pid}" 2>/dev/null || { echo "synthetic target exited" >&2; exit 1; }
	sleep 0.05
done
[[ -s "${port_file}" ]] || { echo "synthetic target did not become ready" >&2; exit 1; }
target_port="$(<"${port_file}")"
[[ "${target_port}" =~ ^[0-9]+$ ]] || { echo "synthetic target returned an invalid port" >&2; exit 1; }

WISPKEY_BIN="${wispkey_bin}" "${verify_script}" \
	synthetic-in-scope synthetic-out-of-scope "http://127.0.0.1:${target_port}/"
echo "Disposable WispKey vault and synthetic credentials removed"
