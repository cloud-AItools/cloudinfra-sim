# CloudInfra Simulator

**CMP-Oriented VMware SDDC API Simulator** · 저장소 `cloudinfra-sim` — 실제 vCenter, ESXi, NSX Manager 없이 VMware API와 상태를 구성하고 자체 테스트하는 시뮬레이터

> **최신 문서: v1.1 (2026-10-10)**  
> - PRD 개요: [`00-planning/v1.1/cloudinfra-sim-PRD.md`](00-planning/v1.1/cloudinfra-sim-PRD.md)  
> - PRD-1 기능별 요구사항: [`00-planning/v1.1/cloudinfra-sim-PRD-1-기능별-요구사항.md`](00-planning/v1.1/cloudinfra-sim-PRD-1-기능별-요구사항.md)  
> - PRD-2 테스트 요구사항: [`00-planning/v1.1/cloudinfra-sim-PRD-2-테스트-요구사항.md`](00-planning/v1.1/cloudinfra-sim-PRD-2-테스트-요구사항.md)  
> - PRD-3 비기능 요구사항: [`00-planning/v1.1/cloudinfra-sim-PRD-3-비기능-요구사항.md`](00-planning/v1.1/cloudinfra-sim-PRD-3-비기능-요구사항.md)  
> - PRD-4 개발계획 검토: [`00-planning/v1.1/cloudinfra-sim-PRD-4-개발계획-검토.md`](00-planning/v1.1/cloudinfra-sim-PRD-4-개발계획-검토.md)  
> - PRD-B 권고안 B 대안 구성: [`00-planning/v1.1/cloudinfra-sim-PRD-B-권고안B-대안-구성.md`](00-planning/v1.1/cloudinfra-sim-PRD-B-권고안B-대안-구성.md)  
> - CMP 요구기능 목록 (참고): [`00-planning/v1.1/cloudinfra-sim-PRD 1.1 CMP 요구기능 목록.md`](<00-planning/v1.1/cloudinfra-sim-PRD 1.1 CMP 요구기능 목록.md>)  
> - 호환성 목표범위 (참고): [`00-planning/v1.1/cloudinfra-sim-PRD 1.2 호환성 목표범위.md`](<00-planning/v1.1/cloudinfra-sim-PRD 1.2 호환성 목표범위.md>)  
> - 개정 이력 (전체 버전): [`00-planning/PRD-revision.md`](00-planning/PRD-revision.md)

---

## 프로젝트 요약

CMP는 vCenter의 SOAP/REST API와 NSX Manager의 Manager/Policy API를 호출해 VM과 네트워크를 관리한다. 이 연동 기능을 개발하고 검증하려면 보통 실제 VMware 인프라가 필요하지만, 구축 비용이 크고 버전별 환경을 갖추기 어렵다.

이 프로젝트는 실제 가상머신이나 데이터플레인을 실행하지 않고, **VMware API로 관찰되는 객체·상태·Task·이벤트·오류를 소프트웨어로 재현**한다. 이번 범위는 시뮬레이터 구성과 프로젝트 내부의 SDK·API 자체 테스트까지이다. CMP 문서는 기능 범위를 정하는 참고 자료로만 사용하며, CMP 소스 분석·실제 CMP 연동 시험·CMP 상품 제약 구현은 범위 밖이다(PRD-4 D13).

## 핵심 목표

| 목표 | 내용 |
|---|---|
| **A. 버전 기반 API 계약 재현** | 승인된 시뮬레이션 API 범위에서 지정 vCenter·NSX 버전의 계약과 상태 동작을 구현하고 자체 테스트로 검증한다. |
| **B. 경량 실행** | 개인 PC(Windows 포함)에서 Docker 없이 최소 리소스로 동작한다. 단일 Go 실행파일(권고안 A)로 구성한다. |

승인된 시뮬레이션 범위는 공식 API 명세와 CMP 기능 참고 목록을 바탕으로 Phase 0b에서 잠정 정의하고 Phase 2b에서 MVP 범위를 확정한다. 완료 판정은 요구사항·API·자체 테스트의 추적 증거로 한다. 자체 테스트의 검증 상한은 `Integration Tested`이며, '승인 범위 자체 테스트 통과'로 표시한다. 실제 CMP 호환 또는 실제 VMware와의 '100% 호환'을 입증한 것으로 표시하지 않는다. 외부 연동·실환경 검증과 Record 모드는 현재 범위에서 보류한다(D5·D13, AC-20).

