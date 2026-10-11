# CMP-Oriented VMware SDDC API Simulator
## 목표 아키텍처 상세 명세서 / Codex 분석 및 개발계획 수립 지침

**문서 버전:** 1.0  
**프로젝트 유형:** API Compatibility Simulator / Stateful Behavioral Emulator  
**주요 기술:** Go, govmomi/vcsim, Python, FastAPI, SQLite, Docker  
**목표 사용자:** VMware vCenter 및 NSX Manager를 연동하는 CMP 개발·QA 조직

---

# 1. 프로젝트 목적

## 1.1 최종 목표

실제 VMware 인프라(vCenter Server, ESXi, NSX Manager)를 설치하거나 연결하지 않고도 CMP가 VMware 환경을 정상적인 관리 대상으로 인식하고, VMware 관련 기능을 실행·검증할 수 있는 고충실도 API Simulator를 개발한다.

본 프로젝트에서 '시뮬레이션'이란 실제 가상머신이나 네트워크를 실행하는 것이 아니라, VMware API를 통해 관찰되는 객체, 상태, 작업, 이벤트, 오류, 응답 시간을 소프트웨어적으로 재현하는 것을 의미한다.

최종 목표는 다음과 같다.

1. CMP의 VMware 연동 코드를 수정하지 않고 Simulator를 사용할 수 있어야 한다.
2. CMP가 사용하는 vCenter SOAP 및 REST API를 지원해야 한다.
3. CMP가 사용하는 NSX Manager API 및 NSX Policy API를 지원해야 한다.
4. API 호출에 따른 상태 변화와 리소스 관계가 실제 VMware 제품의 동작 계약에 부합해야 한다.
5. 가상의 VM, Host, Cluster, Datastore, Network 및 NSX 리소스를 생성·변경·삭제·조회할 수 있어야 한다.
6. 인증, 인가, 세션, 비동기 작업, 이벤트, 동시성, 충돌 및 장애 동작을 재현할 수 있어야 한다.
7. 재현 가능한 테스트 환경을 생성하고 초기화할 수 있어야 한다.
8. 개발자 개인 PC에서 경량으로 실행할 수 있어야 한다.
9. 새로운 API와 VMware 버전을 확장할 수 있어야 한다.
10. CMP의 정상·예외 시나리오에 대한 자동화된 회귀 테스트를 지원해야 한다.

## 1.2 호환성의 정의

'완벽하게 동일한 동작'은 VMware 전체 기능의 복제를 뜻하지 않는다.

다음 조건을 모두 만족하는 것을 프로젝트의 목표 호환성으로 정의한다.

- 사전에 승인한 CMP 기능 및 시나리오 범위에서 API 요청·응답이 지정 VMware 버전의 계약과 일치한다.
- API로 관찰되는 논리적 상태와 객체 관계가 일관성을 유지한다.
- 여러 API를 연속적으로 호출했을 때 기대되는 상태 변화가 발생한다.
- 비동기 작업과 이벤트 감시가 CMP의 대기·완료 판단 로직을 충족한다.
- 오류·재시도·롤백 경로가 테스트 가능하다.
- 인증·권한·버전 관련 동작이 CMP에서 사용하는 범위 내에서 호환된다.

정확성은 단순한 HTTP 성공 코드가 아닌, API 계약과 시나리오 기반 테스트로 입증한다.

실제 제품에서 검증하지 못한 동작은 '검증 완료'로 표시하지 않는다.

## 1.3 범위에서 제외할 항목

다음은 구현하지 않는다.

- 실제 ESXi 하이퍼바이저
- 실제 가상머신 및 Guest OS 실행
- 실제 vCenter Appliance
- 실제 NSX Manager Appliance
- 실제 NSX Edge, TEP, Overlay 및 데이터플레인
- 실제 L2/L3 패킷 전달
- 실제 분산 방화벽 패킷 처리
- EVE-NG, GNS3, Containerlab 등의 네트워크 에뮬레이터
- 실제 스토리지 볼륨 및 VMDK 데이터 저장
- 실제 VM 이미지의 전송·부팅
- 실제 물리 장비 제어

다만 위 기능의 API 관찰 결과는 필요한 범위에서 가상 상태로 표현할 수 있다.

예를 들어 VM을 Power On하면 Guest OS를 실행하는 대신 해당 VM의 API 전원 상태를 `poweredOn`으로 변경한다. Guest IP나 VMware Tools 상태가 필요한 경우에는 테스트 데이터와 상태 전이 규칙에 따라 가상 값을 제공한다.

파일 업로드, 템플릿 배포 등 실제 바이너리 전송을 동반하는 기능은 CMP가 사용하는 API 계약 수준에서 별도 지원 여부를 결정한다.

---

# 2. 전체 시스템 아키텍처

## 2.1 아키텍처 개요

시스템은 다음 계층으로 구성한다.

**Layer 1 — API Consumer**
- CMP
- Terraform
- Ansible
- VMware SDK
- 기타 VMware API 클라이언트

**Layer 2 — API Compatibility Gateway**
- HTTPS 및 인증서 처리
- 도메인 기반 라우팅
- API 경로 및 헤더 전달
- 실제 VMware 응답 계약 보존
- 요청 추적
- 내부 관리자 API 분리

**Layer 3 — VMware API Simulation Services**
- vCenter SOAP API Engine
- vCenter REST/vAPI Engine
- NSX Manager API Engine
- NSX Policy API Engine

**Layer 4 — Behavioral Simulation Engine**
- 논리적 인프라 상태 모델
- 리소스 상태 전이
- 작업 스케줄러
- 이벤트 및 알람 생성
- Policy Realization 처리
- API 간 관계 동기화
- 용량 및 배치 제약조건
- 오류 및 지연 시뮬레이션

**Layer 5 — Virtual SDDC State Repository**
- vCenter 시뮬레이터 인벤토리
- NSX 리소스 저장소
- Cross-System Resource Mapping
- Task 및 Operation 기록
- Event 및 Audit Log
- 테스트 환경 Snapshot
- 시나리오 정의와 설정

**Layer 6 — Test & Administration**
- 환경 Seed 및 Reset
- Scenario Runner
- Fault Injection
- API Coverage 검증
- Contract Test
- Test Report

## 2.2 외부 연결 구조

CMP가 접근하는 엔드포인트는 다음처럼 구성한다.

| 논리적 서비스 | 예시 주소 | 외부 포트 |
|---|---|---|
| vCenter | https://vc.lab.local | 443 |
| NSX Manager | https://nsx.lab.local | 443 |
| Simulator Admin | http://127.0.0.1:9000 | 9000 |

내부 서비스 예시:

| 서비스 | 내부 포트 | 기술 |
|---|---:|---|
| vCenter Simulator | 8989 | Go / vcsim |
| NSX Simulator | 8000 | Python / FastAPI |
| Simulation Controller | 8100 | Python |
| vCenter Bridge | 8200 | Go |
| HTTPS Gateway | 443 | Caddy 또는 Nginx |

포트는 초기 설계안이며 충돌 여부를 분석해 조정할 수 있다.

