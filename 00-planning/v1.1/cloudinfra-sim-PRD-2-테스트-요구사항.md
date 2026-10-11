# CloudInfra Simulator
## PRD-2. 테스트 요구사항

**부제:** CMP-Oriented VMware SDDC API Simulator  
**저장소:** `cloudinfra-sim`  
**문서 버전:** 1.1  
**상위 문서:** `cloudinfra-sim-PRD.md`  
**범위:** PRD 10~12장 (Scenario & Test Controller, 필수 E2E 시나리오, API Coverage 및 호환성 검증)

---

## 목차

- 요구사항 표 작성 기준
- 10. Scenario & Test Controller
  - 10.1 목적
  - 10.2 Fixture 기반 환경 생성
  - 10.3 시나리오 정의
- 11. 필수 End-to-End 테스트 시나리오
  - Scenario 01 — API 연결 및 환경 인식
  - Scenario 02 — 인벤토리 동기화
  - Scenario 03 — VM 생성
  - Scenario 04 — VM 변경
  - Scenario 05 — VM 삭제
  - Scenario 06 — NSX Segment 생성
  - Scenario 07 — NSX Security Policy 변경
  - Scenario 08 — vCenter + NSX 통합 Workflow
  - Scenario 09 — 장애와 복구
  - Scenario 10 — 세션 만료
- 12. API Coverage 및 호환성 검증
  - 12.1 API Coverage Matrix
  - 12.2 API 구현 상태 구분
  - 12.3 검증 방식
  - 12.4 자체 검증 근거와 향후 외부 비교
- 요구사항 집계

---

## 요구사항 표 작성 기준

장·절 번호는 상위 PRD의 번호 체계를 그대로 사용한다.

**요구사항 ID:** `접두어-SSNN` (SS = 절 번호 또는 시나리오 번호 2자리, NN = 일련번호 2자리). 예: `TR-E2E-0302` = Scenario 03의 2번째 항목.

| 접두어 | 대상 장 |
|---|---|
| TR-SCN | 10. Scenario & Test Controller |
| TR-E2E | 11. 필수 End-to-End 테스트 시나리오 |
| TR-COV | 12. API Coverage 및 호환성 검증 |

**표의 열:** 수준(필수 / 권장 / 선택 / 검토), MVP(● MVP 범위 / ○ §19 결정에 따름 / – MVP 이후 또는 적용 제외), 상태(정의 / 분석 / 설계 / 구현 / 검증 / 완료 / 보류 / 삭제)의 정의는 PRD-1의 '요구사항 표 작성 기준'과 같다.

11장 시나리오 표는 '관련 요구사항' 열에 각 성공 기준이 검증하는 기능 요구사항 ID(PRD-1)를 기록한다. 시나리오의 '절차' 행은 시험 절차이며, '성공 기준' 행이 합격 판정 대상이다. 이 E2E 연결만으로 전체 요구사항의 검증을 대신하지 않는다. 기능·테스트·비기능 요구사항 전체의 작업 패키지, 검증 방법, 완료 기준 및 증거는 TR-COV-0103의 추적표로 관리한다. 대안별 적용은 PRD-1의 '대안별 적용' 원칙을 따른다.

---

# 10. Scenario & Test Controller

## 10.1 목적

시뮬레이터의 환경 구성과 자체 테스트를 재현하기 위한 관리 기능이다. 실제 CMP는 실행하지 않으며 모든 시험은 프로젝트 내부 테스트 클라이언트·선정 SDK·Fixture로 수행한다(D13).

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-SCN-0101 | Environment Create | 테스트 환경을 생성한다. | 필수 | ● | 정의 |
| TR-SCN-0102 | Environment Reset | 테스트 환경을 초기 상태로 되돌린다. | 필수 | ● | 정의 |
| TR-SCN-0103 | Seed Apply | Seed(Fixture)를 적용한다. | 필수 | ● | 정의 |
| TR-SCN-0104 | State Snapshot | 현재 상태의 Snapshot을 만든다. | 필수 | – | 정의 |
| TR-SCN-0105 | Snapshot Restore | Snapshot으로 상태를 복원한다. | 필수 | – | 정의 |
| TR-SCN-0106 | Scenario Start | 시나리오를 시작한다. | 필수 | – | 정의 |
| TR-SCN-0107 | Scenario Stop | 시나리오를 중지한다. | 필수 | – | 정의 |
| TR-SCN-0108 | Virtual Clock Control | 가상 시간을 제어한다(PRD-1 §5.6). | 필수 | – | 정의 |
| TR-SCN-0109 | Fault Rule Add/Remove | Fault Rule을 추가·제거한다(PRD-1 §8.2). | 필수 | – | 정의 |
| TR-SCN-0110 | Event Inspection | 발생한 이벤트를 조회한다. | 필수 | – | 정의 |
| TR-SCN-0111 | API Call Trace | API 호출 기록을 조회한다. | 필수 | ● | 정의 |
| TR-SCN-0112 | Coverage Report | API Coverage Report를 생성한다(§12). | 필수 | – | 정의 |