## 주요 기능 (계획)

- **vCenter API:** SOAP(`/sdk`), REST(`/api`, 구형 `/rest`), Lookup Service, STS, PBM, Content Library/OVF
- **NSX API:** Manager API(`/api/v1`), Policy API(`/policy/api/v1`, Hierarchical API 포함), Realization 상태
- **상태 기반 시뮬레이션:** VM 생명주기, Task·PropertyCollector, NSX Revision 충돌, vCenter ↔ NSX 리소스 관계
- **버전 프로파일:** 공식 WSDL·OpenAPI 스펙 기반, 버전별 응답 필터링(버전 인식 직렬화)
- **테스트 지원:** Fixture 기반 환경 생성, Snapshot/Restore, Fault Injection, Virtual Clock, 자체 테스트 호출 기록, API Coverage Report

## 아키텍처 (권고안 A)

```
자체 테스트 클라이언트 / SDK
        │  HTTPS 443 (vcenter.cloudinfra-sim.test, nsx.cloudinfra-sim.test, esx-NN.cloudinfra-sim.test)
        ▼
┌────────────────────── cloudinfra-sim (단일 Go 실행파일) ──────────────────────┐
│  내장 Gateway (TLS, SNI 라우팅, 전송 계층 Fault, 자체 호출 기록)                    │
│  Version Profile + Version-Aware Serializer                                   │
│  vCenter: vcsim 임베드 + vAPI 확장 + Behavior Adapter                          │
│  NSX: 스펙 기반 범용 핸들러 + Realization Controller                            │
│  Engine: Task Scheduler, Virtual Clock, Capacity, Event, Relations            │
│  Admin API (127.0.0.1:9000) · SQLite                                          │
└───────────────────────────────────────────────────────────────────────────────┘
```

프로세스 구성은 권고안 A(단일 Go 실행파일)로 확정했다(PRD-4 D3). Go + Python 2개 프로세스로 구성하는 권고안 B는 PRD-B에 분리해 두었으며, Phase 0a의 PoC 결과가 PRD-B §B2의 재검토 조건에 해당할 때만 다시 검토한다.

가상 호스트의 기본 도메인은 `cloudinfra-sim.test`이며 `--domain` 옵션 또는 `SIM_DOMAIN` 환경변수로 바꿀 수 있다. `.test`는 테스트 전용 예약 TLD라 실제 운영 VMware와 혼동되지 않는다.

## 예상 리소스 요구사항

PRD의 계획(권고안 A: 단일 Go 실행파일, 네이티브 실행, `lite` 기본 프로필)대로 구성했을 때 예상되는 요구사항이다. 아래 값은 구성 요소 특성에 근거한 **설계 단계 추정치**이며, Phase 0a PoC와 `40-test/footprint/` 측정으로 확정한다(PRD-3 §13.1, NFR-0107 ~ NFR-0113). **CMP 자체가 사용하는 리소스는 포함하지 않는다.**

### 호스트 PC 요구사항

| 항목 | 최소 | 권장 |
|---|---|---|
| OS | Windows 10 (22H2) / 11, macOS 13 이상, Linux (x86_64 또는 ARM64) | Windows 10 (22H2) / 11, macOS 13 이상, Linux (x86_64 또는 ARM64) (최소와 동일) |
| CPU | 2코어 | 4코어 이상 (동시 요청 50~100개 테스트 시) |
| 시뮬레이터 가용 메모리 | 512 MB | 1 GB (`lite`) / 2 GB 이상 (`full`, 대규모 Fixture) |
| PC 전체 메모리 | 8 GB | 16 GB 이상 (자체 테스트·개발 도구 병행 시) |
| 디스크 | 500 MB (실행파일 + 버전 프로파일 1개 조합 + 데이터) | 2 GB 이상 (여러 버전 프로파일, Snapshot) / 5 GB 이상 (`full` 프로필에서 API Trace 본문 기록 시) |
| 필수 소프트웨어 | 없음 (Go/Python 런타임, Docker 불필요) | 테스트 도구 사용 시 Python 3.11 이상 |
| 권한 | 최초 1회 관리자 권한 (hosts 파일, 루트 CA, 루프백 별칭 등록) | 좌동 |
| 네트워크 포트 | 루프백 별칭 주소의 443, 127.0.0.1:9000 | 좌동 |

설치 자동화 스크립트는 MVP에서 Windows용을 먼저 제공하고, macOS·Linux는 수동 설치 절차 문서로 대체한다(자동화는 Phase 5).