vCenter와 NSX의 실제 API 경로·메서드·헤더를 임의로 변경해서는 안 된다.

vCenter의 `/sdk`, `/api`, 구형 `/rest` 및 NSX의 `/api/v1`, `/policy/api/v1`을 각각 해당 서비스로 전달한다.

시뮬레이터 전용 관리 API는 VMware API와 명확히 분리하며 CMP로 노출하지 않는다.

개발 환경은 Windows, macOS, Linux 호스트에서 Docker 기반으로 실행 가능하도록 한다. Windows 환경에서는 Docker Desktop/WSL2 지원을 우선 검토한다.

---

# 3. vCenter API Simulation Engine

## 3.1 기반 기술

vCenter의 핵심 엔진은 `govmomi/vcsim`을 활용한다.

참고 저장소:

https://github.com/vmware/govmomi

https://github.com/vmware/govmomi/tree/main/vcsim

최초 개발 시 vcsim의 지원 메서드와 시뮬레이션 기능을 조사한다.

가능하면 vcsim 원본을 직접 대량 수정하지 않고 확장 모듈을 작성한다.

그러나 확장 포인트가 부족해 핵심 인벤토리·Task·PropertyCollector와 동기화할 수 없다면, 특정 버전을 고정한 Fork 방식을 검토한다.

Fork를 사용한다면 업스트림 변경을 추적하고 필요한 패치만 최소화한다.

## 3.2 지원 API 계열

다음 API를 지원 대상으로 분석한다.

### A. vSphere Web Services API — SOAP

주요 경로:
- `/sdk`

주요 영역:
- ServiceInstance
- SessionManager
- PropertyCollector
- ViewManager
- SearchIndex
- Folder
- Datacenter
- ClusterComputeResource
- ComputeResource
- HostSystem
- ResourcePool
- VirtualMachine
- Datastore
- Network
- DistributedVirtualSwitch
- DistributedVirtualPortgroup
- TaskManager
- EventManager
- PerformanceManager
- CustomFieldsManager
- AuthorizationManager

위 전체를 무조건 구현하는 것이 아니라 CMP의 실제 호출 여부를 분석하여 구현 우선순위를 결정한다.

### B. vSphere Automation REST API

주요 경로:
- `/api/session`
- `/api/vcenter/vm`
- `/api/vcenter/datacenter`
- `/api/vcenter/cluster`
- `/api/vcenter/host`
- `/api/vcenter/datastore`
- `/api/vcenter/network`
- `/api/cis/tagging/...`

### C. Legacy REST API

주요 경로:
- `/rest/com/vmware/cis/session`
- `/rest/vcenter/...`
- `/rest/com/vmware/cis/tagging/...`

구형 REST API와 최신 REST API는 응답의 래핑 구조 등이 다를 수 있으므로 반드시 버전별 계약을 검증한다.

구형 REST 경로를 최신 API로 단순 변환하면서 응답 형태를 바꾸지 않는다.

## 3.3 vCenter 가상 리소스

최소 지원 대상:

**Datacenter**
- ID 및 MoRef
- Name
- Folder 관계
- Host Folder
- VM Folder
- Datastore Folder
- Network Folder

**Cluster**
- ID 및 MoRef
- Name
- Datacenter 관계
- Host 목록
- ResourcePool
- DRS/HA 설정의 논리적 상태
- 전체 CPU/Memory Capacity

**Host**
- ID 및 MoRef
- Name
- Management IP
- Connection State
- Power State
- CPU/Memory Capacity
- Cluster 관계
- Datastore 및 Network 관계
- Maintenance Mode

**Datastore**
- ID 및 MoRef
- Name
- Datastore Type
- Capacity
- Free Space
- Accessibility
- Host Mount 관계

**VM**
- ID 및 MoRef
- Instance UUID
- BIOS UUID
- Name
- Guest OS ID
- CPU Count
- Memory Size
- Power State
- Connection State
- Guest Information
- VM Tools 상태
- Datacenter/Cluster/Host 관계
- ResourcePool 관계
- Datastore 관계
- Virtual Disk 목록
- vNIC 목록
- Network 연결
- Snapshot 목록
- Template 여부
- Tags 및 Custom Attributes

**Network**
- Standard Network
- Distributed Portgroup
- NSX-backed Network
- Network ID 및 MoRef
- VM 연결 관계
- DVS 관계

리소스는 VMware API 스키마에 맞는 값으로 표현하고 내부에서 별도의 도메인 모델을 유지할 수 있다.

## 3.4 VM 생명주기

다음 작업을 지원하도록 설계한다.

- Create
- Clone
- Register
- Reconfigure
- Power On
- Power Off
- Reset
- Suspend
- Delete
- Rename
- Move
- Snapshot Create
- Snapshot Revert
- Snapshot Delete
- Template Convert
- Guest Customization의 논리적 상태 전이

모든 작업은 실제 해당 API가 요구하는 사전조건을 확인한다.

예를 들어:
- 존재하지 않는 Host에 VM을 배치할 수 없다.
- 존재하지 않는 Datastore는 사용할 수 없다.
- CPU와 Memory는 허용 범위를 벗어나면 오류가 발생한다.
- 전원 상태에 따라 특정 작업이 실패할 수 있다.
- 이미 삭제한 VM을 조회하면 지정 API의 Not Found 동작을 반환한다.
- VM 삭제 시 연결된 리소스 관계가 일관되게 정리되어야 한다.

실제 VMware의 버전별 규칙을 확인하고 구현한다. 모든 전원·Snapshot 작업의 규칙을 임의로 동일하게 취급하지 않는다.

## 3.5 SOAP Task 및 PropertyCollector

vCenter 동작 호환성의 핵심 요소이다.

특히 다음을 구현 또는 검증한다.

**Task**
- ManagedObjectReference 발급
- queued/running/success/error 상태
- 시작·종료 시각
- 진행률
- 결과 객체 참조
- Fault 정보
- 완료 후 인벤토리 변경
- 이벤트 발생
- Task 조회 및 대기

**PropertyCollector**
- RetrievePropertiesEx
- ContinueRetrievePropertiesEx
- CreateFilter
- DestroyPropertyFilter
- WaitForUpdatesEx
- 세션별 필터 관리
- 버전 토큰 관리
- 변경 속성 반환
- Timeout 및 부분 응답
- 인벤토리 변화 감지

특히 SOAP Task, REST 조회, PropertyCollector 결과가 동일한 VM 상태를 나타내야 한다.

vcsim에서 이미 지원하는 동작은 재사용하고 누락된 부분만 보완한다.

## 3.6 버전 호환성

선택 가능한 Compatibility Profile 구조를 적용한다.

예시:
- vSphere 7.x
- vSphere 8.x
- 향후 검증된 vSphere 버전

프로파일에는 다음을 포함한다.

- Product Name
- Version
- Build
- API Version
- 지원 API Method
- 지원 Resource Property
- 필수 필드
- 오류 코드 및 Fault
- 응답 구조
- 버전별 동작 차이

Version 문자열만 변경해 지원하지 않는 API가 지원되는 것처럼 보이게 하지 않는다.