## 10.2 Fixture 기반 환경 생성

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-SCN-0201 | 선언형 환경 정의 | YAML 또는 JSON으로 테스트 환경을 선언할 수 있다. | 필수 | ● | 정의 |
| TR-SCN-0202 | 예시 환경 지원 | 아래 '예시 환경' 규모의 vCenter·NSX 구성을 Fixture로 생성할 수 있다. | 필수 | ● | 정의 |
| TR-SCN-0203 | 규모 변경 | 리소스 수는 예시이며 환경별로 변경할 수 있다. | 필수 | ● | 정의 |
| TR-SCN-0204 | 다수 논리 리소스 | 가상 리소스는 실제 CPU·RAM을 소비하지 않으므로 다수의 논리 리소스를 생성할 수 있다. 단, 객체 수 증가에 따른 시뮬레이터 자체의 메모리·처리 부하는 발생한다(PRD-3 §13.1). | 필수 | ● | 정의 |

**참고 — 예시 환경**

| 영역 | 구성 |
|---|---|
| Site A (vCenter) | Datacenter: DC-A, Cluster: CL-A, Host 4개, ResourcePool 3개, Datastore 5개, Network 6개, VM 50개 |
| NSX | Compute Manager 1개, Transport Zone 2개, Tier-0 Gateway 1개, Tier-1 Gateway 3개, Segment 10개, Group 5개, Security Policy 5개 |

## 10.3 시나리오 정의

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-SCN-0301 | MVP 시나리오 단계 구성 | ① 초기 환경 생성 → ② 지정 사용자 로그인 → ③ 자체 테스트의 API Workflow 실행 → ④ 중간 상태 조회 → ⑤ 정상 완료 또는 잘못된 입력 등 기본 API 오류 확인 → ⑥ 상태 검증 → ⑦ 마스킹된 API Trace 메타데이터 저장 → ⑧ Test Report 생성 → ⑨ 환경 초기화. MVP에서는 자동화 테스트 코드로 실행하며 Fault Injection과 관리 API의 Scenario Start/Stop은 요구하지 않는다. | 필수 | ● | 정의 |
| TR-SCN-0302 | 선언형 시나리오 관리 | 시나리오는 가능하면 선언형 파일로 정의해 Git에서 버전 관리한다. | 권장 | ● | 정의 |
| TR-SCN-0303 | 장애 시나리오 단계 확장 | MVP 이후 기본 절차에 Fault Injection, 가상 시간 제어, Task 실패·Timeout·재시도·보상·복구 검증을 추가한다. Phase 4a에서는 테스트 코드로 실행하고, Phase 5에서 Scenario Start/Stop을 제공하는 Runner에 통합한다. | 필수 | – | 정의 |

---

# 11. 필수 End-to-End 테스트 시나리오

여기서 E2E는 자체 테스트 클라이언트 → Gateway → vCenter/NSX 엔진 → 저장소 및 상태 조회의 전체 경로를 뜻한다. 실제 CMP 연결·등록·상품 Workflow 검증은 범위 밖이다(D13). 요청·기대 응답·오류·상태 전이의 근거는 공식 스펙·문서·공개 SDK에서 정하며, 시뮬레이터의 현재 응답을 그대로 정답으로 삼지 않는다.

