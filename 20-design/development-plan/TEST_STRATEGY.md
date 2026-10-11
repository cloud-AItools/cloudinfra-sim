# TEST_STRATEGY — 검증 전략과 요구사항 추적표

> 관련: PRD-2 §12(TR-COV-0101 ~ TR-COV-0409), PRD-4 §15.5·§17·WP 0.15·WP 0.17, D2·D5·D13 · 작성일 2026-10-10 (Phase 0b)  
> 검증 상한은 `Integration Tested`이다. 자체 시험만으로 CMP Verified, Reference VMware Verified, '100% 호환'을 표시하지 않는다(TR-COV-0409).

## 1. 원칙

1. **기대값은 독립 근거에서 정한다(TR-COV-0403).** 공식 WSDL·타입 메타데이터(pyVmomi), NSX OpenAPI, 제품 문서, 공개 SDK 동작을 근거로 한다. 시뮬레이터의 현재 응답을 정답으로 복사하지 않는다.
2. **근거가 없는 동작은 '가정·미검증'으로 기록한다(FR-API-0308).** 각 시험에는 근거 ID(§5)를 붙인다.
3. **계획과 증거를 구분한다.** 추적표의 '결과'는 실제 실행 기록이 생기기 전까지 '미실행'이다.
4. **증거는 프로파일·구현 식별자에 묶는다(WP 1.13).** 프로파일·계약·구현이 바뀌면 영향받는 증거를 무효화한다.

## 2. 자체 테스트 도구 (D2 결정)

| 대상 | 도구 | 보완 |
|---|---|---|
| vCenter SOAP | **govmomi v0.56.0** (Go) | govmomi는 vcsim과 같은 계열이라 관대할 수 있다(TR-COV-0305). 응답 XML을 pyVmomi 메타데이터로 검사하는 **스키마 대조 검사기**(타입·필드 존재·세션 버전 필터·네임스페이스)를 함께 실행한다 |
| vCenter REST | Go `net/http` + govmomi `vapi/rest`(가능 범위) | vSphere Automation 7.0.3 스펙 확보 후 스키마 검증(미확보, §6) |
| NSX Manager/Policy | Go `net/http` | NSX-T 3.2.1 OpenAPI로 요청·응답 스키마, 상태 코드, 오류 계약 검증 |
| 시나리오 | Go 테스트 코드(`40-test/simulator-e2e`) | MVP는 테스트 코드, Phase 5에 Scenario Runner로 통합(TR-SCN-0303) |
| Footprint | `40-test/footprint` | `RESOURCE_BUDGET.md` §2 측정 경계 |

선택 도구(pyVmomi 시나리오, Terraform·Ansible)는 MVP 범위가 아니다(NFR-ARC-0113, TR-COV-0306).

## 3. 시험 계층과 폴더

| 계층 | 폴더 | 주 대상 | 단계 |
|---|---|---|---|
| Unit | `30-implementation/internal/**/_test.go` | 직렬화 필터, 경로 색인, 상태 전이, 락 경계 | Phase 1~ |
| Contract | `40-test/contract` | SOAP·REST·NSX 요청/응답·오류·버전 필터 | Phase 1~ |
| Stateful | `40-test/integration` | Create→Get, Update→Get, Delete→Get, 비동기 완료, API 간 일관성 | Phase 2a~ |
| Negative | `40-test/contract` | MVP: 잘못된 인증·입력·ID, 중복, Revision, 미지원, SOAP Fault(TR-COV-0303) | Phase 2a~ |
| SDK | `40-test/contract/sdk` | govmomi 계약 테스트(TR-COV-0304) | Phase 2b |
| E2E | `40-test/simulator-e2e` | Scenario 01~10 성공 기준별 | Phase 1~5 |
| Footprint | `40-test/footprint` | NFR-0107 ~ NFR-0113 | Phase 1 이후 매 빌드 |
| 보안·설치 | `40-test/integration/security`, `50-deployment/install` | NFR-03xx, NFR-ARC-0308 | Phase 1 |

**시험 임시 파일 격리(2026-10-11):** 시뮬레이터를 띄우는 `40-test` 패키지(contract, deployment, footprint, performance, sdk, simulator-e2e)는 `TestMain`에서 `testenv.Main`을 부른다. 시험 실행파일마다 `TMP`·`TEMP`·`TMPDIR`를 `<OS 임시>/cloudinfra-sim-test/<pid>`로 바꿔 vcsim 데이터스토어 디렉터리(`govcsim-*`), `t.TempDir`, 벤치마크가 띄운 시뮬레이터 자식 프로세스의 임시 파일을 모두 그 안에 만들고, 끝날 때 지운다. Timeout 등으로 강제 종료돼 남은 디렉터리는 다음 시험 실행이 지운다(해당 pid의 프로세스가 실행 중이 아닐 때만). 시험 밖에서 실행 중인 시뮬레이터의 `govcsim-*`는 건드리지 않는다. 격리·정리는 제품 실행파일과 같은 `internal/tempdir`를 쓴다(제품은 `<data-dir>/tmp/<pid>`). 시험: `tempdir.TestSweep`·`TestIsolate`, `TestVcsimTempIsolated`, `TestForcedKillTempCleanup`(실행파일 강제 종료 → 재기동이 정리).

## 4. 단계별 판정 요약

| 단계 | 판정에 필요한 시험 | 연결 AC |
|---|---|---|
| Phase 1 | Scenario 01, Footprint, release 모드 기동·거부, 설치·인증서 지문 | AC-01, AC-02, AC-15, AC-18 |
| Phase 2a | Scenario 02~06(development 모드), MVP Negative | AC-04(누적) |
| Phase 2b | Scenario 01~06 회귀, govmomi 계약, 협상 버전 하향 세션(7.0.3 프로파일 + `urn:vim25/6.7.3`), MVP release 기동, 추적표 증거 | AC-16(MVP), AC-17, AC-19(MVP) |
| Phase 3 | Scenario 08, 전송 계층 Fault | AC-06, AC-07 |
| Phase 4a | Scenario 09·10, 확장 Negative | AC-05, AC-08 |
| Phase 4b | Scenario 07, 버전 간 Differential | AC-04, AC-16(확장) |
| Phase 5 | 전체 회귀·Coverage Report·추적표 최종 대조 | AC-01 ~ AC-19 |

## 5. 계약 근거와 미검증 영역 기록 (WP 0.15)

| 근거 ID | 출처 | 버전 | 보관 |
|---|---|---|---|
| SRC-VIM-TYPEINFO | pyVmomi 9.1.1.0 `_typeinfo_{core,query,vim}.py` 추출본 | API 2.0~9.1 도입 버전 | 저장소 밖(D11) |
| SRC-GOVMOMI | govmomi v0.56.0 vim25 타입·vcsim | — | Go 모듈 |
| SRC-KB-326316 | Broadcom KB 326316 vCenter 버전·빌드 표 | 7.0.2.00000/17694817, 7.0.3.02500/24927011 | 링크(조회 2026-10-10) |
| SRC-VIM-API-702 | vSphere Web Services API 7.0U2 참조(New and Changed in 7.0.2.0) | 7.0.2 프로파일 api_version | 링크(조회 2026-10-10) |
| SRC-NSX-321 | NSX-T 3.2.1.0.0 Manager·Policy OpenAPI | 3.2.4.3 프로파일 적용, 차이 미검증 | 저장소 밖(D11) |
| SRC-NSX-RN | NSX-T 3.2.2 ~ 3.2.4.3 릴리스 노트 API 변경 항목 | — | 미수집 |
| SRC-VAPI-703 | vSphere Automation API 7.0.3 스펙 | — | **미확보.** 2026-10-10 다운로드 시도: Broadcom 개발자 포털의 v7.0U2·v7.0U3·v8.0.0·latest 모두 'API Details Page is temporarily unavailable'(Related Resources·오퍼레이션 페이지) |
| SRC-VAPI-SDK-7032 | vsphere-automation-sdk-python v7.0.3.2(MIT, archived): vapi_client_bindings 3.8.0, vapi_runtime 2.34.0에서 추출한 REST 계약 | 7.0 U3 | 저장소 밖(원본 머리말 저작권 표기 확인 전), `50-deployment/install/Get-VapiContract.ps1`로 재현(SHA-256 131da7d5…917ec1, Phase 4b에 content 모듈·요청 매핑 추가. 이전 a6953bec…d8db7) |
| SRC-DOC | Broadcom 공식 API 문서(PRD-1 §4.4 계약 근거 링크 등) | 조회일 기록 | 링크 |

**현재 미검증·가정 목록** (Phase 0 PoC에서 드러난 항목, 본 구현에서 근거를 찾거나 가정으로 유지):

| 항목 | 현재 처리 | 근거 필요 |
|---|---|---|
| NSX 오류 본문 | 필드는 스펙 `ApiError`로 확정(module_name, error_message, error_code, details, error_data, related_errors). 상황별 `error_code` 값은 가정 | error_code 값은 API Guide 문서 확인 |
| NSX PATCH에 오래된 `_revision`이 있을 때 412 | 가정 | 스펙은 PUT만 명시 |
| 삭제된 intent의 Realization 조회 | 404 가정 | FR-NSX-0508 계약 근거 필요 |
| H-API 적용 중 실패 시 원자성 | 전체 롤백 가정 | FR-NSX-0225 문서 근거 필요 |
| `WaitForUpdatesEx` 버전 토큰 | vcsim은 항상 `"-"` | 실제 vCenter는 증가 토큰. 계약 문서로 형식 확인 |
| 완료된 Task의 `progress` | vcsim은 0 | 문서 확인 |
| 하위 호환 타입 대체(세션 버전에 없는 xsi:type) | 미구현(제거) | WSDL·SDK 근거 |
| REST `POST /api/session` 성공 코드 201 | 가정(바인딩에 서버 success_response_code 없음, 런타임 기본 200) | 서버 메타데이터 근거 |
| REST guest_OS·하드웨어 버전 이름 변환(`otherGuest`→`OTHER`, `vmx-13`→`VMX_13`) | 규칙 기반, 계약 시험에서 열거형 소속 확인 | 변환 표 근거 |

## 6. 남은 확보 대상

- vSphere Automation API 7.0.3 스펙(REST 계약 근거, SRC-VAPI-703): 다운로드 승인 필요.
- NSX-T 3.2.2 ~ 3.2.4.3 릴리스 노트(SRC-NSX-RN).

## 7. 요구사항 검증 추적표 (TR-COV-0103)

Phase 0b 기준(PRD-4 §15.5): MVP 범위(● 및 채택된 ○)와 Phase 1 대상 ID는 아래 개별 행으로 전개한다. 그 밖의 ID는 PRD-4 §15.5의 요구사항 범위 표로 주 WP·목표 단계를 배정해 두고, 해당 단계에 진입할 때 개별 행으로 전개한다. 시험·검토 ID `T-<요구사항 ID>`는 계획 식별자이며 구현 시 실제 테스트 이름과 연결한다.

<!-- TRACE:BEGIN -->

자동 생성: `.agentic/scripts/gen_trace.py` · 요구사항 438건 중 개별 행 426건(MVP ● 274건 + Phase 0·1 대상 + 목표 단계 Phase 5 이내), 보류 12건.

**자동 검사 결과**

- ID 중복: 0건
- 미정의 참조: 0건
- WP 미배정(보류·TR-E2E 제외): 0건
- 결정 대기 ○ 항목: 0건 — 
- 시험 결과(개별 행 기준): 시험 통과 365건, 실패 0건, 자체 검토 53건, 이월 8건, 미실행 0건 (근거: 40-test/results*.json, 테스트 코드의 T-<ID> 표기, 40-test/review-evidence.yaml). '자체 검토'는 AI 에이전트의 문서·코드 검토이며 사람의 검토 승인을 대신하지 않는다

**보류·적용 제외**

| 요구사항 ID | 요구사항 | 제외 근거 |
|---|---|---|
| FR-BE-0305 | 교차 시스템 절차 (권고안 B) | 적용 제외 — D3 |
| FR-BE-0405 | 권고안 B 연동 | 적용 제외 — D3 |
| FR-NSX-0230 | Projects / VPC | 적용 제외 — D1 |
| TR-COV-0401 | Trace 비교 항목 | 적용 제외 — D5·D13 |
| TR-COV-0402 | 동적 값 정규화 | 적용 제외 — D5·D13 |
| TR-COV-0404 | Trace 비식별화 | 적용 제외 — D5·D13 |
| TR-COV-0405 | Record 모드 | 적용 제외 — D5·D13 |
| TR-COV-0406 | 기록 시 비식별 치환 | 적용 제외 — D5·D13 |
| TR-COV-0407 | Trace 활용 | 적용 제외 — D5·D13 |
| TR-COV-0408 | 향후 실환경 캡처 계획 | 적용 제외 — D5·D13 |
| NFR-0312 | Record 모드 보안 | 적용 제외 — D5·D13 |
| NFR-ARC-0302 | 프로세스 구성 (권고안 B) | 적용 제외 — D3 |

