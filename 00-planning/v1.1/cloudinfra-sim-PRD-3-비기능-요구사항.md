# CloudInfra Simulator
## PRD-3. 비기능 요구사항

**부제:** CMP-Oriented VMware SDDC API Simulator  
**저장소:** `cloudinfra-sim`  
**문서 버전:** 1.1  
**상위 문서:** `cloudinfra-sim-PRD.md`  
**범위:** PRD 13~14장 (성능 및 비기능 요구사항, 권장 기술 아키텍처 및 디렉터리 구조)

---

## 목차

- 요구사항 표 작성 기준
- 13. 성능 및 비기능 요구사항
  - 13.1 경량 실행
  - 13.2 데이터 안정성
  - 13.3 보안
  - 13.4 Observability
  - 13.5 데이터 보존 한도
- 14. 권장 기술 아키텍처 및 디렉터리 구조
  - 14.1 기술 스택
  - 14.2 Repository 구조
  - 14.3 프로세스 및 네트워크 구성
- 요구사항 집계

---

## 요구사항 표 작성 기준

장·절 번호는 상위 PRD의 번호 체계를 그대로 사용한다.

**요구사항 ID:** `접두어-SSNN` (SS = 절 번호 2자리, NN = 절 안의 일련번호 2자리). 예: `NFR-0109` = 13.1절의 9번째 요구사항.

| 접두어 | 대상 장 |
|---|---|
| NFR | 13. 성능 및 비기능 요구사항 |
| NFR-ARC | 14. 권장 기술 아키텍처 및 디렉터리 구조 |

**표의 열:** 수준(필수 / 권장 / 선택 / 검토), MVP(● MVP 범위 / ○ §19 결정에 따름 / – MVP 이후 또는 적용 제외), 상태(정의 / 분석 / 설계 / 구현 / 검증 / 완료 / 보류 / 삭제)의 정의는 PRD-1의 '요구사항 표 작성 기준'과 같다.

**적용 범위(D13):** 비기능 시험도 시뮬레이터와 자체 테스트 도구를 대상으로 한다. 실제 CMP 및 상품 제약 시험은 수행하지 않는다. Record 전용 NFR-0312는 ID를 유지하고 '적용 제외 — D5·D13'으로 기록한다.

계획 구성 기준의 호스트 PC 최소·권장 사양과 시나리오별 예상 사용량은 저장소 루트 `README.md`의 '예상 리소스 요구사항'을 참고한다.

---

# 13. 성능 및 비기능 요구사항