## Scenario 01 — API 연결 및 환경 인식

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0101 | 절차 | ① 테스트 클라이언트가 가상 vCenter에 로그인 → ② 제품 버전·Datacenter·Cluster·Host 인벤토리 확인 → ③ SOAP/REST 세션으로 후속 조회 → ④ 가상 NSX에 선정한 인증 방식으로 로그인 → ⑤ NSX 버전·Node/Cluster·Compute Manager·Transport Zone 조회 → ⑥ 세션 종료 및 인증 상태 확인. 실제 CMP 등록은 수행하지 않는다. | — | ● | 정의 |
| TR-E2E-0102 | 성공 기준 | API 오류 없이 가상 vCenter·NSX의 인증과 인증 후 조회가 각각 성공한다. | FR-API-0201, FR-API-0202, FR-API-0206 ~ FR-API-0208, FR-VC-0201, FR-API-0306 | ● | 정의 |
| TR-E2E-0103 | 성공 기준 | 양 제품의 환경 인식에 필요한 리소스 조회가 모두 성공한다. | FR-VC-0301 ~ FR-VC-0306, FR-NSX-0202 ~ FR-NSX-0205, FR-NSX-0601 | ● | 정의 |
| TR-E2E-0104 | 성공 기준 | 테스트 클라이언트가 양 제품의 타입·버전을 활성 프로파일과 일치하게 인식한다. | FR-VC-0307, FR-VC-0606, FR-API-0306 | ● | 정의 |

## Scenario 02 — 인벤토리 동기화

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0201 | 절차 | ① 테스트 클라이언트가 VM 목록 조회 → ② 시뮬레이터에서 VM의 CPU 값 변경 → ③ 테스트 클라이언트가 재조회 또는 변경 감시 수행 → ④ 변경된 CPU 값 반영 | — | ● | 정의 |
| TR-E2E-0202 | 성공 기준 | 동일 VM의 ID가 유지된다. | FR-BE-0201, FR-DM-0206 | ● | 정의 |
| TR-E2E-0203 | 성공 기준 | 변경된 속성이 반영된다. | FR-BE-0202 | ● | 정의 |
| TR-E2E-0204 | 성공 기준 | 불필요한 중복 VM이 생성되지 않는다. | FR-DM-0205 | ● | 정의 |
| TR-E2E-0205 | 성공 기준 | 지원하는 이벤트 감시 방식과 일치한다. | FR-VC-0509 | ● | 정의 |

## Scenario 03 — VM 생성

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0301 | 절차 | ① 테스트 클라이언트가 VM Create 호출 → ② 시뮬레이터가 Task 생성 → ③ Task 상태 진행 → ④ 가상 VM 객체 생성 → ⑤ Task 성공 → ⑥ 테스트 클라이언트가 VM 조회 | — | ● | 정의 |
| TR-E2E-0302 | 성공 기준 | VM 생성 Workflow가 완료된다. | FR-VC-0401, FR-VC-0501 ~ FR-VC-0506 | ● | 정의 |
| TR-E2E-0303 | 성공 기준 | 고유 ID가 생성된다. | FR-DM-0203, FR-DM-0205 | ● | 정의 |
| TR-E2E-0304 | 성공 기준 | Host/Cluster/Datastore 관계가 정상이다. | FR-VC-0305, FR-BE-0203 | ● | 정의 |
| TR-E2E-0305 | 성공 기준 | Task와 인벤토리 상태가 일치한다. | FR-VC-0505, FR-VC-0510 | ● | 정의 |

## Scenario 04 — VM 변경

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0401 | 절차 | ① 테스트 클라이언트가 CPU 및 Memory 변경 요청 → ② 변경 전제조건 확인 → ③ 비동기 작업 수행 → ④ VM Config와 Summary 변경 → ⑤ 테스트 클라이언트가 변경 결과 확인 | — | ● | 정의 |
| TR-E2E-0402 | 성공 기준 | 실제 API 규칙에 맞는 결과를 낸다. | FR-VC-0404, FR-VC-0417, FR-VC-0418 | ● | 정의 |
| TR-E2E-0403 | 성공 기준 | 변경 전후 조회가 일관된다. | FR-VC-0510, FR-BE-0202 | ● | 정의 |