**○ 항목 MVP 채택 결과** (`40-test/o-decisions.yaml`, PRD-4 §15.3·§19)

| 요구사항 ID | 요구사항 | 결과 |
|---|---|---|
| FR-NSX-0208 | MP: 기타 API | 제외 — D9: Manager API는 node/version·cluster/status·fabric/compute-managers·transport-zones 조회로 한정 |
| FR-NSX-0223 | Hierarchical API | 제외 — D9: Hierarchical API는 MVP 이후(Phase 3·4b) |
| FR-NSX-0224 | H-API 참조·순서 | 제외 — D9: Hierarchical API는 MVP 이후 |
| FR-NSX-0225 | H-API 부분 실패 | 제외 — D9: Hierarchical API는 MVP 이후 |
| FR-NSX-0226 | Fabric VM 및 VM Tag | 제외 — D9: Fabric VM·VM Tag는 MVP 이후 |
| FR-NSX-0227 | VIF | 제외 — D9: VIF는 MVP 이후 |
| FR-NSX-0228 | Policy 실현 VM 조회 | 제외 — D9: Policy 실현 VM 조회는 MVP 이후 |
| FR-NSX-0607 | 버전별 네트워크 표현 | 제외 — D8: VDS 7.0 DVPG 표현 매핑 구현은 Phase 3(WP 3.2) |
| FR-NSX-0608 | NSX VM 인벤토리 파생 | 제외 — D9: NSX VM 인벤토리 파생은 MVP 이후(Phase 3) |
| FR-NSX-0609 | NSX VM Tag 소유권 | 제외 — D9: NSX VM Tag 소유권은 MVP 이후 |
| FR-VC-0204 | Legacy REST API | 제외 — D10: 구형 REST 경로는 MVP 이후(Phase 4b) |
| FR-VC-0206 | Content Library 및 OVF | 제외 — D7: Content Library·OVF는 MVP 이후(WP 4.11) |
| FR-VC-0208 | Lookup Service | 제외 — D10: Lookup Service는 MVP 이후 |
| FR-VC-0209 | SSO STS | 제외 — D10: SSO STS(LoginByToken)는 MVP 이후 |
| FR-VC-0210 | PBM | 제외 — D7: PBM은 MVP 이후 |
| FR-VC-0211 | HttpNfcLease 및 ESXi 직접 경로 | 제외 — D7: HttpNfcLease·ESXi 직접 경로(OVF 업로드)는 MVP 이후 |
| FR-VC-0402 | Clone | 제외 — D7: MVP는 기본 CreateVM_Task만, Clone은 MVP 이후(WP 4.11) |
| FR-VC-0415 | Template Convert | 제외 — D7: Template 변환은 MVP 이후(WP 4.11) |
| NFR-0311 | 외부 바인딩 통제 | 제외 — D6: 같은 PC 네이티브(토폴로지 ①)만 MVP, 외부 바인딩은 MVP 이후 |
| NFR-ARC-0104 | vCenter 보조 서비스 | 제외 — D10: vcsim 보조 서비스(Lookup/STS/PBM/Content Library) 재사용은 MVP 이후 |

**개별 추적 행**