---

# 4. NSX API Simulation Engine

## 4.1 기반 기술

NSX Simulator는 Python FastAPI로 개발한다.

권장 기술:
- Python
- FastAPI
- Pydantic
- SQLAlchemy
- SQLite
- pytest
- OpenAPI/JSON Schema 기반 모델 생성

NSX 제품 버전별 공식 API 명세를 기본 계약으로 사용한다.

참고 문서:

https://developer.broadcom.com/xapis/nsx-t-data-center-rest-api/latest/

공식 API 문서에서 제공하는 OpenAPI 스키마를 활용한다. 필요시 라이선스 및 접근 권한을 확인하여 버전별 스키마를 확보한다.

## 4.2 지원 API 영역

**A. Manager API**
- 인증 및 세션
- NSX Manager 버전
- Node/Cluster 상태 조회
- Fabric 및 Compute Manager
- Transport Zone
- Transport Node 관련 논리적 정보
- 필요한 Logical Networking 조회
- 기타 CMP에서 실제 사용하는 API

**B. Policy API**
- Infrastructure
- Transport Zones
- Segments
- Tier-0 Gateways
- Tier-1 Gateways
- Gateway Interfaces
- Groups
- Services
- Security Policies
- Firewall Rules
- Tags
- Realization State
- Policy Search
- 관련 Statistics 및 Status API

CMP의 실제 API 호출 패턴에 따라 목록을 추가·제외한다.

## 4.3 NSX Policy Resource 모델

모든 Policy 리소스는 다음과 같은 공통 개념을 지원한다.

- id
- display_name
- description
- path
- relative_path
- parent_path
- resource_type
- tags
- _revision
- _create_time
- _last_modified_time
- _create_user
- _last_modified_user
- _links 및 _self (해당 API에서 요구할 경우)

모든 필드가 모든 리소스에서 동일하게 존재한다고 가정하지 않는다.

리소스별 스키마의 필수·읽기전용·선택 필드를 준수해야 한다.

## 4.4 NSX Policy 동작

다음 동작을 구현한다.

- Resource Create
- Resource Read
- Resource List
- Resource Update
- Resource Delete
- PATCH 및 PUT 의미 구분
- Revision Update
- 동시 수정 충돌 검사
- 참조 리소스 유효성 검사
- 계층적 Path 처리
- 부모·자식 Resource 관계
- 필터·검색
- Pagination 및 Cursor
- 삭제 시 의존관계 검사

`_revision`은 저장된 객체의 최신 버전과 비교한다.

PUT, PATCH, DELETE에 대한 충돌 동작은 API별·버전별 공식 계약을 따른다. 모든 충돌을 일률적으로 HTTP 409로 처리하지 않으며, 해당 명세에서 요구하는 412 등도 구분한다.

응답 본문과 오류 형식 역시 실제 버전에 맞춰야 한다.

## 4.5 Policy Realization

NSX Policy에서 가장 중요한 기능 중 하나이다.

Policy API에 설정을 저장한 것과 해당 정책이 실제 적용된 것으로 보고되는 상태는 다를 수 있다.

Simulator는 실제 데이터플레인을 구성하지 않지만 다음 논리적 단계를 재현한다.

1. CMP가 NSX Policy 생성 또는 수정 요청
2. 구성 상태 저장
3. Realization 작업 예약
4. Realization 진행 상태 반영
5. 설정된 지연 시간이 지난 뒤 완료 또는 오류 상태 반영
6. CMP가 상태 조회 API를 호출하면 해당 시점의 상태 반환

다음 상황을 지원한다.

- 즉시 정상 완료
- 지연 후 정상 완료
- Realization 실패
- 설정 변경 중 재수정
- Realization Timeout
- 의존 리소스 누락
- 이미 삭제된 리소스 조회

실제 API의 `consolidated_status`, realized entity, alarm 및 error 필드와의 매핑은 버전별 문서를 기준으로 구현한다.

## 4.6 NSX와 vCenter 연동

NSX의 Compute Manager 등록 정보와 vCenter 시뮬레이터 사이의 관계를 관리한다.

다음 관계를 표현한다.

- NSX Compute Manager → 가상 vCenter
- NSX Fabric/Host 관련 객체 → 가상 Host
- NSX Segment → 대응 가능한 vCenter Network
- VM vNIC → 가상 Network
- VM 관련 NSX 정책 참조
- Network ID와 NSX Policy Path 간 매핑

단, 모든 NSX Segment가 자동으로 vCenter Network에 생성된다고 가정하지 않는다.

대상 VMware 버전 및 기능의 실제 노출 조건을 분석해 매핑 규칙을 구현한다.

---

# 5. Behavioral Simulation Engine

## 5.1 역할

Behavioral Simulation Engine은 본 프로젝트의 핵심이다.

단순히 API 요청에 대해 JSON을 반환하는 것이 아니라, 요청의 의미를 해석하여 내부 가상 인프라 상태를 변경하고 향후 조회 결과를 일관되게 생성해야 한다.

권장 모듈:

- Resource State Manager
- State Transition Engine
- Task Scheduler
- Virtual Clock
- Resource Relationship Manager
- Constraint Validator
- Capacity Calculator
- Event Generator
- Fault Injection Engine
- NSX Realization Controller

## 5.2 상태 관리 원칙

다음 원칙을 적용한다.

1. 동일 리소스는 지속적으로 안정적인 ID를 가진다.
2. 정상적으로 완료된 변경은 이후 조회에 반영된다.
3. 리소스 간 참조 무결성을 유지한다.
4. 같은 API를 반복 호출하면 실제 제품의 멱등성 규칙에 따른다.
5. 동시 변경의 충돌을 검사한다.
6. 삭제한 리소스와 연관된 참조를 정리한다.
7. 비동기 작업에서는 작업 상태와 리소스 상태를 구분한다.
8. 재시작 이후 상태 복원 여부를 선택할 수 있다.
9. 모든 상태 변화의 원인을 추적할 수 있어야 한다.
10. 동일한 Seed와 테스트 시나리오로 재현 가능한 결과를 생성한다.

## 5.3 vCenter와 NSX 상태 소유권

다중 저장소로 인한 데이터 불일치를 방지해야 한다.

권장 데이터 소유권:

- vCenter Managed Object의 원본 상태: vcsim
- NSX Policy/Manager Resource의 원본 상태: NSX State Repository
- vCenter와 NSX 간 연결 정보: Relationship Repository
- Simulator의 테스트 시나리오·작업 기록: Simulation Repository

다른 저장소에 같은 객체의 원본 상태를 중복 보관하지 않는다.

읽기 성능을 위한 캐시는 허용하지만 원본 변경과 동기화해야 한다.

## 5.4 vCenter Bridge

Python Simulation Engine이 vcsim의 내부 객체를 안전하게 조작하고 변경 이벤트를 수신하기 위한 Go 기반 Bridge를 설계한다.

필요한 기능:

- vCenter 객체 조회
- 가상 객체 생성·변경·삭제
- VM과 Network 관계 업데이트
- Task 생성 및 상태 변경
- PropertyCollector 변경 이벤트 반영
- vCenter 변경 이벤트 수집
- 인벤토리 스냅샷 생성
- 상태 복원

