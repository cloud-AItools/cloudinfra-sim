#!/usr/bin/env bash
# Ansible 자체 테스트(TR-COV-0306)를 WSL(Linux)에서 실행한다. Windows는 Ansible 제어 노드를 지원하지 않는다.
#
# 준비(한 번): Windows에서 Linux용 시험 실행파일을 만든다(WSL에 Go 불필요).
#   cd 40-test && GOOS=linux GOARCH=amd64 go test -c -o ansible/ansible.linux.test ./ansible
# 실행(WSL 안):
#   bash /mnt/d/AISCP/cloudinfra-sim/40-test/ansible/run-wsl.sh
# 필요: python3·python3-venv(Ubuntu: sudo apt install -y python3-venv). 처음 실행 때 PyPI·Ansible Galaxy에서
# requirements.txt·requirements.yml의 패키지를 내려받는다(가상 환경은 ~/.cache/cloudinfra-sim-ansible).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
venv="${ANSIBLE_VENV:-$HOME/.cache/cloudinfra-sim-ansible}"
bin="$here/ansible.linux.test"
[ -x "$bin" ] || chmod +x "$bin" 2>/dev/null || true
[ -f "$bin" ] || { echo "시험 실행파일이 없다: $bin (Windows에서 GOOS=linux go test -c로 만든다)"; exit 2; }
if [ ! -x "$venv/bin/python3" ]; then
  python3 -m venv "$venv"
fi
# 요구사항이 늘면 기존 가상 환경에도 반영한다(이미 맞으면 내려받지 않는다)
"$venv/bin/pip" install --disable-pip-version-check -q -r "$here/requirements.txt"
# collection list는 컬렉션이 없어도 0으로 끝나므로 출력에서 이름을 찾는다
if ! "$venv/bin/ansible-galaxy" collection list community.vmware 2>/dev/null | grep -q '^community\.vmware '; then
  "$venv/bin/ansible-galaxy" collection install -r "$here/requirements.yml"
fi
export PATH="$venv/bin:$PATH" SIM_TEST_REPO="$repo" ANSIBLE_REQUIRED=1
cd "$here"
# /mnt/d는 실행 권한이 없을 수 있어 Linux 임시 위치로 복사해 실행한다
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
cp "$bin" "$tmp/ansible.test"; chmod +x "$tmp/ansible.test"
"$tmp/ansible.test" -test.v -test.count=1 -test.timeout=20m "$@"
