# API_COVERAGE_MATRIX — API Coverage 기준선 (Phase 0b 잠정)

> 관련: PRD-2 §12.1(TR-COV-0101, TR-COV-0102), PRD-1 FR-NSX-0233·FR-API-0404, PRD-4 WP 0.2·0.3·0.7·Task 3, D2 · 작성일 2026-10-10  
> 상태: **잠정 기준선(D2).** 승인 시뮬레이션 범위는 Phase 2b(WP 2.12)에서 확정한다. 채택 범위의 API 상태는 이 문서가 아니라 구현 후 `verification.yaml`과 Coverage Report로 관리한다.

## 1. 선정 절차 (Task 3)

1. **후보:** 공식 계약(pyVmomi 추출 메타데이터 = vim25 WSDL 대응, NSX-T 3.2.1 OpenAPI, vSphere Automation 문서)과 CMP 참고 목록(참고 문서 1.1)에서 후보를 찾는다.
2. **전개:** 선정 기능을 정상·오류·조회·인증·비동기·교차 시스템 API까지 전개한다(예: VM 생성 → `CreateVM_Task` + Task 조회 + `WaitForUpdatesEx` + 인벤토리 조회).
3. **기록:** 채택·제외 근거, 관련 FR, 테스트, 계약 근거를 행마다 기록한다.
4. **보완:** 자체 테스트 실행 Trace와 Unsupported API Log로 빠진 API를 찾는다. 관측되지 않은 API를 자동으로 제외하지 않는다.
5. **CMP 상품 제약은 범위 밖이다**(D13). 기대값은 독립 근거에서 정한다.

## 2. 조사 결과 요약 (WP 0.2·0.3)

| 영역 | 조사 결과 | 원본 |
|---|---|---|
| vCenter SOAP | 주요 34개 관리 객체 타입의 7.0.3 메서드 398개 중 vcsim이 구현한 것은 152개. **MVP에 필요한 SOAP 메서드는 모두 구현되어 있다**(§3.1) | `soap-method-survey.csv` |
| vCenter REST (`/api`, 7.0 U2+) | vcsim이 제공하는 것: 구형 `/rest/com/vmware/cis/session`, Tagging, Content Library, OVF/VMTX 라이브러리 항목, VM data-sets, cluster modules. **제공하지 않는 것: `/api/session`, `/api/vcenter/vm`·`datacenter`·`cluster`·`host`·`datastore`·`network`.** MVP REST는 자체 구현(vcsim 인벤토리 뷰, FR-VC-0205)해야 한다 | vapi 시뮬레이터 소스, PoC-02·03 |
| NSX Manager/Policy | 3.2.1 공식 스펙: 3,671 오퍼레이션(Manager 1,376 + Policy 2,295). 범용 핸들러로 전 경로를 색인한다. MVP 행위 리소스는 전용 로직 필요 | `nsx-operation-inventory.csv` |
| 스펙 밖 NSX 경로 | `POST /api/session/create`, `/api/v1/spec/openapi/*`는 OpenAPI 파일에 없다. 계약 근거를 API Guide 본문에서 확보해야 한다 | — |

`*.csv` 파일은 공식 스펙·메타데이터에서 생성한 색인이다(생성: `.agentic/poc/survey_test.go`, 이 문서 작성 시 스크립트). D11 확정 전에는 저장소 공개 범위를 다시 확인한다.

## 3. MVP 후보 행 (Scenario 01~06)

열 약어: **Ver** = 도입 버전(7.0.3 프로파일 포함 여부), **vcsim** = 재사용 가능 여부(Y 재사용 / P 부분 / N 자체 구현), **근거** = TEST_STRATEGY §5 근거 ID, **상태** = TR-COV-0201 분류(현재 전부 `Not Analyzed` → 아래는 계약 정의 착수 대상). 공통 열 중 인증은 SOAP 세션 쿠키 / REST `vmware-api-session-id` / NSX Basic·세션이다.

### 3.1 vCenter SOAP (`/sdk`, urn:vim25)