가능하면 vcsim 프로세스 내부 확장 또는 동일한 Go 런타임 내 Adapter를 우선 고려한다.

외부 Python에서 임의의 SOAP API를 반복 호출해 상태를 간접 변경하는 방식은 보조 수단으로만 검토한다.

특히 CMP가 직접 호출한 SOAP 요청의 결과를 Python Coordinator가 어떻게 감지할지 반드시 설계해야 한다.

실현 가능한 이벤트 Hook가 없을 경우 다음 대안을 비교한다.

- vcsim 내부 Method Handler 확장
- vcsim 변경 이벤트 구독
- PropertyCollector 기반 변경 추적
- 정기적 Inventory Reconciliation
- 최소 변경 Fork

이 항목은 개발 착수 전 기술 검증이 필요한 핵심 과제이다.

## 5.5 비동기 Task Engine

논리적 작업의 상태 전이를 지원한다.

예시:

QUEUED → RUNNING → SUCCESS

QUEUED → RUNNING → ERROR

비동기 작업은 다음을 가진다.

- Operation ID
- Target Resource
- Operation Type
- Initiator
- Current State
- Progress
- Started At
- Completed At
- Result
- Error
- Parent/Child Operation 관계

내부 상태 모델을 외부 API에 노출할 때는 SOAP Task, vCenter REST 작업, NSX Policy Realization의 각기 다른 계약으로 변환한다.

모든 VMware API가 동일한 Task 객체를 반환한다고 가정해서는 안 된다.

## 5.6 Virtual Clock

테스트 가능한 가상 시간 제어 기능을 제공한다.

기능:
- 기본 실시간 동작
- 테스트 시간 가속
- 시간 정지
- 특정 시점으로 진행
- Task 완료 지연 설정
- NSX Realization 지연 설정
- Session 만료 시뮬레이션
- 통계 데이터 시간 생성

정상 모드에서는 일반적인 API 호출 순서에 따라 동작한다.

테스트 모드에서는 관리자가 가상 시간을 진행시켜 장시간 대기 상황을 빠르게 검증할 수 있어야 한다.

가상 시간 제어로 인해 HTTP 타임아웃 및 실제 네트워크 요청 대기 동작이 깨지지 않도록 실제 경과 시간과 논리적 이벤트 시간을 분리한다.

## 5.7 Capacity Simulation

실제 CPU나 RAM을 할당하지 않고도 가상의 자원 배치를 검증할 수 있어야 한다.

예를 들어:
- Host CPU 64 Core
- Host Memory 256GB
- Datastore Capacity 10TB
- VM CPU 8 Core
- VM Memory 32GB
- VM Disk 200GB

CMP가 자원 생성 가능 여부를 판단할 수 있도록 용량·할당량·여유 공간을 계산한다.

Overcommit, Reservation, Provisioned Capacity 등은 지원할 VMware 버전과 CMP 요구에 따라 선택적으로 모델링한다.

메트릭에 사용되는 가상 사용률과 실제 할당량은 서로 모순되지 않도록 한다.

---

# 6. Virtual SDDC 데이터 모델

## 6.1 주요 엔터티

데이터 모델은 다음 논리적 영역을 포함해야 한다.

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

ID는 다음 형식을 명확하게 구분한다.

- Simulator Internal UUID
- vCenter MoRef
- vCenter VM Instance UUID
- vCenter BIOS UUID
- NSX Resource ID
- NSX Policy Path
- NSX Realization ID
- Task MoRef
- REST Resource ID

하나의 내부 UUID를 모든 API의 ID로 무조건 반환하지 않는다.

API에 노출되는 ID는 해당 VMware API 계약에 맞게 생성한다.

ID 생성 정책은 다음을 지원한다.

- 중복 방지
- 안정적 참조
- 환경 간 Namespace 분리
- 삭제 후 ID 재사용 방지 또는 실제 규칙 준수
- Snapshot 복원 시 참조 일관성
- 결정론적 Fixture ID

## 6.3 Database 및 Persistence

초기 구현은 SQLite를 사용한다.

예상 테이블:

- environments
- compatibility_profiles
- nsx_resources
- resource_relationships
- simulation_operations
- operation_transitions
- nsx_realizations
- event_outbox
- audit_events
- identity_sessions
- fault_rules
- scenarios
- scenario_executions
- snapshots
- api_trace_metadata

vCenter 전체 인벤토리를 SQLite에 중복 저장하는 것은 기본 설계에서 제외한다.

vcsim의 저장·복원 기능을 분석하고, NSX DB 및 관계 데이터와 함께 일관된 Snapshot을 만들 수 있는 방법을 설계한다.

장애·재시작 중 데이터 무결성을 위해 Transaction, Lock 및 Recovery 절차를 정의한다.

## 6.4 Cross-System Consistency

예시 상황:

1. NSX Segment 생성
2. 해당 Segment와 연결된 Network 관계 등록
3. VM vNIC이 해당 Network 참조
4. CMP가 NSX Segment 조회
5. CMP가 vCenter VM 조회
6. CMP가 NSX 정책 또는 연결 정보를 조회

모든 조회 결과는 동일한 논리적 구성을 표현해야 한다.

여러 시스템의 객체를 함께 변경해야 하는 경우 단순한 DB 트랜잭션만으로 충분하지 않을 수 있다.

따라서 다음을 검토한다.

- Resource Mapping
- Transaction Boundary
- Outbox Event
- Reconciliation
- Saga/Compensation
- 상태 불일치 검출
- 재시도 처리

내부 통합 동작이 실패했을 때 한쪽 API만 성공한 것처럼 영구적으로 남지 않도록 복구 규칙을 정의한다.

동기화 완료 전 상태가 CMP에 어떻게 표시되어야 하는지도 테스트 대상이다.

---

# 7. API Compatibility Layer

## 7.1 핵심 역할

API Compatibility Layer는 실제 VMware 서버와 동일한 외부 인터페이스를 제공한다.

고려해야 할 요소:

- HTTP Method
- URL Path
- Query Parameter
- Request Header
- Response Header
- HTTP Status Code
- Content-Type
- JSON/XML Serialization
- SOAP Namespace
- Cookie
- Session
- Redirect
- TLS Certificate
- API Version
- 오류 응답
- Timeout

각 항목은 API별 기준 계약을 따르며, Gateway가 응답을 임의로 표준화하지 않는다.

## 7.2 인증 및 세션

### vCenter

- SOAP SessionManager.Login
- SOAP Logout
- REST Session 생성
- `vmware-api-session-id`
- Session Validation
- Session Timeout
- 사용자별 Session 분리

### NSX

- Basic Authentication
- Session Create/Destroy
- Cookie
- XSRF Token
- API별 인증 방식
- Session Timeout

### 인증·인가 정책

- 사용자 목록
- 사용자별 Credential
- Role
- Permission
- Resource Scope
- API Access Control
- 권한 부족 오류

