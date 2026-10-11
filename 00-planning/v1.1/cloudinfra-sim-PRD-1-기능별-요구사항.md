# CloudInfra Simulator
## PRD-1. 기능별 요구사항

**부제:** CMP-Oriented VMware SDDC API Simulator  
**저장소:** `cloudinfra-sim`  
**문서 버전:** 1.1  
**상위 문서:** `cloudinfra-sim-PRD.md`  
**범위:** PRD 3~9장 (vCenter·NSX 엔진, 행위 엔진, 데이터 모델, API 호환 계층, 오류 재현, 메트릭·이벤트)

---

## 목차

- 요구사항 표 작성 기준
- 3. vCenter API Simulation Engine
  - 3.1 기반 기술
  - 3.2 지원 API 계열
  - 3.3 vCenter 가상 리소스
  - 3.4 VM 생명주기
  - 3.5 SOAP Task 및 PropertyCollector
  - 3.6 버전 호환성
- 4. NSX API Simulation Engine
  - 4.1 기반 기술
  - 4.2 지원 API 영역
  - 4.3 NSX Policy Resource 모델
  - 4.4 NSX Policy 동작
  - 4.5 Policy Realization
  - 4.6 NSX와 vCenter 연동
- 5. Behavioral Simulation Engine
  - 5.1 역할
  - 5.2 상태 관리 원칙
  - 5.3 vCenter와 NSX 상태 소유권
  - 5.4 vCenter Behavior Adapter
  - 5.5 비동기 Task Engine
  - 5.6 Virtual Clock
  - 5.7 Capacity Simulation
- 6. Virtual SDDC 데이터 모델
  - 6.1 주요 엔터티
  - 6.2 ID 관리
  - 6.3 Database 및 Persistence
  - 6.4 Cross-System Consistency
- 7. API Compatibility Layer
  - 7.1 핵심 역할
  - 7.2 인증 및 세션
  - 7.3 버전 프로파일
  - 7.4 미지원 API 처리 정책
- 8. 오류 및 장애 재현
  - 8.1 정상 동작 외의 필수 검증
  - 8.2 Fault Injection Engine
- 9. 가상 메트릭·이벤트·알람
  - 9.1 Metrics
  - 9.2 Events / Alarms
- 요구사항 집계

---

## 요구사항 표 작성 기준

장·절 번호는 상위 PRD의 번호 체계를 그대로 사용한다. 문서 간 참조는 현재 절 번호(예: PRD-3 §13.1, PRD-4 §19)와 요구사항 ID를 함께 사용한다.

**요구사항 ID:** `접두어-SSNN` (SS = 절 번호 2자리, NN = 절 안의 일련번호 2자리). 예: `FR-VC-0302` = 3.3절의 2번째 요구사항.

| 접두어 | 대상 장 |
|---|---|
| FR-VC | 3. vCenter API Simulation Engine |
| FR-NSX | 4. NSX API Simulation Engine |
| FR-BE | 5. Behavioral Simulation Engine |
| FR-DM | 6. Virtual SDDC 데이터 모델 |
| FR-API | 7. API Compatibility Layer |
| FR-FLT | 8. 오류 및 장애 재현 |
| FR-MET | 9. 가상 메트릭·이벤트·알람 |

**표의 열:**

| 열 | 값 | 의미 |
|---|---|---|
| 수준 | 필수 / 권장 / 선택 / 검토 | 필수: 승인 적용 범위에서 반드시 충족. 권장: 특별한 사유가 없으면 충족. 선택: 시뮬레이션 요구·대상 버전에 따라 결정. 검토: 분석·검증 후 채택 여부 결정 |
| MVP | ● / ○ / – | ●: MVP 범위(PRD §18 Task 6). ○: §19 결정 사항에 따라 MVP 포함 여부 결정. –: MVP 이후 또는 적용 제외 |
| 상태 | 정의 / 분석 / 설계 / 구현 / 검증 / 완료 / 보류 / 삭제 | 요구사항 추적 상태. 최초 값은 '정의' |

ID는 한 번 부여하면 변경·재사용하지 않는다. 삭제된 요구사항은 행을 지우지 않고 상태를 '삭제'로 바꾼다. 개별 API의 구현·검증 상태는 이 표가 아니라 API Coverage Matrix(PRD-2, §12)로 관리한다.

**프로젝트 범위(D13):** CMP는 범위 선정용 참고 자료이다. 아래 요구사항의 클라이언트 동작은 프로젝트 내부 테스트 클라이언트로 재현하며 CMP 소스 분석·실제 CMP 실행·상품 제약 구현을 요구하지 않는다. API 계약은 대상 버전의 공식 스펙·문서와 공개 SDK를 기준으로 한다. 외부 검증·Record는 D5·D13에 따라 보류한다.

**대상 버전에 따른 적용:** D1에서 대상 범위를 vSphere/vCenter 7.0 U2 이상(8.x 포함), NSX-T 3.x(Policy API 기반, NSX 4.x 전용 기능 미사용)로 정했다. NSX 4.x 전용 요구사항(FR-NSX-0230)은 상태를 '보류', MVP를 '–'로 두고 추적표에 '적용 제외 — D1'로 기록한다.

**대안별 적용:** D3에서 권고안 A를 채택했다. 이 문서는 A를 기준으로 기술한다. 권고안 B 전용 요구사항(FR-BE-0305, FR-BE-0405)은 상태를 '보류', MVP를 '–'로 두고 상세를 `cloudinfra-sim-PRD-B-권고안B-대안-구성.md`(이하 PRD-B)로 옮겼으며, 추적표에는 '적용 제외 — D3'로 기록한다. 이는 구현 상태의 '완료'나 요구사항 삭제를 뜻하지 않는다. D3를 재검토하는 조건과 절차는 PRD-B §B2를 따른다.

---

# 3. vCenter API Simulation Engine

