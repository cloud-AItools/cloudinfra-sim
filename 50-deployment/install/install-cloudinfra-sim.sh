#!/usr/bin/env bash
# cloudinfra-sim macOS·Linux 로컬 실행 환경 구성(NFR-ARC-0308 Phase 5, 배포 토폴로지 ① — PRD-4 D6).
# Windows는 Install-CloudinfraSim.ps1을 쓴다.
#
# 하는 일(각 단계는 건너뛸 수 있다):
#   1) hosts 등록: vcenter·esx-01..NN → 127.0.0.2, nsx → 127.0.0.3 (관리자 권한 필요)
#   2) 루프백 별칭: macOS는 lo0에 127.0.0.2·127.0.0.3 별칭 추가(관리자 권한, 재부팅 시 사라짐 — --persist로 LaunchDaemon 등록).
#      Linux는 127.0.0.0/8 전체가 lo로 가므로 필요 없다.
#   3) 루트 CA 신뢰: macOS 키체인(기본 로그인 키체인, --system이면 System 키체인), Linux 시스템 저장소
#      (Debian 계열 update-ca-certificates, RHEL 계열 update-ca-trust). Java·Python certifi 등 별도 저장소는 다루지 않는다.
#   4) 443 바인딩: Linux는 1024 미만 포트에 권한이 필요하다 → --binary로 실행파일에 cap_net_bind_service를 준다
#      (또는 --vcenter-addr 127.0.0.2:8443처럼 높은 포트를 쓴다). macOS 10.14 이상은 권한 없이 가능하다(가정, 미검증).
#
# 사용:
#   ./install-cloudinfra-sim.sh [--domain cloudinfra-sim.test] [--data-dir DIR] [--esx-hosts 3]
#                               [--skip-hosts] [--skip-alias] [--skip-ca] [--system] [--persist] [--binary PATH]
#                               [--uninstall] [--dry-run] [--os linux|darwin] [--hosts-file FILE]
#   --dry-run: 바꾸지 않고 실행할 명령만 출력한다. --hosts-file: 시험용 hosts 경로(기본 /etc/hosts).
set -euo pipefail

DOMAIN="cloudinfra-sim.test"
DATA_DIR=""
ESX_HOSTS=3
SKIP_HOSTS=0 SKIP_ALIAS=0 SKIP_CA=0 SYSTEM=0 PERSIST=0 UNINSTALL=0 DRY=0
BINARY=""
OS_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
HOSTS_FILE="/etc/hosts"
MARKER="# cloudinfra-sim"

die() { echo "오류: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --esx-hosts) ESX_HOSTS="$2"; shift 2 ;;
    --skip-hosts) SKIP_HOSTS=1; shift ;;
    --skip-alias) SKIP_ALIAS=1; shift ;;
    --skip-ca) SKIP_CA=1; shift ;;
    --system) SYSTEM=1; shift ;;
    --persist) PERSIST=1; shift ;;
    --binary) BINARY="$2"; shift 2 ;;
    --uninstall) UNINSTALL=1; shift ;;
    --dry-run) DRY=1; shift ;;
    --os) OS_NAME="$2"; shift 2 ;;
    --hosts-file) HOSTS_FILE="$2"; shift 2 ;;
    -h|--help) sed -n '2,24p' "$0"; exit 0 ;;
    *) die "알 수 없는 옵션: $1" ;;
  esac
done

case "$DOMAIN" in
  *.test|*.internal) ;;
  *) die "예약 TLD(.test/.internal)만 허용한다(NFR-0314): $DOMAIN" ;;
esac
case "$ESX_HOSTS" in ''|*[!0-9]*) die "--esx-hosts는 숫자" ;; esac
case "$OS_NAME" in darwin|linux) ;; *) die "지원하지 않는 OS: $OS_NAME (Windows는 Install-CloudinfraSim.ps1)" ;; esac

# 데이터 디렉터리 기본값은 시뮬레이터와 같다(Go os.UserConfigDir + /cloudinfra-sim).
if [ -z "$DATA_DIR" ]; then
  if [ "$OS_NAME" = darwin ]; then DATA_DIR="$HOME/Library/Application Support/cloudinfra-sim"
  else DATA_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/cloudinfra-sim"; fi
fi
CA="$DATA_DIR/certs/ca.pem"

# run은 명령을 실행하거나(--dry-run이면) 출력한다. 관리자 권한이 필요하면 sudo를 붙인다.
run() {
  if [ "$DRY" = 1 ]; then echo "+ $*"; return 0; fi
  "$@"
}
SUDO=""
if [ "$(id -u)" != 0 ]; then SUDO="sudo"; fi
priv() { if [ -n "$SUDO" ]; then run sudo "$@"; else run "$@"; fi; }

# 1) hosts
if [ "$SKIP_HOSTS" = 0 ]; then
  tmp="$(mktemp)"
  grep -v -F "$MARKER" "$HOSTS_FILE" > "$tmp" || true
  if [ "$UNINSTALL" = 0 ]; then
    {
      echo "127.0.0.2 vcenter.$DOMAIN $MARKER"
      echo "127.0.0.3 nsx.$DOMAIN $MARKER"
      i=1
      while [ "$i" -le "$ESX_HOSTS" ]; do printf '127.0.0.2 esx-%02d.%s %s\n' "$i" "$DOMAIN" "$MARKER"; i=$((i + 1)); done
    } >> "$tmp"
  fi
  if [ "$HOSTS_FILE" = /etc/hosts ]; then
    priv cp "$tmp" /etc/hosts
  else
    run cp "$tmp" "$HOSTS_FILE"
  fi
  rm -f "$tmp"
  if [ "$UNINSTALL" = 1 ]; then echo "hosts 항목 제거: $HOSTS_FILE"; else echo "hosts 등록: $((ESX_HOSTS + 2))건 ($HOSTS_FILE)"; fi