### 시나리오별 시뮬레이터 예상 사용량

| 시나리오 | 예상 메모리 (RSS) | CPU |
|---|---|---|
| 기동 직후 유휴, `lite`, 기본 Fixture (VM 20, Segment 10) | 약 120~250 MB | 유휴 시 거의 0% |
| 중규모: VM 1,000, NSX Segment 200 | 약 250~450 MB | 요청 처리 시 1~2코어 순간 사용 |
| 대규모 `full`: VM 5,000, Segment 1,000, 동시 요청 100 | 약 0.8~1.5 GB | 2~4코어 |
| (참고) Docker Desktop으로 실행할 때 추가분 | WSL2 VM 등 약 1.5~3 GB 추가 | 백그라운드 사용 발생 |

권고안 B로 구성할 때의 추정치는 PRD-B §B9를 참고한다.

### 추정 근거와 해석

- **주된 메모리 사용처:** vcsim 인벤토리(VM 1개당 수십 KB 수준으로 추정, VM 수에 비례), 활성 버전의 NSX OpenAPI 스펙 인덱스(전체 모델을 생성하지 않고 지연 컴파일하므로 수십 MB 수준), PropertyCollector 세션별 필터 상태.
- **리소스 예산과의 관계:** PRD-3 §13.1(NFR-0107 ~ NFR-0113)의 기본 예산은 유휴 300 MB 이하 등이며, D4(2026-10-10)에서 제안값대로 확정했다. Phase 0a PoC 실측값은 유휴 Private 메모리 약 71 MB, 기동 약 1.2초, 유휴 CPU 0, 설치 크기 약 34 MB이다(`20-design/development-plan/POC_PLAN.md`).
- **증가 요인:** 동시 테스트 클라이언트 세션 수와 각 세션의 PropertyCollector 필터 범위, API Trace 본문 기록, 보존 한도 설정(PRD-3 §13.5), 여러 버전 프로파일의 동시 로드(기본은 활성 버전 1개만 로드).
- **Docker:** 선택 실행 수단이며, Windows에서는 Docker Desktop 자체가 시뮬레이터보다 큰 메모리를 차지할 수 있다.

## 현재 상태

- **단계:** Phase 0a ~ 5 완료(2026-10-10 ~ 11). Phase 5에서 Phase 4b 추가 API 20건을 실행 범위에 넣어 기준 조합 61개 API(모두 Integration Tested, release 기동 확인)로 하고, Snapshot/Restore·재기동 복원·불일치 검사, 관리 API Scenario Runner(선언형 시나리오 7종, 시작·중지·실행 로그·리포트), 이벤트 조회, API Coverage Report, full 보존·회전(Trace 본문·기간·용량), 성능 벤치마크(VM 5,000까지 측정, 목표는 VM·Segment 1,000·동시 50 이하 오류 0. 증분 관계 조정으로 Segment 쓰기 지연 VM 1,000개 334 → 25 ms, R-45). 수신 TLS는 프로파일 `tls.max_version`으로 vCenter 7.0.x·NSX-T 3.2.x 모두 TLS 1.2까지만 받는다(실제 제품과 같음, R-45 ③). soak 메모리 증가(R-45 ④)는 유휴 연결 정리와 vcsim 세션 객체 락 누수 보완으로 평탄해졌다, CI 워크플로·배포 패키지·macOS/Linux 설치 스크립트를 추가했다. AC-01 ~ AC-19 중 18개를 자체 시험으로 충족했고, AC-11(CI 실행)은 저장소·러너가 없어 로컬 전체 회귀로 대체 확인했다(`20-design/development-plan/DEVELOPMENT_PLAN.md` §13). 사용자 조치: 저장소·CI 구성, 설치 스크립트 실행, Phase 4b 이월 결정. 검증 상한은 Integration Tested이며 실환경·CMP 호환성은 검증 범위 밖이다.
- **개발 단계:** Phase 0a 핵심 기술 검증 → 0b 설계·계획 기준선 → 1 기본 API → 2a 상태 기반 생명주기 → 2b MVP 검증·호환성 판정 → 3 교차 시스템 → 4a 장애·세션·권한 → 4b 기능 확장·추가 버전 → 5 자동화·품질
- **결정 상태:** D3는 권고안 A, D13은 시뮬레이터 구성·자체 테스트까지로 결정. D1 대상은 vSphere/vCenter 7.0 U2 이상(8.x 포함), NSX-T 3.x이며 vSphere 9는 제외한다. 첫 기준 조합은 **vCenter 7.0 U3w(7.0.3.02500, 빌드 24927011) + NSX-T 3.2.4.3(빌드 24942842)**이다. NSX 계약 근거는 공개된 공식 스펙 NSX-T 3.2.1을 쓰고 3.2.4.3과의 차이는 미검증으로 표시한다. D4 리소스 예산은 제안값대로 확정했다(보존 한도는 Phase 0b). D2는 Phase 2b에서 최종 확정했다(실행 범위 28개 API, 자체 테스트 SDK govmomi v0.56.0, 범용 govc·pyVmomi). D5의 실환경 검증·Record는 현재 범위에서 보류한다. D6~D12는 권고안대로 결정했다: 같은 PC 네이티브 실행, MVP VM 생성은 기본 경로만, VDS 7.0 DVPG 표현, NSX는 Segment·Realization·조회 중심, 인증은 SOAP Login·`/api/session`·NSX Basic/세션, NSX 스펙은 사용자 측 다운로드, 단일 vCenter·NSX(PRD-4 §19).