## 3.1 기반 기술

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-VC-0101 | vcsim 기반 엔진 | vCenter 핵심 엔진은 `govmomi/vcsim`을 활용한다. (참고: https://github.com/vmware/govmomi , https://github.com/vmware/govmomi/tree/main/vcsim) | 필수 | ● | 정의 |
| FR-VC-0102 | vcsim 라이브러리 임베드 | vcsim을 별도 프로세스로 띄우지 않고 Go 라이브러리로 임베드하는 것을 기본으로 한다. | 필수 | ● | 정의 |
| FR-VC-0103 | vcsim 지원 범위 조사 | 최초 개발 시 vcsim의 지원 메서드와 시뮬레이션 기능을 조사한다. | 필수 | ● | 정의 |
| FR-VC-0104 | 확장 모듈 우선 | vcsim 원본을 직접 대량 수정하지 않고 확장 모듈을 작성한다. | 권장 | ● | 정의 |
| FR-VC-0105 | Fork 정책 | 확장 포인트가 부족해 핵심 인벤토리·Task·PropertyCollector와 동기화할 수 없으면 특정 버전을 고정한 Fork를 검토한다. Fork 시 업스트림 변경을 추적하고 패치를 최소화한다. | 검토 | – | 정의 |
| FR-VC-0106 | 확장 지점 검증 | 아래 '확장 지점' 표의 항목이 실제로 존재하는지와 그 한계를 Phase 0에서 확인한다(현재는 추정). | 필수 | ● | 정의 |
| FR-VC-0107 | 컨테이너 기반 VM 비활성화 | vcsim의 컨테이너 기반 VM 실행 기능(VM을 실제 컨테이너로 구동)은 경량 실행 목표에 따라 비활성화한다. | 필수 | ● | 정의 |

**참고 — vcsim 확장 지점 (추정)**

| 확장 지점 | 용도 |
|---|---|
| 레지스트리 변경 핸들러 (객체 Put/Update/Remove 통지, PropertyCollector가 사용하는 메커니즘) | 변경 감지, Event/Outbox 발생, NSX Fabric 인벤토리 파생 |
| 타입 임베딩을 통한 Managed Object 메서드 오버라이드 | 사전조건·용량 검증, 의미적 Fault, Task 지연 |
| `DelayConfig` (메서드별 지연·지터) | 응답 지연 시뮬레이션 |
| `govc object.save` / `vcsim -load` | 인벤토리 Snapshot/Restore |
| Model 기반 인벤토리 생성 | Fixture Seed |
| 내장 Lookup/STS/PBM/vAPI(Tagging, Content Library) 시뮬레이터 | 보조 서비스 재사용 |

## 3.2 지원 API 계열

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-VC-0201 | SOAP 엔드포인트 | vSphere Web Services API(SOAP)를 `/sdk` 경로로 제공한다. | 필수 | ● | 정의 |
| FR-VC-0202 | SOAP 관리 객체 영역 | 지원 분석 대상: ServiceInstance, SessionManager, PropertyCollector, ViewManager, SearchIndex, Folder, Datacenter, ClusterComputeResource, ComputeResource, HostSystem, ResourcePool, VirtualMachine, Datastore, Network, DistributedVirtualSwitch, DistributedVirtualPortgroup, TaskManager, EventManager, PerformanceManager, CustomFieldsManager, AuthorizationManager. 전체를 무조건 구현하지 않고 승인할 자체 테스트 API 범위를 분석해 구현 우선순위를 정한다. | 필수 | ● | 정의 |
| FR-VC-0203 | Automation REST API | `/api/session`, `/api/vcenter/vm`, `/api/vcenter/datacenter`, `/api/vcenter/cluster`, `/api/vcenter/host`, `/api/vcenter/datastore`, `/api/vcenter/network`, `/api/cis/tagging/...`를 지원 대상으로 분석한다. | 필수 | ● | 정의 |
| FR-VC-0204 | Legacy REST API | `/rest/com/vmware/cis/session`, `/rest/vcenter/...`, `/rest/com/vmware/cis/tagging/...`를 지원 대상으로 분석한다. 구형·최신 REST의 응답 래핑 구조 차이는 버전별 계약으로 검증한다. 구형 경로를 최신 API로 단순 변환하면서 응답 형태를 바꾸지 않는다. | 필수 | ○ | 정의 |
| FR-VC-0205 | REST = vcsim 인벤토리 뷰 | vCenter REST 리소스는 별도 저장소를 두지 않고 vcsim 인벤토리에 대한 뷰로 구현하며, vcsim과 같은 Go 프로세스에 위치한다. | 필수 | ● | 정의 |
| FR-VC-0206 | Content Library 및 OVF | `/api/content/library`, `/api/content/library/item`, `/api/vcenter/ovf/library-item`, `/api/vcenter/vm-template/library-items`, 구형 `/rest/com/vmware/content/...`를 지원한다. MVP 포함 여부는 §19(D7)에서 결정한다. | 권장 | ○ | 정의 |
| FR-VC-0207 | 버전 협상 경로 | `/sdk/vimServiceVersions.xml`, `/sdk/vimService.wsdl`을 활성 프로파일의 버전에 맞게 제공한다. | 필수 | ● | 정의 |
| FR-VC-0208 | Lookup Service | `/lookupservice/sdk`를 통해 vCenter 서비스 엔드포인트 탐색을 지원한다. | 권장 | ○ | 정의 |
| FR-VC-0209 | SSO STS | `/sts/STSService`를 통해 SAML 토큰 기반 로그인(`LoginByToken`)을 지원한다. | 권장 | ○ | 정의 |
| FR-VC-0210 | PBM | `/pbm/sdk`를 통해 VM 프로비저닝 시 스토리지 정책 조회·지정을 지원한다. | 권장 | ○ | 정의 |
| FR-VC-0211 | HttpNfcLease 및 ESXi 직접 경로 | OVF 업로드 등에서 테스트 클라이언트가 vCenter가 반환한 호스트 주소로 직접 접속할 수 있도록, 가상 ESXi 호스트 이름이 Gateway로 해석되게 구성한다(§2.2). | 권장 | ○ | 정의 |
| FR-VC-0212 | 내장 보조 시뮬레이터 재사용 | vcsim에 포함된 Lookup/STS/PBM/vAPI 시뮬레이터를 우선 재사용하고 부족한 부분만 보완한다. | 권장 | ● | 정의 |

## 3.3 vCenter 가상 리소스

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-VC-0301 | Datacenter | ID 및 MoRef, Name, Folder 관계, Host/VM/Datastore/Network Folder | 필수 | ● | 정의 |
| FR-VC-0302 | Cluster | ID 및 MoRef, Name, Datacenter 관계, Host 목록, ResourcePool, DRS/HA 설정의 논리적 상태, 전체 CPU/Memory Capacity | 필수 | ● | 정의 |
| FR-VC-0303 | Host | ID 및 MoRef, Name, Management IP, Connection State, Power State, CPU/Memory Capacity, Cluster 관계, Datastore 및 Network 관계, Maintenance Mode | 필수 | ● | 정의 |
| FR-VC-0304 | Datastore | ID 및 MoRef, Name, Datastore Type, Capacity, Free Space, Accessibility, Host Mount 관계 | 필수 | ● | 정의 |
| FR-VC-0305 | VM | ID 및 MoRef, Instance UUID, BIOS UUID, Name, Guest OS ID, CPU Count, Memory Size, Power State, Connection State, Guest Information, VM Tools 상태, Datacenter/Cluster/Host 관계, ResourcePool 관계, Datastore 관계, Virtual Disk 목록, vNIC 목록, Network 연결, Snapshot 목록, Template 여부, Tags 및 Custom Attributes | 필수 | ● | 정의 |
| FR-VC-0306 | Network | Standard Network, Distributed Portgroup, NSX-backed Network, Network ID 및 MoRef, VM 연결 관계, DVS 관계 | 필수 | ● | 정의 |
| FR-VC-0307 | 스키마 정합 표현 | 리소스는 VMware API 스키마에 맞는 값으로 표현한다. 내부에서는 별도의 도메인 모델을 유지할 수 있다. | 필수 | ● | 정의 |

## 3.4 VM 생명주기

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-VC-0401 | Create | VM 생성 | 필수 | ● | 정의 |
| FR-VC-0402 | Clone | VM/템플릿 복제. 자체 테스트의 프로비저닝 방식(§19 D7)에 따라 MVP 포함 결정 | 필수 | ○ | 정의 |
| FR-VC-0403 | Register | VM 등록 | 선택 | – | 정의 |
| FR-VC-0404 | Reconfigure | CPU·Memory 등 구성 변경 | 필수 | ● | 정의 |
| FR-VC-0405 | Power On | 전원 켜기 | 필수 | ● | 정의 |
| FR-VC-0406 | Power Off | 전원 끄기 | 필수 | ● | 정의 |
| FR-VC-0407 | Reset | VM 전원 재설정(하드 리셋, `ResetVM_Task`). 게스트 OS 재부팅(`RebootGuest`)과 구분하며, Guest Reboot는 채택 시 별도 FR로 정의한다(D7) | 권장 | – | 정의 |
| FR-VC-0408 | Suspend | 일시 중단 | 권장 | – | 정의 |
| FR-VC-0409 | Delete | VM 삭제 | 필수 | ● | 정의 |
| FR-VC-0410 | Rename | 이름 변경 | 권장 | – | 정의 |
| FR-VC-0411 | Move | 폴더·리소스 이동 | 권장 | – | 정의 |
| FR-VC-0412 | Snapshot Create | 스냅샷 생성 | 권장 | – | 정의 |
| FR-VC-0413 | Snapshot Revert | 스냅샷 복귀 | 권장 | – | 정의 |
| FR-VC-0414 | Snapshot Delete | 스냅샷 삭제 | 권장 | – | 정의 |
| FR-VC-0415 | Template Convert | VM ↔ 템플릿 변환 | 권장 | ○ | 정의 |
| FR-VC-0416 | Guest Customization | 게스트 커스터마이징의 논리적 상태 전이 | 선택 | – | 정의 |
| FR-VC-0417 | 사전조건 검증 | 모든 작업은 실제 API가 요구하는 사전조건을 확인한다. 예: 존재하지 않는 Host·Datastore 사용 불가, CPU·Memory 허용 범위 초과 시 오류, 전원 상태에 따른 작업 실패, 삭제한 VM 조회 시 지정 API의 Not Found 동작 반환, VM 삭제 시 연결 리소스 관계 정리 | 필수 | ● | 정의 |
| FR-VC-0418 | 버전별 동작 규칙 | 실제 VMware의 버전별 규칙을 확인하고 구현한다. 전원·Snapshot 작업의 규칙을 임의로 동일하게 취급하지 않는다. | 필수 | ● | 정의 |

## 3.5 SOAP Task 및 PropertyCollector

vCenter 동작 호환성의 핵심 요소이다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-VC-0501 | Task MoRef 발급 | 비동기 작업마다 ManagedObjectReference를 발급한다. | 필수 | ● | 정의 |
| FR-VC-0502 | Task 상태 | queued / running / success / error 상태를 제공한다. | 필수 | ● | 정의 |
| FR-VC-0503 | Task 시각·진행률 | 시작·종료 시각과 진행률을 제공한다. | 필수 | ● | 정의 |
| FR-VC-0504 | Task 결과·Fault | 결과 객체 참조와 Fault 정보를 제공한다. | 필수 | ● | 정의 |
| FR-VC-0505 | Task 완료 효과 | 완료 후 인벤토리 변경과 이벤트 발생이 이루어진다. | 필수 | ● | 정의 |
| FR-VC-0506 | Task 조회·대기 | Task 조회 및 완료 대기를 지원한다. | 필수 | ● | 정의 |
| FR-VC-0507 | 속성 조회 | `RetrievePropertiesEx`, `ContinueRetrievePropertiesEx`를 지원한다. | 필수 | ● | 정의 |
| FR-VC-0508 | 필터 관리 | `CreateFilter`, `DestroyPropertyFilter`, 세션별 필터 관리를 지원한다. | 필수 | ● | 정의 |
| FR-VC-0509 | 변경 감시 | `WaitForUpdatesEx`: 버전 토큰 관리, 변경 속성 반환, Timeout 및 부분 응답, 인벤토리 변화 감지 | 필수 | ● | 정의 |
| FR-VC-0510 | API 간 상태 일치 | SOAP Task, REST 조회, PropertyCollector 결과가 동일한 VM 상태를 나타낸다. | 필수 | ● | 정의 |
| FR-VC-0511 | vcsim 기능 재사용 | vcsim에서 이미 지원하는 동작은 재사용하고 누락된 부분만 보완한다. | 권장 | ● | 정의 |

## 3.6 버전 호환성

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-VC-0601 | Compatibility Profile 구조 | 선택 가능한 Compatibility Profile 구조를 적용한다. 예: vSphere 7.x, vSphere 8.x, 향후 검증된 vSphere 버전 | 필수 | ● | 정의 |
| FR-VC-0602 | 프로파일 포함 항목 | Product Name, Version, Build, API Version, 지원 API Method, 지원 Resource Property, 필수 필드, 오류 코드 및 Fault, 응답 구조, 버전별 동작 차이 | 필수 | ● | 정의 |
| FR-VC-0603 | 버전 문자열 위장 금지 | Version 문자열만 변경해 지원하지 않는 API가 지원되는 것처럼 보이게 하지 않는다. | 필수 | ● | 정의 |
| FR-VC-0604 | 버전 메타데이터 로드 | 타입·속성·메서드별 도입 버전 정보를 데이터로 확보한다. 후보 소스: 버전별 WSDL/XSD, pyVmomi 타입 정의(속성별 도입 버전 포함). 버전 간 비교로 검증한다. | 필수 | ● | 정의 |
| FR-VC-0605 | 협상 버전 결정 | 요청의 SOAPAction 네임스페이스 버전(`urn:vim25/<version>`)과 활성 프로파일의 최대 버전 중 낮은 쪽을 세션 버전으로 사용한다. | 필수 | ● | 정의 |
| FR-VC-0606 | 버전 인식 응답 필터링 | 직렬화 단계에서 세션 버전에 없는 속성을 제거하고, 없는 타입은 대상 버전의 공식 문서·WSDL·공개 SDK로 하위 호환 규칙(상위 타입 대체 등)을 확인한 뒤 처리한다. 수용 기준 예: 7.0.3 기준 프로파일에서 7.0 미만으로 협상한 세션(예: `urn:vim25/6.7.3` 요청)의 응답에는 `DVPortgroupConfigInfo.backingType`, `segmentId`가 포함되지 않는다. | 필수 | ● | 정의 |
| FR-VC-0607 | 버전 기반 요청 검증 | 세션 버전에 없는 메서드 호출은 실제 제품의 미지원 Fault로 응답한다. | 필수 | ● | 정의 |
| FR-VC-0608 | REST·NSX 스펙 기반 판단 | REST와 NSX는 활성 프로파일의 OpenAPI 스펙으로 경로·필드 존재 여부를 판단한다. | 필수 | ● | 정의 |

**참고 — 버전 인식 직렬화가 필요한 이유:** 실제 vCenter는 클라이언트가 협상한 API 버전(SOAPAction 헤더, `/sdk/vimServiceVersions.xml` 기반 협상)에 따라 그 버전에 없는 타입과 속성을 응답에서 제외한다. vcsim은 govmomi에 포함된 최신 vim25 타입 하나로 응답을 직렬화하며 요청 버전별 필터링을 하지 않는 것으로 추정된다(Phase 0 확인). 이 경우 About 버전만 바꾸는 것은 FR-VC-0603이 금지한 '버전 문자열 위장'에 해당한다.

---

# 4. NSX API Simulation Engine

## 4.1 기반 기술

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-NSX-0101 | 구현 언어 | Go로 vCenter 엔진과 같은 프로세스에 구현한다(D3: 권고안 A). D3를 B로 재결정하면 PRD-B §B4를 따른다. | 필수 | ● | 정의 |
| FR-NSX-0102 | 공식 명세 기반 계약 | NSX 제품 버전별 공식 API 명세를 기본 계약으로 사용한다. (참고: https://developer.broadcom.com/xapis/nsx-t-data-center-rest-api/latest/) | 필수 | ● | 정의 |
| FR-NSX-0103 | 버전별 스키마 확보 | 공식 문서가 제공하는 OpenAPI 스키마를 활용하며, 필요시 라이선스·접근 권한을 확인해 버전별 스키마를 확보한다. | 필수 | ● | 정의 |
| FR-NSX-0104 | 스펙 제공 경로 | 대상 버전 NSX Manager의 `/api/v1/spec/openapi/nsx_api.json`, `/api/v1/spec/openapi/nsx_policy_api.json` 등 스펙 제공 경로를 Phase 0에서 공식 문서로 확인한다. 실환경 접근은 D5·D13에 따라 보류하므로 스펙은 공식 배포본으로 확보해 프로파일에 포함하고, 시뮬레이터는 확인된 경로로 활성 프로파일의 스펙을 제공한다. 문서로 확인되지 않은 경로는 미검증으로 기록한다. | 권장 | ● | 정의 |
| FR-NSX-0105 | 단일 버전 스펙 로드 | 활성 프로파일의 스펙 한 개 버전만 로드한다. | 필수 | ● | 정의 |
| FR-NSX-0106 | 스키마 지연 컴파일 | 스키마는 실제 호출 시점에 지연 컴파일하고 캐시한다. 전체 스펙을 정적 모델(Pydantic 등)로 생성·로드하지 않는다. | 필수 | ● | 정의 |
| FR-NSX-0107 | 범용 리소스 핸들러 | 일반 CRUD 리소스는 스펙 기반 범용 핸들러로 처리한다. 필수/읽기전용 필드, 타입, enum, 기본값을 스펙에서 판단한다. | 필수 | ● | 정의 |
| FR-NSX-0108 | 행위 리소스 전용 로직 | Segment, Tier-0/Tier-1, Group, Service, Security Policy, Rule, Realization 대상 리소스는 해당 API를 지원 범위에 넣는 단계에서 전용 로직을 구현한다. MVP는 기본 Segment·Realization과 채택된 ○ 기능에 적용하며, 나머지는 §4.2의 MVP 표시 및 D9에 따른다. | 필수 | ● | 정의 |

## 4.2 지원 API 영역

공식 API 계약과 자체 테스트 대상에 따라 목록을 추가·제외한다(FR-NSX-0233).

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-NSX-0201 | MP: 인증 및 세션 | Manager API 인증 및 세션 | 필수 | ● | 정의 |
| FR-NSX-0202 | MP: 버전 조회 | NSX Manager 버전 조회 | 필수 | ● | 정의 |
| FR-NSX-0203 | MP: Node/Cluster 상태 | Node/Cluster 상태 조회 | 필수 | ● | 정의 |
| FR-NSX-0204 | MP: Fabric·Compute Manager | Fabric 및 Compute Manager | 필수 | ● | 정의 |
| FR-NSX-0205 | MP: Transport Zone | Transport Zone | 필수 | ● | 정의 |
| FR-NSX-0206 | MP: Transport Node | Transport Node 관련 논리적 정보 | 권장 | – | 정의 |
| FR-NSX-0207 | MP: Logical Networking | 필요한 Logical Networking 조회 | 권장 | – | 정의 |
| FR-NSX-0208 | MP: 기타 API | 기타 시뮬레이션 범위로 선정한 Manager API (분석 후 결정) | 선택 | ○ | 정의 |
| FR-NSX-0209 | Policy: Infrastructure | Infra 루트 및 하위 구조 | 필수 | ● | 정의 |
| FR-NSX-0210 | Policy: Transport Zones | Policy Transport Zones | 필수 | ● | 정의 |
| FR-NSX-0211 | Policy: Segments | Segments | 필수 | ● | 정의 |
| FR-NSX-0212 | Policy: Tier-0 Gateways | Tier-0 Gateways | 필수 | – | 정의 |
| FR-NSX-0213 | Policy: Tier-1 Gateways | Tier-1 Gateways | 필수 | – | 정의 |
| FR-NSX-0214 | Policy: Gateway Interfaces | Gateway Interfaces | 권장 | – | 정의 |
| FR-NSX-0215 | Policy: Groups | Groups | 필수 | – | 정의 |
| FR-NSX-0216 | Policy: Services | Services | 필수 | – | 정의 |
| FR-NSX-0217 | Policy: Security Policies | Security Policies | 필수 | – | 정의 |
| FR-NSX-0218 | Policy: Firewall Rules | Firewall Rules | 필수 | – | 정의 |
| FR-NSX-0219 | Policy: Tags | Policy 리소스 Tags | 필수 | ● | 정의 |
| FR-NSX-0220 | Policy: Realization State | Realization 상태 조회 | 필수 | ● | 정의 |
| FR-NSX-0221 | Policy: Search | Policy Search. 지원 문법 범위는 FR-NSX-0232에 따른다. | 권장 | – | 정의 |
| FR-NSX-0222 | Policy: Statistics·Status | 관련 Statistics 및 Status API | 선택 | – | 정의 |
| FR-NSX-0223 | Hierarchical API | `PATCH /policy/api/v1/infra`(및 하위 경로)에 `children`과 `Child<ResourceType>` 래퍼로 여러 리소스를 한 번에 생성·수정·삭제(`marked_for_delete`)한다. Terraform NSX Provider 등 자동화 도구가 자주 사용한다. | 권장 | ○ | 정의 |
| FR-NSX-0224 | H-API 참조·순서 | 하나의 요청 안의 리소스 간 참조와 처리 순서를 실제 제품과 같게 처리한다. | 권장 | ○ | 정의 |
| FR-NSX-0225 | H-API 부분 실패 | 부분 실패 시의 원자성 동작을 대상 버전의 공식 문서·스펙 근거로 재현한다. 근거가 없는 동작은 가정·미검증으로 기록한다(FR-API-0308). | 권장 | ○ | 정의 |
| FR-NSX-0226 | Fabric VM 및 VM Tag | `/api/v1/fabric/virtual-machines` 조회 및 `?action=update_tags` / `add_tags` / `remove_tags` | 권장 | ○ | 정의 |
| FR-NSX-0227 | VIF | `/api/v1/fabric/vifs` 조회 | 권장 | ○ | 정의 |
| FR-NSX-0228 | Policy 실현 VM 조회 | `/policy/api/v1/infra/realized-state/enforcement-points/{ep}/virtual-machines` 등 | 권장 | ○ | 정의 |
| FR-NSX-0229 | 태그 기반 Group 멤버십 | VM 태그를 기준으로 Group 멤버십을 계산한다. 이 데이터는 vCenter VM에서 파생된다(§4.6). | 권장 | – | 정의 |
| FR-NSX-0230 | Projects / VPC | 멀티테넌시 경로는 도입 버전 이후 프로파일에서만 노출한다. 대상 NSX가 NSX-T 3.x(4.x 전용 기능 미사용)로 결정되어 적용 제외 — D1. 대상 버전이 4.x로 확장되면 재검토한다. | 선택 | – | 보류 |
| FR-NSX-0231 | Manager API deprecation | 상위 버전 프로파일에서 제거된 Manager API는 미지원 응답을 반환한다. | 필수 | ● | 정의 |
| FR-NSX-0232 | Policy Search 문법 범위 | 지원할 쿼리 문법 범위(필드 일치, AND/OR, 와일드카드 등)를 Coverage Matrix에 별도로 정의한다. | 권장 | – | 정의 |
| FR-NSX-0233 | 계약·자체 테스트 기반 범위 조정 | CMP 기능 참고 목록에서 후보를 찾고, 공식 API 계약·선행 요구사항·자체 테스트 가능 범위를 기준으로 지원 목록을 선정한다. 미관찰 API는 미검증으로 유지하며, 제외 시 D2·D9 결정 근거를 기록한다. CMP 소스 분석·실제 호출 수집은 수행하지 않는다(D13). | 필수 | ● | 정의 |

## 4.3 NSX Policy Resource 모델

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-NSX-0301 | 공통 필드 | id, display_name, description, path, relative_path, parent_path, resource_type, tags, `_revision`, `_create_time`, `_last_modified_time`, `_create_user`, `_last_modified_user`, `_links` 및 `_self`(해당 API에서 요구할 경우) | 필수 | ● | 정의 |
| FR-NSX-0302 | 리소스별 필드 차이 | 모든 필드가 모든 리소스에서 동일하게 존재한다고 가정하지 않는다. | 필수 | ● | 정의 |
| FR-NSX-0303 | 필드 속성 준수 | 리소스별 스키마의 필수·읽기전용·선택 필드를 준수한다. | 필수 | ● | 정의 |

## 4.4 NSX Policy 동작

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-NSX-0401 | Read | 리소스 단건 조회 | 필수 | ● | 정의 |
| FR-NSX-0402 | List | 리소스 목록 조회 | 필수 | ● | 정의 |
| FR-NSX-0403 | Create (upsert) | 생성은 대부분 POST가 아니라 클라이언트가 ID를 지정한 경로에 대한 `PATCH` 또는 `PUT`이다(예: `PATCH /policy/api/v1/infra/segments/{segment-id}`). 같은 ID로 다시 호출하면 갱신(upsert)이 된다. | 필수 | ● | 정의 |
| FR-NSX-0404 | Update | 리소스 수정 | 필수 | ● | 정의 |
| FR-NSX-0405 | PATCH·PUT 의미 구분 | PUT은 해당 API의 전체 교체 계약을 따른다. PATCH를 일률적인 부분 병합으로 처리하지 않는다. 활성 버전의 기본 동작, 시스템의 Partial Patch 설정, 요청 헤더 `nsx-enable-partial-patch`의 우선순위를 적용한다. 부분 PATCH 활성/비활성, 생략 필드·배열 처리, Infra 및 Security Policy rules 등 리소스별 예외를 계약 테스트로 검증한다. | 필수 | ● | 정의 |
| FR-NSX-0406 | Delete | 리소스 삭제 | 필수 | ● | 정의 |
| FR-NSX-0407 | id와 unique_id 분리 | 경로 ID(`id`)와 시스템 발급 `unique_id`를 별개로 관리한다. | 필수 | ● | 정의 |
| FR-NSX-0408 | Revision Update | 변경 시 `_revision`을 갱신한다. | 필수 | ● | 정의 |
| FR-NSX-0409 | 동시 수정 충돌 검사 | `_revision`을 저장된 객체의 최신 버전과 비교한다. PUT·PATCH·DELETE의 충돌 동작은 API별·버전별 공식 계약을 따르며, 일률적으로 409로 처리하지 않고 명세가 요구하는 412 등을 구분한다. | 필수 | ● | 정의 |
| FR-NSX-0410 | 참조 리소스 유효성 | 참조 대상 리소스의 존재·유효성을 검사한다. | 필수 | ● | 정의 |
| FR-NSX-0411 | 계층적 Path | 계층적 Path를 처리한다. | 필수 | ● | 정의 |
| FR-NSX-0412 | 부모·자식 관계 | 부모·자식 Resource 관계를 관리한다. | 필수 | ● | 정의 |
| FR-NSX-0413 | 필터·검색 | 목록 필터와 검색을 지원한다. | 권장 | – | 정의 |
| FR-NSX-0414 | Pagination·Cursor | Pagination 및 Cursor를 지원한다. | 필수 | ● | 정의 |
| FR-NSX-0415 | 삭제 의존관계 검사 | 삭제 시 의존관계를 검사한다. | 필수 | ● | 정의 |
| FR-NSX-0416 | 응답·오류 형식 | 응답 본문과 오류 형식을 실제 버전에 맞춘다. | 필수 | ● | 정의 |

**계약 근거:** [공식 NSX API Guide](https://developer.broadcom.com/xapis/nsx-t-data-center-rest-api/latest/)의 Partial PATCH 설명은 기본 비활성, 요청 헤더의 시스템 설정 우선, 배열 교체 및 리소스별 예외를 명시한다. [Segment PATCH](https://developer.broadcom.com/xapis/nsx-t-data-center-rest-api/latest/method_PatchInfraSegment.html)의 성공 응답은 `200 OK`, 본문 없음이다. 따라서 객체·`_revision`은 후속 GET으로 확인하며 생성 응답 본문을 임의로 추가하지 않는다. 위 링크는 2026-10-10 확인한 공식 문서이며, `latest` 내용을 모든 버전에 복사하지 않고 D1에서 정한 버전·빌드의 공식 스펙·문서 근거를 프로파일에 고정한다(FR-API-0307 ~ FR-API-0309).

## 4.5 Policy Realization

Policy API에 설정을 저장한 것과 해당 정책이 실제 적용된 것으로 보고되는 상태는 다를 수 있다. 시뮬레이터는 데이터플레인을 구성하지 않지만 그 논리적 단계를 재현한다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-NSX-0501 | Realization 논리 단계 | ① 테스트 클라이언트가 Policy 생성·수정 요청 → ② 구성 상태 저장 → ③ Realization 작업 예약 → ④ 진행 상태 반영 → ⑤ 설정된 지연 후 완료 또는 오류 반영 → ⑥ 상태 조회 시 해당 시점의 상태 반환 | 필수 | ● | 정의 |
| FR-NSX-0502 | 즉시 정상 완료 | 지연 없이 완료 상태로 보고한다. | 필수 | ● | 정의 |
| FR-NSX-0503 | 지연 후 정상 완료 | 설정된 지연 후 완료 상태로 보고한다. | 필수 | ● | 정의 |
| FR-NSX-0504 | Realization 실패 | 실패 상태와 오류 정보를 보고한다. | 필수 | – | 정의 |
| FR-NSX-0505 | 변경 중 재수정 | Realization 진행 중 설정이 다시 수정되는 경우를 처리한다. | 권장 | – | 정의 |
| FR-NSX-0506 | Realization Timeout | Timeout 상황을 재현한다. | 권장 | – | 정의 |
| FR-NSX-0507 | 의존 리소스 누락 | 의존 리소스 누락으로 인한 미완료·오류를 재현한다. | 권장 | – | 정의 |
| FR-NSX-0508 | 삭제된 리소스 조회 | 이미 삭제된 리소스의 Realization 상태 조회를 계약대로 응답한다. | 필수 | ● | 정의 |
| FR-NSX-0509 | 상태 필드 매핑 | `consolidated_status`, realized entity, alarm, error 필드와의 매핑을 버전별 문서 기준으로 구현한다. | 필수 | ● | 정의 |

## 4.6 NSX와 vCenter 연동

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-NSX-0601 | Compute Manager ↔ vCenter | NSX Compute Manager 등록 정보와 가상 vCenter의 관계를 관리한다. 등록 정보의 vCenter 인증서 지문(thumbprint)은 가상 vCenter가 실제로 제시하는 인증서와 일치해야 한다(PRD-3 NFR-0315). | 필수 | ● | 정의 |
| FR-NSX-0602 | Fabric/Host ↔ Host | NSX Fabric/Host 관련 객체와 가상 Host의 관계를 표현한다. | 필수 | – | 정의 |
| FR-NSX-0603 | Segment ↔ vCenter Network | NSX Segment와 대응 가능한 vCenter Network의 관계를 표현한다. 모든 Segment가 자동으로 vCenter Network에 생성된다고 가정하지 않으며, 대상 버전·기능의 실제 노출 조건을 분석해 매핑 규칙을 구현한다. | 필수 | – | 정의 |
| FR-NSX-0604 | VM vNIC ↔ Network | VM vNIC과 가상 Network의 연결 관계를 표현한다. | 필수 | – | 정의 |
| FR-NSX-0605 | VM 관련 정책 참조 | VM과 관련된 NSX 정책 참조를 표현한다. | 권장 | – | 정의 |
| FR-NSX-0606 | Network ID ↔ Policy Path | Network ID와 NSX Policy Path 간 매핑을 관리한다. | 필수 | – | 정의 |
| FR-NSX-0607 | 버전별 네트워크 표현 | 활성 프로파일과 Fixture의 스위치 유형으로 vCenter 측 표현과 vNIC Backing을 결정한다(아래 표). §19 D8에서 지원 방식을 결정한다. | 필수 | ○ | 정의 |
| FR-NSX-0608 | NSX VM 인벤토리 파생 | vCenter VM의 생성·변경·삭제를 vcsim 변경 핸들러로 NSX Fabric VM, VIF, (연결 시) Segment Port에 반영한다. 연결 키는 VM `instanceUuid`(NSX `external_id`)와 vNIC 정보다. | 권장 | ○ | 정의 |
| FR-NSX-0609 | NSX VM Tag 소유권 | NSX에서 부여한 VM Tag는 NSX 저장소가 소유하며 vCenter Tag와 혼동하지 않는다. | 필수 | ○ | 정의 |

**참고 — 버전 프로파일이 결정하는 네트워크 표현 규칙**

| 환경 | vCenter 측 표현 | VM vNIC Backing |
|---|---|---|
| N-VDS 기반 (구형 NSX-T) | `OpaqueNetwork` | `VirtualEthernetCardOpaqueNetworkBackingInfo` |
| VDS 7.0+ 기반 NSX | `DistributedVirtualPortgroup` (`backingType = nsx`, `logicalSwitchUuid`, `segmentId`) | `VirtualEthernetCardDistributedVirtualPortBackingInfo` |

---

# 5. Behavioral Simulation Engine

## 5.1 역할

Behavioral Simulation Engine은 본 프로젝트의 핵심이다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-BE-0101 | 의미 기반 상태 시뮬레이션 | 단순히 JSON을 반환하지 않고, 요청의 의미를 해석해 내부 가상 인프라 상태를 변경하며 이후 조회 결과를 일관되게 생성한다. | 필수 | ● | 정의 |
| FR-BE-0102 | 모듈 구성 | 권장 모듈: Resource State Manager, State Transition Engine, Task Scheduler, Virtual Clock, Resource Relationship Manager, Constraint Validator, Capacity Calculator, Event Generator, Fault Injection Engine, NSX Realization Controller | 권장 | ● | 정의 |

## 5.2 상태 관리 원칙

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-BE-0201 | 안정적 ID | 동일 리소스는 지속적으로 안정적인 ID를 가진다. | 필수 | ● | 정의 |
| FR-BE-0202 | 변경 반영 | 정상적으로 완료된 변경은 이후 조회에 반영된다. | 필수 | ● | 정의 |
| FR-BE-0203 | 참조 무결성 | 리소스 간 참조 무결성을 유지한다. | 필수 | ● | 정의 |
| FR-BE-0204 | 멱등성 | 같은 API를 반복 호출하면 실제 제품의 멱등성 규칙을 따른다. | 필수 | ● | 정의 |
| FR-BE-0205 | 동시 변경 충돌 | 동시 변경의 충돌을 검사한다. | 필수 | ● | 정의 |
| FR-BE-0206 | 연관 참조 정리 | 삭제한 리소스와 연관된 참조를 정리한다. | 필수 | ● | 정의 |
| FR-BE-0207 | 작업·리소스 상태 구분 | 비동기 작업에서는 작업 상태와 리소스 상태를 구분한다. | 필수 | ● | 정의 |
| FR-BE-0208 | 재기동 후 복원 선택 | 시뮬레이터 재기동 이후 상태 복원 여부를 선택할 수 있다. | 권장 | – | 정의 |
| FR-BE-0209 | 변경 원인 추적 | 모든 상태 변화의 원인을 추적할 수 있다. | 필수 | – | 정의 |
| FR-BE-0210 | 재현성 | 동일한 Seed와 테스트 시나리오로 재현 가능한 결과를 생성한다. | 필수 | ● | 정의 |

## 5.3 vCenter와 NSX 상태 소유권

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-BE-0301 | 데이터 소유권 | vCenter Managed Object 원본: vcsim / NSX Policy·Manager Resource 원본: NSX State Repository / vCenter–NSX 연결 정보: Relationship Repository / 테스트 시나리오·작업 기록: Simulation Repository | 필수 | ● | 정의 |
| FR-BE-0302 | 원본 중복 보관 금지 | 다른 저장소에 같은 객체의 원본 상태를 중복 보관하지 않는다. | 필수 | ● | 정의 |
| FR-BE-0303 | 캐시 동기화 | 읽기 성능을 위한 캐시는 허용하되 원본 변경과 동기화한다. | 필수 | ● | 정의 |
| FR-BE-0304 | 단일 프로세스 일관성 (권고안 A) | 모든 저장소가 하나의 프로세스 안에 있으므로 교차 시스템 변경을 단일 락 또는 단일 트랜잭션 경계 안에서 처리한다. Outbox·Saga는 재기동 복구 용도로만 최소한으로 사용한다. | 필수 | ● | 정의 |
| FR-BE-0305 | 교차 시스템 절차 (권고안 B) | 권고안 B 전용. D3에서 A를 채택해 보류한다. 상세는 PRD-B §B4, §B6. | 선택 | – | 보류 |

## 5.4 vCenter Behavior Adapter

vcsim과 분리된 별도 프로세스(Bridge + 외부 행위 엔진) 구조에서는 클라이언트의 SOAP 요청(예: `CreateVM_Task`)이 vcsim으로 직행하므로, 행위 엔진이 처리 후에야 변경을 알게 되어 요청을 거부할 수 없다. 이를 해결하기 위해 Adapter를 vcsim과 같은 프로세스에 둔다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-BE-0401 | 프로세스 내 Adapter | vCenter 측 행위 로직은 vcsim과 같은 Go 프로세스 내부의 Adapter로 구현한다. | 필수 | ● | 정의 |
| FR-BE-0402 | 요청 경로 내 동기 처리 | 사전조건 검사, 용량 검증, 의미적 Fault 주입, Task 지연·실패 제어는 요청 처리 경로 안에서 동기로 실행한다. | 필수 | ● | 정의 |
| FR-BE-0403 | 요청 후 비동기 처리 | Event 생성, NSX Fabric 인벤토리 파생, Audit 기록은 요청 처리 후 비동기로 실행할 수 있다. | 권장 | – | 정의 |
| FR-BE-0404 | Adapter 기능 | vCenter 객체 조회, 가상 객체 생성·변경·삭제, VM–Network 관계 업데이트, Task 생성 및 상태 변경, PropertyCollector 변경 이벤트 반영, vCenter 변경 이벤트 수집을 MVP에 구현한다. 인벤토리 스냅샷 생성·상태 복원은 Phase 5의 WP 5.2에서 구현·검증한다. | 필수 | ● | 정의 |
| FR-BE-0405 | 권고안 B 연동 | 권고안 B 전용. D3에서 A를 채택해 보류한다. 상세는 PRD-B §B4. | 선택 | – | 보류 |
| FR-BE-0406 | 간접 변경 금지 | 외부 프로세스에서 임의의 SOAP API를 반복 호출해 상태를 간접 변경하는 방식은 사용하지 않는다. | 필수 | ● | 정의 |
| FR-BE-0407 | Hook 부족 시 대안 비교 | 요청 경로 안의 Hook가 부족하면 vcsim 내부 Method Handler 확장, vcsim 변경 이벤트 구독, PropertyCollector 기반 변경 추적, 정기적 Inventory Reconciliation, 최소 변경 Fork를 비교한다. | 검토 | ● | 정의 |
| FR-BE-0408 | 착수 전 기술 검증 | 본 절의 구현 가능성은 개발 착수 전 기술 검증이 필요한 핵심 과제로 관리한다. | 필수 | ● | 정의 |

## 5.5 비동기 Task Engine

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-BE-0501 | 작업 상태 전이 | QUEUED → RUNNING → SUCCESS, QUEUED → RUNNING → ERROR | 필수 | ● | 정의 |
| FR-BE-0502 | 작업 속성 | Operation ID, Target Resource, Operation Type, Initiator, Current State, Progress, Started At, Completed At, Result, Error, Parent/Child Operation 관계 | 필수 | ● | 정의 |
| FR-BE-0503 | API별 계약 변환 | 내부 작업 모델을 외부에 노출할 때 SOAP Task, vCenter REST 작업, NSX Policy Realization의 각기 다른 계약으로 변환한다. 모든 VMware API가 같은 Task 객체를 반환한다고 가정하지 않는다. | 필수 | ● | 정의 |

## 5.6 Virtual Clock

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-BE-0601 | 기본 실시간 동작 | 기본은 실시간으로 동작한다. | 필수 | ● | 정의 |
| FR-BE-0602 | 시간 가속 | 테스트 시간을 가속할 수 있다. | 권장 | – | 정의 |
| FR-BE-0603 | 시간 정지 | 가상 시간을 정지할 수 있다. | 권장 | – | 정의 |
| FR-BE-0604 | 특정 시점 진행 | 관리자가 특정 시점까지 가상 시간을 진행시켜 장시간 대기 상황을 빠르게 검증할 수 있다. | 권장 | – | 정의 |
| FR-BE-0605 | Task 완료 지연 설정 | Task 완료 지연을 설정할 수 있다. | 권장 | – | 정의 |
| FR-BE-0606 | Realization 지연 설정 | NSX Realization 지연을 설정할 수 있다. | 필수 | ● | 정의 |
| FR-BE-0607 | Session 만료 시뮬레이션 | 가상 시간에 따른 Session 만료를 재현한다. | 권장 | – | 정의 |
| FR-BE-0608 | 통계 데이터 시간 생성 | 가상 시간 기준으로 통계 데이터를 생성한다. | 선택 | – | 정의 |
| FR-BE-0609 | 정상·테스트 모드 | 정상 모드는 일반 API 호출 순서대로 동작하고, 테스트 모드는 관리자가 가상 시간을 제어한다. | 권장 | – | 정의 |
| FR-BE-0610 | 실제·논리 시간 분리 | HTTP 타임아웃과 실제 네트워크 대기 동작이 깨지지 않도록 실제 경과 시간과 논리적 이벤트 시간을 분리한다. | 필수 | ● | 정의 |
| FR-BE-0611 | vcsim 시간 소스 | vcsim이 Task·Event 시각, 세션 만료에 실제 시계를 쓰는지 확인하고, 가상 시간 적용을 위한 시간 소스 교체 확장 지점(없으면 최소 패치 Fork)을 검증한다(Risk 12). | 검토 | – | 정의 |
| FR-BE-0612 | 이벤트 기준 타이머 | Virtual Clock과 Task Scheduler는 주기적 폴링이 아닌 다음 예정 이벤트 기준 타이머로 동작하여 유휴 시 CPU를 사용하지 않는다. | 필수 | ● | 정의 |

## 5.7 Capacity Simulation

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-BE-0701 | 가상 용량 모델 | 실제 CPU·RAM을 할당하지 않고 가상 자원 배치를 검증한다. 예: Host CPU 64 Core, Host Memory 256GB, Datastore 10TB, VM CPU 8 Core, VM Memory 32GB, VM Disk 200GB | 필수 | ● | 정의 |
| FR-BE-0702 | 용량 계산 | 테스트 클라이언트가 자원 생성 가능 여부를 판단할 수 있도록 용량·할당량·여유 공간을 계산한다. | 필수 | ● | 정의 |
| FR-BE-0703 | 고급 용량 모델 | Overcommit, Reservation, Provisioned Capacity 등은 대상 버전과 시뮬레이션 요구에 따라 선택적으로 모델링한다. | 선택 | – | 정의 |
| FR-BE-0704 | 메트릭·할당량 정합 | 메트릭에 쓰는 가상 사용률과 실제 할당량이 서로 모순되지 않도록 한다. | 필수 | – | 정의 |

---

# 6. Virtual SDDC 데이터 모델

## 6.1 주요 엔터티

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-DM-0101 | 논리 도메인 구성 | 데이터 모델은 아래 표의 논리 영역과 엔터티를 포함한다. | 필수 | ● | 정의 |

**참고 — 도메인별 주요 엔터티**

| Domain | 주요 엔터티 |
|---|---|
| Environment | Environment, Site, CompatibilityProfile |
| Identity | User, Role, Permission, Session |
| vCenter | Datacenter, Cluster, Host, ResourcePool, Datastore, VM, Network |
| NSX | Segment, Tier0, Tier1, Group, Service, SecurityPolicy, Rule |
| Relations | ComputeManagerLink, NetworkMapping, VMNicMapping |
| Operations | Operation, Task, Transition, Realization |
| Events | Event, Alarm, PropertyChange, OutboxEvent |
| Testing | Scenario, Fixture, FaultRule, Snapshot, TestRun |

## 6.2 ID 관리

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-DM-0201 | ID 형식 구분 | Simulator Internal UUID, vCenter MoRef, VM Instance UUID, BIOS UUID, NSX Resource ID, NSX Policy Path, NSX Realization ID, Task MoRef, REST Resource ID를 명확히 구분한다. | 필수 | ● | 정의 |
| FR-DM-0202 | 단일 UUID 반환 금지 | 하나의 내부 UUID를 모든 API의 ID로 무조건 반환하지 않는다. | 필수 | ● | 정의 |
| FR-DM-0203 | 계약 기반 ID 생성 | API에 노출되는 ID는 해당 VMware API 계약에 맞게 생성한다. | 필수 | ● | 정의 |
| FR-DM-0204 | REST ID = MoRef | vSphere Automation API(`/api`, `/rest`)의 VM·Host·Cluster·Datastore·Network·Datacenter·Folder·ResourcePool 식별자는 SOAP MoRef 값과 동일한 문자열이다(예: `vm-42`, `host-15`, `domain-c8`, `datastore-11`, `datacenter-3`, `group-v4`, `resgroup-9`). 'REST Resource ID'는 Tagging 등 MoRef가 없는 vAPI 리소스의 ID 형식을 가리킨다. | 필수 | ● | 정의 |
| FR-DM-0205 | 중복 방지 | ID 중복을 방지한다. | 필수 | ● | 정의 |
| FR-DM-0206 | 안정적 참조 | ID로 안정적으로 참조할 수 있다. | 필수 | ● | 정의 |
| FR-DM-0207 | 환경 간 Namespace 분리 | 환경 간 ID Namespace를 분리한다. | 권장 | – | 정의 |
| FR-DM-0208 | 삭제 후 재사용 규칙 | 삭제 후 ID 재사용을 방지하거나 실제 제품 규칙을 따른다. | 필수 | ● | 정의 |
| FR-DM-0209 | 복원 시 참조 일관성 | Snapshot 복원 시 참조 일관성을 유지한다. | 필수 | – | 정의 |
| FR-DM-0210 | 결정론적 Fixture ID | 같은 Fixture에서 같은 ID가 생성된다. | 필수 | ● | 정의 |

## 6.3 Database 및 Persistence

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-DM-0301 | SQLite 사용 | 초기 구현은 SQLite를 사용한다. | 필수 | ● | 정의 |
| FR-DM-0302 | 예상 테이블 | environments, compatibility_profiles, nsx_resources, resource_relationships, simulation_operations, operation_transitions, nsx_realizations, event_outbox, audit_events, identity_sessions, fault_rules, scenarios, scenario_executions, snapshots, api_trace_metadata | 권장 | ● | 정의 |
| FR-DM-0303 | vCenter 인벤토리 중복 저장 제외 | vCenter 전체 인벤토리를 SQLite에 중복 저장하지 않는다. | 필수 | ● | 정의 |
| FR-DM-0304 | 일관된 Snapshot 설계 | vcsim의 저장·복원 기능을 분석하고, NSX DB 및 관계 데이터와 함께 일관된 Snapshot을 만드는 방법을 설계한다. | 필수 | – | 정의 |
| FR-DM-0305 | 무결성 절차 | 장애·재기동 중 데이터 무결성을 위해 Transaction, Lock, Recovery 절차를 정의한다. | 필수 | ● | 정의 |
| FR-DM-0306 | 단일 락 Snapshot (권고안 A) | Snapshot 시 단일 락을 잡고 vcsim 인벤토리 덤프와 SQLite 백업을 함께 수행해 같은 시점의 Snapshot을 만든다. | 필수 | – | 정의 |
| FR-DM-0307 | lite 인메모리 저장소 | `lite` 실행 프로필에서는 SQLite를 인메모리 모드로 사용하고, 명시적 Snapshot 시에만 파일로 기록할 수 있다. | 필수 | ● | 정의 |

## 6.4 Cross-System Consistency

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-DM-0401 | 교차 조회 일관성 | 예: ① NSX Segment 생성 → ② Segment와 Network 관계 등록 → ③ VM vNIC이 해당 Network 참조 → ④ 테스트 클라이언트가 Segment 조회 → ⑤ 테스트 클라이언트가 VM 조회 → ⑥ 테스트 클라이언트가 NSX 정책·연결 정보 조회. 모든 조회 결과는 동일한 논리적 구성을 표현한다. | 필수 | – | 정의 |
| FR-DM-0402 | 일관성 기법 검토 | 단순 DB 트랜잭션으로 부족한 경우를 위해 Resource Mapping, Transaction Boundary, Outbox Event, Reconciliation, Saga/Compensation, 상태 불일치 검출, 재시도 처리를 검토한다. | 검토 | – | 정의 |
| FR-DM-0403 | 부분 성공 방지 | 내부 통합 동작 실패 시 한쪽 API만 성공한 것처럼 영구히 남지 않도록 복구 규칙을 정의한다. | 필수 | – | 정의 |
| FR-DM-0404 | 동기화 전 상태 표시 | 동기화 완료 전 상태가 클라이언트에 어떻게 보여야 하는지 정의하고 테스트 대상으로 삼는다. | 필수 | – | 정의 |

---

# 7. API Compatibility Layer

## 7.1 핵심 역할

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-API-0101 | 동일 외부 인터페이스 | 실제 VMware 서버와 동일한 외부 인터페이스를 제공한다. 대상 요소: HTTP Method, URL Path, Query Parameter, Request/Response Header, HTTP Status Code, Content-Type, JSON/XML Serialization, SOAP Namespace, Cookie, Session, Redirect, TLS Certificate, API Version, 오류 응답, Timeout | 필수 | ● | 정의 |
| FR-API-0102 | 응답 임의 표준화 금지 | 각 요소는 API별 기준 계약을 따르며, Gateway가 응답을 임의로 표준화하지 않는다. | 필수 | ● | 정의 |

## 7.2 인증 및 세션

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-API-0201 | vCenter SOAP 로그인 | SOAP `SessionManager.Login`, Logout | 필수 | ● | 정의 |
| FR-API-0202 | vCenter REST 세션 | REST Session 생성, `vmware-api-session-id` | 필수 | ● | 정의 |
| FR-API-0203 | vCenter 세션 검증 | Session Validation | 필수 | ● | 정의 |
| FR-API-0204 | vCenter 세션 만료 | Session Timeout | 필수 | – | 정의 |
| FR-API-0205 | 사용자별 세션 분리 | 사용자별 Session을 분리한다. | 필수 | ● | 정의 |
| FR-API-0206 | NSX Basic 인증 | Basic Authentication | 필수 | ● | 정의 |
| FR-API-0207 | NSX 세션 | Session Create/Destroy, Cookie, XSRF Token | 필수 | ● | 정의 |
| FR-API-0208 | NSX API별 인증 방식 | API별 인증 방식 차이를 따른다. | 필수 | ● | 정의 |
| FR-API-0209 | NSX 세션 만료 | Session Timeout | 필수 | – | 정의 |
| FR-API-0210 | 사용자·Credential | 사용자 목록과 사용자별 Credential을 관리한다. | 필수 | ● | 정의 |
| FR-API-0211 | 권한 모델 | Role, Permission, Resource Scope, API Access Control | 권장 | – | 정의 |
| FR-API-0212 | 권한 부족 오류 | 권한 부족 시 계약에 맞는 오류를 반환한다. | 필수 | – | 정의 |
| FR-API-0213 | 자체 권한 시나리오 | 실제 VMware 권한 모델 전체를 처음부터 구현할 필요는 없으나, 테스트 클라이언트가 검증하는 권한 시나리오는 지원한다. | 필수 | – | 정의 |
| FR-API-0214 | 인증 정보 하드코딩 금지 | 인증 정보를 코드에 하드코딩하지 않는다. | 필수 | ● | 정의 |

## 7.3 버전 프로파일

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-API-0301 | 실행 시 버전 입력 | 실행 시 대상 버전을 입력받는다. 예: `cloudinfra-sim --vcenter-version 7.0.3.02500 --nsx-version 3.2.4.3` 또는 `SIM_VCENTER_VERSION=7.0.3.02500 SIM_NSX_VERSION=3.2.4.3 cloudinfra-sim` (첫 기준 조합의 버전 값) | 필수 | ● | 정의 |
| FR-API-0302 | 프로파일 기동 검증 | 미등록 프로파일, 잘못된 스키마, 허용되지 않은 vCenter·NSX 조합은 항상 기동을 거부한다. 검증 수준은 `--verification-mode release` 또는 `development`로 지정하며 아래 '프로파일 기동 검증 모드'를 따른다. 지정하지 않으면 배포 패키지 빌드는 `release`, 소스·개발 빌드는 `development`를 기본값으로 사용하고, 사용 중인 모드를 기동 로그에 표시한다. 거부 시 미달 API·증거와 사용 가능한 프로파일의 검증 상태를 출력한다. | 필수 | ● | 정의 |
| FR-API-0303 | 실행 중 프로파일 변경 | 실행 중 프로파일 변경은 Environment Reset을 동반한다. | 권장 | – | 정의 |
| FR-API-0304 | 활성 프로파일 조회 | Admin API로 활성 프로파일과 검증 상태를 조회할 수 있다. | 필수 | ● | 정의 |
| FR-API-0305 | 프로파일 구성 | 아래 '프로파일 디렉터리 구성'에 따라 vCenter·NSX 버전별 데이터와 허용 조합(`compatibility-matrix.yaml`)을 관리한다. | 필수 | ● | 정의 |
| FR-API-0306 | 프로파일 결정 항목 | 버전 노출 응답 값(`ServiceContent.about`, `/api/appliance/system/version`, NSX `/api/v1/node/version` 등), 버전 협상 응답과 버전 인식 직렬화(§3.6), 지원 경로·메서드·필드, 오류·Fault 계약, 버전별 동작 규칙(§4.6 등), Deprecated/Removed API 처리 정책 | 필수 | ● | 정의 |
| FR-API-0307 | 데이터 출처 | 프로파일 데이터는 공식 산출물(WSDL/XSD, OpenAPI 스펙, 공개 SDK 타입 메타데이터)에서 생성하며 출처·수집 일자·대상 버전과 빌드를 기록한다. 향후 실환경 Trace 사용은 D5·D13 재결정 시 별도 범위로 다룬다. | 필수 | ● | 정의 |
| FR-API-0308 | 수작업 규칙 근거 | 손으로 작성한 규칙에는 대상 버전의 공식 문서 또는 공개 SDK 근거를 표기한다. 자체 실행 결과만으로 기대 계약을 정의하지 않으며 근거가 부족한 동작은 미검증으로 기록한다. | 필수 | ● | 정의 |
| FR-API-0309 | 빌드 간 무검증 복사 금지 | 같은 메이저 버전이라도 다른 빌드의 동작을 검증 없이 복사하지 않는다. | 필수 | ● | 정의 |
| FR-API-0310 | 초기 적용 범위 | Phase 0~1: 프로파일 로더, 버전 인식 직렬화 골격, 기동 시 버전 검증을 구현하고 대상 버전 1개 조합의 데이터를 채운다. | 필수 | ● | 정의 |
| FR-API-0311 | 추가 버전 확장 | Phase 4b 이후: 추가 버전 프로파일 데이터를 채우고 버전 간 Differential Test를 수행한다. | 필수 | – | 정의 |
| FR-API-0312 | 대상 버전 결정 | 대상 버전 목록과 첫 기준 조합은 §19(D1)에서 결정한다. 결정된 범위는 vSphere/vCenter 7.0 U2 이상(8.x 포함), NSX-T 3.x(Policy API 기반, NSX 4.x 전용 기능 미사용)이다. vSphere 9(VCF 9 계열)는 현재 범위에 포함하지 않는다. vCenter × NSX 허용 조합은 VMware 상호운용성 매트릭스를 기준으로 `compatibility-matrix.yaml`(FR-API-0305)에 정의한다(vSphere 8.x는 NSX-T 3.2.x 일부 빌드와만 조합 가능). 첫 기준 조합은 vCenter 7.0 U3w(7.0.3.02500, 빌드 24927011) + NSX-T 3.2.4.3(빌드 24942842)(선정된 기준 패치)이며, 버전·빌드를 `profile.yaml`에 기록한다. 공식 스펙(WP 0.6)의 대상 버전·빌드와 출처를 기록한다. NSX 계약 근거는 Broadcom이 공개한 최신 3.2 계열 공식 스펙인 NSX-T 3.2.1(3.2.1.0.0) Manager·Policy OpenAPI로 한다. 3.2.4.x 스펙은 공개되지 않았으므로 3.2.4.3 프로파일에 3.2.1 스펙을 적용하고, 두 빌드 간 차이는 '미검증'으로 표시하며 3.2.2 ~ 3.2.4.3 릴리스 노트의 API 변경 사항으로 보완한다(FR-API-0309, 2026-10-10 결정). 실환경 Trace 확보는 현재 완료 조건이 아니다(D5·D13). | 필수 | ● | 정의 |

**프로파일 기동 검증 모드 (FR-API-0302)**

| 모드 | 최소 조건 | 용도 및 표시 |
|---|---|---|
| `release` (배포 패키지 빌드의 기본값) | 실행 범위에 포함된 모든 API가 현재 자체 검증 단계인 `Integration Tested`에 도달했으며, 현재 프로파일·구현에 대응하는 통과 증거가 존재한다. 증거는 CI가 프로파일·구현 식별자(커밋 해시 등)에 묶어 자동 생성한다. | 배포된 프로파일 실행용. 기동 허용은 최종 릴리스 승인이나 '100% 호환'을 뜻하지 않는다. |
| `development` (소스·개발 빌드의 기본값) | 실행 범위에 포함된 모든 API의 요청·응답·오류 계약이 정의되어 있다(`Contract Defined` 또는 이후 검증 단계). 구현된 미검증 API는 검증 목적으로 실행할 수 있고, 미구현 호출은 미지원 정책(§7.4)을 따른다. | 새 프로파일의 구현·테스트용. 로그·Admin API·리포트에 개발 모드와 미검증 상태를 표시한다. |

검증 대상인 실행 범위는 `profile.yaml`에 API Coverage Matrix의 API 식별자 목록으로 명시한다. 빈 목록은 허용하지 않으며, 선택한 Phase 1/MVP/릴리스 범위의 필수 로그인·버전·등록 API를 포함한다. 범위 밖 API는 기동 판정에서 제외하되 '지원됨'으로 표시하지 않는다. 현재 프로젝트는 자체 테스트로 `Integration Tested`까지만 판정하며 외부 검증 상태는 기동의 선행 조건으로 사용하지 않는다(D13). Phase 1에서는 개발 모드로 통합 테스트를 수행한 뒤 증거를 등록하고 `release` 모드 기동을 검증한다. 프로파일·계약·구현 변경 시 영향을 받는 증거를 무효화한다. 이 모드는 저장·리소스 설정인 `lite`/`full`과 별개의 축이다.

**참고 — 프로파일 디렉터리 구성**

```
30-implementation/data/compatibility-profiles/
  vcenter/7.0.3.02500/    # 첫 기준 조합: 7.0 U3w, 빌드 24927011
    profile.yaml          # 제품명, 버전, 빌드, API 버전, about/version 응답 값
    vim-typeinfo.json     # 타입·속성·메서드별 도입 버전 메타데이터
    vimServiceVersions.xml
    vapi-openapi.json     # vSphere Automation API 스펙
    behavior-rules.yaml   # 버전별 동작 차이 (네트워크 표현, 사전조건 등)
    faults.yaml           # 오류 코드·Fault 계약
    verification.yaml     # API별 검증 상태 (§12.2)
  nsx/3.2.4.3/            # 첫 기준 조합: 빌드 24942842 (계약 스펙은 공개본 3.2.1.0.0, 차이 미검증)
    profile.yaml          # node/version, product_version 응답 값 등
    nsx_api.json          # Manager API 스펙
    nsx_policy_api.json   # Policy API 스펙
    behavior-rules.yaml   # Realization, H-API, deprecation 등 (Projects/VPC 노출은 4.x 프로파일 전용)
    errors.yaml           # error_code, httpStatus 형식
    verification.yaml
  compatibility-matrix.yaml  # 허용되는 vCenter × NSX 조합
```

## 7.4 미지원 API 처리 정책

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-API-0401 | 성공 위장 금지 | 미지원 API를 정상 성공으로 위장하지 않는다. | 필수 | ● | 정의 |
| FR-API-0402 | 버전별 미지원 응답 | 가능하면 대상 VMware 버전의 적절한 미지원 동작 또는 오류를 반환한다. | 필수 | ● | 정의 |
| FR-API-0403 | 미지원 호출 Audit | 호출 API, Request Method, API Version, Request Parameters, 지원 여부, 미지원 사유, Test Scenario, 발생 횟수를 기록한다. | 필수 | ● | 정의 |
| FR-API-0404 | 미지원 API 자동 식별 | 자체 테스트 클라이언트가 호출하지만 시뮬레이터가 지원하지 않는 API를 자동 식별한다. 미호출 API를 자동으로 지원 범위에서 제거하지 않는다. | 필수 | ● | 정의 |

---

# 8. 오류 및 장애 재현

## 8.1 정상 동작 외의 필수 검증

자체 테스트 클라이언트로 오류 처리·복구 흐름을 검증할 수 있도록 다음 상황을 지원한다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-FLT-0101 | API 오류: 인증 실패 | 인증 실패 응답 | 필수 | ● | 정의 |
| FR-FLT-0102 | API 오류: 세션 만료 | 세션 만료 응답 | 필수 | – | 정의 |
| FR-FLT-0103 | API 오류: 권한 부족 | 권한 부족 응답 | 필수 | – | 정의 |
| FR-FLT-0104 | API 오류: 잘못된 입력 | 입력 검증 오류 | 필수 | ● | 정의 |
| FR-FLT-0105 | API 오류: 존재하지 않는 리소스 | Not Found 계열 응답 | 필수 | ● | 정의 |
| FR-FLT-0106 | API 오류: 중복 생성 | 중복 생성 오류 | 필수 | ● | 정의 |
| FR-FLT-0107 | API 오류: 리소스 사용 중 | 사용 중 리소스 변경·삭제 오류 | 필수 | – | 정의 |
| FR-FLT-0108 | API 오류: 버전 충돌 | Revision 등 버전 충돌 오류 | 필수 | ● | 정의 |
| FR-FLT-0109 | API 오류: 미지원 기능 | 미지원 기능 오류 | 필수 | ● | 정의 |
| FR-FLT-0110 | 인프라: Host Disconnected | Host 연결 끊김 | 필수 | – | 정의 |
| FR-FLT-0111 | 인프라: Host Maintenance Mode | Host 유지보수 모드 | 필수 | – | 정의 |
| FR-FLT-0112 | 인프라: Datastore 용량 부족 | Datastore Capacity 부족 | 필수 | – | 정의 |
| FR-FLT-0113 | 인프라: VM Power Operation 실패 | 전원 작업 실패 | 필수 | – | 정의 |
| FR-FLT-0114 | 인프라: VM Clone 실패 | Clone 실패 | 필수 | – | 정의 |
| FR-FLT-0115 | 인프라: ResourcePool 할당 실패 | ResourcePool 할당 실패 | 필수 | – | 정의 |
| FR-FLT-0116 | 인프라: Realization 실패 | NSX Policy Realization 실패 | 필수 | – | 정의 |
| FR-FLT-0117 | 통신: 응답 지연 | API 응답 지연 | 필수 | – | 정의 |
| FR-FLT-0118 | 통신: HTTP Timeout | HTTP Timeout | 필수 | – | 정의 |
| FR-FLT-0119 | 통신: Connection Reset | Connection Reset | 필수 | – | 정의 |
| FR-FLT-0120 | 통신: 지정 HTTP 오류 | 지정된 HTTP 상태 코드 반환 | 필수 | – | 정의 |
| FR-FLT-0121 | 작업: SOAP Fault | SOAP Fault 반환 | 필수 | ● | 정의 |
| FR-FLT-0122 | 작업: 비동기 Task 실패 | Task error 상태 전환 | 필수 | – | 정의 |
| FR-FLT-0123 | 통신: 일시적 503 | 일시적인 503 오류 | 필수 | – | 정의 |
| FR-FLT-0124 | 통신: 간헐적 오류 | 간헐적 오류 | 필수 | – | 정의 |
| FR-FLT-0125 | 통신: 지정 횟수 후 복구 | 지정 횟수 실패 후 정상 복구 | 필수 | – | 정의 |

## 8.2 Fault Injection Engine

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-FLT-0201 | 시나리오 기반 제어 | 오류 주입은 시나리오를 기준으로 제어한다. | 필수 | – | 정의 |
| FR-FLT-0202 | 주입 조건 | 특정 API Method, 특정 Resource ID, 특정 사용자, 특정 호출 순서, N번째 호출, 특정 시각, 특정 Task 상태, 지정된 확률 및 Seed | 필수 | – | 정의 |
| FR-FLT-0203 | 주입 동작 | 예: 다음 VM Power On 실패, 특정 Datastore 여유 공간 0, NSX Segment Realization 실패, 다음 요청 5초 지연, 특정 사용자 Session 만료, 특정 횟수 503 후 정상 응답 | 필수 | – | 정의 |
| FR-FLT-0204 | 주입 계층 분리 | 전송 계층, vCenter 의미 계층, NSX 의미 계층, 세션 계층으로 나누어 주입한다(아래 표). | 필수 | – | 정의 |
| FR-FLT-0205 | 공통 Fault Rule 모델 | 각 계층은 같은 Fault Rule 모델(조건 + 동작 + 횟수 + Seed)을 공유하고, 실제 오류 응답은 활성 프로파일의 오류 계약에 맞게 생성한다. | 필수 | – | 정의 |
| FR-FLT-0206 | Admin API 전용 제어 | Fault Injection 관리 기능은 전용 Admin API로만 제어한다. | 필수 | – | 정의 |
| FR-FLT-0207 | 임의 오류 금지 | 운영 중 정상 요청에 임의로 오류를 발생시키지 않는다. | 필수 | ● | 정의 |

**참고 — 주입 계층**

| 계층 | 위치 | 대상 예시 |
|---|---|---|
| 전송 계층 | 내장 Gateway | 응답 지연, HTTP Timeout, Connection Reset, 지정 HTTP 상태(503 등), N회 실패 후 복구 |
| 의미 계층 — vCenter | vCenter Behavior Adapter (요청 경로 내) | 특정 Fault 타입(`InsufficientResourcesFault`, `InvalidPowerState` 등), Task error 전환, Datastore 용량 0 |
| 의미 계층 — NSX | NSX 엔진 / Realization Controller | Realization 실패, 의존 리소스 누락, Revision 충돌 강제 |
| 세션 계층 | 세션 관리자 | 특정 사용자 세션 만료 |

---

# 9. 가상 메트릭·이벤트·알람

## 9.1 Metrics

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-MET-0101 | 성능·용량 조회 지원 | 테스트 클라이언트가 성능 또는 용량 정보를 조회하는 경우를 지원한다. 메트릭은 실제 하드웨어 사용량이 아닌 논리적 시뮬레이션 데이터다. | 권장 | – | 정의 |
| FR-MET-0102 | 가상 지표 | CPU Usage, CPU Capacity, Memory Usage, Memory Capacity, Storage Usage, Storage Free Space, VM CPU Utilization, VM Memory Utilization, Network Usage, Host/Cluster Summary | 권장 | – | 정의 |
| FR-MET-0103 | 메트릭 생성 기능 | 정적 데이터, 시간에 따른 변동, 패턴 기반 생성, 특정 리소스 사용량 변경, Threshold 초과, 결측값 또는 수집 실패, 지정 기간 조회 | 권장 | – | 정의 |
| FR-MET-0104 | 결정론적 즉석 계산 | 메트릭 시계열은 저장하지 않는다. 조회 시점에 (리소스 ID, 카운터, 시각, Seed, 패턴 규칙, Fault/Override)로 결정론적으로 계산한다. 명시적으로 덮어쓴 값(Override)만 저장한다. | 필수 | – | 정의 |

## 9.2 Events / Alarms

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| FR-MET-0201 | 이벤트 종류 | VM Created, VM Removed, VM Powered On, VM Powered Off, Host Disconnected, Datastore Usage Threshold, Task Failed, NSX Policy Realization Failed | 권장 | – | 정의 |
| FR-MET-0202 | 계약 기준 구현 | 실제 vCenter EventManager, PropertyCollector, NSX Alarm API 계약을 기준으로 구현한다. | 필수 | – | 정의 |
| FR-MET-0203 | 이벤트 속성 | 각 이벤트는 Resource ID, Timestamp, Initiator, Event Type 및 상세 내용을 가진다. | 필수 | – | 정의 |

---

## 요구사항 집계

각 장의 요구사항 수와 수준·MVP 분포이다. 요구사항을 추가·삭제하면 이 표도 갱신한다.

| 장 | 접두어 | 전체 | 필수 | 권장 | 선택 | 검토 | MVP ● | MVP ○ | MVP – |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| 3. vCenter API Simulation Engine | `FR-VC` | 63 | 44 | 16 | 2 | 1 | 45 | 8 | 10 |
| 4. NSX API Simulation Engine | `FR-NSX` | 78 | 56 | 19 | 3 | 0 | 44 | 10 | 24 |
| 5. Behavioral Simulation Engine | `FR-BE` | 44 | 29 | 9 | 4 | 2 | 29 | 0 | 15 |
| 6. Virtual SDDC 데이터 모델 | `FR-DM` | 22 | 19 | 2 | 0 | 1 | 14 | 0 | 8 |
| 7. API Compatibility Layer | `FR-API` | 32 | 30 | 2 | 0 | 0 | 25 | 0 | 7 |
| 8. 오류 및 장애 재현 | `FR-FLT` | 32 | 32 | 0 | 0 | 0 | 8 | 0 | 24 |
| 9. 가상 메트릭·이벤트·알람 | `FR-MET` | 7 | 3 | 4 | 0 | 0 | 0 | 0 | 7 |
| **합계** |  | **278** | **213** | **52** | **9** | **4** | **165** | **18** | **95** |