## Scenario 05 — VM 삭제

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0501 | 절차 | ① 테스트 클라이언트가 VM 삭제 요청 → ② 전원 상태 등 VM 사전조건 검사(권한 검사는 RBAC를 구현하는 Phase 4a부터 회귀 절차에 추가) → ③ 삭제 Task 처리 → ④ 인벤토리에서 VM 제거 → ⑤ 관련 리소스 참조 정리 | — | ● | 정의 |
| TR-E2E-0502 | 성공 기준 | 삭제 Workflow가 완료된다. | FR-VC-0409 | ● | 정의 |
| TR-E2E-0503 | 성공 기준 | 삭제한 VM의 재조회 동작이 실제 API 계약과 일치한다. | FR-VC-0417 | ● | 정의 |
| TR-E2E-0504 | 성공 기준 | 관련 관계에서 고아 참조가 발생하지 않는다. | FR-BE-0206 | ● | 정의 |

## Scenario 06 — NSX Segment 생성

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0601 | 절차 | ① 테스트 클라이언트가 선택한 경로·메서드로 NSX Segment 생성 → ② 버전별 성공 응답 확인(PATCH는 본문 없는 성공 응답, PUT은 해당 계약 적용) → ③ GET으로 객체·Path·Revision 확인 → ④ Realization 상태 조회 → ⑤ 완료 후 Segment 재조회. 즉시 완료와 설정된 지연 후 완료를 각각 실행한다. | — | ● | 정의 |
| TR-E2E-0602 | 성공 기준 | PATCH/PUT의 설정·헤더별 갱신 의미 및 성공 응답의 상태 코드·본문 유무가 계약과 일치한다. | FR-NSX-0403, FR-NSX-0405, FR-NSX-0416 | ● | 정의 |
| TR-E2E-0603 | 성공 기준 | GET으로 저장된 객체·Path·Revision을 확인할 수 있다. | FR-NSX-0401, FR-NSX-0408, FR-NSX-0411 | ● | 정의 |
| TR-E2E-0604 | 성공 기준 | 즉시 완료와 지연 후 완료 모두 테스트 클라이언트의 Realization 완료 판단이 정상이며, 지연 중 상태와 완료 상태가 버전별 계약을 따른다. | FR-NSX-0501 ~ FR-NSX-0503, FR-NSX-0509, FR-BE-0606 | ● | 정의 |

## Scenario 07 — NSX Security Policy 변경

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0701 | 절차 | ① 테스트 클라이언트가 Group 생성 → ② Service 생성 → ③ Security Policy와 Rule 생성 → ④ 기존 Rule 수정 → ⑤ 최신 Revision 조회 → ⑥ 잘못된 Revision으로 변경 시도 | — | – | 정의 |
| TR-E2E-0702 | 성공 기준 | 정상 변경에 성공한다. | FR-NSX-0215 ~ FR-NSX-0218 | – | 정의 |
| TR-E2E-0703 | 성공 기준 | Revision이 증가한다. | FR-NSX-0408 | – | 정의 |
| TR-E2E-0704 | 성공 기준 | 충돌 응답이 해당 API 계약과 일치한다. | FR-NSX-0409 | – | 정의 |

## Scenario 08 — vCenter + NSX 통합 Workflow

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0801 | 절차 | ① NSX Segment 생성 → ② 요구되는 Network 관계 구성 → ③ VM 생성 → ④ VM vNIC을 Network에 연결 → ⑤ 테스트 클라이언트가 양 시스템에서 관련 리소스 조회 → ⑥ VM 삭제 → ⑦ NSX Segment 삭제 | — | – | 정의 |
| TR-E2E-0802 | 성공 기준 | API 간 Resource ID 관계가 일치한다. | FR-NSX-0603, FR-NSX-0606 | – | 정의 |
| TR-E2E-0803 | 성공 기준 | 단일 프로세스 일관성 절차에 따라 상태 불일치가 없다(D3를 B로 재결정하면 PRD-B §B6의 절차 적용). | FR-DM-0401, FR-BE-0304 | – | 정의 |
| TR-E2E-0804 | 성공 기준 | 삭제 의존관계가 정상 처리된다. | FR-NSX-0415, FR-BE-0206 | – | 정의 |