| API ID | 오퍼레이션 | Ver | vcsim | 부수 효과·비동기 | 주요 Fault | 관련 FR | 시험 | 우선순위 |
|---|---|---|---|---|---|---|---|---|
| VC-SOAP-001 | `ServiceInstance.RetrieveServiceContent` | 2.0 | Y (About 프로파일 값으로 덮어씀) | — | — | FR-API-0306, FR-VC-0607 | Scenario 01 | P1 |
| VC-SOAP-002 | `SessionManager.Login` / `Logout` | 2.0 | Y | 세션 생성·삭제 | `InvalidLogin` | FR-API-0201 | Scenario 01, Negative | P1 |
| VC-SOAP-003 | `SessionManager.SessionIsActive` | 2.5 | Y | — | `NotAuthenticated` | FR-API-0203 | Contract | P1 |
| VC-SOAP-004 | `ViewManager.CreateContainerView` / `View.DestroyView` | 2.5 | Y | 세션별 View | — | FR-VC-0202 | Scenario 01·02 | P1 |
| VC-SOAP-005 | `PropertyCollector.RetrievePropertiesEx` / `ContinueRetrievePropertiesEx` | 4.1 | Y (버전 필터는 Gateway) | — | `InvalidProperty` | FR-VC-0507, FR-VC-0606 | Scenario 01~05 | P1 |
| VC-SOAP-006 | `PropertyCollector.CreateFilter` / `PropertyFilter.DestroyPropertyFilter` | 2.0 | Y | 세션별 필터 | — | FR-VC-0508 | Scenario 02 | P1 |
| VC-SOAP-007 | `PropertyCollector.WaitForUpdatesEx` / `CancelWaitForUpdates` | 4.1 | P (버전 토큰이 항상 `"-"`) | 변경 감시, Timeout | — | FR-VC-0509 | Scenario 02, Contract | P1 |
| VC-SOAP-008 | `Folder.CreateVM_Task` | 2.0 | Y + Adapter(사전조건) | Task, 인벤토리 추가 | `InsufficientResourcesFault`, `InvalidArgument`, `DuplicateName` | FR-VC-0401, FR-VC-0417 | Scenario 03 | P1 |
| VC-SOAP-009 | `VirtualMachine.ReconfigVM_Task` | 2.0 | Y + Adapter | Task, config 변경 | `InvalidPowerState`, `InvalidArgument` | FR-VC-0404, FR-VC-0417 | Scenario 04 | P1 |
| VC-SOAP-010 | `VirtualMachine.PowerOnVM_Task` / `PowerOffVM_Task` | 2.0 | Y | Task, runtime 변경 | `InvalidPowerState` | FR-VC-0405, FR-VC-0406 | Scenario 03·04 | P1 |
| VC-SOAP-011 | `VirtualMachine.Destroy_Task` | 2.0 | Y + Adapter(참조 정리) | Task, 인벤토리 제거 | `InvalidPowerState` | FR-VC-0409, FR-BE-0206 | Scenario 05 | P1 |
| VC-SOAP-012 | `Task` 속성 조회(`info.state/progress/error/result`), `TaskManager.recentTask` | 2.0 | P (완료 Task progress 0, 완료 Task 객체 미정리 → 보존 한도 정리로 보완, R-22) | — | — | FR-VC-0501 ~ FR-VC-0506, NFR-0505 | Scenario 03 | P1 |
| VC-SOAP-013 | `/sdk/vimServiceVersions.xml`, `/sdk/vimService.wsdl` | — | P (priorVersions 고정) | — | — | FR-VC-0207 | Contract | P1 |

### 3.2 vCenter REST (`/api`, 구형 `/rest`는 D 결정 대상 FR-VC-0204 ○)

