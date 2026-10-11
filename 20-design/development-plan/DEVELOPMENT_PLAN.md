# DEVELOPMENT_PLAN — 단계별 개발계획과 결정 기록

> 관련: PRD-4 §15~§19, Task 5·Task 7 · 작성일 2026-10-10 (Phase 0b)  
> 이 문서는 PRD-4 기준선을 상세화한다. WP 번호와 요구사항 ID는 PRD-4를 따른다.

## 1. 진행 현황과 기간 상한

| 단계 | 기간 상한 | 상태 | 비고 |
|---|---|---|---|
| Phase 0a | **최소(1일)** — 2026-10-10 | 종료 | 우선 PoC 7건 판정, D3 유지, D4 확정 |
| Phase 0b | **최소(1일)** — 2026-10-10 | **종료** | D1~D13 결정 완료, 산출물 12종, 추적표 자동 검사 통과 |
| Phase 1 | 최소(1일) — 2026-10-10 | **종료**(이월 13행은 Phase 2a로 이관 결정) | 본 구현 `30-implementation/`, 시험 `40-test/` |
| Phase 2a | 최소(1일) — 2026-10-10 | **종료**(남은 이월 8행) | §8. Scenario 02~06·Negative 통과, 실행 범위 28개 API Integration Tested |
| Phase 2b | 최소(1일) — 2026-10-10 | **종료**(D2 확정. CI·설치 실행은 사용자 조치 대기) | §9. SDK 계약·스키마 대조·범용 SDK·선언형 시나리오, 실행 범위 28개 API Integration Tested |
| Phase 3 | 최소(1일) — 2026-10-10 | **종료**(진입 조건 MVP 마일스톤의 사용자 조치 2건은 대기, 사용자 지시로 진행) | §10. Scenario 08·전송 계층 Fault Rule 게이트 통과 |
| Phase 4a | 최소(1일) — 2026-10-10 | **종료** | §11. Scenario 09·10 게이트 통과, 실행 범위 41개 API Integration Tested |
| Phase 4b | 최소(1일) — 2026-10-11 | **종료**(추가 API 실행 범위 2026-10-11 확정. 채택 범위·7.0.2 조합 상호운용성·D1 재검토는 사용자 확정 대기) | §12. Scenario 07·버전 간 Differential Test 게이트 통과 |
| Phase 5 | 최소(1일) — 2026-10-11 | **자체 시험 기준 종료, 릴리스 판정은 조건부**(AC-11 CI 실행·설치 스크립트 실제 실행은 사용자 조치 대기) | §13. AC-01 ~ AC-19 중 18개 자체 증거 충족, AC-11은 로컬 전체 회귀로 대체 확인 |

기간 상한은 2026-10-10 결정(최소)에 따른다. 상한 안에 결론이 나지 않은 항목은 `RISK_REGISTER.md`의 미해결 위험으로 둔다(R-12, R-18, R-20 등).

## 2. Phase 0 산출물 (Task 7)

| 문서 | 위치 | 상태 |
|---|---|---|
| ARCHITECTURE.md | `20-design/architecture/` | 작성 |
| OPENSOURCE_ASSESSMENT.md | `20-design/architecture/` | 작성 |
| DATA_MODEL.md | `20-design/architecture/` | 작성 |
| VERSION_PROFILE_SPEC.md | `20-design/architecture/` | 작성 |
| API_COVERAGE_MATRIX.md (+ CSV 2종) | `20-design/api-coverage/` | 잠정 기준선 |
| BEHAVIOR_SPEC.md | `20-design/behavior-spec/` | 작성 |
| POC_PLAN.md | `20-design/development-plan/` | 작성(PoC 판정 완료) |
| DEVELOPMENT_PLAN.md | `20-design/development-plan/` | 본 문서 |
| MVP_SCOPE.md | `20-design/development-plan/` | 잠정 |
| TEST_STRATEGY.md (+ 추적표 281행) | `20-design/development-plan/` | 작성 |
| RISK_REGISTER.md | `20-design/development-plan/` | 작성(R-17~R-20 신규) |
| RESOURCE_BUDGET.md | `20-design/development-plan/` | 예산 확정, 측정 경계·보존 한도 검토 필요 |

## 3. 결정 기록 (PRD-4 §19)

| ID | 상태 | 결정 내용 / 권고 |
|---|---|---|
| D1 | 결정 | 7.0 U2 이상(8.x 포함)·NSX-T 3.x. 첫 조합 7.0 U3w + 3.2.4.3. NSX 계약 근거는 공개 스펙 3.2.1(차이 미검증) |
| D2 | **최종 확정**(2026-10-10, Phase 2b), Phase 3 후 41개로 확대 | 실행 범위 28개 → 41개 API(P1 중 NSX-001B 제외 — 근거 미확보), ○ 20건 MVP 제외, SDK = govmomi v0.56.0(선정)·govc·pyVmomi 9.1.1.0(범용). §9 |
| D3 | 결정·유지 확인 | 권고안 A. PoC-17·PoC-21로 유지 확인 |
| D4 | 결정 | 300 MB / 5초 / 200 MB, 측정 경계·CI 기준·보존 한도는 RESOURCE_BUDGET §2~§4대로 확정 |
| D5 | 결정 | 실환경 비교·Record 보류 |
| D6 | 결정 (권고안 채택) | 토폴로지 ① 같은 PC 네이티브만 MVP. ②③은 Phase 5 이후 |
| D7 | 결정 (권고안 채택) | MVP는 기본 `CreateVM_Task`만. Clone·Content Library/OVF·Guest Customization·VM 재시작(Reset·Guest Reboot)·디스크 변경은 MVP 이후(WP 4.11) |
| D8 | 결정 (권고안 채택) | 7.0.3 프로파일은 VDS 7.0 DVPG(`backingType=nsx`, `segmentId`). OpaqueNetwork(N-VDS)는 추가 프로파일에서 |
| D9 | 결정 (권고안 채택) | MVP는 Segment·Realization·조회 계열만. H-API(PoC-16 확인)·Fabric VM/VIF·VM Tag·Search는 Phase 3·4b. Manager API는 MVP 조회 5종(node/version, cluster/status, compute-managers, transport-zones)으로 한정 |
| D10 | 결정 (권고안 채택) | SOAP Login, REST `/api/session`, NSX Basic + `/api/session/create`. STS(`LoginByToken`)·Lookup·PBM은 MVP 이후 |
| D11 | 결정 (권고안 채택) | govmomi는 go.mod 의존성. pyVmomi 추출 `vim-typeinfo.json`은 출처를 표시해 저장소에 포함. NSX·vSphere Automation 스펙은 저장소·배포물에서 빼고 사용자 측 다운로드 스크립트로 확보 |
| D12 | 결정 (권고안 채택) | MVP는 단일 vCenter + 단일 NSX(호스트 이름 `vcenter`·`nsx`·`esx-NN`). 다중 인스턴스는 MVP 이후 |
| D13 | 결정 | 시뮬레이터 구성·자체 테스트까지 |

## 4. Phase 1 작업 상세 (Task 5 형식)

선행 관계: 1.1 → 1.2 → 1.3, 1.4 → 1.6 → 1.7, 1.5 → 1.7, 1.8은 1.4·1.5 이후, 1.13은 1.2 이후. 복잡도: S/M/L.

| WP | 목적 | 상세 작업 | 모듈 | 테스트 | 완료 기준 | 복잡도 | 위험 | 우선 |
|---|---|---|---|---|---|---|---|---|
| WP 1.1 | 단일 실행파일·Gateway | TLS(지문 고정 인증서), SNI 라우팅, 421, 교차 락 진입점, 호출 기록 골격, `--domain` | `cmd/cloudinfra-sim`, `internal/gateway` | Unit, Footprint | 3개 호스트 라우팅, 미등록 거부, 예산 측정 가능 | M | R-13 | P1 |
| WP 1.2 | 프로파일·버전 | 로더, 검증 모드, 세션 버전, 응답 필터(PoC-13 이관), 메서드 버전 검사, `vimServiceVersions.xml` | `internal/profile` | Unit, Contract | 7.0.3/6.7.3 세션 필터 계약 통과, 잘못된 프로파일 거부 | L | R-11 | P1 |
| WP 1.3 | 기준 프로파일 데이터 | 7.0.3·3.2.4.3 `profile.yaml`, `vim-typeinfo.json` 생성 스크립트, NSX 스펙 다운로드 스크립트, 출처·SHA-256 | `data/compatibility-profiles` | 검토 | 출처·근거 기록 완비 | M | R-16, R-18 | P1 |
| WP 1.4 | vcsim 임베드 | 리스너 없는 초기화, About 덮어쓰기, 컨테이너 VM 비활성, 보조 시뮬레이터 범위 | `internal/vcenter` | Unit | SOAP 로그인·조회 | S | — | P1 |
| WP 1.5 | NSX 엔진 | 범용 핸들러(PoC-17 이관): 경로·쿼리 색인, 같은 모양 템플릿 병합, 지연 스키마, `ApiError` | `internal/nsx` | Unit, Contract | 전 경로 색인, 스키마 검증 | M | R-14 | P1 |
| WP 1.6 | 인증·세션 | SOAP Login, `/api/session` 어댑터, NSX Basic·`/api/session/create`·XSRF, 사용자·Credential(환경변수) | `internal/gateway`, `vcenter`, `nsx` | Contract, Negative | 정상·실패 인증 계약 | M | R-17 | P1 |
| WP 1.7 | 조회 API | REST 뷰(datacenter·cluster·host·datastore·network·vm), `/api/appliance/system/version`, NSX MP 조회 5종, Compute Manager 관계·지문 | `internal/vcenter`, `nsx` | Contract, E2E | Scenario 01 | L | R-17, R-20 | P1 |
| WP 1.8 | Fixture·Reset | default-small(vCenter+NSX), Seed 결정론, Reset | `data/fixtures`, `internal/admin` | Stateful | 같은 Seed → 같은 ID | M | MoRef 결정론 | P1 |
| WP 1.9 | 로그 | 요청·작업·상태·오류 로그, 사람/JSON 형식, 상관 ID | `internal/gateway` | 로그 검사 | NFR-0401~0405 | S | — | P2 |
| WP 1.10 | 예산 CI | `40-test/footprint`(RESOURCE_BUDGET §2), 회귀 경고 | `40-test/footprint` | Footprint | 예산 충족 | S | R-09 | P1 |
| WP 1.11 | 설치·보안 | Windows 설치 스크립트(hosts, 사용자/머신 CA), 지문 유지, Admin 인증, 마스킹, lite 보존 한도 | `50-deployment/install`, `internal/admin`, `store` | 보안·설치 시험 | NFR-03xx·05xx ● | M | R-13 | P1 |
| WP 1.12 | 미지원 처리 | 미지원 응답·로그·집계 키 상한, deprecation | `internal/gateway`, `vcenter`, `nsx` | Negative | FR-API-0401~0404 | S | — | P1 |
| WP 1.13 | 증거 자동화 | CI가 `verification.yaml` 생성, 커밋·프로파일 해시 연결, 무효화 | CI, `internal/profile` | 기동 모드 시험 | release 기동·거부 | M | — | P1 |

Phase 2a 이후의 WP는 PRD-4 §15.2 표를 기준으로 각 단계에 진입할 때 같은 형식으로 상세화한다.