## Scenario 09 — 장애와 복구

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-0901 | 절차 | ① VM 생성 요청 → ② Task 실행 중 실패 주입 → ③ 테스트 클라이언트 오류 감지 → ④ 테스트 클라이언트의 재시도 또는 보상 처리 → ⑤ 최종 상태 조회 | — | – | 정의 |
| TR-E2E-0902 | 성공 기준 | 지정한 오류를 정확하게 반환한다. | FR-FLT-0122, FR-FLT-0205 | – | 정의 |
| TR-E2E-0903 | 성공 기준 | 테스트 클라이언트 복구 Workflow가 정상 동작한다. | FR-FLT-0201 | – | 정의 |
| TR-E2E-0904 | 성공 기준 | 불필요한 중복 리소스가 없다. | FR-DM-0205, FR-BE-0204 | – | 정의 |

## Scenario 10 — 세션 만료

| ID | 구분 | 내용 | 관련 요구사항 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-E2E-1001 | 절차 | ① 테스트 클라이언트 로그인 → ② Session 생성 → ③ 만료 이벤트 주입 → ④ 테스트 클라이언트 재요청 → ⑤ 인증 실패 확인 → ⑥ 테스트 클라이언트 재로그인 또는 복구 | — | – | 정의 |
| TR-E2E-1002 | 성공 기준 | Session 처리 계약을 준수한다. | FR-API-0204, FR-API-0209, FR-FLT-0102 | – | 정의 |
| TR-E2E-1003 | 성공 기준 | 테스트 클라이언트의 재인증 로직이 정상 동작한다. | FR-API-0201, FR-API-0207 | – | 정의 |

---

# 12. API Coverage 및 호환성 검증

## 12.1 API Coverage Matrix

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-COV-0101 | Coverage Matrix 관리 | 개발 초기부터 API Coverage Matrix를 관리한다. | 필수 | ● | 정의 |
| TR-COV-0102 | 필수 열 | API 식별자, Product(vCenter/NSX), Product Version 및 Build, API Family(SOAP/REST/Policy/Manager), Method, Path 또는 SOAP Operation, Simulation Function, 참고 CMP 기능 ID(선택), 실행 범위 포함 여부·결정 근거, Request Schema, Response Schema, Authentication, Side Effect, Async Behavior, Fault Behavior, 계약 근거·출처, Simulator Implementation, Verification Status, Priority, Related Requirement ID, Related Test Case, 자체 검증 증거 ID·일자·프로파일/구현 식별자 | 필수 | ● | 정의 |
| TR-COV-0103 | 요구사항 검증 추적표 | PRD-1~3의 모든 요구사항을 ID별로 작업 패키지, 적용 여부·결정 근거, 목표 단계, 검증 방법(검토/PoC/Unit/Contract/Stateful/Negative/E2E/Footprint 등), 테스트·검토 항목 ID, 완료 기준(AC), 증거·결과에 연결한다. Phase 0에서 `TEST_STRATEGY.md`에 작성한다. MVP 범위(● 및 채택된 ○)와 Phase 1 대상 ID는 Phase 0에서 개별 행으로, 그 밖의 ID는 요구사항 범위 단위로 작업·목표 단계를 배정하고 해당 단계 진입 시 개별 행으로 전개한다. 단계 종료마다 갱신한다. 미구현은 계획 상태로 기록하며, 적용 제외는 결정 ID와 사유를 남긴다. 해당 단계까지 필요한 ID의 누락, 검증 방법·작업 배정 누락 또는 근거 없는 제외가 있으면 그 단계의 종료 게이트를 통과하지 못한다(PRD-4 §15.5, AC-19). | 필수 | ● | 정의 |

## 12.2 API 구현 상태 구분

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-COV-0201 | 구현 상태 분류 | 현재 범위의 API는 Not Analyzed, Unsupported, Contract Defined, Implemented, Unit Tested, Integration Tested로 분류한다. 기존 CMP Verified와 Reference VMware Verified 명칭은 향후 외부 검증용으로만 유지하며 현재 프로젝트에서 부여하지 않는다(D13). | 필수 | ● | 정의 |
| TR-COV-0202 | 구현·검증 상태 구분 | `Implemented`와 `Integration Tested`를 구분하며, 자체 검증 상태를 `CMP Verified`·`Reference VMware Verified`로 승격하지 않는다. | 필수 | ● | 정의 |