## 13.1 경량 실행

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-0101 | VMware 제품 설치 불필요 | 물리 VMware 제품을 설치하지 않고 실행된다. | 필수 | ● | 정의 |
| NFR-0102 | 실제 VM 실행 불필요 | 실제 VM을 실행하지 않는다. | 필수 | ● | 정의 |
| NFR-0103 | 단일 PC 운영 | 단일 PC에서 운영할 수 있다. | 필수 | ● | 정의 |
| NFR-0104 | 네이티브 실행 기본 | Windows, macOS, Linux에서 Docker 없는 네이티브 실행을 기본으로 한다. Go 단일 실행파일로 배포한다(D3: 권고안 A). D3를 B로 재결정하면 PRD-B §B5의 조건을 따른다. Docker 이미지·Docker Compose는 CI와 선택 실행용으로 제공하며 필수 의존성으로 삼지 않는다. | 필수 | ● | 정의 |
| NFR-0105 | 외부 의존성 최소화 | 불필요한 외부 서비스 의존성을 최소화하며, 런타임 사전 설치 없이 실행한다. 권고안 B의 예외 조건은 PRD-B §B5를 따른다. | 필수 | ● | 정의 |
| NFR-0106 | 기본 저장소 | 기본 저장소는 SQLite이며, `lite` 프로필은 인메모리로 사용한다. | 필수 | ● | 정의 |
| NFR-0107 | 리소스 예산 = 완료 기준 | 리소스 목표는 구현 후 측정해 정하는 값이 아니라 사전에 정하는 완료 기준이다. 아래 값은 Phase 0a 실측을 근거로 §19 D4(2026-10-10)에서 확정했으며, CI에서 매 빌드 측정해 회귀를 감시한다. 측정 방법과 경계는 `20-design/development-plan/RESOURCE_BUDGET.md`를 따른다. | 필수 | ● | 정의 |
| NFR-0108 | 기준 Fixture | 예산 측정 기준 Fixture `default-small`: Datacenter 1, Cluster 1, Host 3, Datastore 2, Network 4, VM 20, NSX Segment 10, Tier-1 2, Group 5, Security Policy 3 | 필수 | ● | 정의 |
| NFR-0109 | 유휴 메모리 | 유휴 메모리(RSS, 전체 프로세스 합) ≤ 300 MB (D4 확정) | 필수 | ● | 정의 |
| NFR-0110 | 기동 시간 | 프로세스 시작부터 첫 API 응답 가능까지 ≤ 5초 (D4 확정) | 필수 | ● | 정의 |
| NFR-0111 | 유휴 CPU | 유휴 CPU는 측정 오차 수준이다(주기적 폴링 없음). | 필수 | ● | 정의 |
| NFR-0112 | 설치 크기 | 실행파일 + 기본 프로파일 1개 조합(vCenter·NSX 데이터) ≤ 200 MB (D4 확정). D4에서 확정한 측정 경계(`RESOURCE_BUDGET.md`)로 검증한다(권고안 B의 합산 기준은 PRD-B §B5). | 필수 | ● | 정의 |
| NFR-0113 | 사전 설치 조건 | Docker·Python 런타임 사전 설치가 필요하지 않다. 설치 문서와 CI에서 이 조건을 검증한다(권고안 B의 예외는 PRD-B §B5). | 필수 | ● | 정의 |
| NFR-0114 | `lite` 실행 프로필 (기본) | 개발자 PC 일상 사용용. 소규모 Fixture, 인메모리 저장소, API Trace 본문 기록 off, 이벤트·로그 링버퍼 | 필수 | ● | 정의 |
| NFR-0115 | `full` 실행 프로필 | 성능·대규모·CI 테스트용. 대규모 Fixture, 파일 저장소, Trace 본문 기록 on, 보존 한도 확대 | 필수 | – | 정의 |
| NFR-0116 | 규모 확장 테스트 | `full` 프로필에서 리소스 수를 늘리는 테스트를 지원한다. 예: VM 100 / 1,000 / 5,000개, NSX Segment 1,000개, 동시 테스트 클라이언트 요청 10 / 50 / 100개 | 필수 | – | 정의 |
| NFR-0117 | 규모 목표 결정 방식 | NFR-0116의 값은 검증 시나리오이며 사전 보장 처리량이 아니다. 대규모 시나리오의 처리량·메모리 목표는 벤치마크 결과로 정하되, NFR-0109 ~ NFR-0112의 기본 예산은 벤치마크와 무관하게 유지한다. | 필수 | – | 정의 |

## 13.2 데이터 안정성

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-0201 | 동시 요청 무결성 | 동시 요청에 대해 데이터 무결성을 유지한다. | 필수 | ● | 정의 |
| NFR-0202 | 중복 생성 방지 | 중복 생성을 방지한다. | 필수 | ● | 정의 |
| NFR-0203 | 안정적 ID 관리 | ID를 안정적으로 관리한다(PRD-1 §6.2). | 필수 | ● | 정의 |
| NFR-0204 | SQLite Transaction | SQLite Transaction으로 저장 일관성을 보장한다. | 필수 | ● | 정의 |
| NFR-0205 | 재기동 후 상태 복원 | 시뮬레이터 재기동 후 상태를 복원할 수 있다. | 필수 | – | 정의 |
| NFR-0206 | Snapshot/Restore | Snapshot과 Restore를 지원한다. | 필수 | – | 정의 |
| NFR-0207 | 상태 불일치 검출 | 상태 불일치를 검출한다. | 필수 | – | 정의 |
| NFR-0208 | 작업 재처리 정책 | 작업 재처리 정책을 정의한다. | 필수 | – | 정의 |