## 실행 방법 (Phase 5 기준)

```
# 1) NSX 스펙(사용자 측 확보, D11): 브라우저로 받은 뒤 검증·배치
powershell -File 50-deployment/install/Get-NsxSpec.ps1 -SourceDir $env:USERPROFILE\Downloads
# 2) 빌드·실행
make build && make run          # 또는 30-implementation에서 go run ./cmd/cloudinfra-sim --vcenter-version 7.0.3.02500 --nsx-version 3.2.4.3
# 3) hosts·루트 CA 등록(첫 실행 후 CA 생성됨, hosts는 관리자 권한)
powershell -File 50-deployment/install/Install-CloudinfraSim.ps1
# 4) REST 계약 파일(시험용, 저장소 밖): SDK에서 추출해 VAPI_CONTRACT로 지정
powershell -File 50-deployment/install/Get-VapiContract.ps1 -OutDir D:\AISCP\tools\specs\vapi-bindings-7.0.3.2
# 5) 시험·증거
make test && make evidence
# 6) 범용 SDK(govc·pyVmomi) 시험 준비와 선언형 시나리오 전체 회귀(리포트·Trace·실행 로그: 40-test/reports)
make govc && make scenario
# 7) 전체 회귀(추가 버전 프로파일·증거 재생성 포함), 성능 벤치마크·보존 수렴, 배포 패키지
make regression
make perf && make soak
make dist
```
- macOS·Linux 설치: `50-deployment/install/install-cloudinfra-sim.sh`(hosts·루프백 별칭·루트 CA·443 권한, `--dry-run`으로 먼저 확인, `--uninstall`로 되돌림). 관리자 권한 단계는 사용자가 실행한다.
- Ansible 자체 테스트(TR-COV-0306): `40-test/ansible`(Linux·macOS·WSL 제어 노드). `python3 -m venv .venv && .venv/bin/pip install -r requirements.txt && .venv/bin/ansible-galaxy collection install -r requirements.yml` 뒤 `PATH=$PWD/.venv/bin:$PATH go test ./ansible -v`(40-test에서). Windows에서는 건너뛴다. Windows PC는 WSL로 실행한다: 40-test에서 `GOOS=linux GOARCH=amd64 go test -c -o ansible/ansible.linux.test ./ansible` 뒤 `wsl -- bash /mnt/<드라이브>/<저장소>/40-test/ansible/run-wsl.sh`(WSL에 python3-venv 필요, Go 불필요).
- 큰 Fixture(VM 200개 이상)는 처음 기동 때 vCenter 인벤토리를 데이터 디렉터리에 캐시하고 다음 기동·Reset에서 적재한다(VM 5,000개 210초 → 43 ~ 60초, R-45 ②). 끄려면 `--fixture-cache off`, 작은 Fixture에도 쓰려면 `on`.
- Snapshot/Restore: Admin `POST /v1/snapshots {"name":"s1"}`, `GET /v1/snapshots`, `POST /v1/snapshots/s1/restore`(또는 `last`), `DELETE /v1/snapshots/s1`, 불일치 검사 `GET /v1/consistency`. full 프로필은 정상 종료 때 `_shutdown` Snapshot을 남기고 `--restore-snapshot _shutdown|last|<이름>`으로 기동 시 복원한다(지정하지 않으면 Fixture로 시작).
- Scenario Runner: `GET /v1/scenarios`, `POST /v1/scenarios/{name}/start`, `GET /v1/scenario-runs/{id}`(`/log`, `/report`), `POST /v1/scenario-runs/{id}/stop`. 시나리오 파일은 `30-implementation/data/scenarios/*.yaml`(`--scenarios-dir`).
- 이벤트 조회 `GET /v1/events?type=&entity=`, API Coverage Report `GET /v1/coverage-report[?format=markdown]`.
- full 실행 프로필(`--run-profile full`): 상태·Trace를 데이터 디렉터리 SQLite에 저장, Trace 본문(64 KB 자름·자격 증명 마스킹), 보존 한도 `--retain-trace`·`--retain-trace-mb`·`--retain-days`·`--retain-snapshots`.