실제 VMware와 동일한 권한 모델 전체를 처음부터 구현할 필요는 없지만 CMP가 검증하는 권한 시나리오는 지원해야 한다.

인증 정보를 코드에 하드코딩하지 않는다.

## 7.3 버전 프로파일

다음 정보를 환경별로 구성할 수 있어야 한다.

- vCenter Version
- vCenter Build
- vSphere API Version
- NSX Version
- 지원 Feature Set
- Error Contract
- Response Contract
- Behavior Rules
- Deprecated API 처리 정책

초기 버전에서는 하나의 vCenter/NSX 버전 조합을 기준으로 정확성을 확보한다.

그 후 Compatibility Profile을 추가한다.

## 7.4 미지원 API 처리 정책

미지원 API를 정상 성공으로 위장하지 않는다.

가능하면 대상 VMware 버전의 적절한 미지원 동작 또는 오류를 반환한다.

동시에 내부 Audit에는 다음을 기록한다.

- 호출 API
- Request Method
- API Version
- Request Parameters
- 지원 여부
- 미지원 사유
- CMP Scenario
- 발생 횟수

이를 통해 CMP가 실제 호출하지만 Simulator가 지원하지 않는 API를 자동으로 식별해야 한다.

---

# 8. 오류 및 장애 재현

## 8.1 정상 동작 외의 필수 검증

CMP의 오류 처리 및 복구 로직을 검증하기 위해 다음 상황을 지원한다.

**API 오류**
- 인증 실패
- 세션 만료
- 권한 부족
- 잘못된 입력
- 존재하지 않는 리소스
- 중복 생성
- 리소스 사용 중
- 버전 충돌
- 미지원 기능

**인프라의 논리적 오류**
- Host Disconnected
- Host Maintenance Mode
- Datastore Capacity 부족
- VM Power Operation 실패
- VM Clone 실패
- ResourcePool 할당 실패
- NSX Policy Realization 실패

**통신·작업 오류**
- API 응답 지연
- HTTP Timeout
- Connection Reset
- 지정된 HTTP 오류
- SOAP Fault
- 비동기 Task 실패
- 일시적인 503 오류
- 간헐적 오류
- 지정 횟수 후 정상 복구

## 8.2 Fault Injection Engine

오류 주입은 시나리오를 기준으로 제어한다.

조건 예시:

- 특정 API Method
- 특정 Resource ID
- 특정 사용자
- 특정 호출 순서
- N번째 호출
- 특정 시각
- 특정 Task 상태
- 지정된 확률 및 Seed

동작 예시:

- 다음 VM Power On 요청을 실패시킨다.
- 특정 Datastore의 여유 공간을 0으로 만든다.
- NSX Segment Realization을 실패시킨다.
- 다음 API 요청을 5초 지연한다.
- 특정 사용자의 Session을 만료시킨다.
- 특정 횟수 동안 503을 반환한 뒤 정상 응답한다.

Fault Injection 관리 기능은 전용 Admin API로만 제어한다.

운영 중 정상 요청에 임의로 오류를 발생시키지 않는다.

---

# 9. 가상 메트릭·이벤트·알람

## 9.1 Metrics

CMP가 성능 또는 용량 정보를 조회하는 경우를 지원한다.

가상 지표 예시:

- CPU Usage
- CPU Capacity
- Memory Usage
- Memory Capacity
- Storage Usage
- Storage Free Space
- VM CPU Utilization
- VM Memory Utilization
- Network Usage
- Host/Cluster Summary

메트릭은 실제 하드웨어 사용량이 아니라 논리적 시뮬레이션 데이터다.

지원 기능:
- 정적 데이터
- 시간에 따른 변동
- 패턴 기반 생성
- 특정 리소스의 사용량 변경
- Threshold 초과
- 결측값 또는 수집 실패
- 지정 기간 조회

## 9.2 Events / Alarms

CMP가 이벤트나 알람을 조회하는 경우를 위해 다음을 지원한다.

- VM Created
- VM Removed
- VM Powered On
- VM Powered Off
- Host Disconnected
- Datastore Usage Threshold
- Task Failed
- NSX Policy Realization Failed

실제 vCenter EventManager, PropertyCollector, NSX Alarm API 계약을 기준으로 구현한다.

각 이벤트는 Resource ID, Timestamp, Initiator, Event Type 및 상세 내용을 가진다.

---

# 10. Scenario & Test Controller

## 10.1 목적

Simulator를 단순한 API 서버가 아니라 재현 가능한 CMP 개발·QA 플랫폼으로 사용하기 위한 관리 기능이다.

필수 기능:

- Environment Create
- Environment Reset
- Seed Apply
- State Snapshot
- Snapshot Restore
- Scenario Start
- Scenario Stop
- Virtual Clock Control
- Fault Rule Add/Remove
- Event Inspection
- API Call Trace
- Coverage Report

## 10.2 Fixture 기반 환경 생성

YAML 또는 JSON으로 테스트 환경을 선언할 수 있어야 한다.

예시 환경:

**Site A**
- Datacenter: DC-A
- Cluster: CL-A
- Host: 4개
- ResourcePool: 3개
- Datastore: 5개
- Network: 6개
- VM: 50개

**NSX**
- Compute Manager: 1개
- Transport Zone: 2개
- Tier-0 Gateway: 1개
- Tier-1 Gateway: 3개
- Segment: 10개
- Group: 5개
- Security Policy: 5개

이 숫자는 예시이며 환경별로 변경 가능해야 한다.

가상 리소스는 실제 CPU·RAM을 소비하지 않으므로 다수의 논리적 리소스를 생성할 수 있어야 한다.

단, 객체 수가 증가하면 Simulator 자체의 메모리와 처리 부하는 발생한다.

## 10.3 시나리오 정의

테스트는 다음 순서로 구성할 수 있다.

1. 초기 환경 생성
2. 지정 사용자 로그인
3. CMP API Workflow 실행
4. 중간 상태 조회
5. Fault Injection
6. 완료 또는 실패 확인
7. 상태 검증
8. API Trace 저장
9. Test Report 생성
10. 환경 초기화

시나리오는 가능하면 선언형 파일로 정의해 Git에서 버전 관리한다.

---

# 11. 필수 End-to-End 테스트 시나리오

## Scenario 01 — CMP 연결 등록

1. CMP가 가상 vCenter에 연결한다.
2. 인증에 성공한다.
3. 제품 버전을 조회한다.
4. Datacenter, Cluster, Host 등을 조회한다.
5. CMP가 연결 가능한 VMware 환경으로 등록한다.

성공 기준:
- API 오류 없이 인프라 등록 성공
- 필요한 모든 리소스 조회 성공
- CMP에서 실제와 동일한 타입의 자원으로 인식

## Scenario 02 — 인벤토리 동기화

1. CMP가 VM 목록을 조회한다.
2. Simulator에서 VM의 CPU 값을 변경한다.
3. CMP가 재조회 또는 변경 감시를 수행한다.
4. 변경된 CPU 값이 반영된다.

성공 기준:
- 동일 VM의 ID 유지
- 변경된 속성 반영
- 불필요한 중복 VM 생성 없음
- 지원하는 이벤트 감시 방식과 일치

