# VERSION_PROFILE_SPEC — 버전 프로파일과 버전 인식 직렬화

> 관련: PRD-1 §3.6(FR-VC-0601 ~ FR-VC-0608), §7.3(FR-API-0301 ~ FR-API-0312), PRD-4 WP 0.12·WP 1.2·WP 1.3·WP 1.13, D1·D11 · 작성일 2026-10-10 (Phase 0b)

## 1. 첫 기준 조합 (D1)

| 제품 | 버전 | 빌드 | API 버전 | 계약 근거 |
|---|---|---|---|---|
| vCenter | 7.0 U3w (7.0.3.02500) | 24927011 | 7.0.3.0 | pyVmomi 9.1.1.0 타입 메타데이터(도입 버전 기준 필터), govmomi v0.56.0 |
| NSX-T | 3.2.4.3 | 24942842 | — | NSX-T **3.2.1** 공식 OpenAPI(3.2.4.x 스펙 미공개). 차이는 미검증 표시, 릴리스 노트로 보완 |

**추가 버전(Phase 4b, WP 4.7, FR-API-0311):**

| 제품 | 버전 | 빌드 | API 버전 | 계약 근거 | 조합 상호운용성 |
|---|---|---|---|---|---|
| vCenter | 7.0 U2 (7.0.2.00000) | 17694817 | 7.0.2.0 | KB 326316(버전·빌드), vSphere Web Services API 7.0U2 참조(7.0.2.0), pyVmomi 타입 메타데이터(7.0.2.0 기준 필터). REST는 7.0 U2용 SDK 계약 미확보로 실행 범위 밖 | NSX-T 3.2.4.3과 `interop: unverified`(release 거부, R-42) |

8.x를 고르지 않은 이유: NSX-T 3.2.4는 vCenter 8.x와 공식 지원 조합이 아니라는 보고가 있어(상호운용성 매트릭스 'not supported') D1의 NSX-T 3.x 범위 안에서 짝지을 NSX가 없다. 사용자 D1 재검토 사항(DEVELOPMENT_PLAN §12).

## 2. 디렉터리 구성

```
30-implementation/data/compatibility-profiles/
  vcenter/7.0.3.02500/
    profile.yaml            # 아래 §3
    vim-typeinfo.json       # pyVmomi 추출(타입·필드·메서드 도입 버전, Phase 2b: 속성 선언 순서·필수·F_LINK·메서드 반환형·선언 Fault 추가 — 스키마 대조 검사기용)
    vimServiceVersions.xml  # urn:vim25 7.0.3.0 + priorVersions
    vapi-contract.json      # REST 계약(SRC-VAPI-SDK-7032) — 저장소 밖, Get-VapiContract.ps1로 생성(시험에서 사용)
    behavior-rules.yaml     # 버전별 동작(Hot-add, 네트워크 표현 D8 등)
    faults.yaml             # Fault 계약
    verification.yaml       # API별 검증 상태·증거(CI 생성, WP 1.13)
  vcenter/7.0.2.00000/      # 추가 버전(Phase 4b)
    profile.yaml            # typeinfo: ../7.0.3.02500/vim-typeinfo.json — 버전별 도입 정보를 모두 담은 단일 파일을 공유(복사 아님)
    behavior-rules.yaml     # 7.0.3과 같다고 가정(근거 미확보, R-08)
    verification.yaml       # cmd/evidence -scope-only(실행 범위 API만 승격)
  nsx/3.2.4.3/
    profile.yaml
    nsx_api.json            # 3.2.1.0.0 (spec_source에 기록) — D11 확정 전 사용자 측 다운로드
    nsx_policy_api.json     # 3.2.1.0.0
    behavior-rules.yaml     # Partial Patch, _revision, Realization 지연, H-API 예외
    errors.yaml             # 상황별 error_code (ApiError 형식)
    verification.yaml
  compatibility-matrix.yaml
```

## 3. `profile.yaml` 필드