## 13.3 보안

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-0301 | 루프백 기본 바인딩 | 기본 바인딩은 Localhost(루프백 주소)이다. 서비스별 주소는 PRD 개요 §2.2와 NFR-ARC-0307을 따른다. | 필수 | ● | 정의 |
| NFR-0302 | HTTPS 지원 | HTTPS를 지원한다. | 필수 | ● | 정의 |
| NFR-0303 | 인증서 교체 | 인증서를 교체할 수 있다. | 필수 | ● | 정의 |
| NFR-0304 | Credential 관리 | Credential은 환경변수 또는 Secret을 사용한다. | 필수 | ● | 정의 |
| NFR-0305 | 관리자 API 인증 | Admin API에 인증을 적용한다. | 필수 | ● | 정의 |
| NFR-0306 | 감사 로그 | 감사 로그를 남긴다. | 필수 | – | 정의 |
| NFR-0307 | 민감 정보 마스킹 | 로그·Trace의 민감 정보를 마스킹한다. | 필수 | ● | 정의 |
| NFR-0308 | 외부 네트워크 접근 제한 | 원치 않는 외부 네트워크 접근을 제한한다. | 필수 | ● | 정의 |
| NFR-0309 | 운영 환경 혼동 방지 | 실제 운영 VMware와 접속 대상이 혼동되지 않도록 한다. | 필수 | ● | 정의 |
| NFR-0310 | 운영 자격 증명 재사용 금지 | 실제 운영 환경의 자격 증명을 시뮬레이터에 재사용하지 않는 것을 원칙으로 한다. | 필수 | ● | 정의 |
| NFR-0311 | 외부 바인딩 통제 | 테스트 클라이언트가 다른 호스트·컨테이너에 있어 외부 바인딩이 필요한 경우(NFR-ARC-0309의 배포 토폴로지 2·3), 명시적 설정으로만 허용하고 허용 대상 주소를 제한한다. | 필수 | ○ | 정의 |
| NFR-0312 | Record 모드 보안 | 향후 Record 모드를 별도 승인하는 경우 실제 VMware 자격 증명이 Gateway를 통과하므로 기록 전 비식별화와 기록 파일 접근 제한을 적용한다. 현재 범위에서는 구현·검증하지 않는다(D5·D13). | 필수 | – | 보류 |
| NFR-0313 | 가상 호스트 도메인 설정 | 가상 호스트 이름은 `<서비스>.<도메인>` 형식(`vcenter`, `nsx`, `esx-NN`)이며, 기본 도메인은 `cloudinfra-sim.test`이다. 도메인은 `--domain` 옵션 또는 `SIM_DOMAIN` 환경변수로 변경할 수 있고, 인증서 SAN(`*.<도메인>`)과 설치 스크립트의 hosts 등록이 이 값을 따른다. 코드에 호스트 이름을 고정하지 않는다. | 필수 | ● | 정의 |
| NFR-0314 | 예약 TLD 사용 | 기본값은 테스트 전용 예약 TLD `.test`(RFC 2606, RFC 6761)를 사용해 실제 운영 VMware와 혼동되지 않게 한다. 팀 공용 서버의 내부 DNS 구성에서는 `.internal`을 허용한다. mDNS 전용인 `.local`, 실제 공용 TLD(`.dev`, `.app` 등), 예약되지 않은 임의 TLD(`.lab`, `.corp` 등)는 사용하지 않는다. | 필수 | ● | 정의 |
| NFR-0315 | 인증서 지문 안정성 | 시뮬레이터 인증서와 그 지문(thumbprint)은 Environment Reset, 재기동, 같은 데이터 디렉터리를 사용하는 재설치 후에도 유지한다. 테스트 클라이언트와 NSX Compute Manager 등록(PRD-1 FR-NSX-0601)이 저장한 지문이 깨지지 않도록 하며, 인증서 교체(NFR-0303)는 명시적 명령으로만 수행하고 교체 시 새 지문을 출력한다. | 필수 | ● | 정의 |