fi

# 2) 루프백 별칭(macOS)
PLIST="/Library/LaunchDaemons/test.cloudinfra-sim.loopback.plist"
if [ "$SKIP_ALIAS" = 0 ] && [ "$OS_NAME" = darwin ]; then
  for ip in 127.0.0.2 127.0.0.3; do
    if [ "$UNINSTALL" = 1 ]; then priv ifconfig lo0 -alias "$ip" || true
    else priv ifconfig lo0 alias "$ip" up; fi
  done
  if [ "$PERSIST" = 1 ] && [ "$UNINSTALL" = 0 ]; then
    plist="$(mktemp)"
    cat > "$plist" <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>test.cloudinfra-sim.loopback</string>
  <key>ProgramArguments</key><array><string>/bin/sh</string><string>-c</string>
    <string>/sbin/ifconfig lo0 alias 127.0.0.2 up; /sbin/ifconfig lo0 alias 127.0.0.3 up</string></array>
  <key>RunAtLoad</key><true/>
</dict></plist>
PLIST_EOF
    priv cp "$plist" "$PLIST"
    priv launchctl load -w "$PLIST"
    rm -f "$plist"
    echo "루프백 별칭 영구 등록: $PLIST"
  fi
  if [ "$UNINSTALL" = 1 ]; then
    if [ -f "$PLIST" ] || [ "$DRY" = 1 ]; then priv launchctl unload -w "$PLIST" || true; priv rm -f "$PLIST"; fi
    echo "루프백 별칭 제거"
  else
    echo "루프백 별칭: lo0 127.0.0.2, 127.0.0.3"
  fi
fi

# 3) 루트 CA 신뢰
if [ "$SKIP_CA" = 0 ]; then
  if [ "$OS_NAME" = darwin ]; then
    if [ "$SYSTEM" = 1 ]; then KEYCHAIN="/Library/Keychains/System.keychain"; else KEYCHAIN="$HOME/Library/Keychains/login.keychain-db"; fi
    if [ "$UNINSTALL" = 1 ]; then
      if [ "$SYSTEM" = 1 ]; then priv security delete-certificate -c "cloudinfra-sim Root CA" "$KEYCHAIN" || true
      else run security delete-certificate -c "cloudinfra-sim Root CA" "$KEYCHAIN" || true; fi
      echo "루트 CA 신뢰 제거: $KEYCHAIN"
    elif [ -f "$CA" ] || [ "$DRY" = 1 ]; then
      if [ "$SYSTEM" = 1 ]; then priv security add-trusted-cert -d -r trustRoot -k "$KEYCHAIN" "$CA"
      else run security add-trusted-cert -r trustRoot -k "$KEYCHAIN" "$CA"; fi
      echo "루트 CA 신뢰 등록: $KEYCHAIN ($CA)"
    else
      echo "CA 파일이 아직 없다: $CA — 시뮬레이터를 한 번 기동한 뒤 다시 실행한다."
    fi
  else
    if [ -d /usr/local/share/ca-certificates ] || command -v update-ca-certificates >/dev/null 2>&1; then
      DEST="/usr/local/share/ca-certificates/cloudinfra-sim.crt"; UPDATE="update-ca-certificates"
    elif [ -d /etc/pki/ca-trust/source/anchors ] || command -v update-ca-trust >/dev/null 2>&1; then
      DEST="/etc/pki/ca-trust/source/anchors/cloudinfra-sim.crt"; UPDATE="update-ca-trust"
    else
      DEST="/usr/local/share/ca-certificates/cloudinfra-sim.crt"; UPDATE="update-ca-certificates"
      [ "$DRY" = 1 ] || die "시스템 CA 저장소를 찾지 못했다. $CA를 배포판 방식으로 신뢰 등록한다."
    fi
    if [ "$UNINSTALL" = 1 ]; then
      priv rm -f "$DEST"; priv "$UPDATE" || true
      echo "루트 CA 신뢰 제거: $DEST"
    elif [ -f "$CA" ] || [ "$DRY" = 1 ]; then
      priv cp "$CA" "$DEST"; priv "$UPDATE"
      echo "루트 CA 신뢰 등록: $DEST ($CA)"
    else
      echo "CA 파일이 아직 없다: $CA — 시뮬레이터를 한 번 기동한 뒤 다시 실행한다."
    fi
  fi
fi

# 4) 443 바인딩 권한(Linux)
if [ -n "$BINARY" ] && [ "$OS_NAME" = linux ] && [ "$UNINSTALL" = 0 ]; then
  [ -f "$BINARY" ] || [ "$DRY" = 1 ] || die "실행파일이 없다: $BINARY"
  priv setcap cap_net_bind_service=+ep "$BINARY"
  echo "443 바인딩 권한: $BINARY"
fi