기동 시 검증 수준은 PRD-1 FR-API-0302를 따른다. 현재 검증 상한은 `Integration Tested`이며 선정 SDK의 자체 계약·상태·오류·E2E 시험 증거를 API·시나리오별로 연결한다. `CMP Verified`는 실제 CMP 시험, `Reference VMware Verified`는 실제 제품 비교까지 별도로 승인·수행했을 때만 사용할 미래 상태이다. 자체 테스트나 자체 API Trace만으로 외부 검증 상태를 부여하지 않는다. 계약·프로파일·구현 변경 시 영향을 받는 증거를 무효화하고 재검증한다.

## 12.3 검증 방식

다음 방법을 조합한다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-COV-0301 | Contract Test | 요청/응답 스키마, HTTP 상태 코드, SOAP Envelope/Fault, 필드 타입, 필수/선택 속성, Version Compatibility | 필수 | ● | 정의 |
| TR-COV-0302 | Stateful Test | Create → Get, Update → Get, Delete → Get, 반복 Update, 비동기 완료, 여러 API 간 상태 일치 | 필수 | ● | 정의 |
| TR-COV-0303 | MVP Negative Test | 잘못된 Credential·입력·Resource ID, 계약상 허용되지 않는 중복 생성, Revision Conflict, 미지원 기능 및 SOAP Fault를 검증한다. 권한 모델·Timeout·Task 실패 주입은 TR-COV-0308로 분리한다. upsert가 허용되는 API의 반복 생성은 오류로 간주하지 않는다. | 필수 | ● | 정의 |
| TR-COV-0304 | 선정 SDK 자체 계약 테스트 | D2에서 선정한 공개 SDK와 버전으로 자체 계약 테스트를 구성한다. 특정 CMP의 SDK 확인·소스 분석·실행은 요구하지 않는다. XML 타입·네임스페이스·요소 순서·버전 협상을 공식 계약에 대조하고 테스트한 SDK·버전만 결과에 명시한다. | 필수 | ● | 정의 |
| TR-COV-0305 | 범용 SDK 통합 테스트 | govc, pyVmomi, govmomi로 통합 테스트를 수행한다. 이 SDK들은 vcsim과 같은 계열이거나 관대한 편이므로, 통과가 다른 SDK의 통과를 보장하지 않는다. | 필수 | ● | 정의 |
| TR-COV-0306 | 선택 자동화 도구 자체 테스트 | 프로젝트의 테스트 도구로 채택한 경우 Terraform Provider 또는 Ansible로 시뮬레이터 대상 자체 테스트를 수행한다. 실제 CMP 사용 여부를 조사하는 작업은 포함하지 않는다. | 선택 | – | 정의 |
| TR-COV-0307 | MVP 시뮬레이터 End-to-End Test | Scenario 01~06: API 연결·환경 인식, 자원 동기화, VM 생성·변경·삭제, NSX Segment 생성과 기본 Realization. 테스트 클라이언트로 수행한다. D2·D7·D9에서 MVP로 채택한 기능의 선행 FR·WP·E2E 성공 기준을 Phase 2a·2b 범위에 함께 반영한다. | 필수 | ● | 정의 |
| TR-COV-0308 | 확장 Negative Test | Phase 4a에서 권한 부족, 세션 만료, Timeout, Task Failure, 전송·인프라·Realization 장애 주입 및 재시도·보상을 검증한다. MVP Negative Test는 계속 회귀 실행한다. | 필수 | – | 정의 |
| TR-COV-0309 | MVP 이후 시뮬레이터 End-to-End Test | 자체 테스트 클라이언트로 Phase 3에서 Scenario 08, Phase 4a에서 Scenario 09·10, Phase 4b에서 Scenario 07을 검증한다. Phase 5에서 승인된 전체 자체 시나리오를 회귀 실행한다. | 필수 | – | 정의 |

## 12.4 자체 검증 근거와 향후 외부 비교