## 13.4 Observability

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-0401 | API 로그 | API Request Log, API Response Status, Request Correlation ID | 필수 | ● | 정의 |
| NFR-0402 | 작업 추적 정보 | Task ID, Resource ID, Operation Duration | 필수 | ● | 정의 |
| NFR-0403 | 상태·오류 로그 | State Transition Log, Fault Injection Log, Unsupported API Log, Error Log | 필수 | ● | 정의 |
| NFR-0404 | 테스트 실행 로그 | Test Execution Log | 필수 | – | 정의 |
| NFR-0405 | 로그 형식 | 기본 로그는 사람이 읽을 수 있는 형태와 JSON 형태를 지원한다. | 필수 | ● | 정의 |

## 13.5 데이터 보존 한도

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-0501 | 누적 데이터 상한 | 장시간 실행 시 메모리·디스크가 계속 늘지 않도록 모든 누적 데이터에 상한을 둔다. | 필수 | ● | 정의 |
| NFR-0502 | Event / Alarm 보존 | `lite`: 최근 N건 링버퍼, `full`: 설정값. vCenter EventManager의 조회 계약 범위 내에서 적용한다. | 필수 | ● | 정의 |
| NFR-0503 | Audit / Unsupported API Log 보존 | `lite`: 집계 + 최근 N건. `full`: 설정한 기간·건수·총용량 한도 내 상세 로그. 한도 중 먼저 도달한 조건으로 오래된 기록을 회전·삭제한다. 미지원 호출의 누적 횟수는 제한된 집계 키와 기타 버킷으로 유지하며, 요청별 임의 경로·리소스 ID를 무제한 집계 키로 저장하지 않는다. | 필수 | ● | 정의 |
| NFR-0504 | API Trace 보존 | `lite`: 메타데이터만, `full`: 본문 포함. 개별 본문 크기와 전체 보존 기간·건수·총용량에 상한을 적용한다. 한도 도달 시 오래된 기록을 회전·삭제하며, 잘린 본문과 폐기 건수를 표시한다. 검증 증거로 채택한 Trace는 별도 관리 한도 내에서 보존하고, 삭제 시 관련 증거 링크·검증 상태를 재평가한다. | 필수 | ● | 정의 |
| NFR-0505 | 완료된 Task 보존 | `lite`: 최근 N건, `full`: 설정값. vCenter `recentTask` 계약을 확인한다. | 필수 | ● | 정의 |
| NFR-0506 | Operation Transition 보존 | `lite`: 최근 N건, `full`: 설정값 | 필수 | ● | 정의 |
| NFR-0507 | 성능 메트릭 미저장 | 두 프로필 모두 저장하지 않고 즉석 계산한다(PRD-1 FR-MET-0104). | 필수 | – | 정의 |
| NFR-0508 | 보존 한도 설정 | N, 보존 기간, 총용량, 집계 키 수와 회전 정책은 Phase 0의 `RESOURCE_BUDGET.md`에서 정한다. `lite`·`full` 모두 유한한 기본값을 사용하고 설정으로 조정하되 무제한 보존은 허용하지 않는다. `full`의 실제 저장·회전 동작은 Phase 5에서 검증한다. | 필수 | ● | 정의 |

---

# 14. 권장 기술 아키텍처 및 디렉터리 구조