| 필드 | 예 (vCenter) | 예 (NSX) | 요구사항 |
|---|---|---|---|
| `product`, `version`, `build` | VMware vCenter Server, 7.0.3, 24927011 | VMware NSX-T Data Center, 3.2.4.3, 24942842 | FR-VC-0602 |
| `api_version` | 7.0.3.0 | — | FR-VC-0605 |
| `about` | `ServiceContent.about` 전체 값(fullName, osType, instanceUuid 등) | `/api/v1/node/version` 응답 값 | FR-API-0306 |
| `spec_source` | 근거 ID·파일명·버전·SHA-256·조회일 | 3.2.1.0.0 + "3.2.4.3과 차이 미검증" | FR-API-0307, FR-API-0309 |
| `execution_scope` | API Coverage Matrix의 API ID 목록(빈 목록 금지) | 같음 | FR-API-0302 |
| `tls.max_version`, `tls.basis` | `"1.2"`, KB 377237·313841(vSphere는 8.0 U3부터 TLS 1.3) | `"1.2"`, KB 319144(NSX는 4.1.2부터 TLS 1.3) | 수신 TLS 최대 버전. 빈 값 = 1.3까지, 그 밖의 값은 기동 거부. 하한은 1.2 고정(R-45 ③) |
| `defaults.verification_mode` | 빌드 종류별(release 패키지/개발 빌드) | 같음 | FR-API-0302 |

## 4. 세션 버전과 응답 필터 (PoC-13)

1. **세션 버전** = min(SOAPAction의 `urn:vim25/<ver>`, 프로파일 `api_version`). SOAPAction이 없으면 프로파일 버전을 쓴다(FR-VC-0605).
2. **필터 위치:** Gateway의 `/sdk` 응답 경로. vcsim은 최신 타입 전체를 직렬화하고 SOAPAction을 보지 않는다(R-11 확인).
3. **알고리즘:** 응답 XML 토큰을 순회하면서 Go 타입(govmomi `types`)을 따라간다. 요소마다 다음을 한다.
   - 필드를 선언한 타입과 요소 이름으로 도입 버전을 찾는다(pyVmomi 우선, 없으면 govmomi `vim` 태그).
   - 세션 버전보다 새 필드는 제거한다.
   - `xsi:type`이 세션 버전보다 새 타입이면 제거한다. 하위 호환 대체 규칙은 미구현이며 Phase 2b 전에 근거를 확보한다.
4. **메서드 검사(FR-VC-0607):** 세션 버전보다 새 메서드 호출은 거부한다. pyVmomi 메타데이터의 메서드 963개 버전을 사용한다. Phase 1에 구현한다.
5. **REST·NSX(FR-VC-0608):** 활성 프로파일 스펙에 없는 경로·메서드는 거부한다(NSX는 PoC-17에서 확인).
6. **비용:** 응답을 한 번 더 파싱·직렬화한다. 대규모 `RetrievePropertiesEx` 응답의 지연은 Phase 1에서 측정한다(NFR-0110과 별도).

## 5. 기동 검증 모드 (FR-API-0302, WP 1.2·1.13)

| 모드 | 기동 조건 | 거부 시 출력 |
|---|---|---|
| `development` | `execution_scope`의 모든 API가 `Contract Defined` 이상 | 미달 API ID 목록, 사용 가능한 프로파일과 검증 상태 |
| `release` | `execution_scope`의 모든 API가 현재 프로파일·커밋에 대응하는 `Integration Tested` 증거를 가짐 | 같음 + 무효화된 증거 |

공통 거부 조건: 미등록 프로파일, 스키마 오류, `compatibility-matrix.yaml`에 없는 vCenter × NSX 조합.

**조합 상호운용성(Phase 4b):** `compatibility-matrix.yaml`의 각 조합에 `interop: verified | unverified`를 둔다. `unverified` 조합은 development 모드에서만 기동하고(경고 로그) release 모드는 거부한다. 상호운용성 매트릭스로 확인하면 `verified`로 바꾼다.

**추가 버전 시험:** `SIM_TEST_VCENTER_VERSION`(기본 7.0.3.02500)으로 계약·E2E·SDK 시험 전체를 다른 vCenter 프로파일로 실행하고, `go run ./cmd/evidence -results results-7020.json -vcenter 7.0.2.00000 -scope-only`로 그 프로파일의 실행 범위 API만 증거를 기록한다. 버전 간 비교는 `contract/TestVersionDifferential*`.

## 6. 데이터 출처와 라이선스 (D11 입력)

| 데이터 | 출처 | 저장소 포함 |
|---|---|---|
| `vim-typeinfo.json` | pyVmomi(Apache-2.0) 추출 | 가능할 것으로 판단(출처 표시) — D11 |
| NSX OpenAPI | Broadcom 개발자 포털, 라이선스 표기 없음 | 불명확 → 사용자 측 다운로드 스크립트 권장 — D11 |
| vSphere Automation 스펙 | 미확보 | — |