## 5. 일정 (상대 규모 → 첫 산정)

PRD-4 §15.4의 상대 규모를 Phase 0 결과로 보정했다. 1인 기준 영업일 추정이며 Phase 1 종료 시 다시 산정한다.

| 단계 | 상대 규모 | 추정 | 보정 근거 |
|---|---|---|---|
| Phase 1 | M → **L** | 8~12일 | REST `/api` 자체 구현(R-17), 프로파일 필터 이관 |
| Phase 2a | L | 10~15일 | Adapter·NSX 행위는 PoC 코드 이관으로 단축 가능 |
| Phase 2b | M | 4~6일 | govmomi 계약 테스트 + 스키마 대조 검사기 |
| Phase 3 | M | 6~10일 | D8·D9 결정에 따라 변동 |
| Phase 4a | L | 10~15일 | R-12 포크 결정 |
| Phase 4b | L (변동 큼) | 10~20일 | D9 범위, 추가 버전 수 |
| Phase 5 | M | 8~12일 | — |

## 7. Phase 1 결과 (2026-10-10)

**종료 게이트(PRD-4 §15.2):**

| 조건 | 결과 | 증거 |
|---|---|---|
| Scenario 01(TR-E2E-0101 ~ 0104) 통과 | 통과 | `40-test/simulator-e2e` 3건 |
| 기본 Fixture 리소스 예산(NFR-0109 ~ 0112) | 충족: 기동 1.15초, 메모리 96.5 MB, 유휴 CPU 0, 설치 35 MB | `40-test/footprint` |
| development 모드 통합 시험 증거 | 실행 범위 15개 API 모두 Integration Tested | `verification.yaml`(commit `src-04f5b9f8c65a7cba`) |
| release 모드 기동·검증 미달 거부 | 증거 일치 빌드는 기동, 다른 구현 식별자는 15건 사유와 함께 거부 | 수동 확인, `TestStartupRefusals` |
| 잘못된 프로파일 거부(AC-15) | 미등록 버전·허용되지 않은 조합·스키마 오류·release 증거 없음 거부 | `TestStartupRefusals` |

시험: 구현 단위 2개 패키지 + `40-test` 최상위 21건, 실패 0건. 추적표(TEST_STRATEGY §7): 시험 통과 99행, 자체 검토 48행, 미실행 134행(대부분 Phase 2a·2b 대상).

**구현 중 발견·조치:**
- vcsim 기본 SessionManager는 비어 있지 않은 아무 계정이나 허용한다 → `ValidLogin`으로 사용자 저장소(`users.json`, 무작위 비밀번호) 검증.
- vcsim vAPI는 `Authorization: SIGN …`을 검증 없이 허용한다 → Gateway에서 거부(D10: STS는 MVP 밖).
- 새 실행파일의 첫 기동은 백신 검사로 약 3초 → RESOURCE_BUDGET §2 규칙대로 첫 실행 제외(Footprint 시험에 예열 단계).

**REST 계약 근거(2026-10-10 결정 ②):** vSphere Automation SDK v7.0.3.2 바인딩·런타임에서 계약을 추출해 `/api` REST 4개 API(VC-REST-001 ~ 004)를 실행 범위에 넣고 Integration Tested까지 검증했다. 계약 시험이 찾은 불일치 2건(데이터스토어 형식 OTHER, 어플라이언스 버전 필수 필드 누락)을 수정했다. release 모드 기동(실행 범위 19개 API) 재확인.

**Phase 1 이월 항목(13행) — Phase 2a로 이관(2026-10-10 결정, `40-test/carryover.yaml`):**

| 요구사항 | 상태 | 제안 |
|---|---|---|
| NFR-0402, NFR-0403 (작업·상태 로그) | Task·상태 전이 엔진이 없어 미구현 | Phase 2a WP 2.7과 함께 구현 |
| NFR-0502, 0505, 0506, 0508 (Event·Task·Transition 보존 한도) | vcsim `recentTask` 무제한 | Phase 2a WP 2.7에서 한도 적용 |
| FR-NSX-0231 (Manager API deprecation 응답) | 미구현 | Phase 2a 초기에 구현 |
| FR-NSX-0104 (스펙 제공 경로) | 미구현. 경로가 스펙 밖이라 근거 확보 필요 | Phase 2a, 근거 확보 후 |
| FR-API-0101, FR-API-0102 (외부 인터페이스 동일성·응답 비표준화) | SOAP·NSX는 시험으로 일부 확인, REST는 계약 근거 없음(R-20) | REST 근거 확보 후 계약 시험 |
| NFR-ARC-0111 (CI) | 저장소가 git이 아니고 CI 미구성 | 저장소·CI 결정 필요 |
| NFR-ARC-0308 (설치 자동화) | `Install-CloudinfraSim.ps1` 작성, 실행 안 함(hosts·인증서 저장소 변경) | 사용자 실행 확인 필요 |
| FR-API-0303 (실행 중 프로파일 변경, 권장 –) | 미구현 | MVP 이후 |

## 8. Phase 2a 결과 (2026-10-10)

**종료 게이트(PRD-4 §15.2):** development 모드에서 Scenario 02~06과 기본 Negative 테스트(TR-COV-0303) 통과, Scenario 06은 즉시 완료와 설정 지연 후 완료를 모두 포함.

| 조건 | 결과 | 증거 |
|---|---|---|
| Scenario 02~05(인벤토리 동기화, VM 생성·재구성·삭제) | 통과 | `40-test/simulator-e2e/scenario02_05_test.go` 4건 |
| Scenario 06(NSX Segment, 즉시·지연 후 Realization) | 통과 | `scenario06_test.go` 2건(지연 700 ms: IN_PROGRESS → SUCCESS) |
| 기본 Negative(TR-COV-0303) | 통과: 잘못된 입력·ID, 중복 생성, Revision 충돌, 미지원 API·SOAP Fault, 미인증, 참조·삭제 의존성 | `contract/negative_test.go` 4건 |
| 리소스 예산(NFR-0109 ~ 0112, SQLite 추가 후) | 충족: 기동 1.24초, 메모리 102 MB, 유휴 CPU 0, 설치 39 MB | `40-test/footprint` |
| 실행 범위 확대(VC-SOAP-008 ~ 012, NSX-008 ~ 011) | 실행 범위 28개 API 모두 Integration Tested, release 모드 기동 확인 | `verification.yaml`(commit `src-f3c9942310548f5c`) |

시험: 구현 단위 7건 + `40-test` 최상위 40건, 실패 0건. 추적표(TEST_STRATEGY §7): 시험 통과 205행, 자체 검토 54행, 이월 8행, 미실행 14행(전부 Phase 2b 이후 대상).

**구현:** SQLite 저장소(`internal/store`, lite 인메모리·full WAL), NSX 영속화(쓰기마다 트랜잭션), 부분 PATCH 설정·헤더, 참조·삭제 의존성 검사, 스펙 기본값, 스펙 제공 경로, Behavior Adapter(용량·전원 규칙·폴더 내 이름 중복·익명 조회 제한), Task 관찰자(상태 전이 기록·작업 로그·보존 한도 정리), 미지원 API·오류 로그.

**구현 중 발견·조치(RISK_REGISTER):**
- R-22 정정: vcsim은 `recentTask`만 200건으로 자르고 완료 Task 객체는 지우지 않는다 → 보존 한도 초과 Task를 레지스트리·`recentTask`에서 함께 제거.
- R-23: vcsim `ReconfigVM`이 PropertyCollector에 변경을 알리지 않는다 → 재구성 성공 시 재통지.
- R-24: vcsim이 익명 속성 조회와 폴더 내 VM 이름 중복을 허용한다 → Adapter에서 제한·`DuplicateName`(`Fetch`는 제한 못 함).
- R-25: 알 수 없는 SOAP 메서드 응답(`InvalidRequest`)이 미지원 집계에서 빠졌다 → Gateway 판정 보완.
- R-02 확인: vcsim 내부 타입 단언 때문에 래퍼 객체 대신 실패 Task + 응답 변환 방식으로 Adapter 구현.

**가정으로 남은 동작(BEHAVIOR_SPEC):** 부분 PATCH 비활성 시 교체 의미, `display_name` 기본값, 참조·삭제 의존성 오류의 상태·메시지, 삭제 후 Realization 404, 스펙 제공 경로, 익명 조회 제한 범위, NSX `error_code` 값.

**남은 이월(8행, `40-test/carryover.yaml`):**

| 요구사항 | 상태 | 이관 |
|---|---|---|
| NFR-0403 | State Transition·Unsupported API·Error Log 구현·시험. Fault Injection Log 미구현 | Phase 3 WP 3.8·Phase 4a WP 4.3 |
| NFR-0508 | lite 유한 기본값 적용·시험. 설정 조정 노출·full 회전 미검증 | Phase 5 WP 5.9 |
| FR-NSX-0104 | 구현·시험 완료. 경로가 스펙 밖이고 API Guide 근거 미확보 | 근거 확보 후 |
| FR-API-0101, FR-API-0102 | 계약 시험 범위 확대 필요 | Phase 2b(스키마 대조 검사기) |
| NFR-ARC-0111 (CI) | 저장소·CI 미구성 | 사용자 결정 필요 |
| NFR-ARC-0308 (설치 자동화) | 스크립트 작성, 실행 안 함 | 사용자 실행 필요 |
| FR-API-0303 (권장 –) | 미구현 | MVP 이후 |

## 9. Phase 2b 결과 — MVP 검증·호환성 판정 (2026-10-10)

**종료 게이트(PRD-4 §15.2) 판정:**

| 조건 | 결과 | 증거 |
|---|---|---|
| 승인 시뮬레이션 기능 범위 확정(D2) | **확정(2026-10-10, 사용자 결정)** — 아래 'D2 최종 범위' | `o-decisions.yaml`, 실행 범위 28개 API |
| Scenario 01~06 회귀 | 통과 | `simulator-e2e` 10건(Scenario 01~06 9건 + 선언형 시나리오 실행 1건: 시나리오 파일 2개) |
| MVP Contract·Stateful·기본 Negative | 통과 | `contract/` 33건 |
| 선정 SDK 자체 계약 테스트(AC-17, TR-COV-0304) | 통과: govmomi v0.56.0로 실행 범위 SOAP 호출 수행, 응답 약 230건(세션 2개)을 공식 계약(SRC-VIM-TYPEINFO)에 대조 — 위반 0 | `contract/TestSOAPSchemaContract`, 검사기 자체 시험 `vimcheck/TestCheckerDetectsViolations` |
| 협상 버전 하향 세션(AC-16 MVP) | 통과: 7.0.3 프로파일에 6.7.3 세션 — 구조 필드·속성 경로·changeSet·스키마 대조 모두 | `TestVersionAwareSerialization`, `TestSOAPSchemaContract/6.7.3`, `sdk/TestGovcVersionDowngrade` |
| 범용 SDK 통합(TR-COV-0305) | 통과: govc(govmomi v0.56.0 태그 커밋), pyVmomi 9.1.1.0 | `sdk/TestGovcLifecycle`, `sdk/TestGovcVersionDowngrade`, `sdk/TestPyVmomi` |
| MVP 실행 범위 release 기동 | 확인 | `verification.yaml`(commit `src-f8967d336efdeb2e`) |
| 추적표 MVP 요구사항 증거 | 시험 통과 218행, 자체 검토 55행, 이월 5행, 미실행 3행(Phase 4a 이후), ○ 결정 대기 0건 | TEST_STRATEGY §7 |
| 리소스 예산 | 기동 1.21초, 메모리 81 MB, 유휴 CPU 0, 설치 39.9 MB | `40-test/footprint` |