## 14.1 기술 스택

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-ARC-0101 | 프로세스 구성안 확정 | §19 D3에서 권고안 A(단일 Go 바이너리)로 확정했다. 권고안 B와의 비교, 재검토 조건과 전환 절차는 PRD-B(`cloudinfra-sim-PRD-B-권고안B-대안-구성.md`)를 따른다. | 필수 | ● | 정의 |
| NFR-ARC-0102 | vCenter SOAP | Go + govmomi/vcsim (라이브러리 임베드) | 필수 | ● | 정의 |
| NFR-ARC-0103 | vCenter REST | govmomi vAPI Simulator + Go 확장 (vcsim 인벤토리 뷰) | 필수 | ● | 정의 |
| NFR-ARC-0104 | vCenter 보조 서비스 | vcsim 내장 Lookup/STS/PBM/Content Library 시뮬레이터 재사용 | 권장 | ○ | 정의 |
| NFR-ARC-0105 | 버전 인식 직렬화 | Go (프로파일 메타데이터 기반) | 필수 | ● | 정의 |
| NFR-ARC-0106 | State Repository | SQLite (`lite`: 인메모리) | 필수 | ● | 정의 |
| NFR-ARC-0107 | HTTPS Gateway | 실행파일 내장 (별도 Caddy/Nginx 없음) | 필수 | ● | 정의 |
| NFR-ARC-0108 | Local Deployment | 네이티브 실행파일 (Docker 이미지는 선택) | 필수 | ● | 정의 |
| NFR-ARC-0109 | Test | Go Test + pytest/pyVmomi 시나리오 테스트 + D2에서 선정한 SDK·버전의 자체 계약 테스트 | 필수 | ● | 정의 |
| NFR-ARC-0110 | API Schema Validation | 프로파일의 WSDL/OpenAPI 스펙 | 필수 | ● | 정의 |
| NFR-ARC-0111 | CI | GitHub Actions 등 (리소스 예산 측정 포함) | 필수 | ● | 정의 |
| NFR-ARC-0112 | Configuration | YAML / JSON | 필수 | ● | 정의 |
| NFR-ARC-0113 | Python 테스트 도구 | Python은 권고안 A에서도 테스트 도구(Scenario Runner, pyVmomi 기반 테스트, 리포트 생성)로 활용할 수 있으며, 시뮬레이터 실행에는 필요하지 않다. | 선택 | – | 정의 |

**참고 — 프로세스 구성안:** 권고안 A(단일 Go 바이너리)를 채택했다. 권고안 A 전용 항목(NFR-ARC-0301, PRD-1 FR-BE-0304, FR-DM-0306)은 필수 구현 의무로 적용하고, 권고안 B 전용 항목(NFR-ARC-0302, PRD-1 FR-BE-0305, FR-BE-0405)은 '보류' 상태·MVP '–'로 TR-COV-0103 추적표에 '적용 제외 — D3'로 기록한다. 구성안 비교표, B의 적용 조건과 재검토 절차는 PRD-B를 따른다. Docker 없는 실행·API 호환·보안·데이터 무결성 의무는 구성안과 관계없이 공통이다.

## 14.2 Repository 구조

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-ARC-0201 | 디렉터리 구성 | 저장소 이름은 `cloudinfra-sim`이며, 아래 '디렉터리 구성 초안'을 기준으로 저장소를 구성한다. 최상위 폴더는 작업 단계 번호 접두어로 구분한다(00 기획, 20 설계, 30 구현, 40 테스트, 50 배포, 90 Trace). `.agentic/`은 AI 에이전트의 작업 공간으로 제품 산출물과 분리한다. | 권장 | ● | 정의 |
| NFR-ARC-0202 | 주요 설정 파일 위치 | 저장소 루트: `README.md`, `Makefile` 또는 동등한 작업 실행 스크립트(하위 폴더의 빌드·테스트·배포 명령을 묶음), `.env.example`. `30-implementation/`: `go.mod`, `go.sum`(권고안 B 구성은 PRD-B §B8). `40-test/`: 테스트 도구용 `pyproject.toml`. `50-deployment/docker/`: `compose.yaml`(선택), `Dockerfile`. | 권장 | ● | 정의 |
| NFR-ARC-0203 | 빌드·배포 구조 조정 | 멀티언어 빌드 및 배포 구조를 검토해 조정할 수 있다. | 검토 | – | 정의 |