D13에 따라 자체 테스트의 계약 근거·검증 상한(TR-COV-0403, TR-COV-0409)은 현재 범위이다. 실환경 수집·Record·실환경 Differential Test(TR-COV-0401, TR-COV-0402, TR-COV-0404 ~ TR-COV-0408)는 '적용 제외 — D5·D13'으로 보류한다. 자체 시뮬레이터의 호출 기록(TR-SCN-0111)은 계속 구현하며 실환경 Trace와 구분한다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| TR-COV-0401 | Trace 비교 항목 | 실제 VMware에서 수집한 Trace가 있으면 Method 및 Path, 요청 Body, Response Status, Response Header, Response Body, Task Polling Sequence, State Transition, Error Response를 비교한다. | 필수 | – | 보류 |
| TR-COV-0402 | 동적 값 정규화 | 동적 ID, Timestamp, Session Token 등은 의미를 보존하는 정규화 규칙으로 처리한다. | 필수 | – | 보류 |
| TR-COV-0403 | 자체 테스트의 계약 근거 | 공식 WSDL/OpenAPI·제품 문서·공개 SDK의 대상 버전 근거로 기대 응답·오류·상태 전이를 정의한다. 자체 호출 기록은 실행 증거이며 외부 계약의 근거를 대체하지 않는다. CMP 소스 분석·실환경 Trace를 선행 조건으로 두지 않는다(D13). | 필수 | ● | 정의 |
| TR-COV-0404 | Trace 비식별화 | Trace는 인증 정보, 개인정보, 운영 인프라 정보가 포함될 수 있으므로 저장 전 비식별화한다. | 필수 | – | 보류 |
| TR-COV-0405 | Record 모드 | 내장 Gateway가 실제 vCenter/NSX 앞단의 투명 프록시로 동작하며 CMP ↔ 실제 VMware 간 요청·응답을 기록한다. | 필수 | – | 보류 |
| TR-COV-0406 | 기록 시 비식별 치환 | 기록 시 자격 증명, 세션 토큰, 호스트명, IP, 사용자명을 비식별화 규칙으로 치환한다. | 필수 | – | 보류 |
| TR-COV-0407 | Trace 활용 | 향후 외부 검증이 별도 승인되면 비식별화된 실환경 Trace를 프로파일 근거 보완과 Differential Test 기준으로 사용한다. 현재의 범위 선정·자체 테스트에는 요구하지 않는다. | 필수 | – | 보류 |
| TR-COV-0408 | 향후 실환경 캡처 계획 | 실환경 비교가 별도로 승인되는 경우 대상 버전·빌드별 캡처 계획을 수립한다. 현재 Phase 0 산출물·종료 게이트에서는 제외한다(D5·D13). | 필수 | – | 보류 |
| TR-COV-0409 | 자체 검증 상한 명시 | 현재 검증 상한은 Integration Tested이며 '승인 범위 자체 테스트 통과, CMP·실환경 미검증'으로 표시한다. 적용 대상 AC-01 ~ AC-19는 자체 시험 증거로 판정한다. 외부 증거 부재로 현재 범위를 차단하지 않으며, 자체 시험만으로 CMP Verified·Reference VMware Verified 또는 '100% 호환'을 표시하지 않는다. AC-20은 향후 외부 검증 조건으로 보류한다. | 필수 | ● | 정의 |

---

## 요구사항 집계

각 장의 요구사항 수와 수준·MVP 분포이다. 보류·적용 제외 항목도 ID 보존을 위해 전체에 포함하며, 현재 구현·완료 의무 수와 다르다. MVP –에는 후속 및 적용 제외 항목이 포함된다. 요구사항·MVP·적용 여부를 바꾸면 이 표도 갱신한다.

| 장 | 접두어 | 전체 | 필수 | 권장 | 선택 | 검토 | MVP ● | MVP ○ | MVP – |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| 10. Scenario & Test Controller | `TR-SCN` | 19 | 18 | 1 | 0 | 0 | 10 | 0 | 9 |
| 11. 필수 E2E 테스트 시나리오 | `TR-E2E` | 40 | — | — | — | — | 25 | 0 | 15 |
| 12. API Coverage 및 호환성 검증 | `TR-COV` | 23 | 22 | 0 | 1 | 0 | 13 | 0 | 10 |
| **합계** |  | **82** | **40** | **1** | **1** | **0** | **48** | **0** | **34** |

`TR-E2E`는 수준 열 대신 구분 열을 사용한다 (절차 10건, 성공 기준 30건). 위 표의 수준 합계에는 포함되지 않는다.
