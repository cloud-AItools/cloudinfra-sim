#!/usr/bin/env bash
# 배포 패키지 생성(NFR-ARC-0203, WP 5.7): 대상별 release 빌드 + 프로파일·Fixture·시나리오·설치 스크립트 + SHA256SUMS.
# NSX 스펙·vAPI 계약 파일은 포함하지 않는다(PRD-4 D11, 사용자 측 확보). 순수 Go(cgo 없음)라 교차 빌드한다.
#
# 사용: 50-deployment/dist.sh [출력 디렉터리(기본 dist)] [대상 ...(기본 전체)]
#   예: 50-deployment/dist.sh dist linux/amd64
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-dist}"
shift || true
PLATFORMS=("$@")
if [ ${#PLATFORMS[@]} -eq 0 ]; then PLATFORMS=(windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64); fi
GO="${GO:-go}"
IMPL="$ROOT/30-implementation"
case "$OUT" in /*|[A-Za-z]:*) ;; *) OUT="$ROOT/$OUT" ;; esac
rm -rf "$OUT" && mkdir -p "$OUT"
digest="$(cd "$IMPL" && "$GO" run ./cmd/srcdigest)"
for p in "${PLATFORMS[@]}"; do
  os="${p%/*}" arch="${p#*/}" ext=""
  [ "$os" = windows ] && ext=".exe"
  name="cloudinfra-sim-$os-$arch"
  dir="$OUT/$name"
  mkdir -p "$dir/data" "$dir/install"
  (cd "$IMPL" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" "$GO" build -trimpath \
    -ldflags "-s -w -X cloudinfra-sim/internal/sim.BuildCommit=$digest -X cloudinfra-sim/internal/config.BuildKind=release" \
    -o "$dir/cloudinfra-sim$ext" ./cmd/cloudinfra-sim)
  cp -r "$IMPL/data/compatibility-profiles" "$IMPL/data/fixtures" "$IMPL/data/scenarios" "$dir/data/"
  find "$dir/data/compatibility-profiles" \( -name 'nsx_api.json' -o -name 'nsx_policy_api.json' -o -name 'vapi-*.json' \) -delete
  if [ "$os" = windows ]; then cp "$ROOT"/50-deployment/install/*.ps1 "$dir/install/"; else cp "$ROOT/50-deployment/install/install-cloudinfra-sim.sh" "$dir/install/"; fi
  cat > "$dir/README.txt" <<EOF
cloudinfra-sim $os/$arch (release, commit $digest)
1) NSX 스펙(D11)을 data/compatibility-profiles/nsx/3.2.4.3/에 둔다(install/Get-NsxSpec.ps1 또는 문서의 절차).
2) ./cloudinfra-sim$ext --vcenter-version 7.0.3.02500 --nsx-version 3.2.4.3
3) hosts·루트 CA: install/ 스크립트(관리자 권한 단계는 사용자가 실행).
검증 상한: Integration Tested(승인 범위 자체 테스트, CMP·실환경 미검증).
EOF
  (cd "$OUT" && tar -czf "$name.tar.gz" "$name")
  echo "packaged $name"
done
(cd "$OUT" && sha256sum ./*.tar.gz > SHA256SUMS && cat SHA256SUMS)