**참고 — 디렉터리 구성 초안**

```
README.md                      # 저장소 루트: 프로젝트 요약
Makefile                       # 빌드·테스트·배포 작업 실행 진입점
.env.example                   # 실행 환경변수 예시 (SIM_DOMAIN, SIM_VCENTER_VERSION 등)
00-planning/                   # 기획: PRD, 개정 이력 (버전별 폴더)
.decisions/                    # 개발 중 요청·결정 이력 (PRD-4 '작업 규칙')
20-design/                     # 설계 산출물
  architecture/  api-coverage/  behavior-spec/  design-decisions/  development-plan/
30-implementation/             # 구현
  go.mod                       # Go 모듈 정의 (go.sum 포함)
  cmd/
    cloudinfra-sim/            # 단일 실행파일 진입점
  internal/                    # 권고안 A 기준 (B 구성은 PRD-B §B8)
    gateway/                   # 내장 TLS, SNI 라우팅, 전송 계층 Fault, 자체 호출 기록
    profile/                   # 버전 프로파일 로더, 버전 인식 직렬화
    vcenter/                   # vcsim 임베드, vAPI 확장, Behavior Adapter
    nsx/                       # 스펙 기반 범용 핸들러, 전용 리소스 로직, Realization
    relations/                 # vCenter ↔ NSX 매핑, Fabric 인벤토리 파생
    engine/                    # Task Scheduler, Virtual Clock, Capacity, Event
    fault/                     # Fault Rule 모델 및 계층별 주입
    admin/                     # Admin API (Seed, Reset, Snapshot, Scenario, Coverage)
    store/                     # SQLite, Snapshot
  shared/
    schemas/  contracts/  resource-models/
  data/
    fixtures/  scenarios/  compatibility-profiles/
40-test/                       # 테스트
  pyproject.toml               # 테스트 도구(pytest, pyVmomi 시나리오) 의존성
  unit/  contract/  integration/  simulator-e2e/  performance/
  footprint/                   # 리소스 예산 측정
50-deployment/                 # 배포
  docker/                      # 선택: compose.yaml, Dockerfile
  certificates/
  install/                     # hosts·CA·루프백 별칭 설정 스크립트 (Windows/macOS/Linux)
90-trace/                      # 향후 외부 검증용 예약 경로 (현재 미사용)
.agentic/                      # 개발 산출물 외에 AI 에이전트가 작업하는 폴더
  scripts/
  examples/
```

## 14.3 프로세스 및 네트워크 구성

서비스별 접근 주소, 기본 바인딩, 가상 호스트 도메인은 PRD 개요 §2.2를 따른다. 도메인 설정과 인증서 SAN은 NFR-0313, NFR-0314에서 정의한다.