- 추가 버전: `--vcenter-version 7.0.2.00000 --nsx-version 3.2.4.3`(상호운용성 미확인 조합이라 development 모드만). 시험은 `SIM_TEST_VCENTER_VERSION=7.0.2.00000`으로 같은 시험을 이 프로파일에 실행한다.
- 임시 파일은 데이터 디렉터리의 `tmp\<pid>`에 만들고 종료 때 지운다. 작업 관리자 등으로 강제 종료해 남은 것은 다음 기동이 지운다.
- 사용자·비밀번호는 데이터 디렉터리(`%APPDATA%\cloudinfra-sim`)의 `users.json`에 첫 기동 시 무작위로 생성된다. Admin API 토큰은 같은 폴더의 `admin-token`이다.
- 기본 주소: vCenter `https://vcenter.cloudinfra-sim.test`(127.0.0.2:443), NSX `https://nsx.cloudinfra-sim.test`(127.0.0.3:443), Admin `http://127.0.0.1:9000`.
- 보존 한도: `--retain-tasks`·`--retain-transitions`·`--retain-operations`·`--retain-trace`(0=기본, 무제한 불가). 적용값은 Admin `/v1/status`의 `retention`.
- Admin API: `/v1/faults`(Fault Rule — 전송·vCenter·NSX·세션 계층), `/v1/clock`(Virtual Clock), `/v1/users`(시험용 역할 사용자), (Phase 3), `/v1/reconcile`·`/v1/relations`(vCenter ↔ NSX 조정), `/v1/audit?cid=`(감사·변경 원인), `/v1/metrics/overrides`(가상 메트릭 덮어쓰기).
- 범용 SDK는 hosts 등록 없이 IP로 접속할 수 있다(예: `govc` `GOVC_URL=https://127.0.0.2/sdk`, 인증서는 Admin 상태의 지문으로 검증).
- 상태 저장: `lite`는 인메모리 SQLite(재기동 시 Fixture로 초기화), `full`은 데이터 디렉터리의 `cloudinfra-sim.db`(WAL). NSX Realization 지연은 Admin API `PUT /v1/nsx/realization-delay`(`{"ms":700}`)로 바꾼다.

## 요구사항 문서 구성

| 문서 | 포함 장 | 요구사항 ID 접두어 | 요구사항 수 |
|---|---|---|---:|
| PRD 개요 | 1~2장(아키텍처 구성도 포함), 세부 문서 목록과 추적 원칙 | — | — |
| PRD-1 기능별 요구사항 | 3~9장 | FR-VC, FR-NSX, FR-BE, FR-DM, FR-API, FR-FLT, FR-MET | 278 |
| PRD-2 테스트 요구사항 | 10~12장 | TR-SCN, TR-E2E, TR-COV | 82 |
| PRD-3 비기능 요구사항 | 13~14장 | NFR, NFR-ARC | 78 |
| PRD-4 개발계획 검토 | 15~19장 (단계 로드맵, 작업 패키지, 위험, 완료 기준, AI 에이전트 지시, 결정 사항) | 계획 식별자: WP, R, AC, PoC, D | — |
| PRD-B 권고안 B 대안 구성 | 채택하지 않은 권고안 B의 설계, 재검토 조건·전환 절차 | 새 ID 없음 | — |