## Scenario 03 — VM 생성

1. CMP가 VM Create 요청을 호출한다.
2. Simulator가 필요한 Task를 생성한다.
3. Task 상태가 진행된다.
4. 가상 VM 객체가 생성된다.
5. Task가 성공 상태가 된다.
6. CMP가 VM을 조회한다.

성공 기준:
- VM 생성 Workflow 완료
- 고유 ID 생성
- Host/Cluster/Datastore 관계 정상
- Task와 인벤토리 상태 일치

## Scenario 04 — VM 변경

1. CMP가 CPU 및 Memory 변경 요청을 보낸다.
2. 변경 전제조건을 확인한다.
3. 비동기 작업을 수행한다.
4. VM Config와 Summary가 변경된다.
5. CMP가 변경 결과를 확인한다.

성공 기준:
- 실제 API 규칙에 맞는 결과
- 변경 전후 조회 일관성

## Scenario 05 — VM 삭제

1. CMP가 VM 삭제를 요청한다.
2. VM 상태 및 권한을 검사한다.
3. 삭제 Task를 처리한다.
4. 인벤토리에서 VM을 제거한다.
5. 관련 리소스 참조를 정리한다.

성공 기준:
- 삭제 Workflow 완료
- 삭제한 VM의 재조회 동작이 실제 API 계약과 일치
- 관련 관계에서 고아 참조가 발생하지 않음

## Scenario 06 — NSX Segment 생성

1. CMP가 NSX Segment를 생성한다.
2. API가 객체 및 Revision을 반환한다.
3. Resource Path가 생성된다.
4. Realization 상태가 변경된다.
5. CMP가 Segment를 조회한다.

성공 기준:
- API 계약 준수
- 상태 저장 및 재조회 정상
- Realization 완료 판단 정상

## Scenario 07 — NSX Security Policy 변경

1. CMP가 Group을 생성한다.
2. Service를 생성한다.
3. Security Policy와 Rule을 생성한다.
4. 기존 Rule을 수정한다.
5. 최신 Revision을 조회한다.
6. 잘못된 Revision으로 변경을 시도한다.

성공 기준:
- 정상 변경 성공
- Revision 증가
- 충돌 응답이 해당 API 계약과 일치

## Scenario 08 — vCenter + NSX 통합 Workflow

1. NSX Segment를 생성한다.
2. 요구되는 Network 관계를 구성한다.
3. VM을 생성한다.
4. VM vNIC을 Network에 연결한다.
5. CMP가 양 시스템에서 관련 리소스를 조회한다.
6. VM을 삭제한다.
7. NSX Segment를 삭제한다.

성공 기준:
- API 간 Resource ID 관계 일치
- 상태 불일치 없음
- 삭제 의존관계 정상 처리

## Scenario 09 — 장애와 복구

1. VM 생성 요청
2. Task 실행 중 실패 주입
3. CMP 오류 감지
4. CMP의 재시도 또는 보상 처리
5. 최종 상태 조회

성공 기준:
- 지정 오류를 정확하게 반환
- CMP 복구 Workflow 정상
- 불필요한 중복 리소스 없음

## Scenario 10 — 세션 만료

1. CMP 로그인
2. Session 생성
3. 만료 이벤트 주입
4. CMP 재요청
5. 인증 실패 확인
6. CMP 재로그인 또는 복구

성공 기준:
- Session 처리 계약 준수
- CMP의 재인증 로직 정상

---

# 12. API Coverage 및 호환성 검증

## 12.1 API Coverage Matrix

개발 초기부터 API Coverage Matrix를 관리한다.

필수 열:

- Product: vCenter 또는 NSX
- Product Version
- API Family: SOAP / REST / Policy / Manager
- Method
- Path 또는 SOAP Operation
- Consumer Function
- Request Schema
- Response Schema
- Authentication
- Side Effect
- Async Behavior
- Fault Behavior
- Simulator Implementation
- Verification Status
- Priority
- Related Test Case

## 12.2 API 구현 상태 구분

모든 API는 다음 중 하나로 분류한다.

- Not Analyzed
- Unsupported
- Contract Defined
- Implemented
- Unit Tested
- Integration Tested
- CMP Verified
- Reference VMware Verified

`Implemented`와 `Reference VMware Verified`를 동일하게 취급하지 않는다.

## 12.3 검증 방식

다음 방법을 조합한다.

**Contract Test**
- 요청/응답 스키마
- HTTP 상태 코드
- SOAP Envelope/Fault
- 필드 타입
- 필수/선택 속성
- Version Compatibility

**Stateful Test**
- Create → Get
- Update → Get
- Delete → Get
- 반복 Update
- 비동기 완료
- 여러 API 간 상태 일치

**Negative Test**
- 잘못된 Credential
- 권한 부족
- 잘못된 Resource ID
- 중복 생성
- Conflict
- Timeout
- Task Failure

**SDK Integration Test**
- govc
- pyVmomi
- govmomi
- CMP가 실제 사용하는 SDK
- 실제 사용되는 경우 Terraform Provider와 Ansible

**CMP End-to-End Test**
- CMP 연결
- 자원 동기화
- 프로비저닝
- 변경
- 삭제
- 장애 처리

## 12.4 Record / Replay 및 Differential Test

실제 VMware 환경에서 수집한 API Trace가 있다면 다음 데이터를 비교한다.

- Method 및 Path
- 요청 Body
- Response Status
- Response Header
- Response Body
- Task Polling Sequence
- State Transition
- Error Response

동적 ID, Timestamp, Session Token 등은 의미를 보존하는 정규화 규칙으로 처리한다.

실제 Trace가 없는 경우 공식 문서와 SDK 소스코드를 기준으로 테스트를 작성한다.

이 경우 실제 제품과의 미검증 영역을 명시한다.

Trace에는 인증 정보, 개인정보 및 운영 인프라 정보가 포함될 수 있으므로 저장 전 비식별화한다.

---

# 13. 성능 및 비기능 요구사항

## 13.1 경량 실행

요구사항:

- 물리 VMware 제품 설치 불필요
- 실제 VM 실행 불필요
- 단일 PC에서 운영 가능
- Docker Compose 기반 실행
- 불필요한 외부 서비스 의존성 최소화
- 기본 저장소 SQLite

단일 환경뿐 아니라 리소스 수를 증가시키는 테스트도 지원해야 한다.

기본 성능 테스트 규모의 예:

- VM 100개
- VM 1,000개
- VM 5,000개
- NSX Segment 1,000개
- 동시 CMP 요청 10/50/100개

이 값들은 검증 시나리오이지 사전 보장 처리량이 아니다.

실제 성능 목표 및 RAM 한계는 구현 전 벤치마크 계획을 수립하고 측정한 결과로 정한다.

## 13.2 데이터 안정성

- 동시 요청에 대한 데이터 무결성
- 중복 생성 방지
- 안정적인 ID 관리
- SQLite Transaction
- 재시작 후 상태 복원
- Snapshot/Restore
- 상태 불일치 검출
- 작업 재처리 정책

## 13.3 보안