| API ID | 오퍼레이션 | vcsim | 관련 FR | 시험 | 우선순위 |
|---|---|---|---|---|---|
| VC-REST-001 | `POST /api/session`, `DELETE /api/session`, `GET /api/session` | N (구형 세션 저장소에 연결하는 어댑터, PoC-02) | FR-API-0202 | Scenario 01 | P1 |
| VC-REST-002 | `GET /api/vcenter/vm`, `GET /api/vcenter/vm/{vm}` | N (인벤토리 뷰, PoC-03) | FR-VC-0203, FR-VC-0205, FR-DM-0204 | Scenario 01~05 | P1 |
| VC-REST-003 | `GET /api/vcenter/datacenter`, `/cluster`, `/host`, `/datastore`, `/network` | N | FR-VC-0203, FR-VC-0301 ~ FR-VC-0306 | Scenario 01 | P1 |
| VC-REST-004 | `GET /api/appliance/system/version` | N | FR-API-0306 | Scenario 01 | P1 |
| VC-REST-005 | `POST /api/vcenter/vm/{vm}/power?action=start`, `…?action=stop` | N | FR-VC-0405, FR-VC-0406 | Scenario 04(선택) | P2 |
| VC-REST-006 | `/api/cis/tagging/...` | P (구형 경로 구현 있음) | FR-VC-0203 | Contract | P2 |

REST 계약 근거는 SRC-VAPI-SDK-7032(vSphere Automation SDK v7.0.3.2의 바인딩·런타임에서 추출한 오퍼레이션·구조체·열거형·선언 오류·오류 HTTP 매핑)다(2026-10-10 결정 ②, 포털 스펙 SRC-VAPI-703 미확보 대체). `GET /api/vcenter/vm/{vm}`의 출력은 `VM.Info`다(`VM.Summary` 아님). VC-REST-001 ~ 004는 Phase 1에서 Integration Tested에 도달했다. 가정으로 남은 부분: POST /api/session 성공 코드 201, guest_OS·하드웨어 버전 이름 변환 규칙.

### 3.3 NSX (Manager `/api/v1`, Policy `/policy/api/v1`)

| API ID | 오퍼레이션 (operationId) | 근거 | 관련 FR | 시험 | 우선순위 |
|---|---|---|---|---|---|
| NSX-001 | `POST /api/session/create`, Basic 인증 | API Guide 본문(스펙 밖). SRC-NSX-321에는 `HttpServiceProperties` 설명에 경로 언급만 있음(요청·응답 계약 없음) | FR-API-0206, FR-API-0207 | Scenario 01 | P1 |
| NSX-002 | `GET /api/v1/node/version` (ReadNodeVersion) | SRC-NSX-321 | FR-NSX-0202, FR-API-0306 | Scenario 01 | P1 |
| NSX-003 | `GET /api/v1/cluster/status` (ReadClusterStatus) | SRC-NSX-321 | FR-NSX-0203 | Scenario 01 | P1 |
| NSX-004 | `GET/POST /api/v1/fabric/compute-managers`, `GET/PUT/DELETE …/{compute-manager-id}` | SRC-NSX-321 | FR-NSX-0204, FR-NSX-0601 | Scenario 01 | P1 |
| NSX-005 | `GET /api/v1/transport-zones` (ListTransportZones) | SRC-NSX-321 | FR-NSX-0205 | Scenario 01 | P1 |
| NSX-006 | `GET /policy/api/v1/infra` (ReadInfra) | SRC-NSX-321 | FR-NSX-0209 | Scenario 01 | P1 |
| NSX-007 | `GET …/sites/{site-id}/enforcement-points/{ep}/transport-zones` | SRC-NSX-321 | FR-NSX-0210 | Scenario 01 | P1 |
| NSX-008 | `GET /policy/api/v1/infra/segments` (ListAllInfraSegments) | SRC-NSX-321 | FR-NSX-0211, FR-NSX-0402, FR-NSX-0414 | Scenario 06 | P1 |
| NSX-009 | `PATCH/PUT/GET/DELETE /policy/api/v1/infra/segments/{segment-id}` | SRC-NSX-321 (412 응답, `_revision` 규칙), SRC-DOC(PATCH 200 본문 없음) | FR-NSX-0401 ~ FR-NSX-0409, FR-NSX-0416 | Scenario 06, Negative | P1 |
| NSX-010 | `GET /policy/api/v1/infra/realized-state/status?intent_path=` (ReadIntentStatus) | SRC-NSX-321 (`ConsolidatedRealizedStatus`) | FR-NSX-0220, FR-NSX-0501 ~ FR-NSX-0509 | Scenario 06 | P1 |
| NSX-011 | Policy 리소스 `tags` 필드 | SRC-NSX-321 | FR-NSX-0219 | Contract | P1 |
| NSX-012 | `/api/v1/spec/openapi/nsx_api.json`, `nsx_policy_api.json` | API Guide 본문(스펙 밖) | FR-NSX-0104 | Contract | P2 |
| ~~NSX-013~~ | `PATCH /policy/api/v1/infra` (PatchInfra, H-API) — Phase 4b에서 **NSX-027**로 다시 번호를 매겼다(§5의 NSX-013과 ID 충돌) | SRC-NSX-321, 원자성은 가정 | FR-NSX-0223 ~ FR-NSX-0225 (○, D9) | Contract | → §6 |