| 요구사항 ID | 요구사항 | MVP | 적용 | 주 WP | 완료 목표 단계 | 검증 방법 | 시험·검토 ID | 연결 AC | Phase 0 근거 | 결과 |
|---|---|---|---|---|---|---|---|---|---|---|
| FR-API-0101 | 동일 외부 인터페이스 | ● | 적용 | WP 1.1 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0101 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestNSXPhase4bResponseSchema, TestNSXResponseSchemaContract, TestSOAPSchemaContract |
| FR-API-0102 | 응답 임의 표준화 금지 | ● | 적용 | WP 1.12 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0102 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestNSXResponseSchemaContract, TestSOAPSchemaContract |
| FR-API-0201 | vCenter SOAP 로그인 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0201 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | PoC-01 | 통과: TestScenario01_ConnectAndDiscover, TestScenario10_SessionExpiry |
| FR-API-0202 | vCenter REST 세션 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0202 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | PoC-02 | 통과: TestRESTSessionContract, TestRESTViewMatchesSOAP |
| FR-API-0203 | vCenter 세션 검증 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0203 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestNegativeREST, TestRESTSessionContract, TestScenario01_ConnectAndDiscover |
| FR-API-0204 | vCenter 세션 만료 | – | 적용 | WP 4.1 | Phase 4a | Contract·Negative·기동 모드 시험 | T-FR-API-0204 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestScenario10_SessionExpiry |
| FR-API-0205 | 사용자별 세션 분리 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0205 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestScenario01_AuthenticationFailures |
| FR-API-0206 | NSX Basic 인증 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0206 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-API-0207 | NSX 세션 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0207 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestNSXSessionAndXSRF, TestScenario10_SessionExpiry |
| FR-API-0208 | NSX API별 인증 방식 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0208 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestScenario01_AuthenticationFailures |
| FR-API-0209 | NSX 세션 만료 | – | 적용 | WP 4.1 | Phase 4a | Contract·Negative·기동 모드 시험 | T-FR-API-0209 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestScenario10_SessionExpiry |
| FR-API-0210 | 사용자·Credential | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0210 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestScenario01_AuthenticationFailures |
| FR-API-0211 | 권한 모델 | – | 적용 | WP 4.2 | Phase 4a | Contract·Negative·기동 모드 시험 | T-FR-API-0211 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestPrivileges, TestRBAC |
| FR-API-0212 | 권한 부족 오류 | – | 적용 | WP 4.2 | Phase 4a | Contract·Negative·기동 모드 시험 | T-FR-API-0212 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestRBAC |
| FR-API-0213 | 자체 권한 시나리오 | – | 적용 | WP 4.2 | Phase 4a | Contract·Negative·기동 모드 시험 | T-FR-API-0213 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestRBAC |
| FR-API-0214 | 인증 정보 하드코딩 금지 | ● | 적용 | WP 1.6 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0214 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 자체 검토: internal/sim/users.go (첫 기동 시 무작위 생성, 코드에 비밀번호 없음) |
| FR-API-0301 | 실행 시 버전 입력 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0301 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestVersionAndModeDefaults |
| FR-API-0302 | 프로파일 기동 검증 | ● | 적용 | WP 1.2, WP 1.13, WP 2.12 | Phase 2b | Contract·Negative·기동 모드 시험 | T-FR-API-0302 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestStartupRefusals, TestVersionAndModeDefaults |
| FR-API-0303 | 실행 중 프로파일 변경 | – | 적용 | WP 1.2 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0303 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 이월: MVP 이후 — 권장 항목(MVP –) |
| FR-API-0304 | 활성 프로파일 조회 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0304 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestAdminAPIAuth |
| FR-API-0305 | 프로파일 구성 | ● | 적용 | WP 1.2, WP 1.3 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0305 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestStartupRefusals, TestUnverifiedCombinationRefusedInRelease, TestVersionDifferential |
| FR-API-0306 | 프로파일 결정 항목 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0306 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestListenerTLSMaxVersion, TestRESTApplianceVersionContract, TestScenario01_ConnectAndDiscover, TestVersionDifferential |
| FR-API-0307 | 데이터 출처 | ● | 적용 | WP 0.19, WP 1.3 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0307 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 자체 검토: profile.yaml spec_sources (출처·버전·SHA-256·조회일) |
| FR-API-0308 | 수작업 규칙 근거 | ● | 적용 | WP 1.3 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0308 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 자체 검토: BEHAVIOR_SPEC.md 근거 열, 코드 주석의 가정 표시 |
| FR-API-0309 | 빌드 간 무검증 복사 금지 | ● | 적용 | WP 1.3 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0309 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 자체 검토: nsx/3.2.4.3/profile.yaml (3.2.1 스펙, 차이 미검증 표시) |
| FR-API-0310 | 초기 적용 범위 | ● | 적용 | WP 1.3 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0310 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 자체 검토: data/compatibility-profiles (기준 조합 1개) |
| FR-API-0311 | 추가 버전 확장 | – | 적용 | WP 4.7 | Phase 4b | Contract·Negative·기동 모드 시험 | T-FR-API-0311 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestVersionDifferential, TestVersionDifferentialMethods |
| FR-API-0312 | 대상 버전 결정 | ● | 적용 | WP 0.8 | Phase 0a | Contract·Negative·기동 모드 시험 | T-FR-API-0312 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 자체 검토: PRD-4 D1, compatibility-matrix.yaml |
| FR-API-0401 | 성공 위장 금지 | ● | 적용 | WP 1.12 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0401 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestNegativeUnsupported, TestScenario01_UnsupportedAndRouting |
| FR-API-0402 | 버전별 미지원 응답 | ● | 적용 | WP 1.12 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0402 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestNegativeUnsupported, TestScenario01_UnsupportedAndRouting |
| FR-API-0403 | 미지원 호출 Audit | ● | 적용 | WP 1.12 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0403 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestCoverageReport, TestScenario01_UnsupportedAndRouting, TestVerificationCapDisplayed |
| FR-API-0404 | 미지원 API 자동 식별 | ● | 적용 | WP 1.12 | Phase 1 | Contract·Negative·기동 모드 시험 | T-FR-API-0404 | AC-02, AC-03, AC-08, AC-13, AC-15~17 | — | 통과: TestCoverageReport, TestNegativeUnsupported, TestScenario01_UnsupportedAndRouting |
| FR-BE-0101 | 의미 기반 상태 시뮬레이션 | ● | 적용 | WP 2.11 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0101 | AC-04~10, AC-19 | — | 통과: TestScenario03_CreateVM, TestScenario04_Reconfigure |
| FR-BE-0102 | 모듈 구성 | ● | 적용 | WP 2.11 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0102 | AC-04~10, AC-19 | — | 자체 검토: ARCHITECTURE.md §2 — MVP 모듈: 상태(vcsim Registry + internal/nsx 캐시·store), 상태 전이(vcsim Task + vcenter/tasks.go), Constraint·Capacity(vcenter/adapter.go), Event(vcsim EventManager), NSX Realization(internal/nsx). Virtual Clock·Fault Injection Engine은 WP 4.10·3.8·4.3 |
| FR-BE-0201 | 안정적 ID | ● | 적용 | WP 2.3 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0201 | AC-04~10, AC-19 | — | 통과: TestScenario02_InventorySync |
| FR-BE-0202 | 변경 반영 | ● | 적용 | WP 2.3 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0202 | AC-04~10, AC-19 | — | 통과: TestScenario02_InventorySync |
| FR-BE-0203 | 참조 무결성 | ● | 적용 | WP 2.3 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0203 | AC-04~10, AC-19 | — | 통과: TestScenario03_CreateVM |
| FR-BE-0204 | 멱등성 | ● | 적용 | WP 2.3 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0204 | AC-04~10, AC-19 | — | 통과: TestIdempotency, TestScenario09_TaskFailureRecovery |
| FR-BE-0205 | 동시 변경 충돌 | ● | 적용 | WP 2.3 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0205 | AC-04~10, AC-19 | — | 통과: TestNSXConcurrentWrites, TestNegativeNSX |
| FR-BE-0206 | 연관 참조 정리 | ● | 적용 | WP 2.3 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0206 | AC-04~10, AC-19 | — | 통과: TestScenario05_Delete, TestScenario08_CrossSystemWorkflow |
| FR-BE-0207 | 작업·리소스 상태 구분 | ● | 적용 | WP 2.3 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0207 | AC-04~10, AC-19 | — | 통과: TestScenario04_Reconfigure |
| FR-BE-0208 | 재기동 후 복원 선택 | – | 적용 | WP 5.2 | Phase 5 | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0208 | AC-04~10, AC-19 | — | 통과: TestRestartRecovery |
| FR-BE-0209 | 변경 원인 추적 | – | 적용 | WP 3.5 | Phase 3 | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0209 | AC-04~10, AC-19 | — | 통과: TestAuditAndChangeCause |
| FR-BE-0210 | 재현성 | ● | 적용 | WP 1.8 | Phase 1 | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0210 | AC-04~10, AC-19 | — | 통과: TestResetDeterminism |
| FR-BE-0301 | 데이터 소유권 | ● | 적용 | WP 0.9 | Phase 0b | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0301 | AC-04~10, AC-19 | — | 자체 검토: DATA_MODEL.md §1, ARCHITECTURE.md §4 |
| FR-BE-0302 | 원본 중복 보관 금지 | ● | 적용 | WP 0.9 | Phase 0b | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0302 | AC-04~10, AC-19 | — | 자체 검토: ARCHITECTURE.md §4 (REST 뷰는 원본 직접 조회) |
| FR-BE-0303 | 캐시 동기화 | ● | 적용 | WP 0.9 | Phase 0b | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0303 | AC-04~10, AC-19 | — | 자체 검토: ARCHITECTURE.md §4 (캐시 없음) |
| FR-BE-0304 | 단일 프로세스 일관성 (권고안 A) | ● | 적용 | WP 0.9, WP 0.18 | Phase 0b | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0304 | AC-04~10, AC-19 | PoC-21 | 자체 검토: internal/gateway CrossLock, POC_PLAN.md PoC-21 |
| FR-BE-0401 | 프로세스 내 Adapter | ● | 적용 | WP 2.11 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0401 | AC-04~10, AC-19 | PoC-14 | 통과: TestScenario04_Reconfigure |
| FR-BE-0402 | 요청 경로 내 동기 처리 | ● | 적용 | WP 2.11 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0402 | AC-04~10, AC-19 | PoC-14 | 통과: TestNegativeSOAP, TestScenario04_Reconfigure |
| FR-BE-0403 | 요청 후 비동기 처리 | – | 적용 | WP 3.5 | Phase 3 | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0403 | AC-04~10, AC-19 | — | 통과: TestScenario08_CrossSystemWorkflow, TestVCenterEvents |
| FR-BE-0404 | Adapter 기능 | ● | 적용 | WP 2.11 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0404 | AC-04~10, AC-19 | — | 통과: TestScenario02_InventorySync, TestScenario03_CreateVM, TestScenario04_Reconfigure |
| FR-BE-0406 | 간접 변경 금지 | ● | 적용 | WP 2.11 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0406 | AC-04~10, AC-19 | — | 자체 검토: internal/vcenter/adapter.go — Registry.Handler에서 요청 경로 안 동기 검사, 외부 SOAP 반복 호출 없음. publishReconfigure는 같은 프로세스의 ctx.Update(R-23) |
| FR-BE-0407 | Hook 부족 시 대안 비교 | ● | 적용 | WP 0.4 | Phase 0a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0407 | AC-04~10, AC-19 | — | 자체 검토: OPENSOURCE_ASSESSMENT.md §3.1 |
| FR-BE-0408 | 착수 전 기술 검증 | ● | 적용 | WP 0.4 | Phase 0a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0408 | AC-04~10, AC-19 | — | 자체 검토: POC_PLAN.md (PoC-14·21) |
| FR-BE-0501 | 작업 상태 전이 | ● | 적용 | WP 2.7 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0501 | AC-04~10, AC-19 | — | 통과: TestScenario03_CreateVM, TestTaskRetentionAndLogs |
| FR-BE-0502 | 작업 속성 | ● | 적용 | WP 2.7 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0502 | AC-04~10, AC-19 | — | 통과: TestTaskRetentionAndLogs |
| FR-BE-0503 | API별 계약 변환 | ● | 적용 | WP 2.7 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0503 | AC-04~10, AC-19 | — | 통과: TestScenario03_CreateVM |
| FR-BE-0601 | 기본 실시간 동작 | ● | 적용 | WP 2.7 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0601 | AC-04~10, AC-19 | — | 통과: TestScenario06_SegmentCreate |
| FR-BE-0602 | 시간 가속 | – | 적용 | WP 4.10 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0602 | AC-04~10, AC-19 | — | 통과: TestTaskDelayAndVirtualClock, TestVirtualClock |
| FR-BE-0603 | 시간 정지 | – | 적용 | WP 4.10 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0603 | AC-04~10, AC-19 | — | 통과: TestTaskDelayAndVirtualClock, TestVirtualClock |
| FR-BE-0604 | 특정 시점 진행 | – | 적용 | WP 4.10 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0604 | AC-04~10, AC-19 | — | 통과: TestTaskDelayAndVirtualClock, TestVirtualClock |
| FR-BE-0605 | Task 완료 지연 설정 | – | 적용 | WP 4.5 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0605 | AC-04~10, AC-19 | — | 통과: TestTaskDelayAndVirtualClock |
| FR-BE-0606 | Realization 지연 설정 | ● | 적용 | WP 2.10 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0606 | AC-04~10, AC-19 | — | 통과: TestScenario06_SegmentCreate |
| FR-BE-0607 | Session 만료 시뮬레이션 | – | 적용 | WP 4.1, WP 4.10 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0607 | AC-04~10, AC-19 | — | 통과: TestScenario10_SessionExpiry |
| FR-BE-0608 | 통계 데이터 시간 생성 | – | 적용 | WP 4.10 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0608 | AC-04~10, AC-19 | — | 통과: TestShiftTimes, TestTaskDelayAndVirtualClock |
| FR-BE-0609 | 정상·테스트 모드 | – | 적용 | WP 4.10 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0609 | AC-04~10, AC-19 | — | 통과: TestTaskDelayAndVirtualClock, TestVirtualClock |
| FR-BE-0610 | 실제·논리 시간 분리 | ● | 적용 | WP 2.7 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0610 | AC-04~10, AC-19 | — | 자체 검토: internal/clock — 가상 시간은 구간(segment) 대응으로 계산하고 HTTP 타임아웃·네트워크 대기(Gateway 지연·Timeout Fault, 클라이언트 타임아웃)는 실제 시간 그대로. 가상 시간은 Realization·세션 만료·Task 지연·메트릭·SOAP 응답 시각에만 적용 |
| FR-BE-0611 | vcsim 시간 소스 | – | 적용 | WP 0.13, WP 4.10 | Phase 4a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0611 | AC-04~10, AC-19 | — | 통과: TestEventTimeFilterVirtualClock, TestShiftTimes, TestVirtualToRealRange |
| FR-BE-0612 | 이벤트 기준 타이머 | ● | 적용 | WP 2.7 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0612 | AC-04~10, AC-19 | — | 통과: TestVirtualClock |
| FR-BE-0701 | 가상 용량 모델 | ● | 적용 | WP 2.5 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0701 | AC-04~10, AC-19 | — | 통과: TestScenario04_Reconfigure |
| FR-BE-0702 | 용량 계산 | ● | 적용 | WP 2.5 | Phase 2a | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0702 | AC-04~10, AC-19 | — | 통과: TestScenario04_Reconfigure |
| FR-BE-0703 | 고급 용량 모델 | – | 적용 | WP 3.7 | Phase 3 | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0703 | AC-04~10, AC-19 | — | 이월: 선택 항목(채택 시) — Overcommit·Reservation 등 고급 용량 모델은 미채택. 현재 용량 모델: 호스트 가상 용량(Fixture) 상한, VM 단위 검사 |
| FR-BE-0704 | 메트릭·할당량 정합 | – | 적용 | WP 3.7 | Phase 3 | Unit·동시성/Stateful·설계 검토 | T-FR-BE-0704 | AC-04~10, AC-19 | — | 통과: TestVirtualMetrics |
| FR-DM-0101 | 논리 도메인 구성 | ● | 적용 | WP 0.9 | Phase 0b | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0101 | AC-06, AC-09, AC-10, AC-19 | — | 자체 검토: DATA_MODEL.md §2 |
| FR-DM-0201 | ID 형식 구분 | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0201 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestIDKinds |
| FR-DM-0202 | 단일 UUID 반환 금지 | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0202 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestIDKinds |
| FR-DM-0203 | 계약 기반 ID 생성 | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0203 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestScenario03_CreateVM |
| FR-DM-0204 | REST ID = MoRef | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0204 | AC-06, AC-09, AC-10, AC-19 | PoC-03 | 통과: TestRESTVMGetContract, TestRESTViewMatchesSOAP |
| FR-DM-0205 | 중복 방지 | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0205 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestNegativeSOAP, TestScenario02_InventorySync |
| FR-DM-0206 | 안정적 참조 | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0206 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestScenario02_InventorySync |
| FR-DM-0207 | 환경 간 Namespace 분리 | – | 적용 | WP 3.6 | Phase 3 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0207 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestDeterministicVMPlacement, TestEnvironmentNamespace, TestFixtureCache, TestFixtureTopology, TestPlacementMatchesVcsim |
| FR-DM-0208 | 삭제 후 재사용 규칙 | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0208 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestScenario05_Delete |
| FR-DM-0209 | 복원 시 참조 일관성 | – | 적용 | WP 5.2 | Phase 5 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0209 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestSnapshotRestoreRoundTrip |
| FR-DM-0210 | 결정론적 Fixture ID | ● | 적용 | WP 2.4 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0210 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestEnvironmentNamespace, TestResetDeterminism |
| FR-DM-0301 | SQLite 사용 | ● | 적용 | WP 2.6 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0301 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestNSXTransaction |
| FR-DM-0302 | 예상 테이블 | ● | 적용 | WP 2.6 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0302 | AC-06, AC-09, AC-10, AC-19 | — | 자체 검토: internal/store/store.go — MVP 사용 테이블(nsx_resources, simulation_operations, operation_transitions, event_outbox)만 생성. 나머지 예상 테이블은 해당 기능 단계에서 추가 |
| FR-DM-0303 | vCenter 인벤토리 중복 저장 제외 | ● | 적용 | WP 2.6 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0303 | AC-06, AC-09, AC-10, AC-19 | — | 자체 검토: internal/store/store.go 스키마에 vCenter 인벤토리 테이블 없음(원본은 vcsim Registry, REST는 뷰) |
| FR-DM-0304 | 일관된 Snapshot 설계 | – | 적용 | WP 5.2 | Phase 5 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0304 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestEventsSurviveRestore, TestSnapshotRestoreRoundTrip |
| FR-DM-0305 | 무결성 절차 | ● | 적용 | WP 2.6 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0305 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestNSXConcurrentWrites |
| FR-DM-0306 | 단일 락 Snapshot (권고안 A) | – | 적용 | WP 0.18, WP 5.2 | Phase 5 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0306 | AC-06, AC-09, AC-10, AC-19 | PoC-21 | 통과: TestRestoreChainFromLoadedModel, TestSnapshotRestoreRoundTrip |
| FR-DM-0307 | lite 인메모리 저장소 | ● | 적용 | WP 2.6 | Phase 2a | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0307 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestNSXTransaction |
| FR-DM-0401 | 교차 조회 일관성 | – | 적용 | WP 3.6 | Phase 3 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0401 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-DM-0402 | 일관성 기법 검토 | – | 적용 | WP 3.6 | Phase 3 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0402 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestIncrementalReconcile, TestReconcileRepairsDrift |
| FR-DM-0403 | 부분 성공 방지 | – | 적용 | WP 3.6 | Phase 3 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0403 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestIncrementalReconcile, TestRealizationDependenciesAndFailure, TestReconcileRepairsDrift |
| FR-DM-0404 | 동기화 전 상태 표시 | – | 적용 | WP 3.6 | Phase 3 | 모델 검토·ID 계약·무결성 시험 | T-FR-DM-0404 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-FLT-0101 | API 오류: 인증 실패 | ● | 적용 | WP 2.8, WP 4.4 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0101 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeREST, TestNegativeSOAP, TestScenario01_AuthenticationFailures |
| FR-FLT-0102 | API 오류: 세션 만료 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0102 | AC-05, AC-08, AC-13 | — | 통과: TestScenario10_SessionExpiry |
| FR-FLT-0103 | API 오류: 권한 부족 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0103 | AC-05, AC-08, AC-13 | — | 통과: TestRBAC |
| FR-FLT-0104 | API 오류: 잘못된 입력 | ● | 적용 | WP 2.8, WP 4.4 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0104 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeNSX, TestNegativeSOAP |
| FR-FLT-0105 | API 오류: 존재하지 않는 리소스 | ● | 적용 | WP 2.8, WP 4.4 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0105 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeNSX, TestNegativeREST, TestNegativeSOAP, TestRESTVMGetContract, TestScenario05_Delete |
| FR-FLT-0106 | API 오류: 중복 생성 | ● | 적용 | WP 2.8, WP 4.4 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0106 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeSOAP |
| FR-FLT-0107 | API 오류: 리소스 사용 중 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0107 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeNSX |
| FR-FLT-0108 | API 오류: 버전 충돌 | ● | 적용 | WP 2.8, WP 4.4 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0108 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeNSX, TestSemanticAndInfraFaults |
| FR-FLT-0109 | API 오류: 미지원 기능 | ● | 적용 | WP 2.8, WP 4.4 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0109 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeUnsupported |
| FR-FLT-0110 | 인프라: Host Disconnected | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0110 | AC-05, AC-08, AC-13 | — | 통과: TestSemanticAndInfraFaults |
| FR-FLT-0111 | 인프라: Host Maintenance Mode | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0111 | AC-05, AC-08, AC-13 | — | 통과: TestSemanticAndInfraFaults |
| FR-FLT-0112 | 인프라: Datastore 용량 부족 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0112 | AC-05, AC-08, AC-13 | — | 통과: TestSemanticAndInfraFaults |
| FR-FLT-0113 | 인프라: VM Power Operation 실패 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0113 | AC-05, AC-08, AC-13 | — | 통과: TestScenario09_TaskFailureRecovery |
| FR-FLT-0114 | 인프라: VM Clone 실패 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0114 | AC-05, AC-08, AC-13 | — | 통과: TestSemanticAndInfraFaults |
| FR-FLT-0115 | 인프라: ResourcePool 할당 실패 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0115 | AC-05, AC-08, AC-13 | — | 통과: TestScenario09_TaskFailureRecovery |
| FR-FLT-0116 | 인프라: Realization 실패 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0116 | AC-05, AC-08, AC-13 | — | 통과: TestSemanticAndInfraFaults |
| FR-FLT-0117 | 통신: 응답 지연 | – | 적용 | WP 3.8, WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0117 | AC-05, AC-08, AC-13 | — | 통과: TestTransportFaultRules |
| FR-FLT-0118 | 통신: HTTP Timeout | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0118 | AC-05, AC-08, AC-13 | — | 통과: TestTransportTimeoutResetIntermittent |
| FR-FLT-0119 | 통신: Connection Reset | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0119 | AC-05, AC-08, AC-13 | — | 통과: TestTransportTimeoutResetIntermittent |
| FR-FLT-0120 | 통신: 지정 HTTP 오류 | – | 적용 | WP 3.8, WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0120 | AC-05, AC-08, AC-13 | — | 통과: TestTransportFaultRules |
| FR-FLT-0121 | 작업: SOAP Fault | ● | 적용 | WP 2.8, WP 4.4 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0121 | AC-05, AC-08, AC-13 | — | 통과: TestNegativeSOAP, TestSemanticAndInfraFaults |
| FR-FLT-0122 | 작업: 비동기 Task 실패 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0122 | AC-05, AC-08, AC-13 | — | 통과: TestScenario09_TaskFailureRecovery |
| FR-FLT-0123 | 통신: 일시적 503 | – | 적용 | WP 3.8, WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0123 | AC-05, AC-08, AC-13 | — | 통과: TestTransportFaultRules |
| FR-FLT-0124 | 통신: 간헐적 오류 | – | 적용 | WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0124 | AC-05, AC-08, AC-13 | — | 통과: TestTransportTimeoutResetIntermittent |
| FR-FLT-0125 | 통신: 지정 횟수 후 복구 | – | 적용 | WP 3.8, WP 4.4 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0125 | AC-05, AC-08, AC-13 | — | 통과: TestRuleModel, TestTransportFaultRules |
| FR-FLT-0201 | 시나리오 기반 제어 | – | 적용 | WP 4.3 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0201 | AC-05, AC-08, AC-13 | — | 통과: TestScenario09_TaskFailureRecovery, TestSemanticAndInfraFaults |
| FR-FLT-0202 | 주입 조건 | – | 적용 | WP 4.3 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0202 | AC-05, AC-08, AC-13 | — | 통과: TestRuleModel, TestSemanticAndInfraFaults, TestTransportTimeoutResetIntermittent |
| FR-FLT-0203 | 주입 동작 | – | 적용 | WP 4.3 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0203 | AC-05, AC-08, AC-13 | — | 통과: TestScenario09_TaskFailureRecovery, TestSemanticAndInfraFaults |
| FR-FLT-0204 | 주입 계층 분리 | – | 적용 | WP 4.3 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0204 | AC-05, AC-08, AC-13 | — | 통과: TestRuleModel, TestScenario09_TaskFailureRecovery, TestSemanticAndInfraFaults |
| FR-FLT-0205 | 공통 Fault Rule 모델 | – | 적용 | WP 3.8, WP 4.3 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0205 | AC-05, AC-08, AC-13 | — | 통과: TestRuleModel, TestTransportFaultRules |
| FR-FLT-0206 | Admin API 전용 제어 | – | 적용 | WP 3.8, WP 4.3 | Phase 4a | Negative·Fault 주입 시험 | T-FR-FLT-0206 | AC-05, AC-08, AC-13 | — | 통과: TestTransportFaultRules |
| FR-FLT-0207 | 임의 오류 금지 | ● | 적용 | WP 2.8 | Phase 2a | Negative·Fault 주입 시험 | T-FR-FLT-0207 | AC-05, AC-08, AC-13 | — | 자체 검토: Fault Injection 미구현(Phase 3~4a). 오류는 계약 위반 요청에서만 발생: internal/vcenter/adapter.go(용량·전원·중복 이름·익명 조회), internal/nsx/handler.go(스키마·Revision·참조). Scenario 02~06 정상 요청 전부 성공 |
| FR-MET-0101 | 성능·용량 조회 지원 | – | 적용 | WP 3.7 | Phase 3 | Contract·결정론 시험 | T-FR-MET-0101 | AC-03, AC-06, AC-07, AC-19 | — | 통과: TestVirtualMetrics |
| FR-MET-0102 | 가상 지표 | – | 적용 | WP 3.7 | Phase 3 | Contract·결정론 시험 | T-FR-MET-0102 | AC-03, AC-06, AC-07, AC-19 | — | 통과: TestVirtualMetrics |
| FR-MET-0103 | 메트릭 생성 기능 | – | 적용 | WP 3.7 | Phase 3 | Contract·결정론 시험 | T-FR-MET-0103 | AC-03, AC-06, AC-07, AC-19 | — | 통과: TestVirtualMetrics |
| FR-MET-0104 | 결정론적 즉석 계산 | – | 적용 | WP 3.7 | Phase 3 | Contract·결정론 시험 | T-FR-MET-0104 | AC-03, AC-06, AC-07, AC-19 | — | 통과: TestVirtualMetrics |
| FR-MET-0201 | 이벤트 종류 | – | 적용 | WP 3.5 | Phase 3 | Contract·결정론 시험 | T-FR-MET-0201 | AC-03, AC-06, AC-07, AC-19 | — | 통과: TestSemanticAndInfraFaults |
| FR-MET-0202 | 계약 기준 구현 | – | 적용 | WP 3.5, WP 4.6 | Phase 4b | Contract·결정론 시험 | T-FR-MET-0202 | AC-03, AC-06, AC-07, AC-19 | — | 통과: TestEventTimeFilterVirtualClock, TestRealizationDependenciesAndFailure, TestVCenterEvents |
| FR-MET-0203 | 이벤트 속성 | – | 적용 | WP 3.5 | Phase 3 | Contract·결정론 시험 | T-FR-MET-0203 | AC-03, AC-06, AC-07, AC-19 | — | 통과: TestTaskInfoEntityName, TestVCenterEventArchive, TestVCenterEvents |
| FR-NSX-0101 | 구현 언어 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0101 | AC-02~07, AC-09 | — | 자체 검토: internal/nsx (Go, 단일 프로세스) |
| FR-NSX-0102 | 공식 명세 기반 계약 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0102 | AC-02~07, AC-09 | — | 자체 검토: profile.yaml spec_sources SRC-NSX-321 |
| FR-NSX-0103 | 버전별 스키마 확보 | ● | 적용 | WP 0.6, WP 0.19 | Phase 0b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0103 | AC-02~07, AC-09 | — | 자체 검토: VERSION_PROFILE_SPEC.md §6, 50-deployment/install/Get-NsxSpec.ps1 |
| FR-NSX-0104 | 스펙 제공 경로 | ● | 적용 | WP 0.6 | Phase 0a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0104 | AC-02~07, AC-09 | — | 이월: 공식 문서 근거 확보 후 — 스펙 제공 구현·시험 완료(contract/TestNegativeNSX), 경로가 스펙 밖이며 API Guide 근거 미확보(가정) |
| FR-NSX-0105 | 단일 버전 스펙 로드 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0105 | AC-02~07, AC-09 | PoC-17 | 통과: TestScenario01_UnsupportedAndRouting |
| FR-NSX-0106 | 스키마 지연 컴파일 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0106 | AC-02~07, AC-09 | PoC-17 | 자체 검토: internal/nsx/spec.go (Compile 지연·캐시), POC_PLAN.md §2 |
| FR-NSX-0107 | 범용 리소스 핸들러 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0107 | AC-02~07, AC-09 | PoC-17 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0108 | 행위 리소스 전용 로직 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0108 | AC-02~07, AC-09 | — | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0201 | MP: 인증 및 세션 | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0201 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-NSX-0202 | MP: 버전 조회 | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0202 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-NSX-0203 | MP: Node/Cluster 상태 | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0203 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-NSX-0204 | MP: Fabric·Compute Manager | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0204 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-NSX-0205 | MP: Transport Zone | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0205 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-NSX-0206 | MP: Transport Node | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0206 | AC-02~07, AC-09 | — | 통과: TestNSXManagerInventory |
| FR-NSX-0207 | MP: Logical Networking | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0207 | AC-02~07, AC-09 | — | 통과: TestNSXManagerInventory |
| FR-NSX-0208 | MP: 기타 API | ○ | 적용(○ 채택) | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0208 | AC-02~07, AC-09 | — | 통과: TestNSXManagerInventory |
| FR-NSX-0209 | Policy: Infrastructure | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0209 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-NSX-0210 | Policy: Transport Zones | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0210 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover |
| FR-NSX-0211 | Policy: Segments | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0211 | AC-02~07, AC-09 | PoC-08 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0212 | Policy: Tier-0 Gateways | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0212 | AC-02~07, AC-09 | — | 통과: TestNSXRoutingResources |
| FR-NSX-0213 | Policy: Tier-1 Gateways | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0213 | AC-02~07, AC-09 | — | 통과: TestNSXRoutingResources |
| FR-NSX-0214 | Policy: Gateway Interfaces | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0214 | AC-02~07, AC-09 | — | 통과: TestNSXRoutingResources |
| FR-NSX-0215 | Policy: Groups | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0215 | AC-02~07, AC-09 | — | 통과: TestNSXSecurityResources, TestScenario07_SecurityPolicyChange, TestSearchQuerySyntax |
| FR-NSX-0216 | Policy: Services | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0216 | AC-02~07, AC-09 | — | 통과: TestNSXSecurityResources, TestScenario07_SecurityPolicyChange |
| FR-NSX-0217 | Policy: Security Policies | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0217 | AC-02~07, AC-09 | — | 통과: TestNSXSecurityResources, TestScenario07_SecurityPolicyChange |
| FR-NSX-0218 | Policy: Firewall Rules | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0218 | AC-02~07, AC-09 | — | 통과: TestNSXSecurityResources, TestScenario07_SecurityPolicyChange |
| FR-NSX-0219 | Policy: Tags | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0219 | AC-02~07, AC-09 | — | 통과: TestScenario06_ListPaginationDeleteTags |
| FR-NSX-0220 | Policy: Realization State | ● | 적용 | WP 2.10 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0220 | AC-02~07, AC-09 | PoC-10 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0221 | Policy: Search | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0221 | AC-02~07, AC-09 | — | 통과: TestNSXSearchAndFilters |
| FR-NSX-0222 | Policy: Statistics·Status | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0222 | AC-02~07, AC-09 | — | 이월: Phase 4b 미채택(제안) — 선택 항목. Statistics·Status는 실시간 데이터 의미가 없어 결정론 값 규칙(FR-MET) 확정 후 검토 |
| FR-NSX-0223 | Hierarchical API | ○ | 적용(○ 채택) | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0223 | AC-02~07, AC-09 | PoC-16 | 통과: TestNSXHierarchicalAPI |
| FR-NSX-0224 | H-API 참조·순서 | ○ | 적용(○ 채택) | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0224 | AC-02~07, AC-09 | PoC-16 | 통과: TestNSXHierarchicalAPI |
| FR-NSX-0225 | H-API 부분 실패 | ○ | 적용(○ 채택) | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0225 | AC-02~07, AC-09 | PoC-16 | 통과: TestNSXHierarchicalAPI |
| FR-NSX-0226 | Fabric VM 및 VM Tag | ○ | 적용(○ 채택) | WP 3.3 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0226 | AC-02~07, AC-09 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-NSX-0227 | VIF | ○ | 적용(○ 채택) | WP 3.3 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0227 | AC-02~07, AC-09 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-NSX-0228 | Policy 실현 VM 조회 | ○ | 적용(○ 채택) | WP 3.3 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0228 | AC-02~07, AC-09 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-NSX-0229 | 태그 기반 Group 멤버십 | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0229 | AC-02~07, AC-09 | — | 통과: TestNSXGroupMembership |
| FR-NSX-0231 | Manager API deprecation | ● | 적용 | WP 1.12 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0231 | AC-02~07, AC-09 | — | 통과: TestNegativeUnsupported |
| FR-NSX-0232 | Policy Search 문법 범위 | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0232 | AC-02~07, AC-09 | — | 통과: TestNSXSearchAndFilters, TestSearchQuerySyntax |
| FR-NSX-0233 | 계약·자체 테스트 기반 범위 조정 | ● | 적용 | WP 0.7 | Phase 0b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0233 | AC-02~07, AC-09 | — | 자체 검토: API_COVERAGE_MATRIX.md §1 |
| FR-NSX-0301 | 공통 필드 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0301 | AC-02~07, AC-09 | — | 통과: TestNSXCommonFieldsAndSchema |
| FR-NSX-0302 | 리소스별 필드 차이 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0302 | AC-02~07, AC-09 | — | 통과: TestNSXCommonFieldsAndSchema |
| FR-NSX-0303 | 필드 속성 준수 | ● | 적용 | WP 1.5 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0303 | AC-02~07, AC-09 | — | 통과: TestNSXCommonFieldsAndSchema |
| FR-NSX-0401 | Read | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0401 | AC-02~07, AC-09 | PoC-08 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0402 | List | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0402 | AC-02~07, AC-09 | PoC-08 | 통과: TestScenario06_ListPaginationDeleteTags |
| FR-NSX-0403 | Create (upsert) | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0403 | AC-02~07, AC-09 | PoC-08 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0404 | Update | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0404 | AC-02~07, AC-09 | PoC-08 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0405 | PATCH·PUT 의미 구분 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0405 | AC-02~07, AC-09 | PoC-08 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0406 | Delete | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0406 | AC-02~07, AC-09 | PoC-08 | 통과: TestScenario06_ListPaginationDeleteTags |
| FR-NSX-0407 | id와 unique_id 분리 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0407 | AC-02~07, AC-09 | — | 통과: TestNSXCommonFieldsAndSchema |
| FR-NSX-0408 | Revision Update | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0408 | AC-02~07, AC-09 | PoC-09 | 통과: TestScenario06_SegmentCreate, TestScenario07_SecurityPolicyChange |
| FR-NSX-0409 | 동시 수정 충돌 검사 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0409 | AC-02~07, AC-09 | PoC-09 | 통과: TestNegativeNSX, TestScenario07_SecurityPolicyChange |
| FR-NSX-0410 | 참조 리소스 유효성 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0410 | AC-02~07, AC-09 | — | 통과: TestNSXSecurityResources, TestNegativeNSX |
| FR-NSX-0411 | 계층적 Path | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0411 | AC-02~07, AC-09 | — | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0412 | 부모·자식 관계 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0412 | AC-02~07, AC-09 | — | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0413 | 필터·검색 | – | 적용 | WP 4.9 | Phase 4b | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0413 | AC-02~07, AC-09 | — | 통과: TestNSXSearchAndFilters |
| FR-NSX-0414 | Pagination·Cursor | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0414 | AC-02~07, AC-09 | — | 통과: TestScenario06_ListPaginationDeleteTags |
| FR-NSX-0415 | 삭제 의존관계 검사 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0415 | AC-02~07, AC-09 | — | 통과: TestNSXSecurityResources, TestNegativeNSX |
| FR-NSX-0416 | 응답·오류 형식 | ● | 적용 | WP 2.2 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0416 | AC-02~07, AC-09 | — | 통과: TestNSXCommonFieldsAndSchema, TestNSXResponseSchemaContract, TestScenario06_SegmentCreate |
| FR-NSX-0501 | Realization 논리 단계 | ● | 적용 | WP 2.10 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0501 | AC-02~07, AC-09 | PoC-10 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0502 | 즉시 정상 완료 | ● | 적용 | WP 2.10 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0502 | AC-02~07, AC-09 | PoC-10 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0503 | 지연 후 정상 완료 | ● | 적용 | WP 2.10 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0503 | AC-02~07, AC-09 | PoC-10 | 통과: TestScenario06_SegmentCreate |
| FR-NSX-0504 | Realization 실패 | – | 적용 | WP 3.4 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0504 | AC-02~07, AC-09 | — | 통과: TestRealizationDependenciesAndFailure, TestSemanticAndInfraFaults |
| FR-NSX-0505 | 변경 중 재수정 | – | 적용 | WP 3.4 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0505 | AC-02~07, AC-09 | — | 통과: TestRealizationDependenciesAndFailure |
| FR-NSX-0506 | Realization Timeout | – | 적용 | WP 4.5 | Phase 4a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0506 | AC-02~07, AC-09 | — | 통과: TestSemanticAndInfraFaults, TestTaskDelayAndVirtualClock |
| FR-NSX-0507 | 의존 리소스 누락 | – | 적용 | WP 3.4 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0507 | AC-02~07, AC-09 | — | 통과: TestNSXRoutingResources, TestRealizationDependenciesAndFailure |
| FR-NSX-0508 | 삭제된 리소스 조회 | ● | 적용 | WP 2.10 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0508 | AC-02~07, AC-09 | — | 통과: TestScenario06_ListPaginationDeleteTags |
| FR-NSX-0509 | 상태 필드 매핑 | ● | 적용 | WP 2.10 | Phase 2a | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0509 | AC-02~07, AC-09 | — | 통과: TestRealizationDependenciesAndFailure, TestScenario06_SegmentCreate |
| FR-NSX-0601 | Compute Manager ↔ vCenter | ● | 적용 | WP 1.7, WP 3.1 | Phase 1 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0601 | AC-02~07, AC-09 | — | 통과: TestScenario01_ConnectAndDiscover, TestScenario08_CrossSystemWorkflow |
| FR-NSX-0602 | Fabric/Host ↔ Host | – | 적용 | WP 3.1 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0602 | AC-02~07, AC-09 | — | 통과: TestIncrementalReconcile, TestScenario08_CrossSystemWorkflow |
| FR-NSX-0603 | Segment ↔ vCenter Network | – | 적용 | WP 3.2 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0603 | AC-02~07, AC-09 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-NSX-0604 | VM vNIC ↔ Network | – | 적용 | WP 3.3 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0604 | AC-02~07, AC-09 | — | 통과: TestIncrementalReconcile, TestScenario08_CrossSystemWorkflow |
| FR-NSX-0605 | VM 관련 정책 참조 | – | 적용 | WP 3.3 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0605 | AC-02~07, AC-09 | — | 통과: TestNSXGroupMembership, TestScenario08_CrossSystemWorkflow |
| FR-NSX-0606 | Network ID ↔ Policy Path | – | 적용 | WP 3.2 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0606 | AC-02~07, AC-09 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-NSX-0607 | 버전별 네트워크 표현 | ○ | 적용(○ 채택) | WP 3.2 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0607 | AC-02~07, AC-09 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-NSX-0608 | NSX VM 인벤토리 파생 | ○ | 적용(○ 채택) | WP 3.3 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0608 | AC-02~07, AC-09 | — | 통과: TestIncrementalReconcile, TestScenario08_CrossSystemWorkflow |
| FR-NSX-0609 | NSX VM Tag 소유권 | ○ | 적용(○ 채택) | WP 3.3 | Phase 3 | Contract·Stateful(OpenAPI 계약) | T-FR-NSX-0609 | AC-02~07, AC-09 | — | 통과: TestScenario08_CrossSystemWorkflow |
| FR-VC-0101 | vcsim 기반 엔진 | ● | 적용 | WP 0.1, WP 1.4 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0101 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: ARCHITECTURE.md §1, internal/vcenter (vcsim v0.56.0 임베드) |
| FR-VC-0102 | vcsim 라이브러리 임베드 | ● | 적용 | WP 1.4 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0102 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-12 | 자체 검토: internal/vcenter/vcenter.go (라이브러리 임베드, 자체 리스너 닫음) |
| FR-VC-0103 | vcsim 지원 범위 조사 | ● | 적용 | WP 0.1 | Phase 0a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0103 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: OPENSOURCE_ASSESSMENT.md §5, api-coverage/soap-method-survey.csv |
| FR-VC-0104 | 확장 모듈 우선 | ● | 적용 | WP 1.4 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0104 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: internal/vcenter (vcsim 원본 미수정, Registry 훅·래퍼) |
| FR-VC-0105 | Fork 정책 | – | 적용 | WP 0.13 | Phase 0a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0105 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: OPENSOURCE_ASSESSMENT.md §3.4 (포크 불필요 판단, 시간 소스만 WP 4.10 결정) |
| FR-VC-0106 | 확장 지점 검증 | ● | 적용 | WP 0.13 | Phase 0a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0106 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: OPENSOURCE_ASSESSMENT.md §3.1, POC_PLAN.md PoC-14 |
| FR-VC-0107 | 컨테이너 기반 VM 비활성화 | ● | 적용 | WP 1.4 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0107 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: internal/fixture/fixture.go (ContainerImages 미설정) |
| FR-VC-0201 | SOAP 엔드포인트 | ● | 적용 | WP 1.4 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0201 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-01 | 통과: TestScenario01_ConnectAndDiscover |
| FR-VC-0202 | SOAP 관리 객체 영역 | ● | 적용 | WP 0.2 | Phase 0b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0202 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestContainerViewAndPagedRetrieve |
| FR-VC-0203 | Automation REST API | ● | 적용 | WP 0.3 | Phase 0b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0203 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestRESTInventoryContract |
| FR-VC-0204 | Legacy REST API | ○ | 적용(○ 채택) | WP 0.3, WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0204 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 이월: Phase 4b 미채택(제안) — 구형 /rest는 7.0 U2부터 deprecated이고 /rest 응답 래핑 계약 근거(SDK 바인딩은 /api 메타데이터만 제공)를 확보하지 못함. vcsim 구형 경로는 세션 내부 연결에만 사용 |
| FR-VC-0205 | REST = vcsim 인벤토리 뷰 | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0205 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-03 | 통과: TestRESTInventoryContract, TestRESTViewMatchesSOAP |
| FR-VC-0206 | Content Library 및 OVF | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0206 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestContentLibraryContract |
| FR-VC-0207 | 버전 협상 경로 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0207 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVersionDifferential, TestVimServiceVersions |
| FR-VC-0208 | Lookup Service | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0208 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestLookupService |
| FR-VC-0209 | SSO STS | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0209 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 이월: Phase 4b 미채택(제안) — SSO STS(SAML 토큰 발급·LoginByToken)는 보안 기능이며 vcsim은 서명 없는 토큰을 받아들임(D10, Gateway에서 SIGN 거부 유지) |
| FR-VC-0210 | PBM | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0210 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestPBMDiskPolicy, TestPBMStoragePolicy |
| FR-VC-0211 | HttpNfcLease 및 ESXi 직접 경로 | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0211 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 이월: Phase 4b 미채택(제안) — ESXi 직접 경로는 가상 ESXi 호스트 이름의 hosts 등록(설치 스크립트 변경)과 토폴로지 결정(D6)이 필요 |
| FR-VC-0212 | 내장 보조 시뮬레이터 재사용 | ● | 적용 | WP 1.4 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0212 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: internal/vcenter (vcsim vAPI 재사용, STS·Lookup은 D10으로 MVP 밖) |
| FR-VC-0301 | Datacenter | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0301 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestRESTInventoryContract, TestScenario01_ConnectAndDiscover |
| FR-VC-0302 | Cluster | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0302 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestRESTInventoryContract, TestScenario01_ConnectAndDiscover |
| FR-VC-0303 | Host | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0303 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestRESTInventoryContract, TestScenario01_ConnectAndDiscover |
| FR-VC-0304 | Datastore | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0304 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestRESTInventoryContract, TestScenario01_ConnectAndDiscover |
| FR-VC-0305 | VM | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0305 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestRESTInventoryContract, TestRESTVMGetContract, TestScenario01_ConnectAndDiscover |
| FR-VC-0306 | Network | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0306 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestRESTInventoryContract, TestScenario01_ConnectAndDiscover |
| FR-VC-0307 | 스키마 정합 표현 | ● | 적용 | WP 1.7 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0307 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVersionAwareSerialization |
| FR-VC-0401 | Create | ● | 적용 | WP 2.1 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0401 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-04 | 통과: TestScenario03_CreateVM |
| FR-VC-0402 | Clone | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0402 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0403 | Register | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0403 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0404 | Reconfigure | ● | 적용 | WP 2.1 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0404 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestScenario04_Reconfigure |
| FR-VC-0405 | Power On | ● | 적용 | WP 2.1 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0405 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestScenario04_Reconfigure |
| FR-VC-0406 | Power Off | ● | 적용 | WP 2.1 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0406 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestScenario04_Reconfigure |
| FR-VC-0407 | Reset | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0407 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0408 | Suspend | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0408 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0409 | Delete | ● | 적용 | WP 2.1 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0409 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestScenario05_Delete |
| FR-VC-0410 | Rename | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0410 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0411 | Move | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0411 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0412 | Snapshot Create | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0412 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0413 | Snapshot Revert | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0413 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0414 | Snapshot Delete | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0414 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0415 | Template Convert | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0415 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0416 | Guest Customization | – | 적용 | WP 4.11 | Phase 4b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0416 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestVMLifecycleExtended |
| FR-VC-0417 | 사전조건 검증 | ● | 적용 | WP 2.5 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0417 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-14 | 통과: TestNegativeSOAP, TestScenario04_Reconfigure, TestScenario05_Delete, TestVMLifecycleExtended |
| FR-VC-0418 | 버전별 동작 규칙 | ● | 적용 | WP 2.1 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0418 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestScenario04_Reconfigure, TestVMLifecycleExtended |
| FR-VC-0501 | Task MoRef 발급 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0501 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-04 | 통과: TestScenario03_CreateVM |
| FR-VC-0502 | Task 상태 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0502 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-04 | 통과: TestScenario03_CreateVM |
| FR-VC-0503 | Task 시각·진행률 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0503 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-04 | 통과: TestScenario03_CreateVM |
| FR-VC-0504 | Task 결과·Fault | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0504 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-04 | 통과: TestNegativeSOAP, TestScenario03_CreateVM |
| FR-VC-0505 | Task 완료 효과 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0505 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-04 | 통과: TestScenario03_CreateVM |
| FR-VC-0506 | Task 조회·대기 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0506 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-04 | 통과: TestScenario03_CreateVM |
| FR-VC-0507 | 속성 조회 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0507 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-05 | 통과: TestContainerViewAndPagedRetrieve |
| FR-VC-0508 | 필터 관리 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0508 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-05 | 통과: TestFilterAndWaitForUpdatesEx |
| FR-VC-0509 | 변경 감시 | ● | 적용 | WP 0.5, WP 2.7, WP 4.6 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0509 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-05 | 통과: TestFilterAndWaitForUpdatesEx, TestPropertyCollectorVersions, TestScenario02_InventorySync |
| FR-VC-0510 | API 간 상태 일치 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0510 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-03 | 통과: TestScenario03_CreateVM |
| FR-VC-0511 | vcsim 기능 재사용 | ● | 적용 | WP 0.5, WP 2.7 | Phase 2a | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0511 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 자체 검토: internal/vcenter — vcsim 동작 재사용, 누락분만 보완: Task 객체·recentTask 정리(R-22), 재구성 통지(R-23), 폴더 내 이름 중복·익명 조회 제한·용량·전원 규칙(adapter.go) |
| FR-VC-0601 | Compatibility Profile 구조 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0601 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestStartupRefusals |
| FR-VC-0602 | 프로파일 포함 항목 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0602 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestStartupRefusals |
| FR-VC-0603 | 버전 문자열 위장 금지 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0603 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestMethodNotInSessionVersion |
| FR-VC-0604 | 버전 메타데이터 로드 | ● | 적용 | WP 0.12, WP 1.2 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0604 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-13 | 통과: TestVersionAwareSerialization |
| FR-VC-0605 | 협상 버전 결정 | ● | 적용 | WP 0.12, WP 1.2, WP 2.12 | Phase 2b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0605 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-13 | 통과: TestGovcVersionDowngrade, TestPyVmomi, TestVersionAwareSerialization, TestVersionDifferential |
| FR-VC-0606 | 버전 인식 응답 필터링 | ● | 적용 | WP 0.12, WP 1.2, WP 2.12 | Phase 2b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0606 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | PoC-13 | 통과: TestFilterUnit, TestGovcVersionDowngrade, TestSOAPSchemaContract, TestVersionAwareSerialization, TestVersionDifferential |
| FR-VC-0607 | 버전 기반 요청 검증 | ● | 적용 | WP 0.12, WP 1.2, WP 2.12 | Phase 2b | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0607 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestMethodNotInSessionVersion, TestVersionDifferential, TestVersionDifferentialMethods |
| FR-VC-0608 | REST·NSX 스펙 기반 판단 | ● | 적용 | WP 1.2 | Phase 1 | Contract·Stateful·선정 SDK(govmomi) | T-FR-VC-0608 | AC-02~04, AC-06, AC-07, AC-16, AC-17 | — | 통과: TestScenario01_UnsupportedAndRouting |
| TR-COV-0101 | Coverage Matrix 관리 | ● | 적용 | WP 0.7, WP 1.13, WP 5.5 | Phase 1 | 추적표·Coverage 정합성 검사 | T-TR-COV-0101 | AC-03, AC-11~17, AC-19 | — | 통과: TestCoverageReport |
| TR-COV-0102 | 필수 열 | ● | 적용 | WP 0.7, WP 1.13, WP 5.5 | Phase 1 | 추적표·Coverage 정합성 검사 | T-TR-COV-0102 | AC-03, AC-11~17, AC-19 | — | 통과: TestCoverageReport |
| TR-COV-0103 | 요구사항 검증 추적표 | ● | 적용 | WP 0.17, WP 2.9, WP 5.8 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0103 | AC-03, AC-11~17, AC-19 | — | 자체 검토: .agentic/scripts/gen_trace.py — PRD-1~3 전체 ID를 TEST_STRATEGY §7에 작업·단계·검증 방법·시험/검토 ID·AC·결과로 자동 연결, 중복·미정의·WP 누락 자동 검사(rows=281 dup=0 undefined=0 no_wp=0) |
| TR-COV-0201 | 구현 상태 분류 | ● | 적용 | WP 1.2, WP 1.13 | Phase 1 | 추적표·Coverage 정합성 검사 | T-TR-COV-0201 | AC-03, AC-11~17, AC-19 | — | 자체 검토: internal/profile/profile.go Status, verification.yaml |
| TR-COV-0202 | 구현·검증 상태 구분 | ● | 적용 | WP 1.2, WP 1.13 | Phase 1 | 추적표·Coverage 정합성 검사 | T-TR-COV-0202 | AC-03, AC-11~17, AC-19 | — | 자체 검토: 40-test/cmd/evidence (Integration Tested까지만 승격) |
| TR-COV-0301 | Contract Test | ● | 적용 | WP 2.9 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0301 | AC-03, AC-11~17, AC-19 | — | 통과: TestNSXPhase4bResponseSchema, TestNSXResponseSchemaContract, TestSOAPSchemaContract, TestVersionAwareSerialization |
| TR-COV-0302 | Stateful Test | ● | 적용 | WP 2.9 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0302 | AC-03, AC-11~17, AC-19 | — | 통과: TestDeclarativeScenarios, TestGovcLifecycle |
| TR-COV-0303 | MVP Negative Test | ● | 적용 | WP 2.9 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0303 | AC-03, AC-11~17, AC-19 | — | 통과: TestNegativeNSX, TestNegativeREST, TestNegativeSOAP, TestNegativeUnsupported |
| TR-COV-0304 | 선정 SDK 자체 계약 테스트 | ● | 적용 | WP 2.9, WP 2.12 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0304 | AC-03, AC-11~17, AC-19 | PoC-20 | 통과: TestCheckerDetectsViolations, TestSOAPSchemaContract |
| TR-COV-0305 | 범용 SDK 통합 테스트 | ● | 적용 | WP 2.9 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0305 | AC-03, AC-11~17, AC-19 | — | 통과: TestAnsiblePlaybook, TestGovcLifecycle, TestGovcVersionDowngrade, TestPyVmomi |
| TR-COV-0306 | 선택 자동화 도구 자체 테스트 | – | 적용 | WP 5.3 | Phase 5 | 추적표·Coverage 정합성 검사 | T-TR-COV-0306 | AC-03, AC-11~17, AC-19 | — | 통과: TestAnsiblePlaybook, TestClusterGetResourceUsage, TestClusterVsanConfigInfo |
| TR-COV-0307 | MVP 시뮬레이터 End-to-End Test | ● | 적용 | WP 2.9 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0307 | AC-03, AC-11~17, AC-19 | — | 통과: TestDeclarativeScenarios, TestScenario01_ConnectAndDiscover |
| TR-COV-0308 | 확장 Negative Test | – | 적용 | WP 4.8 | Phase 4a | 추적표·Coverage 정합성 검사 | T-TR-COV-0308 | AC-03, AC-11~17, AC-19 | — | 통과: TestRBAC, TestScenario09_TaskFailureRecovery, TestScenario10_SessionExpiry, TestSemanticAndInfraFaults |
| TR-COV-0309 | MVP 이후 시뮬레이터 End-to-End Test | – | 적용 | WP 4.8 | Phase 4a | 추적표·Coverage 정합성 검사 | T-TR-COV-0309 | AC-03, AC-11~17, AC-19 | — | 통과: TestDeclarativeScenarios, TestScenario08_CrossSystemWorkflow, TestScenario09_TaskFailureRecovery, TestScenario10_SessionExpiry |
| TR-COV-0403 | 자체 테스트의 계약 근거 | ● | 적용 | WP 0.15, WP 2.12, WP 5.8 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0403 | AC-03, AC-11~17, AC-19 | — | 통과: TestSOAPSchemaContract |
| TR-COV-0409 | 자체 검증 상한 명시 | ● | 적용 | WP 0.15, WP 2.12, WP 5.8 | Phase 2b | 추적표·Coverage 정합성 검사 | T-TR-COV-0409 | AC-03, AC-11~17, AC-19 | — | 통과: TestCoverageReport, TestDeclarativeScenarios, TestVerificationCapDisplayed |
| TR-E2E-0101 | Scenario 01 절차 | ● | 적용 | 시나리오 게이트 | Phase 1 | 자체 E2E(성공 기준별) | T-TR-E2E-0101 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario01_ConnectAndDiscover |
| TR-E2E-0102 | Scenario 01 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 1 | 자체 E2E(성공 기준별) | T-TR-E2E-0102 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario01_ConnectAndDiscover |
| TR-E2E-0103 | Scenario 01 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 1 | 자체 E2E(성공 기준별) | T-TR-E2E-0103 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario01_ConnectAndDiscover |
| TR-E2E-0104 | Scenario 01 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 1 | 자체 E2E(성공 기준별) | T-TR-E2E-0104 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario01_ConnectAndDiscover |
| TR-E2E-0201 | Scenario 02 절차 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0201 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario02_InventorySync |
| TR-E2E-0202 | Scenario 02 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0202 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario02_InventorySync |
| TR-E2E-0203 | Scenario 02 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0203 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario02_InventorySync |
| TR-E2E-0204 | Scenario 02 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0204 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario02_InventorySync |
| TR-E2E-0205 | Scenario 02 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0205 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario02_InventorySync |
| TR-E2E-0301 | Scenario 03 절차 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0301 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario03_CreateVM |
| TR-E2E-0302 | Scenario 03 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0302 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario03_CreateVM |
| TR-E2E-0303 | Scenario 03 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0303 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario03_CreateVM |
| TR-E2E-0304 | Scenario 03 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0304 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario03_CreateVM |
| TR-E2E-0305 | Scenario 03 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0305 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario03_CreateVM |
| TR-E2E-0401 | Scenario 04 절차 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0401 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario04_Reconfigure |
| TR-E2E-0402 | Scenario 04 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0402 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario04_Reconfigure |
| TR-E2E-0403 | Scenario 04 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0403 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario04_Reconfigure |
| TR-E2E-0501 | Scenario 05 절차 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0501 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario05_Delete |
| TR-E2E-0502 | Scenario 05 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0502 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario05_Delete |
| TR-E2E-0503 | Scenario 05 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0503 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario05_Delete |
| TR-E2E-0504 | Scenario 05 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0504 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario05_Delete |
| TR-E2E-0601 | Scenario 06 절차 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0601 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario06_SegmentCreate |
| TR-E2E-0602 | Scenario 06 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0602 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario06_SegmentCreate |
| TR-E2E-0603 | Scenario 06 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0603 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario06_SegmentCreate |
| TR-E2E-0604 | Scenario 06 성공 기준 | ● | 적용 | 시나리오 게이트 | Phase 2a | 자체 E2E(성공 기준별) | T-TR-E2E-0604 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario06_SegmentCreate |
| TR-E2E-0701 | Scenario 07 절차 | – | 적용 | 시나리오 게이트 | Phase 4b | 자체 E2E(성공 기준별) | T-TR-E2E-0701 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario07_SecurityPolicyChange |
| TR-E2E-0702 | Scenario 07 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4b | 자체 E2E(성공 기준별) | T-TR-E2E-0702 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario07_SecurityPolicyChange |
| TR-E2E-0703 | Scenario 07 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4b | 자체 E2E(성공 기준별) | T-TR-E2E-0703 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario07_SecurityPolicyChange |
| TR-E2E-0704 | Scenario 07 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4b | 자체 E2E(성공 기준별) | T-TR-E2E-0704 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario07_SecurityPolicyChange |
| TR-E2E-0801 | Scenario 08 절차 | – | 적용 | 시나리오 게이트 | Phase 3 | 자체 E2E(성공 기준별) | T-TR-E2E-0801 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario08_CrossSystemWorkflow |
| TR-E2E-0802 | Scenario 08 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 3 | 자체 E2E(성공 기준별) | T-TR-E2E-0802 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario08_CrossSystemWorkflow |
| TR-E2E-0803 | Scenario 08 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 3 | 자체 E2E(성공 기준별) | T-TR-E2E-0803 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario08_CrossSystemWorkflow |
| TR-E2E-0804 | Scenario 08 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 3 | 자체 E2E(성공 기준별) | T-TR-E2E-0804 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario08_CrossSystemWorkflow |
| TR-E2E-0901 | Scenario 09 절차 | – | 적용 | 시나리오 게이트 | Phase 4a | 자체 E2E(성공 기준별) | T-TR-E2E-0901 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario09_TaskFailureRecovery |
| TR-E2E-0902 | Scenario 09 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4a | 자체 E2E(성공 기준별) | T-TR-E2E-0902 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario09_TaskFailureRecovery |
| TR-E2E-0903 | Scenario 09 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4a | 자체 E2E(성공 기준별) | T-TR-E2E-0903 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario09_TaskFailureRecovery |
| TR-E2E-0904 | Scenario 09 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4a | 자체 E2E(성공 기준별) | T-TR-E2E-0904 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario09_TaskFailureRecovery |
| TR-E2E-1001 | Scenario 10 절차 | – | 적용 | 시나리오 게이트 | Phase 4a | 자체 E2E(성공 기준별) | T-TR-E2E-1001 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario10_SessionExpiry |
| TR-E2E-1002 | Scenario 10 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4a | 자체 E2E(성공 기준별) | T-TR-E2E-1002 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario10_SessionExpiry |
| TR-E2E-1003 | Scenario 10 성공 기준 | – | 적용 | 시나리오 게이트 | Phase 4a | 자체 E2E(성공 기준별) | T-TR-E2E-1003 | AC-02, AC-04, AC-05, AC-11, AC-17 | — | 통과: TestScenario10_SessionExpiry |
| TR-SCN-0101 | Environment Create | ● | 적용 | WP 1.8 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0101 | AC-10~12, AC-19 | — | 통과: TestResetDeterminism |
| TR-SCN-0102 | Environment Reset | ● | 적용 | WP 1.8 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0102 | AC-10~12, AC-19 | — | 통과: TestFixtureCache, TestResetDeterminism |
| TR-SCN-0103 | Seed Apply | ● | 적용 | WP 1.8 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0103 | AC-10~12, AC-19 | — | 통과: TestResetDeterminism |
| TR-SCN-0104 | State Snapshot | – | 적용 | WP 5.2 | Phase 5 | 관리 API 기능 시험 | T-TR-SCN-0104 | AC-10~12, AC-19 | — | 통과: TestSnapshotRefusalsAndConsistency, TestSnapshotRestoreRoundTrip |
| TR-SCN-0105 | Snapshot Restore | – | 적용 | WP 5.2 | Phase 5 | 관리 API 기능 시험 | T-TR-SCN-0105 | AC-10~12, AC-19 | — | 통과: TestEventsSurviveRestore, TestRestoreChainFromLoadedModel, TestSnapshotRestoreRoundTrip |
| TR-SCN-0106 | Scenario Start | – | 적용 | WP 5.1 | Phase 5 | 관리 API 기능 시험 | T-TR-SCN-0106 | AC-10~12, AC-19 | — | 통과: TestScenarioRunnerAdminAPI |
| TR-SCN-0107 | Scenario Stop | – | 적용 | WP 5.1 | Phase 5 | 관리 API 기능 시험 | T-TR-SCN-0107 | AC-10~12, AC-19 | — | 통과: TestScenarioRunnerAdminAPI |
| TR-SCN-0108 | Virtual Clock Control | – | 적용 | WP 4.10 | Phase 4a | 관리 API 기능 시험 | T-TR-SCN-0108 | AC-10~12, AC-19 | — | 통과: TestScenario10_SessionExpiry, TestTaskDelayAndVirtualClock, TestVirtualClock |
| TR-SCN-0109 | Fault Rule Add/Remove | – | 적용 | WP 3.8, WP 4.3 | Phase 4a | 관리 API 기능 시험 | T-TR-SCN-0109 | AC-10~12, AC-19 | — | 통과: TestSemanticAndInfraFaults, TestTransportFaultRules |
| TR-SCN-0110 | Event Inspection | – | 적용 | WP 5.1 | Phase 5 | 관리 API 기능 시험 | T-TR-SCN-0110 | AC-10~12, AC-19 | — | 통과: TestEventInspection, TestScenarioRunnerAdminAPI, TestVCenterEventArchive |
| TR-SCN-0111 | API Call Trace | ● | 적용 | WP 1.9 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0111 | AC-10~12, AC-19 | — | 통과: TestCallTrace, TestDeclarativeScenarios |
| TR-SCN-0112 | Coverage Report | – | 적용 | WP 5.5 | Phase 5 | 관리 API 기능 시험 | T-TR-SCN-0112 | AC-10~12, AC-19 | — | 통과: TestCoverageReport |
| TR-SCN-0201 | 선언형 환경 정의 | ● | 적용 | WP 1.8 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0201 | AC-10~12, AC-19 | — | 통과: TestDeterministicVMPlacement, TestFixtureScaleChange, TestFixtureTopology |
| TR-SCN-0202 | 예시 환경 지원 | ● | 적용 | WP 1.8 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0202 | AC-10~12, AC-19 | — | 통과: TestResetDeterminism |
| TR-SCN-0203 | 규모 변경 | ● | 적용 | WP 1.8 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0203 | AC-10~12, AC-19 | — | 통과: TestFixtureScaleChange |
| TR-SCN-0204 | 다수 논리 리소스 | ● | 적용 | WP 1.8 | Phase 1 | 관리 API 기능 시험 | T-TR-SCN-0204 | AC-10~12, AC-19 | — | 통과: TestFixtureScaleChange |
| TR-SCN-0301 | MVP 시나리오 단계 구성 | ● | 적용 | WP 2.9, WP 5.1 | Phase 2b | 관리 API 기능 시험 | T-TR-SCN-0301 | AC-10~12, AC-19 | — | 통과: TestDeclarativeScenarios, TestJSONPath |
| TR-SCN-0302 | 선언형 시나리오 관리 | ● | 적용 | WP 2.9, WP 5.1 | Phase 2b | 관리 API 기능 시험 | T-TR-SCN-0302 | AC-10~12, AC-19 | — | 통과: TestDeclarativeScenarios, TestLoadValidation |
| TR-SCN-0303 | 장애 시나리오 단계 확장 | – | 적용 | WP 4.3, WP 4.8, WP 5.1 | Phase 5 | 관리 API 기능 시험 | T-TR-SCN-0303 | AC-10~12, AC-19 | — | 통과: TestDeclarativeScenarios, TestScenario09_TaskFailureRecovery, TestScenarioRunnerAdminAPI |
| NFR-0101 | VMware 제품 설치 불필요 | ● | 적용 | WP 1.1 | Phase 1 | Footprint·설치 시험 | T-NFR-0101 | AC-01, AC-18 | — | 통과: TestScenario01_ConnectAndDiscover |
| NFR-0102 | 실제 VM 실행 불필요 | ● | 적용 | WP 1.1 | Phase 1 | Footprint·설치 시험 | T-NFR-0102 | AC-01, AC-18 | — | 통과: TestScenario01_ConnectAndDiscover |
| NFR-0103 | 단일 PC 운영 | ● | 적용 | WP 1.1 | Phase 1 | Footprint·설치 시험 | T-NFR-0103 | AC-01, AC-18 | — | 자체 검토: PoC-19·footprint (단일 PC 실행) |
| NFR-0104 | 네이티브 실행 기본 | ● | 적용 | WP 1.1 | Phase 1 | Footprint·설치 시험 | T-NFR-0104 | AC-01, AC-18 | PoC-19 | 통과: TestFootprint |
| NFR-0105 | 외부 의존성 최소화 | ● | 적용 | WP 1.1 | Phase 1 | Footprint·설치 시험 | T-NFR-0105 | AC-01, AC-18 | — | 통과: TestFootprint |
| NFR-0106 | 기본 저장소 | ● | 적용 | WP 2.6 | Phase 2a | Footprint·설치 시험 | T-NFR-0106 | AC-01, AC-18 | — | 통과: TestNSXTransaction |
| NFR-0107 | 리소스 예산 = 완료 기준 | ● | 적용 | WP 0.14, WP 1.10 | Phase 1 | Footprint·설치 시험 | T-NFR-0107 | AC-01, AC-18 | PoC-19 | 통과: TestFootprint |
| NFR-0108 | 기준 Fixture | ● | 적용 | WP 0.14, WP 1.8 | Phase 1 | Footprint·설치 시험 | T-NFR-0108 | AC-01, AC-18 | PoC-19 | 통과: TestResetDeterminism |
| NFR-0109 | 유휴 메모리 | ● | 적용 | WP 0.14 | Phase 0a | Footprint·설치 시험 | T-NFR-0109 | AC-01, AC-18 | PoC-17, PoC-19 | 통과: TestFootprint |
| NFR-0110 | 기동 시간 | ● | 적용 | WP 0.14 | Phase 0a | Footprint·설치 시험 | T-NFR-0110 | AC-01, AC-18 | PoC-17, PoC-19 | 통과: TestFixtureCache, TestFootprint |
| NFR-0111 | 유휴 CPU | ● | 적용 | WP 0.14 | Phase 0a | Footprint·설치 시험 | T-NFR-0111 | AC-01, AC-18 | PoC-19 | 통과: TestFootprint |
| NFR-0112 | 설치 크기 | ● | 적용 | WP 0.14 | Phase 0a | Footprint·설치 시험 | T-NFR-0112 | AC-01, AC-18 | PoC-19 | 통과: TestFootprint |
| NFR-0113 | 사전 설치 조건 | ● | 적용 | WP 0.14 | Phase 0a | Footprint·설치 시험 | T-NFR-0113 | AC-01, AC-18 | PoC-19 | 통과: TestFootprint |
| NFR-0114 | `lite` 실행 프로필 (기본) | ● | 적용 | WP 1.1 | Phase 1 | Footprint·설치 시험 | T-NFR-0114 | AC-01, AC-18 | — | 통과: TestFootprint |
| NFR-0115 | `full` 실행 프로필 | – | 적용 | WP 5.6, WP 5.9 | Phase 5 | Footprint·설치 시험 | T-NFR-0115 | AC-01, AC-18 | — | 통과: TestBenchmark, TestFullProfileRetention, TestSessionObjectLocksReleased, TestSoakRetention |
| NFR-0116 | 규모 확장 테스트 | – | 적용 | WP 5.6 | Phase 5 | Footprint·설치 시험 | T-NFR-0116 | AC-01, AC-18 | — | 통과: TestBenchmark |
| NFR-0117 | 규모 목표 결정 방식 | – | 적용 | WP 5.6 | Phase 5 | Footprint·설치 시험 | T-NFR-0117 | AC-01, AC-18 | — | 통과: TestBenchmark |
| NFR-0201 | 동시 요청 무결성 | ● | 적용 | WP 2.6 | Phase 2a | 동시성·무결성·복구 시험 | T-NFR-0201 | AC-06, AC-09, AC-10, AC-19 | PoC-21 | 통과: TestNSXConcurrentWrites |
| NFR-0202 | 중복 생성 방지 | ● | 적용 | WP 2.6 | Phase 2a | 동시성·무결성·복구 시험 | T-NFR-0202 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestNegativeSOAP |
| NFR-0203 | 안정적 ID 관리 | ● | 적용 | WP 2.6 | Phase 2a | 동시성·무결성·복구 시험 | T-NFR-0203 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestIDKinds, TestResetDeterminism |
| NFR-0204 | SQLite Transaction | ● | 적용 | WP 2.6 | Phase 2a | 동시성·무결성·복구 시험 | T-NFR-0204 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestNSXTransaction |
| NFR-0205 | 재기동 후 상태 복원 | – | 적용 | WP 5.2 | Phase 5 | 동시성·무결성·복구 시험 | T-NFR-0205 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestRestartRecovery |
| NFR-0206 | Snapshot/Restore | – | 적용 | WP 5.2 | Phase 5 | 동시성·무결성·복구 시험 | T-NFR-0206 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestSnapshotRestoreRoundTrip |
| NFR-0207 | 상태 불일치 검출 | – | 적용 | WP 5.2 | Phase 5 | 동시성·무결성·복구 시험 | T-NFR-0207 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestSnapshotRefusalsAndConsistency |
| NFR-0208 | 작업 재처리 정책 | – | 적용 | WP 5.2 | Phase 5 | 동시성·무결성·복구 시험 | T-NFR-0208 | AC-06, AC-09, AC-10, AC-19 | — | 통과: TestFullRetentionAndBackup, TestSnapshotRefusalsAndConsistency |
| NFR-0301 | 루프백 기본 바인딩 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0301 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestVersionAndModeDefaults |
| NFR-0302 | HTTPS 지원 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0302 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestFullProfileRetention, TestListenerTLSMaxVersion, TestMaskBody, TestScenario01_ConnectAndDiscover |
| NFR-0303 | 인증서 교체 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0303 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestCertificateRotation |
| NFR-0304 | Credential 관리 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0304 | AC-08, AC-14, AC-18, AC-19 | — | 자체 검토: internal/sim/users.go, admin-token (데이터 디렉터리 파일) |
| NFR-0305 | 관리자 API 인증 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0305 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestAdminAPIAuth |
| NFR-0306 | 감사 로그 | – | 적용 | WP 3.5 | Phase 3 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0306 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestAuditAndChangeCause |
| NFR-0307 | 민감 정보 마스킹 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0307 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestAuditAndChangeCause, TestLogFormats, TestMaskHeaders |
| NFR-0308 | 외부 네트워크 접근 제한 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0308 | AC-08, AC-14, AC-18, AC-19 | — | 자체 검토: 코드 검토: 외부로 나가는 연결 없음, 기본 바인딩 루프백 |
| NFR-0309 | 운영 환경 혼동 방지 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0309 | AC-08, AC-14, AC-18, AC-19 | — | 자체 검토: NFR-0314 .test 도메인 강제(config_test), 시뮬레이터 CA 별도 |
| NFR-0310 | 운영 자격 증명 재사용 금지 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0310 | AC-08, AC-14, AC-18, AC-19 | — | 자체 검토: users.json 무작위 생성, 운영 자격 증명 입력 경로 없음 |
| NFR-0311 | 외부 바인딩 통제 | ○ | 적용(○ 채택) | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0311 | AC-08, AC-14, AC-18, AC-19 | — | 자체 검토: 적용 제외 — D6: 토폴로지 ①(같은 PC)만 MVP, 외부 바인딩은 MVP 이후 |
| NFR-0313 | 가상 호스트 도메인 설정 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0313 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestDomainRules |
| NFR-0314 | 예약 TLD 사용 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0314 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestDomainRules |
| NFR-0315 | 인증서 지문 안정성 | ● | 적용 | WP 1.11 | Phase 1 | 보안 검사(바인딩·TLS·인증·마스킹) | T-NFR-0315 | AC-08, AC-14, AC-18, AC-19 | — | 통과: TestCertificateThumbprintStable |
| NFR-0401 | API 로그 | ● | 적용 | WP 1.9 | Phase 1 | 로그 형식·상관관계 검사 | T-NFR-0401 | AC-07, AC-11~13, AC-19 | — | 통과: TestCallTrace, TestLogFormats |
| NFR-0402 | 작업 추적 정보 | ● | 적용 | WP 1.9 | Phase 1 | 로그 형식·상관관계 검사 | T-NFR-0402 | AC-07, AC-11~13, AC-19 | — | 통과: TestTaskRetentionAndLogs |
| NFR-0403 | 상태·오류 로그 | ● | 적용 | WP 1.9 | Phase 1 | 로그 형식·상관관계 검사 | T-NFR-0403 | AC-07, AC-11~13, AC-19 | — | 통과: TestTransportFaultRules |
| NFR-0404 | 테스트 실행 로그 | – | 적용 | WP 5.1 | Phase 5 | 로그 형식·상관관계 검사 | T-NFR-0404 | AC-07, AC-11~13, AC-19 | — | 통과: TestDeclarativeScenarios, TestScenarioRunnerAdminAPI |
| NFR-0405 | 로그 형식 | ● | 적용 | WP 1.9 | Phase 1 | 로그 형식·상관관계 검사 | T-NFR-0405 | AC-07, AC-11~13, AC-19 | — | 통과: TestLogFormats |
| NFR-0501 | 누적 데이터 상한 | ● | 적용 | WP 1.11, WP 5.9 | Phase 1 | 보존 한도·장시간 실행 시험 | T-NFR-0501 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestFullProfileRetention, TestGatewayIdleTimeout, TestSessionEndLocksReleased, TestSessionObjectLocksReleased, TestSoakRetention, TestTraceDBRotation, TestTraceRingBuffer, TestTransitionRetention |
| NFR-0502 | Event / Alarm 보존 | ● | 적용 | WP 1.11, WP 5.9 | Phase 1 | 보존 한도·장시간 실행 시험 | T-NFR-0502 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestEventRetention |
| NFR-0503 | Audit / Unsupported API Log 보존 | ● | 적용 | WP 1.11, WP 5.9 | Phase 1 | 보존 한도·장시간 실행 시험 | T-NFR-0503 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestFullProfileRetention, TestFullRetentionAndBackup, TestUnsupportedKeyCap, TestVCenterEventArchive |
| NFR-0504 | API Trace 보존 | ● | 적용 | WP 1.11, WP 5.9 | Phase 1 | 보존 한도·장시간 실행 시험 | T-NFR-0504 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestCallTrace, TestFullProfileRetention, TestMaskBody, TestSoakRetention, TestTraceDBRotation, TestTraceRingBuffer |
| NFR-0505 | 완료된 Task 보존 | ● | 적용 | WP 1.11, WP 5.9 | Phase 1 | 보존 한도·장시간 실행 시험 | T-NFR-0505 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestTaskRetentionAndLogs |
| NFR-0506 | Operation Transition 보존 | ● | 적용 | WP 1.11, WP 5.9 | Phase 1 | 보존 한도·장시간 실행 시험 | T-NFR-0506 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestFullProfileRetention, TestFullRetentionAndBackup, TestTaskRetentionAndLogs, TestTransitionRetention |
| NFR-0507 | 성능 메트릭 미저장 | – | 적용 | WP 3.7 | Phase 3 | 보존 한도·장시간 실행 시험 | T-NFR-0507 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestVirtualMetrics |
| NFR-0508 | 보존 한도 설정 | ● | 적용 | WP 1.11, WP 5.9 | Phase 1 | 보존 한도·장시간 실행 시험 | T-NFR-0508 | AC-12, AC-14, AC-18, AC-19 | — | 통과: TestForcedKillTempCleanup, TestFullProfileRetention, TestFullRetentionAndBackup, TestRetentionLimits, TestSoakRetention, TestTaskRetentionAndLogs, TestTraceDBRotation |
| NFR-ARC-0101 | 프로세스 구성안 확정 | ● | 적용 | WP 0.11 | Phase 0a | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0101 | AC-01, AC-11, AC-17~19 | — | 자체 검토: PRD-4 D3, ARCHITECTURE.md §8 |
| NFR-ARC-0102 | vCenter SOAP | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0102 | AC-01, AC-11, AC-17~19 | — | 자체 검토: internal/vcenter |
| NFR-ARC-0103 | vCenter REST | ● | 적용 | WP 1.7 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0103 | AC-01, AC-11, AC-17~19 | — | 자체 검토: internal/vcenter/rest.go |
| NFR-ARC-0104 | vCenter 보조 서비스 | ○ | 적용(○ 채택) | WP 4.11 | Phase 4b | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0104 | AC-01, AC-11, AC-17~19 | — | 통과: TestContentLibraryContract, TestLookupService, TestPBMStoragePolicy |
| NFR-ARC-0105 | 버전 인식 직렬화 | ● | 적용 | WP 1.2 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0105 | AC-01, AC-11, AC-17~19 | — | 통과: TestVersionAwareSerialization |
| NFR-ARC-0106 | State Repository | ● | 적용 | WP 2.6 | Phase 2a | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0106 | AC-01, AC-11, AC-17~19 | — | 통과: TestNSXConcurrentWrites, TestNSXTransaction |
| NFR-ARC-0107 | HTTPS Gateway | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0107 | AC-01, AC-11, AC-17~19 | PoC-12 | 통과: TestScenario01_ConnectAndDiscover |
| NFR-ARC-0108 | Local Deployment | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0108 | AC-01, AC-11, AC-17~19 | — | 통과: TestFootprint |
| NFR-ARC-0109 | Test | ● | 적용 | WP 2.9 | Phase 2b | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0109 | AC-01, AC-11, AC-17~19 | — | 통과: TestGovcLifecycle, TestPyVmomi, TestSOAPSchemaContract |
| NFR-ARC-0110 | API Schema Validation | ● | 적용 | WP 1.2 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0110 | AC-01, AC-11, AC-17~19 | — | 자체 검토: internal/profile (vim-typeinfo), internal/nsx (OpenAPI) |
| NFR-ARC-0111 | CI | ● | 적용 | WP 1.10, WP 5.7 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0111 | AC-01, AC-11, AC-17~19 | — | 이월: Phase 5에 CI 워크플로(.github/workflows/ci.yml: 호스트 러너 단위·설치·교차 빌드, 자체 호스트 러너 통합·Footprint·증거)를 작성했으나 저장소(git 원격)·러너가 없어 실행하지 못함 — 사용자 조치 필요. 같은 내용은 make regression으로 로컬 실행 |
| NFR-ARC-0112 | Configuration | ● | 적용 | WP 1.2 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0112 | AC-01, AC-11, AC-17~19 | — | 자체 검토: data/*.yaml |
| NFR-ARC-0113 | Python 테스트 도구 | – | 적용 | WP 2.9 | Phase 2b | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0113 | AC-01, AC-11, AC-17~19 | — | 통과: TestPyVmomi |
| NFR-ARC-0201 | 디렉터리 구성 | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0201 | AC-01, AC-11, AC-17~19 | — | 자체 검토: 저장소 디렉터리 구성 |
| NFR-ARC-0202 | 주요 설정 파일 위치 | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0202 | AC-01, AC-11, AC-17~19 | — | 자체 검토: Makefile, 30-implementation/go.mod, 40-test/go.mod |
| NFR-ARC-0203 | 빌드·배포 구조 조정 | – | 적용 | WP 5.7 | Phase 5 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0203 | AC-01, AC-11, AC-17~19 | — | 자체 검토: Phase 5 검토: 단일 Go 실행파일(cgo 없음) 유지, 50-deployment/dist.sh·Makefile dist/cross(5개 대상 교차 빌드 확인), CI release 작업, ARCHITECTURE.md §배포 구조. Python은 도구(.agentic)·선택 SDK 시험에만 사용 |
| NFR-ARC-0301 | 프로세스 구성 (권고안 A) | ● | 적용 | WP 0.11, WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0301 | AC-01, AC-11, AC-17~19 | — | 통과: TestScenario01_ConnectAndDiscover |
| NFR-ARC-0303 | 별도 서비스 배제 | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0303 | AC-01, AC-11, AC-17~19 | — | 자체 검토: 단일 실행파일(별도 Proxy·Bridge·Controller 없음) |
| NFR-ARC-0304 | Gateway 라우팅 대상 | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0304 | AC-01, AC-11, AC-17~19 | PoC-12 | 통과: TestScenario01_ConnectAndDiscover, TestScenario01_UnsupportedAndRouting |
| NFR-ARC-0305 | 관리 API 분리 | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0305 | AC-01, AC-11, AC-17~19 | — | 통과: TestAdminAPIAuth |
| NFR-ARC-0306 | 포트·바인딩 조정 | ● | 적용 | WP 1.1 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0306 | AC-01, AC-11, AC-17~19 | — | 자체 검토: config --vcenter-addr/--nsx-addr/--admin-addr |
| NFR-ARC-0307 | 루프백 별칭 바인딩 | ● | 적용 | WP 1.11 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0307 | AC-01, AC-11, AC-17~19 | PoC-19 | 자체 검토: config 기본값 127.0.0.2/127.0.0.3, PoC-19 |
| NFR-ARC-0308 | 로컬 환경 설치 자동화 | ● | 적용 | WP 1.11, WP 5.10 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0308 | AC-01, AC-11, AC-17~19 | — | 통과: TestInstallScript |
| NFR-ARC-0309 | 지원 배포 토폴로지 | ○ | 적용(○ 채택) | WP 1.11 | Phase 1 | 아키텍처 검토·설치·CI 시험 | T-NFR-ARC-0309 | AC-01, AC-11, AC-17~19 | — | 통과: TestVersionAndModeDefaults |

<!-- TRACE:END -->