시험: 구현 단위 8건 + `40-test` 최상위 48건, 실패·Skip 0건. 검증 상한은 Integration Tested(승인 범위 자체 테스트, CMP·실환경 미검증)이며 Admin `/v1/status`의 `verification_cap`과 시나리오 리포트에 표시한다(TR-COV-0409).

**구현·도구:**
- 스키마 대조 검사기 `40-test/internal/vimcheck`: pyVmomi 타입 정보(선언 순서·필수·F_LINK·열거값·메서드 반환형·선언 Fault 추가 추출, `extract_pyvmomi.py`)로 SOAP 응답의 네임스페이스·xsi:type·속성 순서·필수·도입 버전·열거값·MethodName·Fault 선언을 검사.
- NSX 응답 스키마 검사기 `40-test/internal/nsxcheck`: 스펙 응답 스키마(`$ref`·`allOf`·discriminator)와 ApiError 대조.
- 범용 SDK 시험 `40-test/sdk`(govc·pyVmomi). 이름 없이 IP로 접속하고 인증서 지문으로 검증(hosts 등록 불필요, Gateway의 리스너 기본 서비스).
- 선언형 시나리오 `40-test/scenarios/*.yaml` + Runner `internal/scenario`: ①~⑨ 단계, 마스킹 Trace 저장, JSON·Markdown 리포트, Reset 후 기준선 복귀 확인(`make scenario`).
- 보존 한도 설정 `--retain-tasks|transitions|operations|trace`(0=기본, 무제한 거부, NFR-0508), Admin 상태에 적용값 표시.

**시험이 찾은 결함과 조치(RISK_REGISTER):** 속성 경로 요청의 버전 필터 누락·재직렬화 문자 참조(R-11), Task `info.name` 형식(R-26), 필수 열거값 빈 값·빈 RetrieveResult·vcsim 전용 옵션(R-27), Realization `publish_status` 누락(R-28). 모두 SDK 단독 시험(govmomi)으로는 드러나지 않았고 범용 SDK·스키마 대조로 발견했다.

**D2 최종 범위(2026-10-10 사용자 확정):**

| 구분 | 확정 내용 |
|---|---|
| 실행 범위(승인 시뮬레이션 API) | VC-SOAP-001 ~ 013, VC-REST-001 ~ 004, NSX-001A, NSX-002 ~ 011 — 28개, 모두 Integration Tested |
| 실행 범위 밖으로 둠 | NSX-001B(`/api/session/create`·XSRF): 구현·시험(`TestNSXSessionAndXSRF`) 완료, 스펙에는 경로 언급만 있고 요청·응답 계약 근거(API Guide 본문) 미확보. NSX-012(스펙 제공, FR-NSX-0104) 같은 사유. VC-REST-005·006(P2) |
| ○ 항목 | 20건 모두 D6~D10에 따라 MVP 제외(`40-test/o-decisions.yaml`) |
| 자체 테스트 SDK | 선정: govmomi v0.56.0(계약 시험). 범용: govc(v0.56.0 태그 커밋), pyVmomi 9.1.1.0. 결과는 시험한 SDK·버전에만 해당 |

**MVP 미충족·사용자 조치 항목(이월 5행):** NFR-ARC-0111(CI — 저장소·CI 결정), NFR-ARC-0308(설치 스크립트 실행 — 관리자 권한), FR-NSX-0104(스펙 제공 경로 문서 근거), NFR-0403(Fault Injection Log — Fault Injection 구현 단계, TR-SCN-0301상 MVP 요구 아님), FR-API-0303(MVP –).

## 10. Phase 3 결과 — Cross-System Behavior (2026-10-10)

**진입:** 진입 조건(MVP 마일스톤) 중 사용자 조치 2건(CI·설치 스크립트 실행)이 남은 상태에서 사용자 지시(2026-10-10)로 진행했다.

**종료 게이트(PRD-4 §15.2):**

| 조건 | 결과 | 증거 |
|---|---|---|
| Scenario 08(TR-E2E-0801 ~ 0804) | 통과: Segment → NSX DVPG(vCenter) → VM vNIC 연결 → 양쪽 조회 관계 ID 일치 → 일관성(조정 drift 0) → 연결 중 Segment 삭제 거부 → VM 삭제 → Segment·DVPG 삭제 | `simulator-e2e/TestScenario08_CrossSystemWorkflow` |
| 전송 계층 Fault Rule 추가·제거·적용 | 통과: 일시적 503(N회 후 복구), 지정 상태(NSX ApiError·vAPI 오류·SOAP 프록시 오류), 지연, 검증 거부, 적용 이력, 로그·Trace, 초기화 시 제거, Seed 결정론 | `contract/TestTransportFaultRules`, `fault.TestRuleModel` |

시험: 구현 단위 9건 + `40-test` 최상위 56건, 실패·Skip 0건. 리소스: 기동 1.41초, 메모리 88 MB, 유휴 CPU 0, 설치 40 MB. 실행 범위 28개 API Integration Tested 유지, release 기동 확인(commit `src-a6ebf879e6800c93`). 추적표: 개별 행 317(Phase 3 목표 행 포함), 시험 통과 252, 자체 검토 55, 이월 7, 미실행 3(Phase 4a 이후).

**WP별 구현:**

| WP | 내용 |
|---|---|
| 3.1 | Compute Manager 상태, Discovered Node(호스트 ↔ NSX) |
| 3.2 | NSX DVPG 매핑(D8, Fixture `vds_transport_zones`), Network ID ↔ Policy Path |
| 3.3 | Fabric VM·VIF·Segment Port 파생, NSX VM Tag(소유·저장), 실현 VM 조회, Group 유효 멤버(Tag·Name 조건) |
| 3.4 | Realization: 동기화 전 IN_PROGRESS, 동기화 실패 ERROR(오류 정보·알람), 의존 리소스 대기, 진행 중 재수정, intent_version |
| 3.5 | 이벤트(Task Failed·Host Disconnected/Connected 추가, Initiator 보정), NSX 알람, 감사 기록(사용자·동작·대상·Task·원인 cid), 변경 원인 사슬 |
| 3.6 | 멱등 전체 조정(Reconciliation), drift 복구, Admin `/v1/reconcile`·`/v1/relations`, 환경 Namespace 분리 |
| 3.7 | 결정론적 QueryPerf(조회 시 계산, 할당·용량 정합), 덮어쓰기·결측, 호스트 하드웨어 = Fixture 용량 |
| 3.8 | 공통 Fault Rule 모델·전송 계층 적용·Admin `/v1/faults`·Fault Injection Log |

**시험이 찾은 결함·vcsim 공백(RISK_REGISTER R-29 ~ R-32):** Registry.Handler 대체가 로그인 세션에서 무시됨(래퍼 객체로 해결), XML 인코딩이 0 표본을 빠뜨림, 호스트 하드웨어와 Fixture 용량 불일치, Task Initiator 고정값. NSX 파생 객체의 공통 메타데이터 오류는 응답 스키마 대조가 찾아 고쳤다.

**이월(Phase 3 목표 행 3건, `40-test/carryover.yaml`):** FR-MET-0201·0103의 Datastore 사용량 임계(알람 엔진, Phase 4a WP 4.3), FR-BE-0703 고급 용량 모델(선택 항목, 미채택). 해소: NFR-0403(Fault Injection Log).

**실행 범위:** Phase 3 추가 API 13건(API_COVERAGE_MATRIX §5)을 2026-10-10 사용자 결정으로 실행 범위에 넣어 41개 API로 확대했다(Phase 4a 시작 시 증거 재생성).

## 11. Phase 4a 결과 — 장애·세션·권한 재현 (2026-10-10)