## 4. 결정 대기 (○) 항목과 Coverage 영향

| 결정 | 대상 API | 영향 |
|---|---|---|
| D7 | Clone(`CloneVM_Task`), Content Library/OVF, Guest Customization, Reset·Guest Reboot, 디스크 변경 | vcsim 구현 있음(Clone·Customize·Reset·RebootGuest Y). Content Library는 vAPI 시뮬레이터 부분 구현. **Phase 4b:** Clone·Template·Customization·Reset·Content Library(/api) 구현(§6), OVF 배포·Guest Reboot는 범위 밖 |
| D8 | DVPG `backingType=nsx` / OpaqueNetwork | 버전 필터 대상 필드(7.0.0.0) 포함 |
| D9 | H-API, Fabric VM·VIF·VM Tag, Manager API 쓰기, Search | Phase 3: NSX-013 ~ 021, **Phase 4b:** NSX-022 ~ 031(§6) |
| D10 | `LoginByToken`(STS), Lookup Service, PBM | vcsim 구현 있음(LoginByToken Y). **Phase 4b:** Lookup·PBM 채택(§6), STS·구형 /rest는 미채택 제안 |

## 5. Phase 3 추가 API (교차 시스템·메트릭·이벤트) — 실행 범위 포함(2026-10-10 D2 확대)

Phase 3에서 구현하고 자체 시험한 API다. 2026-10-10 사용자 결정(D2 범위 확대)으로 실행 범위(`execution_scope`)와 release 기동 조건에 넣었다. 계약 근거·시험은 표와 같다.