- 기본 Localhost 바인딩
- HTTPS 지원
- 인증서 교체 가능
- Credential 환경변수 또는 Secret 사용
- 관리자 API 인증
- 감사 로그
- 민감 정보 마스킹
- 원치 않는 외부 네트워크 접근 제한
- 실제 운영 VMware와 접속 대상 혼동 방지

실제 운영 환경의 자격 증명을 시뮬레이터에 재사용하지 않는 것을 원칙으로 한다.

## 13.4 Observability

다음 정보를 제공한다.

- API Request Log
- API Response Status
- Request Correlation ID
- Task ID
- Resource ID
- Operation Duration
- State Transition Log
- Fault Injection Log
- Unsupported API Log
- Error Log
- Test Execution Log

기본 로그는 사람이 읽을 수 있는 형태와 JSON 형태를 지원한다.

---

# 14. 권장 기술 아키텍처 및 디렉터리 구조

## 14.1 기술 스택

| 영역 | 권장 기술 |
|---|---|
| vCenter SOAP | Go + govmomi/vcsim |
| vCenter REST | govmomi vAPI Simulator + 필요한 Go 확장 |
| NSX API | Python + FastAPI + Pydantic |
| Behavioral Engine | Python |
| vCenter 내부 확장 | Go |
| State Repository | SQLite |
| Database Access | SQLAlchemy |
| HTTPS Gateway | Caddy 또는 Nginx |
| Local Deployment | Docker Compose |
| Test | pytest + Go Test |
| API Schema Validation | JSON Schema / OpenAPI |
| CI | GitHub Actions 등 |
| Configuration | YAML / JSON |

Python과 Go 사이의 내부 통신 방식은 개발 계획에서 비교 후 결정한다.

HTTP 기반 내부 Control API를 초기 후보로 검토하되, vcsim 내부 확장 방식이 더 단순하고 일관성을 보장한다면 우선한다.

## 14.2 Repository 구조

다음 디렉터리 구성을 초안으로 한다.

- `docs/`
  - `architecture/`
  - `api-coverage/`
  - `behavior-spec/`
  - `design-decisions/`
  - `development-plan/`
- `services/`
  - `gateway/`
  - `vcenter-simulator/`
  - `vcenter-bridge/`
  - `nsx-simulator/`
  - `simulation-engine/`
  - `admin-api/`
- `shared/`
  - `schemas/`
  - `contracts/`
  - `resource-models/`
- `data/`
  - `fixtures/`
  - `scenarios/`
  - `compatibility-profiles/`
- `tests/`
  - `unit/`
  - `contract/`
  - `integration/`
  - `cmp-e2e/`
  - `performance/`
- `deployment/`
  - `docker/`
  - `certificates/`
- `scripts/`
- `examples/`

추가로 루트 디렉터리에 다음 파일을 둔다.

- README.md
- compose.yaml
- .env.example
- pyproject.toml 또는 서비스별 환경 설정
- Makefile 또는 동등한 작업 실행 스크립트

멀티언어 빌드 및 배포 구조를 검토하여 조정할 수 있다.

---

# 15. 개발 우선순위 및 단계

Codex는 상세 개발계획을 다음 단계로 구분해 수립한다.

## Phase 0 — 기술 검증 및 설계

- govmomi/vcsim 구조 분석
- vcsim SOAP 지원 Method 조사
- vcsim REST/vAPI 지원 범위 조사
- vcsim 내부 상태 변경 Hook 검증
- vcsim Task/PropertyCollector 동작 검증
- NSX API 스키마 확보
- CMP 요구 API 분석 방법 수립
- 목표 버전 결정
- 통합 상태 소유권 설계
- 기술적 위험 식별

**완료 조건:** 실제 구현 가능한 아키텍처와 주요 기술적 제약을 확인한다.

## Phase 1 — 기본 API Simulator

- Docker Compose 구성
- Gateway 구성
- vcsim 서비스 구성
- NSX FastAPI 서비스 생성
- 기본 인증
- 기본 API 조회
- 초기 Fixture 생성
- API Logging

**완료 조건:** CMP 또는 SDK가 두 API 서버에 연결하고 기본 인벤토리를 조회한다.

## Phase 2 — Stateful Resource Simulation

- vCenter VM CRUD
- NSX Resource CRUD
- 상태 관리
- Resource ID Mapping
- Constraint Validation
- Persistence
- 비동기 Task

**완료 조건:** CMP의 기본 리소스 생명주기 시나리오가 통과한다.

## Phase 3 — Cross-System Behavior

- vCenter와 NSX 관계 동기화
- Network Mapping
- VM vNIC 연계
- NSX Realization
- Task/Event 통합 관리
- State Reconciliation

**완료 조건:** 복합 SDDC Workflow가 통과한다.

## Phase 4 — Fidelity 및 장애 재현

- Session Timeout
- RBAC
- Fault Injection
- API Error Contract
- Async Delay
- Event/PropertyCollector 개선
- API Compatibility Profile
- Negative Test

**완료 조건:** CMP의 주요 정상·오류·복구 시나리오가 통과한다.

## Phase 5 — 자동화 및 품질 강화

- Fixture/Scenario Runner
- Snapshot/Restore
- Regression Test
- Differential Test
- API Coverage Report
- Performance Benchmark
- CI 통합
- 문서화

**완료 조건:** 재현 가능한 자동화 테스트 플랫폼으로 활용할 수 있다.

---

# 16. Codex가 우선 분석해야 할 핵심 기술적 위험

다음 이슈는 구현 전에 반드시 검토한다.

**Risk 01 — vcsim의 실제 API 지원 범위**

vcsim이 지원하는 SOAP 및 REST 기능과 CMP의 호출 범위 간 Gap을 식별한다.

**Risk 02 — vcsim 내부 상태 변경과 외부 연동**

Go 기반 인벤토리 변화가 Python Simulation Engine으로 전달되는 방법을 검증한다.

**Risk 03 — PropertyCollector의 변경 이벤트**

CMP가 실시간 인벤토리 동기화에 사용하는 API를 정확하게 재현할 수 있는지 검토한다.

**Risk 04 — vCenter SOAP와 REST의 일관성**

동일 VM을 서로 다른 API로 조회했을 때 같은 상태를 제공할 수 있어야 한다.

**Risk 05 — NSX API의 방대한 범위**

자동 생성 가능한 모델과 직접 구현해야 하는 로직을 분리한다.

**Risk 06 — NSX Revision과 Realization**

객체 버전 변경, 낙관적 동시성 제어, 비동기 정책 상태의 구현 방법을 검증한다.

**Risk 07 — vCenter와 NSX 간 상태 동기화**

분리된 저장소의 변경 실패와 재시도를 어떻게 처리할지 결정한다.

**Risk 08 — 버전별 API 차이**

하나의 VMware 버전에서 검증한 동작을 다른 버전에 무조건 적용하지 않는다.

**Risk 09 — 성능 및 동시성**

SQLite Lock, vcsim 인벤토리 처리, 비동기 작업 증가에 따른 병목을 분석한다.

**Risk 10 — 호환성 검증 데이터 부족**