| ID | 요구사항 | 세부 내용 / 수용 기준 | 수준 | MVP | 상태 |
|---|---|---|---|---|---|
| NFR-ARC-0301 | 프로세스 구성 (권고안 A) | 단일 바이너리 `cloudinfra-sim`(Go)에 내장 HTTPS Gateway(SNI/Host 라우팅), vcsim 임베드, vAPI 확장, 보조 서비스, NSX 엔진, 행위 엔진, Admin API를 모두 포함한다. | 필수 | ● | 정의 |
| NFR-ARC-0302 | 프로세스 구성 (권고안 B) | 권고안 B 전용. D3에서 A를 채택해 보류한다. 상세는 PRD-B §B3, §B4. | 선택 | – | 보류 |
| NFR-ARC-0303 | 별도 서비스 배제 | 구성안과 관계없이 별도의 Reverse Proxy(Caddy/Nginx), vCenter Bridge, Simulation Controller 서비스를 두지 않는다. | 필수 | ● | 정의 |
| NFR-ARC-0304 | Gateway 라우팅 대상 | Gateway는 아래 'Gateway 라우팅 경로'를 해당 엔진으로 전달한다. 경로별 지원 여부는 API Coverage Matrix와 활성 버전 프로파일로 결정한다. | 필수 | ● | 정의 |
| NFR-ARC-0305 | 관리 API 분리 | 시뮬레이터 전용 관리 API(Admin API)는 VMware API와 명확히 분리하며 VMware API 클라이언트에 노출하지 않는다. | 필수 | ● | 정의 |
| NFR-ARC-0306 | 포트·바인딩 조정 | PRD 개요 §2.2의 포트와 바인딩 주소는 초기 설계안이며, 충돌 여부를 분석해 조정할 수 있다. | 권장 | ● | 정의 |
| NFR-ARC-0307 | 루프백 별칭 바인딩 | vCenter와 NSX가 모두 443을 쓰므로 루프백 별칭 주소(127.0.0.2, 127.0.0.3)에 바인딩하여 다른 프로그램의 443 점유(IIS, VMware Workstation 공유 VM 서비스 등)와의 충돌을 줄인다. | 필수 | ● | 정의 |
| NFR-ARC-0308 | 로컬 환경 설치 자동화 | hosts 파일 등록, 시뮬레이터 루트 CA 신뢰 등록, 루프백 별칭 설정을 설치 스크립트(`50-deployment/install/`)로 자동화하고, 관리자 권한이 필요한 단계를 문서화한다. MVP는 Windows 설치 스크립트를 제공하고 macOS·Linux는 수동 설치 절차 문서로 대체하며, macOS·Linux 자동화는 Phase 5에서 제공한다. | 필수 | ● | 정의 |
| NFR-ARC-0309 | 지원 배포 토폴로지 | 다음 중 지원 대상을 Phase 0에서 확정한다(PRD-4 D6). ① 테스트 클라이언트와 시뮬레이터가 같은 PC에서 네이티브로 실행 ② 테스트 클라이언트가 같은 PC의 컨테이너/VM에서 실행(호스트 주소 노출 필요) ③ 테스트 클라이언트가 원격 서버에서 실행(Localhost 바인딩 해제 및 접근 제한 필요, NFR-0311) | 필수 | ○ | 정의 |

**참고 — Gateway 라우팅 경로**

| 대상 | 경로 |
|---|---|
| vCenter SOAP | `/sdk`, `/sdk/vimServiceVersions.xml`, `/sdk/vimService.wsdl` |
| vCenter REST | `/api/...`, 구형 `/rest/...` |
| Lookup Service | `/lookupservice/sdk` |
| SSO STS | `/sts/STSService/...` |
| Storage Policy (PBM) | `/pbm/sdk` |
| Content Library / OVF | `/api/content/...`, `/api/vcenter/ovf/...` (구형 `/rest/com/vmware/content/...`) |
| ESXi Host 직접 경로 | NFC/OVF 업로드 URL, `/folder/...` 등 vCenter 응답이 가리키는 호스트 주소 |
| NSX | `/api/v1/...`, `/policy/api/v1/...`, `/api/session/create`, `/api/v1/spec/openapi/...` |

---

## 요구사항 집계

각 장의 요구사항 수와 수준·MVP 분포이다. 보류·적용 제외 항목도 ID 보존을 위해 전체에 포함하며, 현재 구현·완료 의무 수와 다르다. MVP –에는 후속 및 적용 제외 항목이 포함된다. 요구사항·MVP·적용 여부를 바꾸면 이 표도 갱신한다.

| 장 | 접두어 | 전체 | 필수 | 권장 | 선택 | 검토 | MVP ● | MVP ○ | MVP – |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| 13. 성능 및 비기능 요구사항 | `NFR` | 53 | 53 | 0 | 0 | 0 | 41 | 1 | 11 |
| 14. 권장 기술 아키텍처 및 디렉터리 구조 | `NFR-ARC` | 25 | 18 | 4 | 2 | 1 | 20 | 2 | 3 |
| **합계** |  | **78** | **71** | **4** | **2** | **1** | **61** | **3** | **14** |