**시작 전 결정:** Phase 3 추가 API 13건을 실행 범위에 넣었다(D2 확대, 41개 API, PRD-revision #66).

**종료 게이트(PRD-4 §15.2):**

| 조건 | 결과 | 증거 |
|---|---|---|
| Scenario 09(TR-E2E-0901 ~ 0904) | 통과: CreateVM_Task 실행 중 실패 주입(running → InsufficientResourcesFault) → 오류 감지 → 부분 생성 없음 확인·재시도 성공, 전원 켜기 실패 주입 → 보상(삭제) → 재생성, 같은 이름 VM 1개, ResourcePool 할당 실패 | `simulator-e2e/TestScenario09_TaskFailureRecovery` |
| Scenario 10(TR-E2E-1001 ~ 1003) | 통과: 세션 계층 만료 주입 → NotAuthenticated → 재로그인. 가상 시간 31분 진행 → SOAP·REST·NSX 세션 유휴 만료(29분은 유효) → 재인증. SOAP 응답 시각도 가상 시각 | `simulator-e2e/TestScenario10_SessionExpiry` |

시험: 구현 단위 13건 + `40-test` 최상위 62건, 실패·Skip 0건. 리소스: 기동 1.43초, 메모리 92 MB, 유휴 CPU 0, 설치 40 MB. 실행 범위 41개 API Integration Tested, release 기동 확인(commit `src-02bc69d746ba7b0f`). 추적표: 개별 행 365(Phase 4a 목표 행 포함), 시험 통과 304, 자체 검토 54, 이월 5, 미실행 2(Phase 4b·5).

**WP별 구현:**

| WP | 내용 |
|---|---|
| 4.10 | Virtual Clock(`internal/clock`, Admin `/v1/clock`): 정상·테스트 모드, 가속·정지·진행, 실제→가상 구간 대응, 이벤트 기반 대기. vcsim 시각은 SOAP 응답에서 변환(R-12 결정: 포크 없음) |
| 4.1 | 세션 유휴 만료(가상 시간): SOAP·REST·NSX, 강제 만료(세션 계층 Fault) |
| 4.2 | RBAC: 메서드 권한 = 공식 타입 정보, 역할 Admin·ReadOnly·VirtualMachineOperator(범위), NSX enterprise_admin·auditor, Admin `/v1/users` |
| 4.3 | Fault 계층 확장: vcenter(task_fail·soap_fault·task_delay·호스트 끊김·유지보수·데이터스토어 가득), nsx(realization_fail·hang·revision_conflict), session(expire), transport(timeout·reset 추가). 조건 nth·after·user·resource·시각 범위 |
| 4.4 | 오류 계약 FR-FLT-0101 ~ 0125 시험 연결(권한 부족·세션 만료·사용 중·인프라·통신·작업) |
| 4.5 | Task 완료 지연(가상 시간), Realization Timeout(정지) |
| 4.8 | 확장 Negative·장애 E2E(`contract/phase4a_test.go`, Scenario 09·10), 기본 알람 Datastore usage on disk |

**시험이 찾은 결함·vcsim 공백(RISK_REGISTER):** 대체 Task와 vcsim 엔터티 락 교착(R-33), 알람 엔진의 메트릭 식 부재(R-34), 공식 타입 정보의 빈 권한(R-35), 시각 응답 타입 미등록으로 변환 누락(수정). R-12는 완화(포크 없이 응답 변환).

**이월:** Phase 3에서 넘긴 FR-MET-0201(알람)·FR-MET-0103(임계 초과 값 생성, 분류 정정)은 해소. 남은 이월은 사용자 조치 2건(CI, 설치 스크립트 실행), 문서 근거 1건(FR-NSX-0104), 선택 항목 2건(FR-BE-0703, FR-API-0303).

## 12. Phase 4b 결과 — 기능 확장·추가 버전 (2026-10-11)

**종료 게이트(PRD-4 §15.2):**

| 조건 | 결과 | 증거 |
|---|---|---|
| Scenario 07(TR-E2E-0701 ~ 0704) | 통과: ① Group(VM Tag 조건) ② Service(TCP 8443) ③ Security Policy + Rule(정책 본문에 포함) → 모두 Realization SUCCESS ④ 최신 `_revision`으로 Rule 수정 ⑤ 조회: 변경 반영, Revision +1, rule_id 유지, 응답 스키마 대조 ⑥ 오래된 Revision → 412 ApiError(스펙 응답 대조), PUT `_revision` 누락 → 412, 거부 뒤 상태 불변 | `simulator-e2e/TestScenario07_SecurityPolicyChange` |
| 추가 버전 프로파일의 버전 간 Differential Test(AC-16 확장) | 통과: vCenter 7.0 U2(7.0.2.00000) ↔ 7.0 U3w 같은 작업·같은 Fixture. 각 응답이 자기 세션 버전 계약을 지킴, 차이는 정확히 버전 도입 정보로 설명(7.0.3 전용 `VirtualMachineConfigInfo.vmOpNotificationToAppEnabled`, 상태에는 있으나 7.0.2 응답에서는 숨김), About·`vimServiceVersions.xml` 프로파일별, 7.0.3.0 도입 메서드는 7.0.2에서 MethodNotFound | `contract/TestVersionDifferential`, `TestVersionDifferentialMethods`, 7.0.2 프로파일로 전체 계약·E2E·SDK 재실행(`results-7020.json`, 77건 통과) |

시험: 구현 단위 16건 + `40-test` 최상위 79건(기준 프로파일), 7.0.2 프로파일 77건, 실패·Skip 0건. 리소스: 기동 1.44초, 메모리 118 MB(예산 300 MB, 4a 대비 증가는 PBM·Lookup 레지스트리와 보안·라우팅 시드), 유휴 CPU 0, 설치 41 MB. 기준 조합 release 기동 확인(commit `src-f80666c42d50db7e`, 실행 범위 41개 API). 추적표: 개별 행 406, 시험 통과 342, 자체 검토 54, 이월 9(미채택 제안 4 포함), 미실행 1(Phase 5).

**WP별 구현:**

| WP | 내용 |
|---|---|
| 4.9 | NSX 보안: Group(expression 규칙 검사, 멤버 VM·Segment·Segment Port·IP, 중첩 Group, 대소문자 무관 평가), Service(내장 service_entries, 기본 Service는 수정·삭제 거부), Security Policy(내장 rules: PATCH는 나열한 Rule만, PUT은 전체 교체, 삭제는 Rule 포함), Rule(rule_id 시스템 할당, "ANY" 단독 규칙, revise 순서 변경). 라우팅: Tier-0·Tier-1·locale-services·interfaces(참조·삭제 검사, Realization 의존), Edge Cluster 시드. H-API(`PATCH /infra`, Child 래퍼·ChildResourceReference·marked_for_delete·enforce_revision_check, 한 트랜잭션·순서 무관 참조). Policy·Manager Search(Lucene 부분집합 문법, API_COVERAGE_MATRIX §6.2). Manager: Host Transport Node·상태(vCenter 호스트 파생), Logical Switch·Port(Policy 실현), 컬렉션 POST 생성(201). 요청 검증의 다형 하위 타입(discriminator)·maxItems |
| 4.11 | VM 생명주기: Clone(용량·데이터스토어 검사), Template 변환(변경 알림, 템플릿 작업 거부), Register/Unregister, Reset·Suspend 사전조건, Rename(DuplicateName)·Move·Relocate, Snapshot 생성·복귀(하드웨어·전원 상태 복원)·삭제, Guest Customization. 보조 서비스: Lookup Service(Gateway 주소·인증서 광고), PBM(정책 조회, VM 정책 연결), Content Library `/api`(라이브러리·항목 CRUD·찾기) |
| 4.6 | PropertyCollector 데이터 버전 발급·검증(InvalidCollectorVersion), maxObjectUpdates 부분 응답 이어 받기 시험, 이벤트·Task 시간 필터의 가상 시각 해석(Phase 4a 알려진 제한 해소) |
| 4.7 | vCenter 7.0 U2 프로파일(KB 326316·7.0U2 API 참조 근거, 타입 메타데이터 공유·버전 필터), compatibility-matrix `interop`(미확인 조합은 development만), 시험 프로파일 선택(`SIM_TEST_VCENTER_VERSION`), 증거 생성 `-scope-only` |

**시험이 찾은 결함·vcsim 공백(RISK_REGISTER R-36 ~ R-42):** 다른 이름 RegisterVM 실패(R-36), PBM VM 정책 연결 없음(R-37), Content Library 내부 필드·계약 밖 필드 노출(R-38), PropertyCollector 데이터 버전 미발급(R-39), 생명주기 공백(템플릿 전원 켜기 허용, Reset 오류 상태, Rename Fault 타입, Clone 사전조건 우회, 스냅샷 복귀가 전원·구성을 되돌리지 않음, 템플릿 변환 변경 미통지, R-40). 기존 결함: Segment 등 PUT 성공 응답에 스펙의 객체 본문 누락, Manager 컬렉션 POST가 컬렉션 경로에 객체를 저장, 스펙에 있으나 미구현 action POST가 객체로 저장됨 → 수정(501 미지원 응답, 미지원 집계).

**사용자 확정이 필요한 사항:**

1. **Phase 4b 추가 API 20건의 실행 범위 포함(D2 확대)** — NSX-022 ~ 031, VC-SOAP-017 ~ 023, VC-LOOKUP-001, VC-PBM-001, VC-REST-007(API_COVERAGE_MATRIX §6). 모두 Integration Tested이며 아직 release 기동 조건에는 넣지 않았다.
2. **채택 범위(WP 4.11·4.9)** — 채택: FR-VC-0206·0208·0210·0402·0403·0407·0408·0410 ~ 0416, NFR-ARC-0104, FR-NSX-0206 ~ 0208·0212 ~ 0218·0221·0223 ~ 0225·0229·0232·0413. 미채택 제안: FR-VC-0204(구형 REST), FR-VC-0209(STS), FR-VC-0211(ESXi 직접 경로), FR-NSX-0222(Statistics, 선택) — `40-test/carryover.yaml`.
3. **추가 버전 조합 상호운용성** — vCenter 7.0.2 × NSX-T 3.2.4.3은 상호운용성 매트릭스(동적 페이지)로 확인하지 못해 `interop: unverified`(release 거부). 확인 후 verified로 바꾼다.
4. **D1 재검토 제안** — 커뮤니티 보고에 따르면 NSX-T 3.2.4는 vCenter 8.x와 공식 지원 조합이 아니다(매트릭스 'not supported'). D1 대상 범위(8.x 포함, NSX-T 3.x)에서 8.x 프로파일은 NSX와 짝지을 수 없으므로, 8.x 추가 시 NSX 4.x 범위 확장 또는 vCenter 단독 조합 규칙이 필요하다. 이번 단계는 8.x 대신 범위 하한인 7.0 U2를 추가했다.

**제한(BEHAVIOR_SPEC §11 ~ §13):** H-API 원자성·내장 Rule PATCH 의미·rule_id 번호·기본 Service 구성·검색 문법은 근거 미확보 가정이다. PBM은 VM 홈 정책만 연결하고 디스크 단위 정책은 다루지 않는다. 지난 데이터 버전으로 WaitForUpdatesEx를 다시 호출해도 그 버전 이후의 변경을 다시 계산하지 않는다. 요청 본문의 속성 버전은 검사하지 않는다(응답만 세션 버전으로 필터링).

## 13. Phase 5 결과 — 자동화·품질 강화 (2026-10-11)

**시작 결정:** Phase 4b 추가 API 20건을 실행 범위에 넣었다(D2 확대, PRD-4 §19·PRD-revision #67). 기준 조합 실행 범위 61개 API(vCenter 31·NSX 30), 추가 버전 vCenter 7.0.2는 SOAP 계열 26개 + NSX 30개.

**종료 게이트(PRD-4 §15.2: 적용 대상 AC-01 ~ AC-19 자체 시험 충족, AC-20 제외):**

| AC | 판정 | 증거 |
|---|---|---|
| AC-01 실제 제품 없이 실행 | 충족 | 단일 실행파일, Footprint(기동 1.39초, 113.8 MB), 교차 빌드 5개 대상 |
| AC-02 인증·버전·인벤토리 | 충족 | Scenario 01 |
| AC-03 승인 범위 API 계약 지원 | 충족 | 실행 범위 61개 API 모두 Integration Tested(`verification.yaml`, commit `src-60c304e3875e0dda`), Coverage Report |
| AC-04 정상 생명주기 | 충족 | Scenario 02~08, 선언형 시나리오 7종 회귀 |
| AC-05 오류·복구 | 충족 | Scenario 09·10, 장애 시나리오 3종(Runner) |
| AC-06·07 상태 일관성·비동기 결과 | 충족 | Stateful 시험, Scenario 08 |
| AC-08 인증·세션 | 충족 | 계약·Negative, 세션 만료 시나리오 |
| AC-09 ID·참조 관계 | 충족 | Snapshot 복원 후 MoRef·NSX ID·revision 유지와 불일치 검사(`TestSnapshotRestoreRoundTrip`, `TestRestartRecovery`) |
| AC-10 초기화·재현 | 충족 | `TestResetDeterminism`, 같은 Snapshot 반복 복원, 시나리오 종료 시 초기화 확인 |
| **AC-11 자동 회귀** | **조건부** | 로컬 전체 회귀(`make regression` 내용: 구현 21·40-test 87·7.0.2 84건 통과)와 CI 워크플로(`.github/workflows/ci.yml`) 작성. 저장소·러너가 없어 CI 실행 결과는 없음 — 사용자 조치 |
| AC-12 Coverage Report | 충족 | Admin `GET /v1/coverage-report`(JSON·Markdown), `TestCoverageReport` |
| AC-13 미지원·미검증 식별 | 충족 | 미지원 호출 집계·Coverage 상태(실행 범위 밖은 지원으로 세지 않음) |
| AC-14 검증 범위·제약 문서화 | 충족 | BEHAVIOR_SPEC §1~§14, RISK_REGISTER R-01~R-45, API_COVERAGE_MATRIX §7·§8, RESOURCE_BUDGET §5 |
| AC-15·16 프로파일 동작·버전 노출 | 충족 | 기동 거부 시험, release 기동(기준 조합 기동·7.0.2 거부), 버전 간 Differential Test |
| AC-17 SDK 계약 | 충족 | govmomi 계약, govc·pyVmomi |
| AC-18 예산·네이티브·지문 | 충족 | Footprint, 인증서 지문 유지 시험 |
| AC-19 추적 | 충족 | 추적표 개별 행 426: 시험 통과 364, 자체 검토 53, 이월 9(결정 근거 기록), 미실행 0 |

**WP별 구현:**

| WP | 내용 |
|---|---|
| 5.1 | 시나리오 엔진을 `internal/scenario`로 옮겨 Admin API Runner(`/v1/scenarios`, `/v1/scenario-runs/{id}`·`/stop`·`/log`·`/report`)와 40-test가 공유. 중지(취소) 후 정리 단계 수행, 실행 로그(NFR-0404), 재시도·전송 오류 기대·요청 Timeout. 시나리오 5종 추가(Security Policy 변경, Task 실패·복구, 세션 만료·가상 시간, NSX Realization·전송 장애, Snapshot) — 총 7종. 이벤트 조회 `/v1/events` |
| 5.2 | Snapshot/Restore(단일 락 vcsim 덤프 + SQLite 백업 + NSX 상태, SHA-256 manifest, 프로파일·Fixture 대조, 실패 시 초기화), 불일치 검사 `/v1/consistency`, 재처리 정책(interrupted), full 종료 시 `_shutdown` 저장·`--restore-snapshot` 기동 복원 |
| 5.3 | 전체 회귀: `make regression`(구현·40-test·7.0.2 프로파일·증거). Terraform·Ansible(선택)은 미채택 |
| 5.5 | API Coverage Report(JSON·Markdown) |
| 5.6 | Performance Benchmark(VM 100/1,000/5,000, Segment 1,000, 동시 10/50/100) — 결과와 목표는 RESOURCE_BUDGET §5 |
| 5.7 | CI 워크플로, `50-deployment/dist.sh`(release 패키지·SHA256SUMS, NSX 스펙 제외), Makefile `regression`·`perf`·`soak`·`cross`·`dist` |
| 5.8 | 문서: BEHAVIOR_SPEC §14, ARCHITECTURE §3·§9, API_COVERAGE_MATRIX §6~§8, RISK_REGISTER R-43~R-45, RESOURCE_BUDGET §5, 추적표 Phase 5 |
| 5.9 | full 보존: Trace 본문(64 KB 자름·자격 증명 마스킹) SQLite 저장, 건수·용량·기간(가상 시각 7일) 회전, 감사 7일·1 GB, 전이 7일. soak 수렴 확인 |
| 5.10 | macOS·Linux 설치 스크립트(hosts·루프백 별칭·CA 신뢰·443 권한, dry-run·uninstall), 임시 hosts·dry-run 시험 |

**시험이 찾은 결함·공백:** vcsim 저장·복원 공백(R-43: Windows 경로 적재 실패, VM 스냅샷 객체·데이터스토어 파일 미저장, 스냅샷 래퍼 누락), vcsim 이벤트 이력 상수(R-44). 측정 중 수정: full Trace 링버퍼 선할당(유휴 268 → 83 MB), full DB 요청마다 fsync, 관계 입력이 아닌 NSX 쓰기의 불필요한 관계 조정, 성능 시험의 세션 토큰 처리. 남은 규모 한계는 R-45(Segment 쓰기마다 전체 관계 조정, VM 수에 비례한 기동 시간).

**가정(BEHAVIOR_SPEC §14):** Snapshot·재처리 정책·복원 부수 효과(세션 무효, Fault Rule 비움)·이름 규칙·보존 한도 동작은 근거가 없는 설계 선택이다.

**사용자 조치·결정 필요:** ① 저장소(git)·CI 러너 구성과 실행(AC-11) ② 설치 스크립트 실제 실행(Windows·macOS·Linux, 관리자 권한) ③ Phase 4b 이월: 미채택 제안 4건, 7.0.2 × NSX-T 3.2.4.3 상호운용성, D1 재검토 ④ 규모 목표(RESOURCE_BUDGET §5) 승인과 R-45 개선(증분 관계 조정) 여부 ⑤ TR-COV-0306(Terraform·Ansible) 채택 여부. **(2026-10-11 결정: ④ 규모 목표 승인, R-45 ①~④ 개선 완료 ⑤ Ansible만 채택·Terraform 미채택. vcsim·Go 상위 프로젝트 결함 보고는 하지 않는다.)**

### 13.1 R-45 증분 관계 조정 (2026-10-11, 사용자 요청)

**문제:** NSX Segment 쓰기마다 교차 락 안에서 관계 조정이 모든 VM의 속성(장치 목록 포함)과 모든 포트그룹을 다시 조회했다(VM 1,000개 255 ms, Segment 1,000개 89 ms). 쓰기가 직렬화되므로 VM 5,000·동시 50 이상에서 60초 Timeout이 났다.

**구현(internal/relations, ARCHITECTURE §3):** vcsim 변경 핸들러가 바뀐 객체 MoRef를 표시하고(별도 뮤텍스만, R-15 유지), 관계 조정은 VM별 파생 결과(Fabric VM·VIF·Segment Port)와 포트그룹 조회 결과를 캐시해 표시된 객체·새 객체·없어진 객체·매핑이 바뀐 포트그룹을 쓰는 VM만 다시 조회한다. 기동·초기화·Snapshot 복원(모델 교체)·관리자 조정(`/v1/reconcile`, 복구 절차)·호스트 UUID 변경은 전체 계산. 조정 보고에 `derive_mode`(full·incremental)·`refreshed_vms`를 더했다.

**정확성 검증:** 검증 모드(`SIM_RELATIONS_VERIFY=1`)는 증분 결과를 매번 전체 조회와 VM·포트그룹 단위로 대조하고, 차이가 있으면 시험 환경 정리 단계에서 실패시킨다. 처음 대조에서 govc 시험 1건이 차이로 보고됐는데, 원인은 vcsim Task가 교차 락 밖에서 VM을 계속 바꾸는 동안 두 조회 사이에 NIC가 추가된 경우였다(이미 변경 표시가 남아 다음 조정이 반영 — 실제 누락 아님). 대조에서 변경 표시가 남은 객체를 제외하도록 고친 뒤 40-test 전체를 검증 모드로 두 번 실행해 차이 0. 전용 시험 `TestIncrementalReconcile`(VM 생성·복제·이름·전원·NIC 연결·이동·삭제, NSX VM Tag, Segment 생성·이름 변경·삭제, 호스트 Fault, 복원).

**결과(RESOURCE_BUDGET §5):** Segment 쓰기 1건 평균 VM 1,000개 334 → 25 ms, Segment 1,000개 129 → 22 ms. 처리량(동시 10) VM 1,000개 10 → 267 req/s, VM 5,000개 2 → 49 req/s, VM 5,000·동시 50·100의 Timeout 해소. 성능 회귀 기준: VM·Segment 1,000개 이하·동시 50 이하에서 HTTP 오류·Timeout 0, 전송 계층 오류 0.1% 이하(전송 계층 오류는 따로 세어 보고).

**시험:** 구현 단위 21건, 40-test 최상위 88건(벤치마크·soak는 별도 실행·통과), 7.0.2 프로파일 85건, 실패 0. Footprint 기동 1.33초·89.4 MB. 증거 재생성(commit `src-b165e1197dd4e0a2`, 61개 API Integration Tested), release 기동 확인. 추적표 개별 행 426: 시험 통과 364, 자체 검토 53, 이월 9, 미실행 0.

**남은 한계(R-45 ②~④):** 기동 시간이 VM 수에 비례(vcsim VM 생성), 고부하에서 드문 TLS alert `bad record MAC`(단독 재현 시험에서는 재현 안 됨, 원인 미확인), soak에서 메모리가 단계마다 5% 안팎 증가(15% 기준 이내, 24시간 운전 미실시). Segment 쓰기의 교차 락 직렬화는 일관성 요건(FR-BE-0304) 때문에 유지한다.

### 13.2 R-45 ③ TLS `bad record MAC` 원인 조사 (2026-10-11, 사용자 요청)

**재현:** `40-test/performance/tls_stress_test.go`(`PERF_RUN=1 TLS_STRESS=1`): 시뮬레이터를 별도 프로세스로 띄우고 매 요청 새 연결로 NSX GET을 50개 동시 보낸다. 60초에 핸드셰이크 약 12~16만 건 중 150~240건 실패(약 0.1~0.2%). 서버 로그에 `http: TLS handshake error ... local error: tls: bad record MAC` — 실패는 연결 뒤가 아니라 **핸드셰이크 중 서버가 클라이언트의 암호화된 핸드셰이크 레코드(Finished)를 복호화하지 못한 것**이다(Go crypto/tls `conn.go`의 AEAD Open 실패 경로). 클라이언트는 그 전에 서버의 암호화된 메시지를 정상 복호화했으므로 키 합의는 일치했다.

| 실험(60초) | 핸드셰이크 | 실패 | 판단 |
|---|---|---|---|
| 기본(TLS 1.3, Go 클라이언트) | 124,416 | 242 | 재현 |
| TLS 1.2로 제한 | 139,681 | 0 | **TLS 1.3 경로에만 있음** |
| 키 교환 X25519 / P-256 / X25519MLKEM768 | 135k ~ 144k | 149 ~ 241 | 키 교환 무관 |
| 시뮬레이터 응용 부하 없음 | 159,923 | 236 | 관계 조정·SQLite 등 응용 부하 무관 |
| 비동기 선점 끔(서버만 / 클라이언트만) | 159k / 164k | 170 / 152 | 선점 무관 |
| **OpenSSL 3.5 클라이언트(Python ssl)** | 271,980 | 서버 127 | **Go 클라이언트가 아니라 Go 서버 쪽** |
| **표준 라이브러리만 쓴 최소 Go TLS 서버(같은 도구 체인)** | 270,277 | 2 | **시뮬레이터 코드 밖 — Go 도구 체인/런타임/OS 수준** |
| 최소 서버 + GC 압력 | 215,007 | 3 | GC 무관 |
| 시뮬레이터 로그 수준 error / info | 273k / 261k | ≈ 90 / 65 | 로그 I/O 무관 |
| 시뮬레이터 GOMAXPROCS 1 / 4 / 20(기본) | 150k / 277k / 272k | 1 / 26 / 127 | **병렬성에 비례 — 동시성 결함의 특징** |

**결론(조사 당시, §13.3에서 수정):** 원인은 시뮬레이터 코드가 아니라 이 PC의 Go 1.27.2(windows/amd64) TLS 1.3 서버 핸드셰이크 경로(또는 그 아래 런타임·Windows 네트워크 계층)의 동시성 결함으로 판단한다. 시뮬레이터 프로세스에서 비율이 높은 것은 고루틴·스레드가 많아 병렬성이 크기 때문으로 보인다. 정확한 코드 위치는 확정하지 못했다: 경합 검출기(`-race`)는 C 컴파일러가 없어 쓸 수 없고, 다른 Go 버전 비교는 도구 체인 다운로드(승인 필요)가 필요하다. 조사 도구: `.agentic/poc/tls-r45/`(OpenSSL 폭주 클라이언트 `tls_storm.py`, 최소 서버 `minsrv/main.go`).

**제품 계약과의 관계(근거):** 시뮬레이션 대상 vCenter 7.0.x와 NSX-T 3.2.x는 TLS 1.3을 지원하지 않는다고 판단된다 — vSphere는 8.0 Update 3에서 TLS 프로파일로 TLS 1.3을 지원하기 시작했고([vSphere 8.0 보안 문서](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-security/GUID-82028A21-8AB5-4E2E-90B8-A01D1FAD77B1.html), [KB 377237](https://knowledge.broadcom.com/external/article/377237)), vSphere 7.0 TLS Configurator 문서는 TLS 1.0 ~ 1.2만 다룬다([KB 313841](https://knowledge.broadcom.com/external/article/313841)). NSX는 4.1.2부터 웹·API에 TLS 1.3을 기본 사용한다([KB 319144](https://knowledge.broadcom.com/external/article/319144)). 7.0·3.2에 대해 "지원 안 함"을 직접 적은 문서는 찾지 못했으므로 추론이다. 지금 Gateway는 TLS 1.2 ~ 1.3을 허용해 실제 제품보다 넓다.

**제안(사용자 결정 필요):** ① 프로파일에 TLS 최대 버전을 두고 vCenter 7.0.x·NSX-T 3.2.x는 TLS 1.2로 제한(실제 제품에 더 가깝고, 실험에서 실패 0) ② 다른 Go 버전으로 같은 시험을 돌려 도구 체인 결함 확정(다운로드 승인 필요) ③ Go 프로젝트에 결함 보고(외부 게시 — 사용자 판단).

### 13.3 R-45 ③ 결정 반영 — TLS 최대 버전 1.2, 다른 Go 버전 확인 (2026-10-11, 사용자 결정)

사용자가 §13.2 제안 ①·②를 승인하고 ③(Go 프로젝트 보고)은 하지 않기로 했다.

**② 다른 Go 버전·비 Go 대조 결과:**

| 실험 | 핸드셰이크 | 실패 | 판단 |
|---|---|---|---|
| 시뮬레이터 Go 1.27.2 빌드 / Go 1.26.9 빌드(같은 소스, Go 클라이언트 폭주 60초 2회) | 회당 약 10만 ~ 16만 | 1.27.2: 16·100, 1.26.9: 24·162 | Go 버전에 따른 차이 없음 |
| **OpenSSL 서버 + OpenSSL 클라이언트(Go 없음), TLS 1.3, 120초 2회** | 255,608 / 222,809 | **7 / 2** | **Go 밖에서도 재현** |
| 같은 OpenSSL 서버·클라이언트, TLS 1.2, 120초 | 241,373 | 0 | TLS 1.3에만 나타남 |

Go 1.26.9는 공식 배포 zip을 SHA-256 확인 후 도구 디렉터리에 풀어 썼다(사용자 승인). 앞선 OpenSSL 서버 대조(24,189건, 실패 0)는 클라이언트 포트 고갈(WinError 10048)로 규모가 작아 실패를 볼 수 없었다; 이번에는 클라이언트를 RST로 닫아 TIME_WAIT를 없앴다(`tls_storm.py`의 `STORM_RST=1`, `STORM_TLS12=1`). 설치된 보안 제품은 Windows Defender뿐이다.

**수정된 결론:** Go 서버·Go 클라이언트·OpenSSL 서버·OpenSSL 클라이언트의 모든 조합에서 TLS 1.3만 드물게 `bad record MAC`가 나고 TLS 1.2는 0건이므로, 원인은 특정 TLS 라이브러리나 Go 버전이 아니라 이 PC의 TLS 1.3 핸드셰이크 트래픽 패턴과 그 아래 계층(Windows 루프백 TCP 경로, 네트워크 필터, 하드웨어 중 하나)으로 판단한다. Go에서 비율이 더 높은 것은 병렬성 차이로 보인다. 정확한 계층은 확정하지 못했다(관리자 권한이 필요한 WFP 필터·패킷 캡처는 하지 않음). 따라서 Go 프로젝트 보고(③)는 근거가 약해 하지 않은 결정과도 맞다.

**① TLS 최대 버전 1.2 반영:** 버전 프로파일에 `tls.max_version`(+`basis`)을 추가하고(VERSION_PROFILE_SPEC §3), vCenter 7.0.2·7.0.3과 NSX-T 3.2.4.3을 `"1.2"`로 정했다. 프로파일 로더가 값(빈 값·1.2·1.3)을 검증하고, Gateway는 리스너(vcenter·nsx)마다 TLS 설정을 복제해 최대 버전을 적용한다(`gateway.MaxTLS`). 하한은 기존대로 1.2.

| 확인 | 결과 |
|---|---|
| 계약 시험 `TestListenerTLSMaxVersion` | vcenter·nsx 모두 기본 클라이언트는 TLS 1.2로 합의, TLS 1.3 전용·TLS 1.1 클라이언트는 거부 |
| 단위 시험 `TestTLSMaxVersionID` | 1.1·1.4 등 잘못된 값은 스키마 오류로 기동 거부 |
| 핸드셰이크 폭주(`TLS_STRESS=1`, 클라이언트 제한 없음, 60초) | 97,183건, **실패 0** (반영 전 같은 시험 0.1 ~ 0.2%) |

R-45 ③은 이 조치로 완화(시뮬레이터 대상 버전에서는 발생 경로 없음)로 바꾼다. TLS 1.3을 쓰는 미래 프로파일(vSphere 8.0 U3·NSX 4.1.2 이상)을 추가하면 같은 현상이 이 PC에서 다시 나타날 수 있다 — 그때 다시 본다.

### 13.4 R-45 ④ soak 메모리 증가 조사 (2026-10-11, 사용자 요청)

**방법:** 조사용 스크래치 빌드(제품 코드 변경 없이 `net/http/pprof`만 추가)로 soak(10단계)를 돌리며 15초마다 강제 GC 뒤 MemStats·고루틴 수·힙 프로필을 모았다. 프로세스 메모리(작업 집합)만으로는 Go 힙 반환 지연과 실제 누수를 구분할 수 없어서다.

**발견(수정 전, 10단계):** 프로세스 메모리 122.6 → 182.2 MB와 함께 GC 뒤 살아 있는 힙 24.6 → 44.0 MB, 힙 객체 12만 → 32만, **고루틴 109 → 478**로 실제로 늘었다. 원인은 둘이다.

| 원인 | 근거 | 조치 |
|---|---|---|
| ① 유휴 keep-alive 연결이 닫히지 않음 | 늘어난 고루틴 361개가 모두 `net/http.(*conn).serve`의 다음 요청 대기. 부하 도구가 단계마다 새 HTTP 클라이언트를 만들고 이전 연결을 닫지 않았고, Gateway에 `IdleTimeout`이 없어 서버도 닫지 않았다(연결마다 TLS·bufio 버퍼). 실제 클라이언트가 연결을 버리고 떠나도 같은 일이 생긴다 | Gateway `IdleTimeout` 2분(가정 — 실제 제품 값 미확인), 부하 도구는 단계 끝에 `CloseIdleConnections` |
| ② vcsim 세션 객체 락 누수(govmomi v0.56.0) | 힙 증가의 약 30%가 `simulator.(*Registry).locker`/`NewObjectLock`. 세션 범위 객체(ContainerView·PropertyFilter·PropertyCollector)는 세션 레지스트리에 있는데 메서드 호출 락은 전역 레지스트리 `locks` 표에 만들어지고, `DestroyView` 등은 세션 레지스트리에서만 지워 전역 항목이 영영 남는다(vcsim `Context.WithLock` 주석의 TODO). 관계 조정이 NSX 쓰기마다 내부 클라이언트로 뷰를 만들고 지워 쓰기 수에 비례해 늘었다. govc·CMP처럼 뷰를 만들고 지우는 외부 클라이언트도 같다 | 어댑터 훅이 세션 객체 파괴 요청(`DestroyView`·`DestroyPropertyFilter`·`DestroyPropertyCollector`)을 기록하고, 객체가 실제로 사라진 뒤 별도 고루틴에서 전역 `Remove`로 락 항목만 지운다(`vcenter/viewlocks.go`). 전역 객체는 건드리지 않는다 |

**수정 후:**

| 항목(10단계, pprof 빌드) | 수정 전 | 수정 후 |
|---|---|---|
| 프로세스 메모리 | 122.6 → 182.2 MB | 110.5 → 120.4 MB(3단계 이후 평탄) |
| GC 뒤 Go 힙 | 24.6 → 44.0 MB | 20.5 → 21.8 MB |
| 힙 객체 | 120,815 → 324,926 | 75,974 → 89,179 |
| 고루틴 | 109 → 478 | 105 → 91 |

정식 `make soak`(6단계, 누적 52,032 요청): 메모리 139.8 → 138.7 MB, 단계 간 변화 ±2% 안(수정 전 141 → 184 MB, 단계마다 약 5% 증가). DB 8.9 → 10.7 MB(보존 한도 안에서 WAL 증가, 수정 전과 같은 추세).

**시험:** `TestSessionObjectLocksReleased`(뷰·수집기 생성/파괴와 관계 조정 60회 반복 뒤 vcsim 락 항목 증가 ≤ 5), `TestGatewayIdleTimeout`(유휴 연결은 제한 시간 뒤 서버가 닫고 그 전에는 keep-alive 유지). Admin `GET /v1/status`의 `retention_usage.runtime`에 고루틴 수·Go 힙·vcsim 객체/락 수를 추가해 장시간 운전 중 같은 현상을 바로 볼 수 있게 했다.

**남은 한계:** ⓐ 클라이언트가 세션 객체를 파괴하지 않고 로그아웃·세션 만료로 떠나면 그 객체들의 전역 락 항목은 남는다(세션당 수십 바이트 × 객체 수, vcsim 구조상 세션 종료 훅 없음). ⓑ 완료 Task는 `TaskRetention`(200)까지 보관하므로 그만큼의 락은 정상적으로 유지된다. ⓒ 24시간 실시간 운전은 하지 않았다(요청 수로 가속한 soak만). ⓓ vcsim 결함은 상위 프로젝트에 보고하지 않는다(2026-10-11 사용자 결정) — 시뮬레이터 안 보완으로 유지.

### 13.5 R-45 ② 기동 시간 개선 — Fixture 캐시·병렬 파일 입출력 (2026-10-11, 사용자 요청)

**원인(CPU 프로파일, VM 2,000개 기동):** 시간 대부분이 vcsim의 VM 생성에서 VM마다 데이터스토어 파일 5개(.vmx·.nvram·.log·.vmdk 2개, 크기 0 ~ 202바이트)를 만드는 파일 시스템 호출(생성·닫기·상태 조회)이다. CPU 사용률은 약 21%로, 계산이 아니라 파일마다의 지연(Windows 파일 시스템·실시간 보안 검사)이 병목이다. vcsim은 VM을 하나씩 순서대로 만들고, VM의 MoRef 번호가 생성 순서에 달려 있어 생성 자체를 병렬로 하면 결정론(FR-DM-0207)이 깨진다.

**조치:**

| 조치 | 내용 |
|---|---|
| Fixture 캐시(`vcenter/fixturecache.go`) | 큰 Fixture(VM 200개 이상, `--fixture-cache auto` 기본)는 처음 만든 인벤토리를 Snapshot 형식으로 `<data-dir>/fixture-cache/<키>`에 저장하고, 다음 기동·Admin Reset에서 vcsim `Model.Load`로 적재한다. 키 = Fixture 전체·vCenter 프로파일 해시·실행파일 해시(구현·vcsim 버전이 바뀌면 자동 무효). 최근 2개만 남긴다. `on`(항상)·`off`(끔) 선택 가능, 사용 결과는 `GET /v1/status`의 `retention_usage.fixture_cache`(hit·miss·off, 소요 시간) |
| MoRef 결정론 보존 | `Model.Load`는 일련번호를 객체 값 숫자의 최댓값(vcsim 기본 알람의 고정 ID `alarm-384` 등)에 맞춰 캐시 없이 만든 경우와 달라진다 → 저장 당시 일련번호를 함께 저장해 적재 후 정확히 되돌린다(vcsim 비공개 필드라 reflect 사용, govmomi v0.56.0) |
| 병렬 파일 입출력(`vcenter/parallel.go`) | Snapshot 저장(XML 쓰기, 홈 경로 정리를 쓰기 전 메모리에서 — 다시 읽어 고쳐 쓰던 단계 제거), 데이터스토어 복사, 무결성 해시, 적재 전 미리 읽기를 작업자 16개로 처리. 결정론과 무관한 작업만 병렬 |
| 이전 모델 정리 | Reset·복원 뒤 이전 모델의 파일 삭제를 백그라운드로(종료 때 기다림) |

**결과(VM 수만 바꾼 Fixture, 프로세스 시작 → Admin API 응답):**

| VM | 캐시 끔(기존) | 첫 기동(miss: 생성 + 저장) | 다음 기동(hit) |
|---|---|---|---|
| 1,000 | 44.1초 | 62.8초 | 16.9초 |
| 2,000 | 약 85초 | 120.3초 | 16.7 ~ 37.0초 |
| 5,000 | 210초(§5 표) | 261.8초 | 43.4 ~ 59.8초 |

hit 시간은 직전 실행이 만든 파일을 보안 검사가 아직 처리 중이면 늘어난다(같은 조건 반복 측정 범위). Admin Reset(VM 2,000개)도 캐시로 15.5초(이전에는 다시 생성, 약 85초). Snapshot 복원(VM 2,000개)은 병렬 입출력으로 22.0 → 8.7초.

**시험:** `TestFixtureCache` — 캐시 없이 만든 상태와 캐시 저장·적재 상태의 VM(MoRef·이름·BIOS/인스턴스 UUID·데이터스토어)이 같고, 적재 뒤 새로 만든 VM의 MoRef가 캐시 없이 만든 경우와 같으며(결정론), 불일치 검사 0건, 관계 조정 검증 모드 차이 0. `TestFixtureCacheAutoAndValidation` — auto는 작은 Fixture에서 캐시를 쓰지 않고, 잘못된 값은 기동 거부. 벤치마크 도구는 비교를 위해 기본 `--fixture-cache off`(`PERF_FIXTURE_CACHE`로 변경).

**발견(기존 동작):** 캐시 없이 만들 때 vcsim은 클러스터 VM의 호스트를 무작위로 고른다(`createVM`, 시드 없는 `rand.Intn`). 그래서 같은 Fixture라도 기동마다 VM의 `runtime.host` 배치가 달랐다 → §13.6에서 결정론적 배치로 고쳤다.

**남은 한계:** ⓐ 첫 기동은 저장 시간만큼 느리다(VM 1,000개 +19초, 5,000개 +52초). ⓑ hit에서도 VM당 파일 5개를 다시 만들어야 해 파일 수에 비례한다(하드 링크는 vcsim이 .log에 덧붙여 쓰고 파일을 제자리에서 다시 만들어 캐시를 바꾸므로 쓰지 않았다). ⓒ vcsim 이벤트 기록(Fixture 생성 이벤트)은 캐시에 없어 적재하면 비어 있다. ⓓ 캐시는 데이터 디렉터리에 Fixture마다 VM 2,000개 기준 약 74 MB(최근 2개 유지). ⓔ Snapshot 저장(VM 2,000개 약 32초)은 객체·파일 1만 4천 개를 쓰는 vcsim 저장 형식 때문에 여전히 파일 수에 비례한다. ⓕ 실시간 보안 검사 제외 등 시스템 보안 설정은 사용자 판단 사항이라 바꾸지 않았다.

### 13.6 결정론적 VM 호스트 배치 (2026-10-11, 사용자 요청)

**문제:** §13.5에서 발견. vcsim `Model.Create`는 클러스터 리소스 풀의 VM을 호스트 지정 없이 만들고 `createVM`이 클러스터 호스트를 시드 없는 `rand.Intn`으로 고른다. 같은 Fixture·Seed라도 기동·Reset마다 `runtime.host`, `host.vm`, 생성 이벤트의 호스트가 달라 결정론(FR-DM-0207, TR-SCN-0201)에 어긋났다.

**검토한 방법:** ① 전역 난수 시드 고정 — Go 1.24부터 `rand.Seed`가 기본적으로 무시되고(GODEBUG `randseednop`), 다른 코드의 난수 사용 순서에 따라 결과가 바뀌며 Reset 때 이어진 수열을 써서 기동과 달라진다 → 쓰지 않음. ② 만든 뒤 호스트 옮기기 — 이미 기록된 생성 이벤트의 호스트와 실제 배치가 어긋남 → 쓰지 않음. ③ **채택:** 모델은 VM 없이(Machine=0) 만들고, 클러스터 VM은 vcsim `addMachine`과 같은 요청(이름·장치·디스크 경로·NIC 백킹·자동 전원 켜기)과 같은 순서로 직접 만들되 호스트를 명시한다(`vcenter/placement.go`).

**배치 규칙:** 풀 안 i번째 VM(`…_RP0_VMi`) → 클러스터 호스트 목록(추가 순서)의 i mod n번째(라운드 로빈). 기본 Fixture(VM 20개, 호스트 3개)는 7·7·6개.

**확인:** 변경 전후 VM의 MoRef·이름·UUID와 이후 새 VM의 MoRef가 같다(요청·순서가 같아 번호 부여가 그대로). `TestDeterministicVMPlacement` — 두 번 새로 만든 인벤토리와 Reset 뒤 인벤토리의 배치가 같고, 라운드 로빈 규칙을 따르며, 모든 호스트에 나뉘고, `VmCreatedEvent`의 호스트가 실제 배치와 같다. `TestFixtureCache`는 이제 호스트까지 포함해 캐시 없이 만든 상태와 같음을 확인한다(이전의 호스트 제외 비교 삭제).

**제한:** 실제 vCenter의 DRS 배치를 흉내 내지 않는다(고정 규칙). 이로써 Fixture가 쓸 수 있는 vcsim 구성(데이터센터·폴더·클러스터·독립 호스트·하위 풀·vApp·스토리지 Pod·포트그룹·데이터스토어)을 모두 다룬다.

**확장(2026-10-11, 사용자 요청): 데이터센터 상위 폴더.** Fixture `vcenter.datacenter_folders`(0 ~ datacenters, vcsim `Model.Folder`): 마지막 N개 데이터센터를 루트 아래 폴더 `F<i>`에 두고 그 자원은 각 기본 폴더(host·network·vm·datastore)의 하위 폴더 `F<i>`에 둔다. placement.go는 vcsim과 같은 조건·경로로 데이터센터·클러스터·독립 호스트·DVPG·VM 폴더를 찾는다. 시험: `TestPlacementMatchesVcsim`에 `Folder=1`(DC1이 폴더 안)을 넣어 vcsim과 MoRef·부모 폴더 등 동일, DC1 VM이 하위 폴더 `F0`에 있음 확인. `TestFixtureTopology`에 `datacenter_folders: 1`을 넣어 데이터센터·VM 폴더 위치, 불일치 검사 0, 관계 조정 검증, SOAP·REST, Reset·캐시 확인. `TestTopologyFields`에 범위 검사(음수·datacenters 초과 거부).

**확장(2026-10-11, 사용자 요청): 스토리지 Pod.** Fixture `vcenter.storage_pods`(0 이상, vcsim `Model.Pod`): 데이터센터마다 빈 스토리지 Pod(Datastore Cluster) `DCn_PODk`를 데이터센터의 기본 datastore 폴더에 만들고 Storage DRS를 켠다(vcsim은 하위 폴더 `F<i>`를 만들기 전에 Pod를 만들므로 데이터센터 폴더를 써도 Pod는 기본 폴더에 있다). 데이터스토어는 넣지 않는다(vcsim과 같음). VM 배치와 무관한 구조물이라 placement.go는 바뀌지 않는다. 시험: `TestPlacementMatchesVcsim`에 `Pod=2`(Pod 4개, VM·MoRef 일련번호 동일 유지), `TestFixtureTopology`에 `storage_pods: 2` — Pod 위치·Storage DRS·빈 상태, 클라이언트가 데이터스토어를 Pod로 옮기기(MoveIntoFolder) 성공 뒤 불일치 검사 0, Reset·Fixture 캐시 적재 뒤 빈 Pod로 복원. `TestTopologyFields`에 음수 거부.

**발견(vcsim 기존 동작, 미해결):** 데이터센터가 2개 이상이면 vcsim이 로컬 데이터스토어를 데이터센터 맵 순회(Go 맵 순서 무작위)로 만들어, 어느 데이터센터의 `LocalDS_k`가 먼저 MoRef 번호를 받는지가 실행마다 다르다(VM·네트워크 MoRef와 VM 배치는 영향 없음). 기본 Fixture(데이터센터 1개)는 해당 없다. `TestPlacementMatchesVcsim`은 데이터스토어를 MoRef 대신 데이터센터 포함 이름으로 비교한다. 결정론이 필요하면 데이터스토어 생성도 직접 수행하는 방식으로 고쳐야 한다.

**확장(2026-10-11, 사용자 요청): 독립 호스트·하위 풀·vApp.** Fixture `vcenter`에 선택 키 `standalone_hosts`·`child_pools`·`vapps`(기본 0, 음수 거부)를 추가해 vcsim `Model.Host`·`Pool`·`App`로 넘기고, placement.go가 vcsim과 같은 순서(데이터센터마다 독립 호스트 → 클러스터마다 루트 풀 또는 하위 풀 → vApp)로 VM을 만든다. 독립 호스트 VM은 그 호스트(원래도 결정론적), 클러스터 VM은 클러스터 안 생성 순서 라운드 로빈(하위 풀마다 0번 호스트부터 다시 시작하지 않도록). 기존 Fixture(셋 다 0)의 배치는 그대로다. Fixture 캐시 임계값의 VM 수 계산도 이 구성을 반영한다.

시험: `TestPlacementMatchesVcsim`(단위) — 데이터센터 2·독립 호스트 2·클러스터 2×호스트 3·하위 풀 2·vApp 1·VM 3(48개)을 vcsim이 직접 만든 것과 비교해 MoRef·이름·풀·vApp·폴더·데이터스토어·네트워크·MoRef 일련번호가 같고, 독립 호스트 VM의 호스트도 같으며, 클러스터 VM은 라운드 로빈, 다시 만들어도 같은 배치. `TestFixtureTopology`(계약) — 이 구성의 Fixture로 기동해 VM 12개, 불일치 검사 0건, 관계 조정 검증 모드 차이 0, SOAP·REST VM 수 일치, Reset·Fixture 캐시 적재 뒤 같은 인벤토리. `TestTopologyFields`(Fixture 키 매핑·음수 거부).

발견·수정: 불일치 검사(`GET /v1/consistency`)가 VM의 리소스 풀이 vApp이면 "resourcePool missing"으로 잘못 보고했다 → vApp도 리소스 풀로 인정하고 vApp의 VM 목록과 대조한다.

### 13.7 사용자 결정 반영과 남은 기술 과제 (2026-10-11, 사용자 요청)

**결정(사용자):** ① TR-COV-0306은 Ansible 자체 테스트만 채택, Terraform 미채택 ② 규모 목표(RESOURCE_BUDGET §5: VM·Segment 1,000 이하·동시 50 이하 오류 0) 승인 ③ vcsim·Go 상위 프로젝트에 결함 보고하지 않음 — 시뮬레이터 안 보완으로 유지. ④ 24시간 실운전은 하지 않는다.

**Ansible 자체 테스트(TR-COV-0306):** `40-test/ansible` — 플레이북 `simulator.yml`(community.vmware: about·데이터센터·클러스터·VM 목록, VM 생성·멱등성 재실행·전원 끄기·재구성·삭제, 인증 실패 / ansible.builtin.uri: NSX 버전·Segment PATCH·조회·삭제·404·인증 실패 403), 컬렉션·Python 의존성 고정 범위(`requirements.yml`, `requirements.txt`), 시험 도구 `TestAnsiblePlaybook`(시뮬레이터를 띄워 환경변수로 주소·자격 증명을 넘기고 PLAY RECAP failed=0 판정, hosts에 이름이 있으면 인증서 검증·없으면 IP 접속), 구조 검사 `TestPlaybookStructure`(Ansible 없이: YAML·작업마다 모듈 하나·허용 컬렉션·자격 증명 환경변수만). CI integration 작업(Linux·macOS 자체 호스트 러너)에서 venv에 설치해 `ANSIBLE_REQUIRED=1`로 실행. **실행(2026-10-11, WSL Ubuntu 24.04 — 사용자가 WSL·배포판 설치):** Linux용 시험 실행파일을 Windows에서 미리 빌드(`GOOS=linux go test -c`, WSL에 Go 불필요)하고 `40-test/ansible/run-wsl.sh`가 venv(ansible-core 2.19.14, pyvmomi 9.1.1, requests)·컬렉션(community.vmware 5.10.0, 의존 vmware.vmware 2.11.0)을 준비해 실행한다. 결과 **PLAY RECAP ok=30 changed=4 failed=0**(약 10초, 시뮬레이터도 WSL 안에서 실행). vcenter·nsx 이름이 WSL hosts에 없어 IP 접속·인증서 검증 끔으로 실행했다(이름 접속·검증 경로는 hosts 등록 시 같은 시험이 자동으로 쓴다 — 미실행). 결과 기록 `40-test/results-ansible.json`.

실행에서 발견·수정:

| 발견 | 조치 | 시험 |
|---|---|---|
| 클러스터 configurationEx.vsanConfigInfo 비어 있음(vcsim) → vmware_cluster_info 실패(`'NoneType' object has no attribute 'enabled'`) | vSAN 미사용 클러스터도 enabled=false로 채움(Fixture 생성·CreateClusterEx 등록 시점) | `TestClusterVsanConfigInfo` |
| ClusterComputeResource.GetResourceUsage 미구현(vcsim, MethodNotFound) → vmware_cluster_info 실패 | Adapter가 결과를 만들어 정상 응답으로 돌려준다(vcsim은 로그인 세션에서 Handler 대체 객체를 무시하므로 표지 Fault → 응답 바꾸기, clusterusage.go) | `TestClusterGetResourceUsage` |
| 플레이북 `register: off` — YAML 1.1에서 불리언 false로 읽혀 플레이북이 로드되지 않음 | 변수 이름 `powered_off` | 실행 |
| community.vmware 모듈이 `requests`를 요구(requirements.txt에 없었음) | requirements.txt에 `requests>=2.31,<3` | 실행 |
| run-wsl.sh: `ansible-galaxy collection list`가 컬렉션이 없어도 0으로 끝나 설치를 건너뜀 | 출력에서 이름 확인, 요구사항은 매 실행 동기화 | 실행 |

참고: community.vmware 5.10은 vmware_cluster_info·vmware_guest_powerstate를 폐기 예정(7.0.0에서 제거, vmware.vmware로 이전)으로 경고한다. 고정 범위(<6.0.0)에서는 동작한다.

**기술 과제:**

| 과제 | 조치 | 시험 |
|---|---|---|
| 데이터스토어 MoRef 결정론(§13.6 발견) | 모델을 Datastore=0으로 만들고 로컬 데이터스토어를 데이터센터 순서·호스트 추가 순서로 같은 요청(CreateLocalDatastore, `<vcsim.home>/DCn-LocalDS_k`)으로 만든다(placement.go). 단일 데이터센터의 MoRef·이름·UUID·다음 MoRef는 변경 전과 같음 확인 | `TestPlacementMatchesVcsim`(두 번 만들어 데이터스토어 MoRef 동일, 4회 반복) |
| vCenter 이벤트 보관(R-44) | vcsim 이벤트를 저장소 `vcenter_events`로 복제(1초마다, 이벤트 키가 바뀔 때만, 교차 락 안): lite 1,000건·full 100,000건·7일. Admin `/v1/events`는 보관본을 읽는다(`vcenter_events_dropped` 표시). Reset은 보관본을 비우고 복원은 복원한 저장소의 보관본을 그대로 쓴다. **발견·수정:** 이전 `/v1/events`는 QueryEvents에 MaxCount를 주지 않아 최근 10건만 돌려주었고, QueryEvents가 최근 것을 먼저 주는데 반대로 가정해 순서가 뒤집혀 있었다 | `TestVCenterEventArchive`(full 1,600건 보관·링버퍼 1,000건·Reset·lite 한도) |
| 적재 뒤 이벤트 기록 | Snapshot·Fixture 캐시에 이벤트 기록(events.xml)을 저장하고 적재 때 같은 순서·같은 키로 다시 게시(vcsim EventManager 키는 reflect로 맞춤) | `TestEventsSurviveRestore`, `TestFixtureCache`(생성 이벤트 키 동일) |
| 세션 종료 락 회수 | Logout·TerminateSession·유휴 만료 때 그 세션 객체의 전역 락 항목을 지운다. **발견·수정(vcsim):** TerminateSession은 Logout과 달리 세션 뷰를 전역 변경 처리기에서 빼지 않아 끝난 세션의 뷰가 이후 모든 변경 통지를 받았다 → 세션 레지스트리를 기억해 두고 Logout과 같은 정리를 한다 | `TestSessionEndLocksReleased`(세션 20개, Logout·Terminate) |
| Snapshot 저장 속도 | 데이터스토어 파일(VM마다 5개)을 데이터스토어마다 `.tar` 하나로 묶는다(병렬 읽기·정렬 순서 쓰기, 병렬 풀기, 이전 디렉터리 형식도 읽음) | `TestTarTreeRoundTrip` |
| 적재 모델의 재저장(발견·수정) | vcsim Load가 `vcsim.home` 설정을 하나 더 덧붙여, 적재한 모델(캐시·복원)로 만든 Snapshot은 홈 경로 정리가 틀려 복원이 실패했다(사용 중인 홈 디렉터리를 다시 만들려 함) → 적재 뒤 마지막 하나만 남긴다 | `TestRestoreChainFromLoadedModel`(캐시 적재 → Snapshot → 복원 3회 연쇄) |
| Task info.entityName(R-32) | vcsim이 MoRef 값을 넣던 것을 엔터티 이름으로 보정 | `TestTaskInfoEntityName` |
| 디스크 단위 스토리지 정책(R-37) | 생성·재구성 deviceChange[].profile, 복제·이전 disk[].profile을 디스크(virtualDiskId "VM:디스크 키")와 연결. 추가 디스크는 Task 성공 뒤 새로 생긴 디스크에 추가 순서대로 대응(재구성은 요청 직전 디스크 목록 기록). 디스크 제거·VM 삭제 뒤 연결 정리(이전에는 VM을 지워도 연결이 남았다) | `TestPBMDiskPolicy`(생성·추가·비우기·제거·삭제) |

**측정(VM 2,000개, Windows, 프로세스 시작 → Admin 응답):** 첫 기동(캐시 저장 포함) 120.3 → 91.7초, 캐시 적재 기동 20.7초, Snapshot 저장 31.7 → 12.2 ~ 12.6초(파일 약 14,300 → 4,332개), 복원 8.7 → 7.3 ~ 7.8초(이전 형식은 적재 모델에서 실패), Reset(캐시) 15.5 → 7.1초.

**남은 한계:** Ansible은 IP 접속·인증서 검증 끔으로만 실행했다(이름 접속 경로 미실행, CI 미실행). SOAP 클라이언트의 QueryEvents는 여전히 vcsim 최근 1,000건만 본다(실제 vCenter도 페이지당 1,000건). 두 보관 사이(1초)에 1,000건 넘게 생기면 일부를 놓친다(dropped로 표시). 7.0 기본 스토리지 정책 목록 차이는 미검증. 24시간 실운전은 하지 않는다(사용자 결정).

## 6. Phase 0 요약 (Task 7)

**최종 권장 아키텍처:** 권고안 A(단일 Go 실행파일) 유지. vcsim은 포크 없이 임베드하고, 행위 로직은 `Registry.Handler`와 래퍼 객체, 버전 필터는 Gateway, NSX는 스펙 기반 지연 핸들러로 구현한다. PRD-B §B2 재검토 조건에 해당하지 않는다.

**가장 위험한 기술 문제 5개와 Phase 0 결과:**

| 순위 | 위험 | 결과 |
|---|---|---|
| 1 | R-11 버전 인식 직렬화 | vcsim 필터 부재 확인 → Gateway 필터 + pyVmomi 메타데이터로 PoC 통과. 하위 호환 타입 대체는 미해결 |
| 2 | R-17 REST `/api` 부재 | 자체 구현 필요(PoC-02·03으로 방식 확인) |
| 3 | R-15 교차 락 | 변경 핸들러에서 락을 잡으면 교착 → Outbox 규칙으로 해소 |
| 4 | R-12 시간 소스 | 교체 지점 없음 → MVP 실시간, Phase 4a 결정 |
| 5 | R-18·R-20 계약 근거 공백 | NSX 3.2.4.x 스펙 미공개(3.2.1 대체), vSphere Automation 스펙 미확보 |

**직접 개발할 핵심 모듈:** Gateway(SNI·교차 락·호출 기록), 버전 프로파일·응답 필터, vCenter REST 뷰·세션 어댑터, Behavior Adapter, NSX 범용 핸들러와 행위 리소스·Realization, relations(Outbox), 스키마 대조 검사기.  
**재사용:** vcsim SOAP 엔진(MVP 메서드 전부), PropertyCollector, Task, vcsim vAPI(구형 세션·Tagging·Content Library), vcsim FaultInjector(래핑), govmomi 클라이언트(테스트).

**MVP 개발 순서:** `MVP_SCOPE.md` §4.

**핵심 목표 달성 가능성:**
- **A(버전 호환):** 가능성 높음. 7.0.3 기준 필터와 계약 근거 체계를 확인했다. 남은 위험은 하위 호환 타입 대체, REST 스펙 미확보, NSX 3.2.1/3.2.4.3 차이다.
- **B(경량 실행):** 달성. 유휴 71 MB, 기동 1.2초, 설치 34 MB(예산의 약 1/4~1/6), 관리자 권한은 hosts·CA에만 필요하다.