실제 VMware를 이용한 비교 테스트가 불가능할 때 검증 신뢰 수준을 어떻게 표기할지 정의한다.

---

# 17. 최종 완료 기준

프로젝트의 성공 여부는 다음 조건으로 판단한다.

1. 실제 VMware 제품을 설치하지 않고 실행 가능하다.
2. CMP가 Simulator를 관리 대상 vCenter와 NSX로 등록할 수 있다.
3. 승인된 범위의 모든 CMP VMware API 호출을 지원한다.
4. 모든 정상 생명주기 시나리오가 통과한다.
5. 모든 필수 오류·복구 시나리오가 통과한다.
6. 동일 리소스에 대한 API 간 상태 일관성을 유지한다.
7. 비동기 Task 및 NSX Realization 결과가 일치한다.
8. 인증과 Session 동작이 검증된다.
9. Resource ID와 참조 관계가 정상적으로 유지된다.
10. 테스트 환경 초기화 및 재현이 가능하다.
11. 자동화된 Regression Test를 실행할 수 있다.
12. API Coverage Report가 생성된다.
13. 지원하지 않는 API와 미검증 동작을 식별할 수 있다.
14. 호환성 검증 범위 및 제약사항이 문서화된다.

'100% 호환'이라는 표현은 승인된 CMP 기능 집합과 지정 버전에 대해 모든 검증 테스트를 통과한 경우에만 사용한다.

---

# 18. Codex에 대한 개발계획 수립 지시

지금부터 위 명세를 기반으로 프로젝트 개발계획을 수립하라.

**이번 작업에서는 전체 시스템을 즉시 구현하지 말고, 기존 오픈소스의 구조와 지원 범위를 검증한 뒤 상세 설계 및 단계별 개발계획을 작성한다.**

다음 순서로 진행한다.

## Task 1 — 오픈소스 분석

다음 저장소와 관련 문서를 분석한다.

- govmomi/vcsim
- govmomi simulator
- govmomi vapi/simulator
- Broadcom vSphere Web Services API
- Broadcom vSphere Automation API
- Broadcom NSX Manager/Policy API

분석 결과에 포함할 항목:
- 재사용할 수 있는 기능
- 미구현 기능
- 확장 가능한 구조
- Fork 필요 여부
- 버전별 호환성
- 개발 위험
- 기술 검증이 필요한 항목

공식 문서·저장소에서 확인한 사실과 추정한 내용을 분리한다.

## Task 2 — 상세 아키텍처 설계

다음 다이어그램을 작성한다.

1. 전체 시스템 Component Architecture
2. vCenter Simulator 내부 Component Diagram
3. NSX Simulator 내부 Component Diagram
4. Behavioral Engine Architecture
5. Virtual SDDC Data Model / ERD
6. vCenter-NSX Resource Relationship Diagram
7. VM Create Sequence Diagram
8. NSX Segment Create 및 Realization Sequence Diagram
9. Fault Injection Sequence Diagram
10. Deployment Diagram

가능하면 Mermaid를 사용해 Markdown에서 바로 확인할 수 있도록 한다.

각 모듈의 책임, 호출 방향, 상태 소유권, 내부 인터페이스를 명시한다.

## Task 3 — API Coverage 초안

CMP에서 사용하는 API 목록을 수집하거나 식별하는 절차를 설계한다.

현재 CMP 코드 또는 API Trace를 사용할 수 없다면 공식 문서와 대표적 CMP Workflow를 이용해 가정 기반 후보 목록을 작성한다.

실제 확인된 API와 가정된 API는 구분한다.

각 API에 대해 다음을 판단한다.

- vcsim 재사용 가능 여부
- 직접 구현 필요 여부
- 예상 구현 복잡도
- API 간 의존관계
- 검증 방법
- 개발 우선순위

## Task 4 — 기술 검증 계획

본격 개발 전 수행해야 할 Proof of Concept을 정의한다.

최소 검증 항목:

- vcsim 시작 및 SOAP 로그인
- vCenter REST Session 생성
- 동일 VM의 SOAP/REST 조회
- VM 생성 및 Task 완료
- PropertyCollector 변경 감지
- Go 기반 vcsim 확장 가능성
- Python ↔ Go 상태 동기화
- NSX Segment CRUD
- NSX Revision 충돌 처리
- NSX Realization 상태 전이
- Docker 기반 통합 실행

각 항목에는 검증 목적, 방법, 성공 조건 및 실패 시 대안을 기술한다.

## Task 5 — 단계별 개발계획

Phase 0부터 Phase 5까지 개발 작업을 분해한다.

각 작업에 다음 정보를 포함한다.

- Work Package ID
- 기능명
- 개발 목적
- 선행 작업
- 상세 작업 내용
- 구현 대상 모듈
- 산출물
- 테스트 방법
- 완료 기준
- 복잡도
- 위험
- 우선순위

대략적인 개발 규모를 산정하되 실제 저장소 분석 전에는 근거 없는 일정이나 정확한 공수를 확정하지 않는다.

## Task 6 — MVP 정의

최초 실행 가능한 MVP를 정의한다.

MVP는 최소한 다음을 지원해야 한다.

- CMP의 vCenter 연결
- CMP의 NSX 연결
- 가상 인벤토리 조회
- 기본 VM 생성·조회·변경·삭제
- 기본 NSX Segment 생성·조회·변경·삭제
- Task 및 상태 변경
- 기본 API 오류 처리
- Seed 및 Reset
- 테스트 자동화

기존 vcsim의 기능을 최대한 재사용하여 직접 구현할 코드를 줄이는 방향으로 설계한다.

## Task 7 — 계획 산출물 생성

최종적으로 다음 Markdown 문서를 작성한다.

1. `ARCHITECTURE.md` — 상세 시스템 설계서
2. `OPENSOURCE_ASSESSMENT.md` — 오픈소스 분석 및 활용 방안
3. `API_COVERAGE_MATRIX.md` — API Coverage 초안
4. `DATA_MODEL.md` — 상태 모델 및 DB 설계
5. `BEHAVIOR_SPEC.md` — 상태 전이·Task·이벤트·오류 명세
6. `POC_PLAN.md` — 기술 검증 계획
7. `DEVELOPMENT_PLAN.md` — 단계별 개발계획
8. `MVP_SCOPE.md` — 최초 구현 범위
9. `TEST_STRATEGY.md` — 검증 및 품질 계획
10. `RISK_REGISTER.md` — 기술 위험과 대응 방안

문서 간 참조와 용어를 일관되게 유지한다.

마지막에는 다음을 요약한다.

- 최종 권장 아키텍처
- 가장 위험한 기술적 문제 5개
- 직접 개발해야 하는 핵심 모듈
- 기존 오픈소스로 대체 가능한 부분
- 가장 먼저 수행할 PoC 5개
- MVP 개발 순서
- 개발 전 결정이 필요한 사항

**중요: 분석 단계에서는 근거 없는 코드 구현을 시작하지 말고, 실현 가능성과 아키텍처를 우선 검증한다. 불확실한 설계에는 대안을 비교하여 권고안을 제시한다.**