MVP는 Phase 2b 종료로 판정한다. 승인된 시뮬레이션 범위에서 선정 SDK의 자체 계약 테스트와 기본 NSX Realization을 포함한 Scenario 01~06을 검증한다. E2E는 테스트 클라이언트 → Gateway → 엔진 → 저장소를 잇는 시뮬레이터 내부 시험이며 실제 CMP를 실행하지 않는다. 요구사항·작업·시험·완료 기준 연결은 PRD-4 §15.5와 TR-COV-0103을 따른다. `development`/`release` 기동 검증 모드(PRD-1 FR-API-0302, 소스 빌드는 `development`, 배포 패키지는 `release`가 기본)는 리소스 설정인 `lite`/`full`과 별개이다.

## 폴더 구조

```
cloudinfra-sim/
├── README.md
├── Makefile                                       # 작업 진입점 (build, test, evidence, scenario, perf, dist 등)
├── .github/workflows/ci.yml                       # CI (unit·integration·release)
├── .decisions/                                    # 개발 중 요청·결정 이력 (DEVELOPMENT-revision {PRD 버전}.md)
├── 00-planning/                                   # 기획
│   ├── PRD-revision.md                            # 전체 버전 개정 이력
│   ├── v1.0/
│   │   └── VMWareNSX-Simulator-PRD.md             # v1.0 (이력, 이전 파일명 유지)
│   └── v1.1/
│       ├── cloudinfra-sim-PRD.md
│       ├── cloudinfra-sim-PRD-1-기능별-요구사항.md
│       ├── cloudinfra-sim-PRD-2-테스트-요구사항.md
│       ├── cloudinfra-sim-PRD-3-비기능-요구사항.md
│       ├── cloudinfra-sim-PRD-4-개발계획-검토.md
│       ├── cloudinfra-sim-PRD-B-권고안B-대안-구성.md
│       ├── cloudinfra-sim-PRD 1.1 CMP 요구기능 목록.md   # 참고: 기준 CMP 기능·API 매핑
│       └── cloudinfra-sim-PRD 1.2 호환성 목표범위.md     # 참고: 시뮬레이션 범위·자체 검증 기준
├── 20-design/                                     # 설계 산출물
│   ├── api-coverage/                              # API 커버리지 매트릭스, NSX 오퍼레이션·SOAP 메서드 조사
│   ├── architecture/                              # 아키텍처, 데이터 모델, 버전 프로파일 명세, 오픈소스 평가
│   ├── behavior-spec/                             # 동작 명세
│   └── development-plan/                          # 개발 계획, MVP 범위, PoC, 리소스 예산, 위험, 시험 전략
├── 30-implementation/                             # 구현 (Go 모듈)
│   ├── cmd/                                       # cloudinfra-sim 실행파일, srcdigest(빌드 식별자)
│   ├── internal/                                  # gateway, vcenter, nsx, store, scenario, fault, profile 등
│   └── data/                                      # 버전 프로파일, Fixture, 선언형 시나리오
├── 40-test/                                       # 시험 (Go 모듈)
│   ├── contract/                                  # API 계약 시험
│   ├── simulator-e2e/                             # E2E·선언형 시나리오 시험
│   ├── sdk/                                       # 범용 SDK 시험 (govc, pyVmomi)
│   ├── ansible/                                   # Ansible 자체 테스트 (Linux·WSL 제어 노드)
│   ├── performance/                               # 벤치마크·soak, 결과(results/)
│   ├── footprint/, deployment/                    # 리소스 예산 측정, 설치 스크립트 시험
│   ├── cmd/evidence/                              # 시험 결과 → verification.yaml 생성
│   ├── internal/, tools/                          # 시험 공용 검사기, govc 빌드 모듈
│   └── *.yaml                                     # 증거 매핑·검토 증거·이월·결정 목록
├── 50-deployment/                                 # 배포 (dist.sh, install/ 설치 스크립트)
└── .agentic/                                      # AI 에이전트 작업 공간 (poc/ 기술 검증, scripts/ 추적표 생성)
```

빌드 산출물(`bin/`, `dist/`), 시험 원자료(`40-test/results*.json`, `40-test/reports/`)와 라이선스 표기가 없는 NSX·vSphere Automation 공식 스펙(PRD-4 D11)은 저장소에 포함하지 않는다(`.gitignore`). NSX 스펙은 Broadcom 개발자 포털에서 받은 뒤 `50-deployment/install/Get-NsxSpec.ps1`로 검증·배치한다.

새 버전 PRD(개요 + 세부 문서)는 `00-planning/vX.Y/` 폴더에 추가하고, 개정 이력은 `00-planning/PRD-revision.md` 한 곳에 누적 기록한다. 상세 규칙은 개정 이력 문서의 '문서 관리 규칙'을 따른다.