| ID | 경로·메서드 | 계약 근거 | 관련 요구사항 | 시험 |
|---|---|---|---|---|
| NSX-013 | `GET /api/v1/fabric/virtual-machines`, `POST …?action=update_tags·add_tags·remove_tags` | SRC-NSX-321 | FR-NSX-0226·0608·0609 | Scenario 08, `TestNSXResponseSchemaContract` |
| NSX-014 | `GET /api/v1/fabric/vifs` | SRC-NSX-321 | FR-NSX-0227·0604 | Scenario 08, 스키마 대조 |
| NSX-015 | `GET /api/v1/fabric/discovered-nodes` | SRC-NSX-321 | FR-NSX-0602 | Scenario 08, 스키마 대조 |
| NSX-016 | `GET /api/v1/fabric/compute-managers/{id}/status` | SRC-NSX-321 | FR-NSX-0601 | Scenario 08, 스키마 대조 |
| NSX-017 | `GET /policy/api/v1/infra/segments/{id}/ports` (시스템 소유 Port) | SRC-NSX-321 | FR-NSX-0604 | Scenario 08, 스키마 대조 |
| NSX-018 | `GET /policy/api/v1/infra/realized-state/realized-entities?intent_path=` | SRC-NSX-321 | FR-NSX-0504·0509 | `TestRealizationDependenciesAndFailure`, 스키마 대조 |
| NSX-019 | `GET /policy/api/v1/infra/realized-state/enforcement-points/{ep}/virtual-machines` | SRC-NSX-321 | FR-NSX-0228 | Scenario 08, 스키마 대조 |
| NSX-020 | `GET /policy/api/v1/infra/domains/{d}/groups/{g}/members/virtual-machines` | SRC-NSX-321 | FR-NSX-0605 | Scenario 08, 스키마 대조 |
| NSX-021 | `GET /api/v1/alarms`, `/alarms/{id}` (Realization 실패 알람) | SRC-NSX-321 | FR-MET-0201·0202 | `TestRealizationDependenciesAndFailure`, 스키마 대조 |
| VC-SOAP-014 | `PerformanceManager.QueryPerf` (결정론적 가상 메트릭) | SRC-VIM-TYPEINFO | FR-MET-0101~0104, FR-BE-0704 | `TestVirtualMetrics`, `TestSOAPSchemaContract` |
| VC-SOAP-015 | `EventManager.QueryEvents` (TaskEvent·HostDisconnected·HostConnected 추가) | SRC-VIM-TYPEINFO | FR-MET-0201~0203 | `TestVCenterEvents`, `TestSOAPSchemaContract` |
| VC-SOAP-016 | `HostSystem.DisconnectHost_Task`·`ReconnectHost_Task` | SRC-VIM-TYPEINFO | FR-MET-0201 | `TestVCenterEvents` |
| VC-NET-001 | NSX DVPG 노출(`DistributedVirtualPortgroup.config.backingType=nsx`, `segmentId`, `logicalSwitchUuid`) | SRC-VIM-TYPEINFO | FR-NSX-0603·0606·0607 | Scenario 08, `TestSOAPSchemaContract`(DVPG 전부) |

## 6. Phase 4b 추가 API — 실행 범위 포함(2026-10-11 D2 확대)

