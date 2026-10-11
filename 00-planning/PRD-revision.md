# CloudInfra Simulator PRD — 개정 이력

> **최신 버전: v1.1 (2026-10-10)**  
> 프로젝트: CloudInfra Simulator (저장소 `cloudinfra-sim`)  
> 문서: `00-planning/v1.1/cloudinfra-sim-PRD.md`

---

## 1. 버전 이력

최신 버전을 맨 위에 기록한다.

| 버전 | 일자 | 문서 위치 | 주요 내용 |
|---|---|---|---|
| **v1.1 (최신)** | 2026-10-10 | `00-planning/v1.1/cloudinfra-sim-PRD.md` 외 세부 요구사항 문서 4종, 권고안 B 분리 문서(PRD-B) | 두 핵심 목표(버전 기반 호환성, 개인 PC 경량 실행) 기준 개정 및 문서 분리·명칭 변경. 같은 버전의 교차검증 수정으로 NSX 계약, MVP/단계 의존성, 검증·기동·릴리스 기준, A/B 적용, 보존 한도와 추적 기준선을 보완. 개발계획 검토 반영으로 세부 단계(0a·0b, 2a·2b, 4a·4b) 도입, 권고안 A 확정 및 B 분리. CMP 참고 범위·자체 테스트 완료 기준으로 정리(D13), CMP 소스 분석·상품 제약·외부 연동 검증 제외. 현재 요구사항 438건(상세 #32~64) |
| v1.0 | — | `00-planning/v1.0/VMWareNSX-Simulator-PRD.md` | 최초 작성. 목표 아키텍처, API 범위, 행위 엔진, 테스트·검증 전략, Codex 개발계획 수립 지침 |

---

## 2. v1.1 개정 배경

v1.0을 다음 두 핵심 목표 기준으로 검토했다.

- **핵심 목표 A — 버전 기반 호환성:** vCenter/NSX 버전을 지정하면 해당 버전 계약에 맞는 응답을 CMP에 제공한다.
- **핵심 목표 B — 경량 실행:** 개인 PC(특히 Windows)에서 최소 리소스로 실행된다.

검토 결과, v1.0은 검증 원칙('100%'를 승인 범위로 한정, 버전 문자열 위장 금지, Implemented와 Verified 구분)은 충실했으나 다음 구조적 공백이 있었다.

- 버전 프로파일이 Phase 4의 부가 기능으로 설계되어 있고, 버전에 따라 응답을 달리 만드는 장치(버전 인식 직렬화)가 없었다.
- 5~6개 서비스, 2개 언어, Docker Desktop 전제 구성이 경량 실행 목표와 충돌했다. 또한 행위 엔진이 vCenter 요청 경로 밖에 있어 사전조건·Fault로 요청을 거부할 수 없었다.

---

## 3. v1.1 상세 변경 내역

같은 버전 내 변경을 발생 순서대로 누적한다. 아래 중간 집계(571·573·582·433·437건)는 당시 이력이며, 최신 집계는 #46의 438건이다. 관련 절은 분리 후의 현재 위치로 정정했다. #54 이후는 2026-10-10 범위 결정(D13)을 반영한 최신 기준이며, 이전 이력의 CMP 검증·Record 의무는 현재 요구사항으로 해석하지 않는다.

| # | 변경 내용 | 관련 절 | 관련 목표 |
|---|---|---|---|
| 1 | 버전 프로파일을 Phase 4 부가 기능에서 Phase 0~1 기반 골격으로 이동, 실행 시 버전 입력 및 미등록 버전 기동 거부 | §1.4, §3.6, §7.3, §15 | A |
| 2 | 버전 인식 직렬화(Version-Aware Serialization) 계층 신설 | §3.6(FR-VC-0604 ~ FR-VC-0608), §7.3, R-11 | A |
| 3 | vCenter 행위 로직을 vcsim과 동일 Go 프로세스에서 동기 처리 (vCenter Bridge → vCenter Behavior Adapter) | §5.3, §5.4 | A, B |
| 4 | 서비스 5~6개 → 프로세스 1개(권고안 A) 또는 2개(권고안 B)로 통합, 내장 Gateway | §2.2, §14.1 | B |
| 5 | 네이티브 실행을 기본 배포 방식으로, Docker는 선택 | §1.4, §2.2, §13.1 | B |
| 6 | 리소스 예산을 사후 측정이 아닌 사전 완료 기준으로 지정 | §13.1(NFR-0107 ~ NFR-0113), §17 | B |
| 7 | 누락 엔드포인트 보강: Lookup Service, STS, PBM, Content Library/OVF, ESXi 직접 경로, 버전 협상 경로 | §2.2, §3.2 | A |
| 8 | NSX 보강: Hierarchical API, PATCH/PUT upsert 의미, Fabric VM/VIF/Tag, Projects/VPC 등 버전별 차이 | §4.2, §4.4 | A |
| 9 | 세그먼트 ↔ vCenter 네트워크 표현의 버전별 규칙, NSX VM 인벤토리 파생 규칙 명시 | §4.6 | A |
| 10 | vSphere REST ID = SOAP MoRef 값 규칙 명시 | §6.2 | A |
| 11 | NSX 전체 스펙 정적 모델 생성 대신 스펙 기반 지연 검증 엔진 | §4.1(FR-NSX-0105 ~ FR-NSX-0108) | B |
| 12 | 메트릭 즉석 계산, 로그·Trace 보존 한도, `lite`/`full` 실행 프로필 | §9.1, §13.1(NFR-0114 ~ NFR-0117), §13.5 | B |
| 13 | Fault Injection 계층 분리 (전송 / vCenter 의미 / NSX 의미 / 세션) | §8.2 | A |
| 14 | Record 모드와 실환경 Trace 확보 계획을 Phase 0 산출물로 지정 | §12.4(TR-COV-0405 ~ TR-COV-0408), §15 | A |
| 15 | CMP와 동일한 SDK·버전으로 계약 테스트 우선 | §12.3, §17 | A |
| 16 | Windows 로컬 네트워크(443, hosts, CA, 루프백 별칭) 및 배포 토폴로지 명시 | §2.2, §13.3 | B |
| 17 | 관리 API 명칭·포트 통일 (Admin API 하나) | §2.2, §14.2 | — |
| 18 | 신규 위험(Risk 11~14), 완료 기준(15~18), Codex 작업 지시 보강, 결정 필요 사항(§19) 추가 | §16, §17, §18, §19 | A, B |
| 19 | 문서 최상단에 목차(대목차) 추가 | 목차 | — |
| 20 | 계획 구성 기준 예상 리소스 요구사항(최소/권장, 시나리오별 사용량) 추가 | 예상 리소스 요구사항 | B |
| 21 | 개정 이력을 본 문서로 분리하고, PRD 본문 내 버전 표시 제거 | — | — |
| 22 | 버전별 폴더 구조(`revision/vX.Y/`) 도입, 각 폴더의 PRD 파일명 통일, 루트 `README.md` 추가 | — | — |
| 23 | 3~19장을 세부 요구사항 문서 4종(기능별·테스트·비기능·개발계획 검토)으로 분리하고, 장·절별 요구사항 ID 표(총 571건)와 문서별 집계 표를 도입. PRD 본문은 1~2장, 예상 리소스 요구사항, 세부 문서 목록과 추적 원칙으로 축소 | 세부 요구사항 문서 | — |
| 24 | 실행파일 이름을 `sddc-sim` → `cloudinfra-sim`(권고안 B의 NSX 프로세스는 `cloudinfra-sim-nsx`)으로, 가상 호스트 주소를 `*.lab.local` → `*.cloudinfra-sim.test`(`vcenter`, `nsx`, `esx-NN`)로 변경. mDNS 전용 TLD `.local`을 테스트 전용 예약 TLD `.test`로 교체하고, 도메인 설정(`--domain`, `SIM_DOMAIN`)과 예약 TLD 원칙을 요구사항 NFR-0313, NFR-0314로 추가(총 573건). 인증서 SAN을 와일드카드 `*.<도메인>`으로 변경 | §2.2, NFR-0313, NFR-0314, FR-API-0301, FR-BE-0405, NFR-ARC-0201 | A, B |
| 25 | 프로젝트명을 CloudInfra Simulator(부제: CMP-Oriented VMware SDDC API Simulator), 저장소명을 `cloudinfra-sim`으로 변경. v1.1 문서 파일명 접두어를 `VMWareNSX-Simulator-PRD` → `cloudinfra-sim-PRD`로 변경(v1.0 이력 파일은 이전 이름 유지) | 전 문서 머리말, NFR-ARC-0201 | — |
| 26 | PRD 개요 구조 정리: '예상 리소스 요구사항'을 README로 이동, §2.1 아키텍처 개요를 Mermaid 구성도와 계층별 구성 요소 표로 변경, §2.2의 프로세스 구성 이하 세부 내용을 PRD-3 §14.3(NFR-ARC-0301 ~ NFR-ARC-0309, 총 582건)으로 이동. 기존 요구사항과 중복되는 API 경로 보존(FR-API-0101·0102), 네이티브 실행(NFR-0104), 인증서 SAN(NFR-0313)은 새로 만들지 않고 기존 ID에 통합 | §2.1, §2.2, README, NFR-0104, NFR-ARC-0301 ~ NFR-ARC-0309 | B |
| 27 | 특정 도구명 'Codex'를 제거. PRD 개요 부제와 18장 제목은 'AI 에이전트', 16장 제목은 주어 없이 표기하고, 15장의 수립 주체를 'AI 에이전트 또는 개발자'로 변경. 18장 공통 지시(DP-TSK-0001)에 개발자도 수행할 수 있음을 명시 | PRD 개요, PRD-4 §15·§16·§18, DP-TSK-0001 | — |
| 28 | PRD-4를 요구사항 표 형식에서 개발계획 기준선 형식으로 재구성: 단계 로드맵(Mermaid), 단계별 목표·진입 조건·작업 패키지(WP)·산출물·대상 모듈·종료 게이트, MVP 마일스톤(Phase 2 종료), 일정 산정 원칙, 위험 대응표(R-01~14), 완료 기준 체크리스트(AC-01~18), AI 에이전트 작업 지시(Task 1~7, PoC-01~20), 결정 기록표(D1~D10). `DP-*` 요구사항 ID 149건을 계획 식별자로 대체해 요구사항 수는 PRD-1~3 기준 433건. MVP 범위와 맞추기 위해 WP 2.8(기본 API 오류 처리), WP 2.9(기본 테스트 자동화)를 추가하고 WP 1.8에 Seed·Reset 포함 | PRD-4 전체, PRD 개요 세부 문서 목록, README | A, B |
| 29 | 저장소 최상위 폴더 구조를 작업 단계 번호 체계로 변경: `revision/` → `00-planning/`, `docs/` → `20-design/`, 구현 코드(`cmd/`, `internal/`, `shared/`, `data/`) → `30-implementation/`, `tests/` → `40-test/`, `deployment/` → `50-deployment/`, `traces/` → `90-trace/`, `scripts/`·`examples/` → `.agentic/`(AI 에이전트 작업 공간). PRD-3 디렉터리 구성 초안과 문서 내 경로 참조 갱신 | NFR-ARC-0201, NFR-ARC-0308, PRD-1 §7.3, PRD-4 §15·§18, README | — |
| 30 | 주요 설정 파일 위치를 새 폴더 구조에 맞게 조정: 저장소 루트에는 `README.md`, `Makefile`, `.env.example`만 두고, `go.mod`·`go.sum`은 `30-implementation/`, 테스트 도구용 `pyproject.toml`은 `40-test/`, `compose.yaml`·`Dockerfile`은 `50-deployment/docker/`로 이동. 디렉터리 구성 초안에 해당 파일 표기 | NFR-ARC-0202, PRD-3 §14.2 | — |
| 31 | 개정 이력 문서 파일명을 `cloudinfra-sim-PRD-revision.md` → `PRD-revision.md`로 단순화 | 개정 이력, PRD 개요 머리말, README | — |
| 32 | NSX PATCH를 항상 부분 병합으로 취급하는 문구 수정. Partial Patch 설정·헤더 우선순위·배열·리소스 예외를 버전별 계약으로 검증하고 공식 근거 링크 추가. Scenario 06은 생성 응답 후 GET으로 객체·Revision 확인 | FR-NSX-0405, FR-NSX-0416, TR-E2E-0601 ~ TR-E2E-0604 | A |
| 33 | 기본 Realization과 설정 지연을 WP 2.10으로 이동. 전용 행위 로직은 해당 리소스의 지원 단계에 적용하며 후속 Realization·장애 동작은 WP 3.4·WP 4.5로 구분. AC-04와 AC-09의 확인 단계를 선행 구현·시험 시점에 맞춤 | FR-NSX-0108, FR-NSX-0501 ~ FR-NSX-0509, FR-BE-0606, §15, AC-04, AC-09 | A |
| 34 | MVP 기본 오류·자동화 테스트와 후속 권한·Timeout·실패 주입·E2E를 분리. 신규 ID TR-SCN-0303, TR-COV-0308, TR-COV-0309 추가 | TR-SCN-0301, TR-SCN-0303, TR-COV-0303, TR-COV-0307 ~ TR-COV-0309, WP 2.9, WP 4.8 | A |
| 35 | Scenario 01에 NSX 인증·버전·기본 조회·CMP 등록 판정을 추가하고 등록용 Compute Manager 관계를 Phase 1에 배정 | TR-E2E-0101 ~ TR-E2E-0104, FR-NSX-0601, WP 1.7, AC-02 | A |
| 36 | 일반 릴리스(AC-01 ~ AC-19)와 '100% 호환' 표시(AC-20)를 구분. API·시나리오별 CMP 및 실환경 Trace 비교 증거와 검증 상한을 명시 | TR-COV-0102, TR-COV-0201, TR-COV-0202, TR-COV-0409, AC-19, AC-20, README | A |
| 37 | 프로파일 기동에 release/development 검증 모드, 최소 API 상태, 실행 범위와 증거 무효화 규칙을 정의. lite/full 설정과 구분 | FR-API-0302, WP 1.2, AC-15 | A |
| 38 | A/B 구성별 런타임·배포·전용 요구사항 적용을 정리. B의 Python 의존성을 명시하되 공통 호환·보안·예산 의무 유지 | NFR-0104, NFR-0105, NFR-0112, NFR-0113, §14.1, D3, D4, README | B |
| 39 | full 전체 로그 보존을 유한 기간·건수·총용량·회전 정책으로 수정. 집계 키·Trace 상한과 증거 삭제 시 검증 상태 재평가 추가 | NFR-0503, NFR-0504, NFR-0508, WP 1.11, WP 5.9, D4 | B |
| 40 | E2E 외 검토·PoC·Unit·Contract·보안·무결성·Footprint까지 요구사항 검증 추적 범위를 확장. 신규 TR-COV-0103과 §15.5 기준선·WP 0.17·AC-19 추가; 후속 기능과 보안·보존 작업 배정 보완 | PRD 개요 추적 원칙, TR-COV-0103, §15.5, §18 Task 7, AC-19 | A, B |
| 41 | 존재하지 않는 하위 절 참조를 현재 절·요구사항 ID로 정정. 기존 ID를 유지하고 테스트 요구사항 4건 추가에 따라 433 → 437건(FR 278, TR 82, NFR 77)으로 README·개요·집계를 갱신. 대상 버전·구성안 등 D 결정은 미결로 유지 | README, PRD-1~4, 본 문서 | A, B |
| 42 | 개발계획 검토 반영: Phase 0을 0a(핵심 기술 검증, 기간 상한)와 0b(설계·계획 기준선)로, Phase 2를 2a(Stateful 생명주기)와 2b(MVP 검증·호환성 판정)로, Phase 4를 4a(장애·세션·권한)와 4b(기능 확장·추가 버전)로 분리. MVP 마일스톤을 Phase 2b 종료로 변경하고 4a 내부 작업 순서(WP 4.10 → WP 4.1) 명시. WP 번호는 유지 | PRD-4 §15.1 ~ §15.3, §16, §17, PRD-2 TR-SCN-0303, TR-COV-0308, TR-COV-0309, README | A, B |
| 43 | '승인된 CMP 기능 범위'는 Phase 0b에서 잠정 기준선으로 기록하고 Phase 2b 종료 게이트(WP 2.12)에서 확정하도록 명시. D2에 승인 기능 범위를 포함 | PRD 개요 §1.2, PRD-4 §15.2, §19 D2, README | A |
| 44 | D3를 권고안 A로 결정(Phase 0a에서 PoC-17·PoC-21로 유지 확인). 권고안 B의 비교표·적용 조건·교차 시스템 절차·PoC-07·예상 리소스를 신규 문서 `cloudinfra-sim-PRD-B-권고안B-대안-구성.md`로 분리. B 전용 요구사항 FR-BE-0305, FR-BE-0405, NFR-ARC-0302는 ID를 유지하고 상태를 '보류'로 변경 | PRD 개요 §1.4, PRD-1 대안별 적용·FR-NSX-0101, PRD-2 TR-E2E-0803, PRD-3 NFR-0104 ~ NFR-0113·§14.1·NFR-ARC-0101·NFR-ARC-0202, PRD-4 R-02·R-07·PoC-07·§18·D3, PRD-B, README | B |
| 45 | 핵심 요구사항의 WP 배정 보완: vCenter Behavior Adapter(WP 2.11), 미지원 API 처리(WP 1.12), 검증 증거 생성 자동화(WP 1.13), 승인 범위 확정·MVP 호환성 판정(WP 2.12), 전송 계층 Fault의 Phase 3 선행(WP 3.8), macOS·Linux 설치 자동화(WP 5.10). 버전 협상 경로·세션·Credential·로그·NSX 공통 필드 등 미배정 MVP 요구사항을 기존 WP에 추가 | PRD-4 §15.2, §15.3, §15.5 | A, B |
| 46 | 인증서 지문 안정성 요구사항 NFR-0315 추가(437 → 438건, NFR 78). FR-NSX-0601에 Compute Manager 등록 지문 일치 조건 추가 | PRD-3 §13.3·집계, PRD-1 FR-NSX-0601, PRD-4 WP 1.11·AC-18, README, PRD 개요 | A |
| 47 | 프로파일 기동 검증 모드의 기본값을 빌드 종류별로 변경(배포 패키지 `release`, 소스·개발 빌드 `development`). release 증거는 CI가 프로파일·구현 식별자에 묶어 자동 생성 | PRD-1 FR-API-0302, PRD-4 WP 1.2, WP 1.13 | A |
| 48 | 신규 계획 식별자: PoC-21(단일 락과 vcsim 내부 락), R-15(락 상호작용), R-16(스펙·Trace 라이선스), D11(라이선스·보관 방식), D12(관리 대상 인스턴스 수), WP 0.18, WP 0.19. 추적표는 MVP·Phase 1 대상만 Phase 0에서 개별 행으로 전개하고 나머지는 단계 진입 시 전개. AC-16 확인 시점을 Phase 2b(협상 버전 하향 세션)로 앞당김. MVP 설치 자동화는 Windows 우선. 단계별 상대 규모(추정) 추가 | PRD-2 TR-COV-0103, PRD-3 NFR-ARC-0308, PRD-4 §15.4, §15.5, §16 ~ §19 | A, B |
| 49 | Scenario 05 절차의 권한 검사를 Phase 4a 이후 회귀 절차로 이동. 시나리오 경계를 넘는 ID 범위 표기(TR-E2E-0901 ~ TR-E2E-1003)를 시나리오별로 분리. 세부 문서 파일명의 공백·마침표를 하이픈으로 변경(예: `cloudinfra-sim-PRD-1. 기능별 요구사항.md` → `cloudinfra-sim-PRD-1-기능별-요구사항.md`) | PRD-2 TR-E2E-0501, PRD-4 Phase 4a 종료 게이트, 전 문서 파일명, README, 본 문서 | — |
| 50 | D1 대상 버전 범위 결정: vSphere/vCenter 7.0 U2 이상, NSX-T 3.x(Policy API 기반, NSX 4.x 전용 기능 미사용). vSphere 상한(8.x 포함 여부)·VCF 9 계열·첫 기준 조합의 빌드는 미결. NSX 4.x 전용 FR-NSX-0230(Projects/VPC)을 '보류'(적용 제외 — D1)로 변경, D9에서 Projects/VPC 제외. FR-API-0301 예시 버전을 대상 범위 안의 값으로 수정. 참고 문서 `cloudinfra-sim-PRD 1.1 CMP 요구기능 목록.md`(기준 CMP 기능 150개·API 매핑), `cloudinfra-sim-PRD 1.2 호환성 목표범위.md`(판정 조합·목표범위 계층·판정 기준) 추가 및 D1 반영(NSX VPC 경로 제외, 네이티브 LB 계층 B 유지). 요구사항 수 변동 없음(438건). 두 참고 문서 파일명은 요청된 이름을 유지해 §5 파일명 규칙(공백·마침표 금지)의 예외로 둔다 | PRD 개요 §1.2·세부 문서 목록, PRD-1 FR-NSX-0230·FR-API-0301·FR-API-0312·대상 버전 적용, PRD-4 §19 D1·D9, 참고 문서 1.1·1.2, README, 본 문서 | A |
| 51 | D1 대상 범위에 vSphere 8.x 포함: vSphere/vCenter 7.0 U2 이상(8.x 포함), NSX-T 3.x. vSphere 9(VCF 9 계열)는 범위에서 제외. vSphere 8.x와 조합 가능한 NSX-T 버전이 3.2.x 일부 빌드로 제한되므로 허용 조합을 `compatibility-matrix.yaml`로 관리하고 VMware 상호운용성 매트릭스로 확인하도록 명시. 요구사항 수 변동 없음 | PRD 개요 §1.2, PRD-1 FR-API-0312·대상 버전 적용·프로파일 디렉터리 예시, PRD-4 §19 D1, 참고 문서 1.1 §17.1·1.2 §1·§6·§7, README | A |
| 52 | D1 첫 기준 조합 결정: vCenter 7.0 U3 + NSX-T 3.2.x. 세부 패치·빌드 번호는 D5(실환경 접근)·WP 0.6(스펙 확보) 시 확정해 `profile.yaml`에 기록. PRD-4 D1 상태를 결정으로 변경. 1.2 판정 조합 C-1을 이 조합으로 고정하고, 1.1의 LB 계약 기준을 NSX-T 3.2.x 스펙으로 지정. 요구사항 수 변동 없음 | PRD 개요 §1.2, PRD-1 FR-API-0301·FR-API-0312, PRD-4 §19 D1, 참고 문서 1.1 §18.3·1.2 §1·§6·§7, README | A |
| 53 | 첫 기준 조합의 패치·빌드를 각 계열의 최신 안정 패치로 확정: vCenter 7.0 U3w(7.0.3.02500, 빌드 24927011, 2025-09-29), NSX-T 3.2.4.3(빌드 24942842, 2025-09-25). 근거: Broadcom KB 326316(vCenter 빌드 번호), KB 317797(NSX 빌드 번호), 2026-10-10 조회. FR-API-0301 예시와 프로파일 디렉터리 이름을 실제 버전 값으로 변경. NSX-T 3.2.4.3의 OVF 서명 인증서 만료(2026-01-03, 알려진 문제 3636919)를 D5 실환경 구축 시 확인 사항으로 기록. 요구사항 수 변동 없음 | PRD 개요 §1.2, PRD-1 FR-API-0301·FR-API-0312·프로파일 디렉터리 예시, PRD-4 §19 D1, 참고 문서 1.1 §18.3·1.2 §1·§6·§7, README | A |
| 54 | 프로젝트 범위 D13 확정: CMP는 기능 범위 선정용 참고 자료로만 사용. CMP 소스 분석·실제 CMP 등록/연동 시험·상품 제약 재현은 제외하며, 시뮬레이터 구성과 자체 테스트까지 수행 | README, PRD 개요, PRD-1~4, 참고 문서 1.1·1.2, D2·D13 | A, B |
| 55 | 자체 E2E 경계(테스트 클라이언트 → Gateway → 엔진 → 상태 저장·조회), 선정 공개 SDK 시험 및 독립 계약 근거를 명시. 현재 검증 상한을 Integration Tested로 통일하고 AC-01~19의 적용 범위로 릴리스 판정. AC-20은 향후 외부 검증용으로 보존·현재 제외 | FR-API-0302·0307·0308, TR-COV-0201·0202·0304·0403·0409, AC-02·03·17·19·20, 참고 문서 1.2 F1~F6·C-1 | A |
| 56 | D5를 현재 외부 검증 보류로 결정. Record·실환경 비교 TR-COV-0401·0402·0404~0408과 NFR-0312는 ID 보존·보류·적용 제외. WP 0.15는 자체 검증 근거 계획으로 변경, WP 5.4·PoC-18은 현재 제외. 자체 API Call Trace와 로그 마스킹은 유지. TR/NFR MVP 집계 갱신, 총 438건 유지 | PRD-2 §12.4·집계, PRD-3 NFR-0312·집계, PRD-4 §15~19, D5·D11·D13 | A, B |
| 57 | 미관찰 API를 미사용으로 자동 제외하는 판정을 제거. CMP 150개 기능은 참고 분류로 유지하고 구현 분모와 분리. Guest Customization·Restart·Clone/Content Library·동적 NSX 관계 등 MVP 후보의 선행 FR·WP·시험을 명시하고, 채택 시 함께 변경하도록 D7·D9에 연결 | FR-NSX-0233, FR-API-0404, TR-COV-0102·0307, PRD-4 §15.3·Task 3·D2·D7, 참고 문서 1.1·1.2 | A |
| 58 | CMP 상품 한도·콘솔 상태를 참고·범위 외로 표시하고 공식 VMware API 제약과 분리. Content Library 파일 추가 경로 정정, Firewall 표의 파이프 이스케이프, README 첫 기준 버전·빌드 누락 및 D 참조 범위 정정. FR-BE-0404의 기본 Adapter와 후속 Snapshot/Restore 시점 구분 | 참고 문서 1.1 §0·§17, README, FR-BE-0404, PRD-4 §17·§19 | A |
| 59 | 이번 교차검증의 수정 결과와 미결 사항을 본 문서 §7에 기록. 요구사항 ID·집계·참조·Markdown 표·README 링크 및 v1.0 보존을 확인하고, 문서 검증과 미실행 제품 테스트를 구분 | 본 문서 §6 | A, B |
| 60 | 교차검증 후속 수정. ① 버전 예시를 첫 기준 프로파일(vCenter 7.0.3)에 맞춤: FR-VC-0606 수용 기준을 '7.0 미만 협상 세션'으로, AC-16·PoC-13 예시를 7.0.3 프로파일과 `urn:vim25/6.7.3` 세션으로 변경. ② WP 미배정 요구사항 18건 배정: FR-VC-0105(WP 0.13), NFR-ARC-0113(WP 2.9), FR-NSX-0226~0228·0605·0609(WP 3.3), FR-BE-0209·NFR-0306(WP 3.5), FR-BE-0703(WP 3.7), FR-NSX-0206~0208·0223~0225(WP 4.9), TR-COV-0306(WP 5.3), NFR-ARC-0203(WP 5.7). MVP로 채택된 ○ 항목은 Phase 2a에 선행 배정. ③ 적용 제외로 결정된 보류 4건(FR-NSX-0230, FR-BE-0305, FR-BE-0405, NFR-ARC-0302)의 MVP를 ○ → –로 정정하고 집계 갱신(FR ○ 21→18·– 92→95, NFR ○ 4→3·– 13→14). ④ 실환경 확인을 전제한 FR-NSX-0104·0225·FR-VC-0606 문구를 공식 문서·스펙 근거 확인과 미검증 기록으로 변경(D5·D13). ⑤ 본 문서의 중복 절 번호(§6)를 §7로, §5의 참고 문서 1.2 설명을 정정. ⑥ 용어 정리: VM 재시작은 'VM 재시작(Reset·Guest Reboot)'으로, FR-VC-0407은 하드 리셋(`ResetVM_Task`)으로 명시하고 Guest Reboot는 채택 시 FR 신설. 시뮬레이터 프로세스 재시작은 '재기동'으로 통일. ⑦ AC-18 완료 기준에 인증서 지문 유지를 명시, 참고 문서 1.1에 이용 조건과 1.2 참고 계층(A·B·C)과의 대응 추가, README 권장 OS를 최소와 동일하게 명시. 기준 제품의 지원 종료는 운영 사이트가 있어 고려하지 않는다. 요구사항 수 변동 없음(438건) | PRD-1 FR-VC-0407·0606·FR-NSX-0104·0225·0230·FR-BE-0208·0305·0405·집계, PRD-3 NFR-0205·NFR-ARC-0302·집계, PRD-4 WP 0.13·2.9·3.3·3.5·3.7·4.9·5.2·5.3·5.7·§15.3·PoC-13·AC-16·AC-18·Task 6·D3·D7, PRD-B §B6, 참고 문서 1.1·1.2, README, 본 문서 | A, B |
| 61 | 개발 진행 중 요청·결정 이력 규칙 추가: PRD-4 상단에 '작업 규칙' 절을 신설하고, 저장소 루트 `.decisions/`에 `DEVELOPMENT-revision {PRD 버전}.md`(예: `DEVELOPMENT-revision v1.1.md`)로 일시·요청 프롬프트·반영 내용 요약(2~3줄)을 표로 기록. 이 파일명은 §5 파일명 규칙의 예외. 요구사항 수 변동 없음 | PRD-4 작업 규칙·목차, PRD-3 §14.2 디렉터리 구성 초안, README, `.decisions/` | — |
| 62 | Phase 0a 결과에 따른 결정 반영. ① D1 NSX 계약 근거: Broadcom이 3.2 계열 스펙을 3.2.0·3.2.1만 공개하므로 NSX-T 3.2.1(3.2.1.0.0) 공식 OpenAPI를 3.2.4.3 프로파일의 계약 근거로 사용하고, 두 빌드 간 차이는 미검증으로 표시해 3.2.2 ~ 3.2.4.3 릴리스 노트로 보완(FR-API-0309). ② D4 리소스 예산을 제안값대로 확정(유휴 ≤ 300 MB, 기동 ≤ 5초, 유휴 CPU 측정 오차 수준, 설치 ≤ 200 MB; Phase 0a 실측 유휴 71 MB·기동 약 1.2초·설치 약 34 MB). 측정 경계는 `20-design/development-plan/RESOURCE_BUDGET.md`에 두고, §13.5 보존 한도는 Phase 0b에서 확정. 요구사항 수 변동 없음 | PRD 개요 §1.2, PRD-1 FR-API-0312·프로파일 디렉터리 예시, PRD-3 NFR-0107·0109·0110·0112, PRD-4 §19 D1·D4, PRD-B §B9, README | A, B |
| 63 | D2 자체 테스트 SDK를 govmomi v0.56.0으로 결정(기능·API 범위는 Phase 0b 잠정, Phase 2b 확정). govmomi는 vcsim과 같은 계열이므로 SOAP 응답의 스키마 대조 검사를 병행하고, NSX는 Go 표준 HTTP 클라이언트로 OpenAPI 계약 테스트를 한다. Phase 기간 상한은 최소로 결정(Phase 0a·0b 각 1일). 요구사항 수 변동 없음 | PRD-4 §15.2 Phase 0 기간 상한, §19 D2, README | A |
| 64 | Phase 0b 종료 결정 반영. D4 측정 경계·CI 회귀 기준·§13.5 보존 한도를 `RESOURCE_BUDGET.md` 초안대로 확정. D6 같은 PC 네이티브만 MVP, D7 기본 CreateVM_Task만 MVP, D8 VDS 7.0 DVPG(backingType=nsx), D9 Segment·Realization·Tag·조회 중심(H-API·Fabric·Search는 MVP 이후), D10 SOAP Login·`/api/session`·NSX Basic/세션, D11 pyVmomi 추출 메타데이터는 저장소 포함·NSX/vSphere Automation 스펙은 사용자 측 다운로드, D12 단일 vCenter·NSX로 결정(권고안 채택). 채택된 ○는 NFR-ARC-0309 토폴로지 ①뿐이며 나머지 ○ 20건은 MVP 이후. 요구사항 수 변동 없음 | PRD-4 §19 D4·D6~D12, README | A, B |
| 65 | D2 최종 확정(Phase 2b, 2026-10-10): 승인 시뮬레이션 실행 범위를 28개 API(VC-SOAP-001~013, VC-REST-001~004, NSX-001A, NSX-002~011)로 확정하고, 계약 근거(API Guide 본문) 미확보인 NSX-001B(`/api/session/create`·XSRF)·NSX-012(스펙 제공 경로)는 근거 확보 전까지 실행 범위 밖, P2(VC-REST-005·006)는 제외. MVP ○ 20건은 D6~D10에 따라 MVP 제외로 기록. 자체 테스트 SDK는 선정 govmomi v0.56.0(계약 시험·스키마 대조)과 범용 govc·pyVmomi 9.1.1.0. 요구사항 수 변동 없음 | PRD-4 §19 D2 | A |
| 66 | D2 실행 범위 확대(2026-10-10, 사용자 결정): Phase 3에서 구현·시험한 API 13건(NSX Fabric VM·VIF·Discovered Node·Compute Manager 상태·Segment Port·realized-entities·실현 VM·Group VM 멤버·Alarm, vCenter QueryPerf·QueryEvents·Host 연결 끊기/다시 연결·NSX DVPG 노출)을 실행 범위에 추가해 41개 API로 한다. 요구사항 수 변동 없음 | PRD-4 §19 D2 | A |
| 67 | D2 실행 범위 확대(2026-10-11, 사용자 결정): Phase 4b에서 구현·시험한 API 20건(NSX 보안 Group·Service·Security Policy·Rule, 라우팅 Tier-0/1·Interface, Hierarchical API, Policy·Manager Search, Manager Transport Node·Logical Switch/Port·Edge Cluster — NSX-022~031, vCenter Clone·Template·Register·Reset/Suspend·Rename/Move/Relocate·Snapshot·Guest Customization — VC-SOAP-017~023, Lookup Service VC-LOOKUP-001, PBM VC-PBM-001, Content Library `/api` VC-REST-007)을 실행 범위에 추가해 기준 조합 61개 API로 한다. 추가 버전 프로파일 vCenter 7.0.2는 SOAP 계열 9건을 더해 26개 API. Phase 4b 미채택 제안 4건·7.0.2 조합 상호운용성·D1 재검토는 결정 대기. 요구사항 수 변동 없음 | PRD-4 §19 D2 | A |


---

## 4. v1.1에서 제거·대체된 구성 요소

| v1.0 구성 요소 | v1.1 처리 | 사유 |
|---|---|---|
| HTTPS Gateway (Caddy 또는 Nginx, 별도 서비스) | 실행파일 내장 Gateway (TLS + SNI 라우팅) | 프로세스 수 감소, 자체 호출 기록·전송 계층 Fault 통합 (Record는 D5·D13에 따라 보류) |
| vCenter Bridge (Go, 별도 서비스, 포트 8200) | vcsim 동일 프로세스 내 vCenter Behavior Adapter | 요청 경로 안에서 사전조건·Fault 처리, 프로세스 간 동기화 제거 |
| Simulation Controller (Python, 포트 8100) | Admin API (127.0.0.1:9000)로 통합 | 관리 API 명칭·포트 일원화 |
| Behavioral Engine (Python) | Go 내부 모듈 (`internal/engine` 등) | 요청 거부 불가 문제 해소, 단일 프로세스 일관성 |
| NSX Simulator (Python FastAPI, 필수) | Go 내장 (D3: 권고안 A). Python 단일 프로세스안(권고안 B)은 PRD-B로 분리 | 경량 실행, 단일 프로세스 일관성 |
| Docker Compose (필수 실행 방식) | 선택 실행 방식 (CI·선택) | Windows Docker Desktop 오버헤드 |
| NSX 전체 Pydantic 모델 생성 | 스펙 기반 지연 검증 | 기동 시간·메모리, 버전별 재생성 비용 |

---

## 5. 문서 관리 규칙

```
cloudinfra-sim/
├── README.md                          # 프로젝트 요약, 최신 버전 안내
└── 00-planning/
    ├── PRD-revision.md                  # 전체 버전 개정 이력 (본 문서, 단일 파일)
    ├── v1.0/
    │   └── VMWareNSX-Simulator-PRD.md                     # v1.0 (이력, 이전 파일명 유지)
    └── v1.1/
        ├── cloudinfra-sim-PRD.md                     # 개요 (1~2장, 세부 문서 목록)
        ├── cloudinfra-sim-PRD-1-기능별-요구사항.md     # 3~9장
        ├── cloudinfra-sim-PRD-2-테스트-요구사항.md     # 10~12장
        ├── cloudinfra-sim-PRD-3-비기능-요구사항.md     # 13~14장
        ├── cloudinfra-sim-PRD-4-개발계획-검토.md       # 15~19장
        ├── cloudinfra-sim-PRD-B-권고안B-대안-구성.md   # 권고안 B 대안 (참조용)
        ├── cloudinfra-sim-PRD 1.1 CMP 요구기능 목록.md # 참고: 기준 CMP 기능·API 매핑
        └── cloudinfra-sim-PRD 1.2 호환성 목표범위.md   # 참고: 시뮬레이션 범위·자체 검증 기준
```

개정 이력은 버전별로 나누지 않고 `00-planning/` 폴더의 본 문서 하나에 누적 관리한다.

새 버전을 만들 때는 다음 순서를 따른다.

1. `00-planning/vX.Y/` 폴더를 만들고, 직전 버전의 PRD 개요(`cloudinfra-sim-PRD.md`)와 세부 요구사항 문서 4종, PRD-B를 같은 이름으로 복사한다. 파일명에는 공백과 마침표(확장자 제외)를 쓰지 않는다. 요구사항 ID는 버전이 바뀌어도 유지하며, 변경된 요구사항은 같은 ID의 내용을 고치고 상세 변경 내역에 ID를 기록한다.
2. 본 문서 상단의 '최신 버전' 표기를 갱신하고, §1 버전 이력 표의 맨 위에 새 버전 행을 추가한다.
3. 새 버전의 개정 배경·상세 변경 내역을 새 절로 추가한다. 최신 버전의 절을 앞에 둔다.
4. 이전 버전 폴더의 문서는 수정하지 않는다.
5. 루트 `README.md`의 최신 버전 안내를 갱신한다.

---

## 6. 참고 사항

- v1.1에 기술한 vcsim·NSX의 구체적 동작(확장 지점, 버전 필터링 여부, 시계 처리, NSX 스펙 제공 경로 등)은 공식 저장소·문서로 확인하기 전까지 추정으로 취급하며, Phase 0 PoC에서 검증한다.
- '예상 리소스 요구사항'과 PRD-3 §13.1(NFR-0107 ~ NFR-0113)의 리소스 예산 수치는 설계 단계 추정·제안값이며, Phase 0 측정 결과에 따라 D4에서 확정한다.

---

## 7. 교차검증 결과 — 2026-10-10 범위 결정 반영

**검토 범위:** README와 현재 v1.1 문서(개요·PRD-1~4·PRD-B·참고 문서 1.1·1.2), 본 개정 이력. 기준은 사용자 결정인 'CMP는 범위 선정 참고 자료, 프로젝트는 시뮬레이션 구성 및 자체 테스트까지'이다(D13). 아래 '반영'은 문서 수정 완료를 뜻하며, 제품 구현·시험 통과를 뜻하지 않는다.

| 확인한 문제 | 반영 내용 | 결과 |
|---|---|---|
| CMP 소스·실제 호출 분석이 API 범위 확정의 필수 절차로 남아 있음 | PRD-4 Task 3·D2를 공식 계약·공개 SDK·CMP 참고 목록과 자체 시험으로 변경 | 반영 |
| 미관찰 API를 미사용으로 제외해 지원 범위가 축소될 수 있음 | 미관찰/미검증 유지, 명시적 제외 결정과 의존 API 검토 의무 | 반영 |
| F1의 실환경 Trace 의무와 Trace 없는 검증 허용 기준 충돌 | F1~F5를 자체 시험 기준으로 통일. 외부 검증은 보류, 현재 상한 Integration Tested | 반영 |
| MVP 후보의 Guest Customization·Restart 등 선행 구현이 후속 단계에 있음 | 참고 후보는 자동 MVP 편입하지 않음. 채택 시 FR의 MVP 표시·WP·시험을 Phase 2b 이전으로 함께 조정 | 반영, 개별 기능 선정은 D2·D7·D9 미결 |
| C-1의 Phase 2b 완료가 Phase 5 전제인 AC-20과 충돌 | C-1은 MVP 자체 검증, 전체 릴리스는 적용 AC-01~19, AC-20은 현재 제외로 분리 | 반영 |
| CMP 상품 제약을 VMware API 사전조건처럼 취급할 위험 | 상품 한도·과금·승인·콘솔 상태는 참고·범위 외. 공식 API 계약 제약은 독립 근거로만 채택 | 반영 |
| Content Library 파일 추가 경로 오류 및 Firewall 표 열 깨짐 | 공식 경로로 정정, 파이프 이스케이프 적용 | 반영 |
| README 첫 조합 누락·오래된 D 범위·집계 불일치 가능성 | 버전/빌드 명시, D1~D13 참조, 보류 요구사항과 MVP 집계 동기화 | 반영 |

**문서 정합성 검사:** 요구사항 ID는 FR 278 + TR 82 + NFR 78 = 438건으로 유지한다. 현재 보류는 총 12건(D1 1건, D3 3건, D5·D13 8건)이며, 보류 상태는 완료를 의미하지 않는다. MVP 분포는 FR 165/21/92, TR 48/0/34, NFR 61/4/13(●/○/–)이었으며, #60에서 보류 항목의 MVP 표시를 '–'로 정정해 FR 165/18/95, TR 48/0/34, NFR 61/3/14가 되었다. 참고 CMP 기능 정의·API 매핑은 각각 150개 ID를 유지한다. 요구사항 ID 집합 보존·집계, 계획 식별자 143개와 참조, Markdown 표 열, README 문서 링크 9개를 검사했으며 누락·중복·불일치가 없었다. v1.0 이력의 SHA-256이 수정 전과 같아 원본 보존도 확인했다.

**남은 계획 결정:** D2(세부 시뮬레이션 범위·SDK), D4(예산), D6~D12의 미결 선택은 Phase 0에서 정한다. 참고 API 매핑은 실제 CMP 소스·호출 확인 결과가 아니며, 채택할 때 지정 버전의 공식 계약과 대조한다. 이번 작업은 문서 교차검증·수정으로 한정했으며 시뮬레이터 구현 및 자체 테스트는 실행하지 않았다.