Phase 4b(WP 4.6·4.9·4.11)에서 구현하고 자체 시험한 API다. 2026-10-11 사용자 결정(D2 확대, PRD-revision #67)으로 실행 범위(`execution_scope`)와 release 기동 조건에 넣었다. 검증 상태는 Phase 5 최종 증거(`verification.yaml`, DEVELOPMENT_PLAN §13)를 따른다. 계약 근거 열의 출처 ID는 `verification.yaml`에도 기록한다(Coverage Report의 contract_sources).

| ID | 경로·메서드 | 계약 근거 | 관련 요구사항 | 시험 |
|---|---|---|---|---|
| NSX-022 | `/policy/api/v1/infra/domains/{d}/groups[/{g}]` CRUD, `…/members/{virtual-machines,segments,segment-ports,ip-addresses}` | SRC-NSX-321(Group·Expression 규칙·PolicyGroupMembersListResult) | FR-NSX-0215·0229·0605 | `TestNSXSecurityResources`, `TestNSXGroupMembership`, Scenario 07 |
| NSX-023 | `/policy/api/v1/infra/services[/{s}]`, `…/service-entries` (기본 Service 시드) | SRC-NSX-321(Service.is_default, ServiceEntry 다형) | FR-NSX-0216 | `TestNSXSecurityResources`, Scenario 07 |
| NSX-024 | `/policy/api/v1/infra/domains/{d}/security-policies[/{p}]` CRUD(내장 rules), `?action=revise` | SRC-NSX-321(Patch/Update/Delete SecurityPolicy 설명) | FR-NSX-0217 | `TestNSXSecurityResources`, Scenario 07 |
| NSX-025 | `…/security-policies/{p}/rules[/{r}]` CRUD, `?action=revise` | SRC-NSX-321(Rule·BaseRule) | FR-NSX-0218, FR-NSX-0408·0409 | `TestNSXSecurityResources`, Scenario 07 |
| NSX-026 | `/policy/api/v1/infra/tier-0s`, `tier-1s`, `…/locale-services`, `…/interfaces` | SRC-NSX-321 | FR-NSX-0212 ~ 0214, FR-NSX-0507 | `TestNSXRoutingResources` |
| NSX-027 | `PATCH /policy/api/v1/infra` (H-API, Child 래퍼·ChildResourceReference·marked_for_delete·enforce_revision_check) | SRC-NSX-321(PatchInfra 설명), 원자성은 가정 | FR-NSX-0223 ~ 0225 | `TestNSXHierarchicalAPI` |
| NSX-028 | `GET /policy/api/v1/search/query`, `GET /api/v1/search/query` | SRC-NSX-321(QuerySearch·SearchResponse), 문법은 §6.2 | FR-NSX-0221·0232·0413 | `TestNSXSearchAndFilters`, `TestSearchQuerySyntax` |
| NSX-029 | `GET /api/v1/transport-nodes[/{id}]`, `…/{id}/state` (vCenter 호스트 파생) | SRC-NSX-321(TransportNode·TransportNodeState) | FR-NSX-0206 | `TestNSXManagerInventory` |
| NSX-030 | `GET /api/v1/logical-switches[/{id}]`, `/api/v1/logical-ports` (Policy 실현, 보호 객체) | SRC-NSX-321 | FR-NSX-0207 | `TestNSXManagerInventory` |
| NSX-031 | Edge Cluster 조회(Policy `…/enforcement-points/{ep}/edge-clusters`, Manager `/api/v1/edge-clusters`), Manager 컬렉션 POST 생성(201) | SRC-NSX-321 | FR-NSX-0208·0213 | `TestNSXRoutingResources`, `TestNSXManagerInventory` |
| VC-SOAP-017 | `VirtualMachine.CloneVM_Task` | SRC-VIM-TYPEINFO | FR-VC-0402, FR-VC-0417 | `TestVMLifecycleExtended` |
| VC-SOAP-018 | `VirtualMachine.MarkAsTemplate` / `MarkAsVirtualMachine` | SRC-VIM-TYPEINFO | FR-VC-0415 | `TestVMLifecycleExtended` |
| VC-SOAP-019 | `Folder.RegisterVM_Task`, `VirtualMachine.UnregisterVM` | SRC-VIM-TYPEINFO | FR-VC-0403 | `TestVMLifecycleExtended` |
| VC-SOAP-020 | `VirtualMachine.ResetVM_Task` / `SuspendVM_Task` | SRC-VIM-TYPEINFO | FR-VC-0407·0408 | `TestVMLifecycleExtended` |
| VC-SOAP-021 | `ManagedEntity.Rename_Task`, `Folder.MoveIntoFolder_Task`, `VirtualMachine.RelocateVM_Task` | SRC-VIM-TYPEINFO | FR-VC-0410·0411 | `TestVMLifecycleExtended` |
| VC-SOAP-022 | `CreateSnapshot_Task`, `RevertToSnapshot_Task`, `RevertToCurrentSnapshot_Task`, `RemoveSnapshot_Task`, `RemoveAllSnapshots_Task` | SRC-VIM-TYPEINFO | FR-VC-0412 ~ 0414, FR-VC-0418 | `TestVMLifecycleExtended` |
| VC-SOAP-023 | `VirtualMachine.CustomizeVM_Task`(+ Clone customization) | SRC-VIM-TYPEINFO | FR-VC-0416 | `TestVMLifecycleExtended` |
| VC-LOOKUP-001 | `/lookupservice/sdk` (`LookupServiceRegistration.List`, `GetSiteId`) | SRC-GOVMOMI(lookup 바인딩, vcsim과 같은 계열 — TR-COV-0305 주의) | FR-VC-0208, NFR-ARC-0104 | `TestLookupService` |
| VC-PBM-001 | `/pbm/sdk` (`PbmQueryProfile`, `PbmRetrieveContent`, `PbmQueryAssociatedProfile(s)`, `PbmCheckRequirements`) | pyVmomi `_typeinfo_pbm`(메서드 목록), SRC-GOVMOMI | FR-VC-0210, NFR-ARC-0104 | `TestPBMStoragePolicy` |
| VC-REST-007 | `/api/content/library`, `/api/content/local-library`, `/api/content/library/item` (list·get·create·update·delete·find) | SRC-VAPI-SDK-7032(content 모듈, Phase 4b 추출 확장) | FR-VC-0206, NFR-ARC-0104 | `TestContentLibraryContract` |

개선된 기존 API: VC-SOAP-007(WaitForUpdatesEx) — 데이터 버전 발급·검증(vcsim 열 P → Y + Adapter, `TestPropertyCollectorVersions`), VC-SOAP-015(QueryEvents) — 시간 필터 가상 시각 해석(`TestEventTimeFilterVirtualClock`).

미채택 제안(사용자 확정 필요, `40-test/carryover.yaml`): 구형 `/rest`(FR-VC-0204), STS `LoginByToken`(FR-VC-0209), HttpNfcLease·ESXi 직접 경로(FR-VC-0211), NSX Statistics·Status(FR-NSX-0222). 스펙에 있으나 행위를 구현하지 않은 NSX `?action=` POST는 501로 응답하고 미지원 API로 집계한다(FR-API-0401).

### 6.2 Policy Search 지원 문법 (FR-NSX-0232)

스펙(QuerySearch)은 `query` 문법을 정하지 않는다. 시뮬레이터는 Lucene query string의 부분집합을 지원한다(가정, 단위 시험 `TestSearchQuerySyntax`).

| 문법 | 예 | 의미 |
|---|---|---|
| `field:value` | `resource_type:Segment` | 필드 값 일치(대소문자 무관). 점 표기로 중첩·배열 원소(`tags.scope:app`) |
| 필드 없는 값 | `web` | 최상위 문자열 필드 또는 `tags.tag`가 일치하거나 값의 단어 하나와 일치 |
| 와일드카드 | `display_name:web*`, `w?b` | `*` 0자 이상, `?` 1자 |
| 따옴표 | `display_name:"T1 Gateway"` | 공백 포함 값, 따옴표 안의 `*`는 문자 그대로 |
| 이스케이프 | `path:\/infra\/segments\/*` | 백슬래시 다음 문자 그대로 |
| 불리언 | `AND`, `OR`, `NOT`, `&&`, `\|\|`, `!`, `-term`, `( )` | 이어 쓴 항은 OR(Lucene 기본) |
| 미지원 → 400 | `[a TO b]`, `~`, `^`, `_exists_`, `field:(a b)` | 범위·퍼지·부스트·존재 검사 |

대상: Policy 검색은 `/infra` 아래 객체(시스템 소유 포함), Manager 검색은 Manager 객체. `sort_by`·`sort_ascending`·`cursor`·`page_size`를 따른다. `/search/dsl`은 미지원(501).

## 7. 프로파일별 실행 범위 (WP 4.7)

| 프로파일 | 실행 범위 | 비고 |
|---|---|---|
| vCenter 7.0.3.02500 + NSX 3.2.4.3 | 61개 API(§3, §5, §6) — vCenter 31·NSX 30 | 기준 조합(D1), `interop: verified` |
| vCenter 7.0.2.00000 + NSX 3.2.4.3 | vCenter SOAP 계열 26개(VC-SOAP-001 ~ 023, VC-NET-001, VC-LOOKUP-001, VC-PBM-001) + NSX 30개 | REST(VC-REST-*, Content Library 포함)는 7.0 U2용 SDK 계약을 확보하지 않아 제외(7.0.3.2 계약을 그대로 적용하지 않음, R-08). `interop: unverified` → development 모드만 기동(R-42) |

## 8. API Coverage Report (Phase 5, TR-SCN-0112)

실행 중인 시뮬레이터가 Admin API `GET /v1/coverage-report`(JSON) 또는 `?format=markdown`으로 이 표의 행 ID별 실행 범위·검증 상태·계약 근거·시험 ID·증거 구현 식별자·관측한 미지원 호출을 만든다. 경로·요구사항·시나리오 같은 설계 열은 이 문서가 원본이다. 리포트는 실행 범위 안이면서 Integration Tested인 API만 '지원'으로 세고, 증거 구현 식별자가 현재 빌드와 다르면 `evidence_matches_build: false`로 표시한다(TR-COV-0409).
