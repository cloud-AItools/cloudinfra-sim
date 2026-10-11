# CloudInfra Simulator
## PRD-4. 개발계획 검토

**부제:** CMP-Oriented VMware SDDC API Simulator  
**저장소:** `cloudinfra-sim`  
**문서 버전:** 1.1  
**상위 문서:** `cloudinfra-sim-PRD.md`  
**범위:** PRD 15~19장 (개발 단계와 작업 패키지, 핵심 기술 위험, 최종 완료 기준, AI 에이전트 개발계획 수립 지시, 착수 전 결정 사항)

---

## 작업 규칙

개발 진행 중(Phase 0a 이후 모든 단계)에는 다음 규칙을 따른다. AI 에이전트와 개발자 모두에게 적용한다.

1. **결정 이력 폴더:** 개발 중 발생한 요청과 결정 사항의 이력은 저장소 루트의 `.decisions/` 폴더에 남긴다.
2. **문서명:** `DEVELOPMENT-revision {버전}.md`. `{버전}`은 작업 기준 PRD의 버전을 따른다(예: `DEVELOPMENT-revision v1.1.md`). PRD 버전이 바뀌면 새 문서를 만들고 이전 버전 문서는 수정하지 않는다. 이 파일명은 지정된 형식이므로 개정 이력 §5의 파일명 규칙(공백·마침표 금지)의 예외로 둔다.
3. **기록 형식:** 아래 열을 가진 표에 요청 1건당 1행을 시간순으로 추가한다.

| 열 | 내용 |
|---|---|
| 일시 | 요청을 받은 일시 (`YYYY-MM-DD HH:MM`) |
| 요청 프롬프트 | 사용자가 요청한 프롬프트 원문. 표 안에서는 줄바꿈을 `<br>`로 바꾸고 파이프 문자는 `\|`로 이스케이프한다 |
| 반영 내용 요약 | 요청에 따라 결정·반영한 내용을 2줄 이상 최대 3줄로 요약한다. 변경한 문서·WP·요구사항 ID·D 결정을 함께 적는다 |

4. **다른 기록과의 관계:** PRD 요구사항이나 계획 기준선을 바꾸는 결정은 이 이력과 별도로 `00-planning/PRD-revision.md`에도 기록한다. 이 이력은 개발 중 요청·결정의 경위를 남기는 용도이며 19장 결정(D1~D13)과 `DEVELOPMENT_PLAN.md`를 대체하지 않는다.

---

## 목차

- 작업 규칙
- 문서 개요
- 15. 개발 우선순위 및 단계
  - 15.1 단계 로드맵
  - 15.2 단계별 계획
  - 15.3 MVP 마일스톤
  - 15.4 일정·공수 산정 원칙
  - 15.5 요구사항 검증 추적 기준선
- 16. 우선 분석해야 할 핵심 기술적 위험
- 17. 최종 완료 기준
- 18. AI 에이전트 개발계획 수립 지시
- 19. 개발 착수 전 결정 필요 사항

---

## 문서 개요

### 목적

이 문서는 PRD-1~3에 정의한 요구사항을 **어떤 순서로, 어떤 작업 단위로, 어떤 위험을 관리하며 구현할지**를 정한 개발계획 기준선이다. 요구사항 자체를 정의하지 않으며, 각 항목은 PRD-1~3의 요구사항 ID를 참조한다.

### 사용 방법

1. AI 에이전트(또는 개발자)는 18장의 지시에 따라 이 기준선을 분석·상세화하여 `DEVELOPMENT_PLAN.md` 등 계획 산출물을 작성한다.
2. Phase 0(0a·0b)에서 19장의 결정 사항을 확정하고, 그 결과를 15장 작업 패키지와 16장 위험 대응에 반영한다.
3. 각 단계는 종료 게이트와 해당 단계의 요구사항 검증 추적 조건(§15.5)을 통과해야 다음 단계로 진행한다. 현재 릴리스는 적용 대상 AC-01 ~ AC-19를 자체 테스트로 판정한다. AC-20은 향후 외부 검증용으로 보존하며 이번 범위에 적용하지 않는다(D13).

**현재 범위(D13):** CMP 문서는 기능 후보 선정용 참고 자료로만 사용한다. 구현·완료 범위는 시뮬레이터 구성과 프로젝트 자체 테스트까지이다. CMP 소스 분석, 실제 CMP 등록·연동 시험, CMP 상품 제약 재현은 제외한다. 실환경 VMware 비교·Record 모드는 D5에 따라 후속 범위로 보류한다. E2E는 자체 테스트 클라이언트 → Gateway → 엔진 → 상태 저장·조회까지를 뜻한다.

### 식별자

| 식별자 | 대상 | 예 |
|---|---|---|
| `WP n.m` | 작업 패키지 (Phase n의 m번째, 세부 단계와 무관하게 번호 유지) | WP 1.2 = Phase 1의 버전 프로파일 골격 |
| `R-nn` | 핵심 기술 위험 | R-11 = 버전 인식 직렬화 |
| `AC-nn` | 최종 완료 기준 | AC-15 = 버전 지정 기동 |
| `PoC-nn` | Phase 0 기술 검증 항목 | PoC-13 = 요청 버전별 응답 필터링 |
| `Dn` | 착수 전 결정 항목 | D3 = 프로세스 구성안 |
| Phase 0a 등 | 세부 단계 (0a·0b, 2a·2b, 4a·4b) | Phase 2b = MVP 검증·호환성 판정 |

`FR-*`, `TR-*`, `NFR-*`는 PRD-1~3의 요구사항 ID이다. 작업 패키지 표의 '대상 모듈'은 저장소 구조(PRD-3 §14.2)를 따르며, `cmd/`, `internal/`, `data/`로 시작하는 경로는 `30-implementation/` 아래 경로이다. 상세 개발계획(`DEVELOPMENT_PLAN.md`)의 Work Package는 이 식별자와 요구사항 ID를 함께 기록한다.

---

# 15. 개발 우선순위 및 단계

## 15.1 단계 로드맵

```mermaid
flowchart LR
    P0a["Phase 0a<br/>핵심 기술 검증"]
    P0b["Phase 0b<br/>설계·계획 기준선"]
    P1["Phase 1<br/>기본 API Simulator"]
    P2a["Phase 2a<br/>Stateful 생명주기"]
    P2b["Phase 2b<br/>MVP 검증·<br/>호환성 판정"]
    MVP{{"MVP"}}
    P3["Phase 3<br/>Cross-System<br/>Behavior"]
    P4a["Phase 4a<br/>장애·세션·권한<br/>재현"]
    P4b["Phase 4b<br/>기능 확장·<br/>추가 버전"]
    P5["Phase 5<br/>자동화 및<br/>품질 강화"]
    REL(["릴리스 판정<br/>17장 완료 기준"])

    P0a -->|"우선 PoC 판정<br/>D1·D3·D4 확정"| P0b
    P0b -->|"D 결정 확정<br/>계획 산출물 12종"| P1
    P1 -->|"Scenario 01<br/>리소스 예산 충족"| P2a
    P2a -->|"Scenario 02~06<br/>(development 모드)"| P2b
    P2b -->|"승인 시뮬레이션 범위 확정<br/>MVP 추적표 검증"| MVP
    MVP --> P3
    P3 -->|"Scenario 08"| P4a
    P4a -->|"Scenario 09·10"| P4b
    P4b -->|"Scenario 07<br/>버전 간 비교"| P5
    P5 -->|"적용 대상 AC-01~19"| REL
```

화살표의 문구는 주요 종료 게이트이다. 모든 단계는 §15.5의 추적 조건도 충족해야 한다. 시나리오 번호는 PRD-2 11장의 E2E 시나리오를 가리키며, Scenario 01은 자체 클라이언트의 vCenter·NSX 인증·버전·기본 조회를 포함한다. 세부 단계(0a/0b, 2a/2b, 4a/4b)는 앞 세부 단계의 종료 게이트를 통과해야 다음으로 진행한다.

| 단계 | 목표 | 종료 게이트 | 핵심 목표 기여 |
|---|---|---|---|
| Phase 0a | 핵심 목표를 좌우하는 기술 제약을 조기에 확인 | 우선 PoC(PoC-12, PoC-13, PoC-14, PoC-17, PoC-19, PoC-20, PoC-21) 판정, D1 첫 기준 조합·D3 유지 확인·D4 확정 | A, B |
| Phase 0b | 상세 설계와 계획 기준선 수립 | 나머지 적용 대상 PoC 판정, D 결정 확정(D2의 승인 시뮬레이션 기능 범위는 잠정), 18장 계획 산출물 작성 | A, B |
| Phase 1 | 지정 버전으로 vCenter·NSX에 연결해 기본 인벤토리 조회 | Scenario 01 통과, 기본 Fixture 리소스 예산 충족 | A, B |
| Phase 2a | VM·NSX 리소스의 상태 기반 생명주기 | Scenario 02~06 통과 (development 모드) | A |
| Phase 2b | MVP 검증과 승인된 시뮬레이션 범위의 호환성 판정 | 승인 시뮬레이션 기능 범위 확정, 선정 SDK 계약 테스트, MVP 범위 release 모드 기동, MVP 추적표 증거 (MVP) | A |
| Phase 3 | vCenter–NSX 교차 시스템 동작, 전송 계층 장애 재현 | Scenario 08 통과 | A |
| Phase 4a | 오류·장애·세션·권한 재현 | Scenario 09·10 통과 | A |
| Phase 4b | NSX 보안·라우팅 등 기능 확장과 추가 버전 | Scenario 07 통과, 버전 간 Differential Test | A |
| Phase 5 | 재현 가능한 자동화 테스트 플랫폼 | 적용 대상 AC-01~19 자체 검증 충족 | A, B |

## 15.2 단계별 계획

### Phase 0 — 기술 검증 및 설계

- **목표:** 실제 구현 가능한 아키텍처와 주요 기술적 제약을 확인하고, 착수 전 결정 사항(19장)을 확정한다.
- **진입 조건:** PRD v1.1(개요, PRD-1~4) 승인
- **수행 방식:** 18장 Task 1~7과 적용 대상 PoC-01~21을 수행한다. PoC-07은 D3 조건부, PoC-18은 D5·D13에 따라 현재 제외한다. 코드는 PoC 범위로 한정하고 본 구현을 시작하지 않는다.
- **세부 단계:** 핵심 목표 A·B를 좌우하는 검증을 Phase 0a에서 먼저 끝내고, 설계·계획 산출물은 Phase 0b에서 작성한다. 0a의 결과로 아키텍처가 바뀌면 0b 산출물을 다시 쓰지 않아도 되도록 하기 위함이다.
- **기간 상한:** 각 세부 단계 착수 시 기간 상한(timebox)을 정해 `DEVELOPMENT_PLAN.md`에 기록한다. 2026-10-10 결정: 기간 상한은 최소로 한다(Phase 0a는 1일로 종료, Phase 0b도 같은 원칙). 상한 안에 결론이 나지 않은 PoC는 '실패 시 대안'을 채택하거나 `RISK_REGISTER.md`에 미해결 위험으로 올리고, 다음 단계 진행 여부를 판정해 기록한다.
- **라이선스 보류:** D11을 확정하기 전에는 공식 스펙(WSDL, OpenAPI)·SDK 메타데이터를 저장소에 커밋하지 않고 로컬에서만 사용한다. 실환경 Trace는 이번 범위에서 수집하지 않는다.

#### Phase 0a — 핵심 기술 검증

| WP | 작업 | 산출물 | 관련 요구사항·결정 |
|---|---|---|---|
| WP 0.1 | govmomi/vcsim 구조 분석 | `OPENSOURCE_ASSESSMENT.md` | FR-VC-0101, FR-VC-0103 |
| WP 0.4 | vcsim 내부 상태 변경 Hook 검증 | `POC_PLAN.md` (PoC-06, PoC-14) | FR-BE-0407, FR-BE-0408 |
| WP 0.6 | NSX API 스키마 확보 (D11 전에는 로컬 보관) | 대상 버전 OpenAPI 스펙 (`30-implementation/data/compatibility-profiles/`) | FR-NSX-0103, FR-NSX-0104 |
| WP 0.8 | 목표 버전 결정 (첫 기준 조합 우선) | `DEVELOPMENT_PLAN.md` | D1, FR-API-0312 |
| WP 0.11 | 권고안 A 유지 확인 (PoC-17 결과가 PRD-B §B2 재검토 조건에 해당하는지 판정) | `ARCHITECTURE.md` | D3, NFR-ARC-0101, NFR-ARC-0301 |
| WP 0.12 | 버전 인식 직렬화 현황 확인·메타데이터 소스 검증 | `VERSION_PROFILE_SPEC.md` (PoC-13) | FR-VC-0604 ~ FR-VC-0607 |
| WP 0.13 | vcsim 확장 지점 검증 (변경 핸들러, 메서드 오버라이드, 시간 소스) 및 Fork 필요 여부 판단 | `POC_PLAN.md` (PoC-06, PoC-14, PoC-15) | FR-VC-0105, FR-VC-0106, FR-BE-0611 |
| WP 0.14 | 리소스 예산 확정·측정 방법 수립 | `RESOURCE_BUDGET.md` (PoC-17, PoC-19) | D4, NFR-0107 ~ NFR-0113 |
| WP 0.16 | 자체 테스트 SDK·언어·배포 토폴로지·시뮬레이션 인스턴스 수 선정 | `DEVELOPMENT_PLAN.md` (PoC-20) | D2, D6, D12, D13 |
| WP 0.18 | 교차 시스템 단일 락 경계와 vcsim 내부 락의 상호작용 검증 | `POC_PLAN.md` (PoC-21) | FR-BE-0304, FR-DM-0306, R-15 |

- **종료 게이트:** 우선 PoC(PoC-12, PoC-13, PoC-14, PoC-17, PoC-19, PoC-20, PoC-21)가 성공했거나 실패 시 대안이 채택되었고, 그 결과가 핵심 목표 A·B에 미치는 영향이 기록된다. D1의 첫 기준 조합, D3(권고안 A 유지), D4가 확정된다. PoC-20은 D2에서 확인한 SDK로 수행하며, 아직 정해지지 않았으면 후보 SDK로 수행하고 0b에서 다시 확인한다.
- **주요 위험:** R-02, R-11, R-12, R-13, R-15

#### Phase 0b — 설계·계획 기준선

| WP | 작업 | 산출물 | 관련 요구사항·결정 |
|---|---|---|---|
| WP 0.2 | vcsim SOAP 지원 Method 조사 | `API_COVERAGE_MATRIX.md` | FR-VC-0202 |
| WP 0.3 | vcsim REST/vAPI 지원 범위 조사 | `API_COVERAGE_MATRIX.md` | FR-VC-0203, FR-VC-0204 |
| WP 0.5 | vcsim Task/PropertyCollector 동작 검증 | `POC_PLAN.md` (PoC-04, PoC-05) | FR-VC-0501 ~ FR-VC-0511 |
| WP 0.7 | 공식 API 계약·공개 SDK와 CMP 참고 목록을 활용한 시뮬레이션 범위 후보·Coverage Matrix 기준선 | `API_COVERAGE_MATRIX.md` (선정 절차, 필수 열) | 18장 Task 3, FR-NSX-0233, TR-COV-0101, TR-COV-0102 |
| WP 0.9 | 통합 상태 소유권 설계 | `ARCHITECTURE.md`, `DATA_MODEL.md` | FR-BE-0301 ~ FR-BE-0304, FR-DM-0101 |
| WP 0.10 | 기술적 위험 식별·대응 수립 | `RISK_REGISTER.md` | R-01 ~ R-16 |
| WP 0.15 | 자체 테스트 근거·기대값·미검증 영역 기록 계획 (Record·실환경 비교는 보류) | `TEST_STRATEGY.md` | D5, D13, TR-COV-0403, TR-COV-0409 |
| WP 0.17 | 요구사항별 적용 여부·작업·검증 방법·완료 기준 연결 | `TEST_STRATEGY.md` 내 요구사항 검증 추적표, `DEVELOPMENT_PLAN.md` | TR-COV-0103, AC-19, D1 ~ D13 |
| WP 0.19 | 공식 스펙·공개 SDK 메타데이터의 라이선스·재배포 검토 | `OPENSOURCE_ASSESSMENT.md` (라이선스 절) | D11, FR-NSX-0103, FR-API-0307 |

- **종료 게이트:** 실제 구현 가능한 아키텍처와 주요 기술적 제약이 확인되고, D1~D13가 확정되며, 18장 Task 7의 계획 산출물이 작성된다. D2의 승인된 시뮬레이션 기능 범위는 잠정 기준선으로 기록하고 Phase 2b에서 확정한다. 추적표는 §15.5의 Phase 0 기준(MVP 범위와 Phase 1 대상은 개별 ID 행, 그 외는 요구사항 범위 단위)을 따른다. 이후 단계의 시험은 '계획'으로 표시하며 이미 검증한 것으로 간주하지 않는다.
- **주요 위험:** R-01, R-03 ~ R-06, R-10, R-14, R-16

### Phase 1 — 기본 API Simulator

- **목표:** 자체 테스트 클라이언트가 선정 SDK·지정 버전으로 vCenter·NSX에 연결해 기본 인벤토리를 조회한다.
- **진입 조건:** Phase 0b 종료 게이트 통과

| WP | 작업 | 대상 모듈 | 관련 요구사항 |
|---|---|---|---|
| WP 1.1 | 단일 실행파일 네이티브 실행 골격 및 내장 Gateway (Docker Compose는 선택) | `cmd/cloudinfra-sim`, `internal/gateway` | FR-API-0101, NFR-0101 ~ NFR-0105, NFR-0114, NFR-ARC-0102, NFR-ARC-0107, NFR-ARC-0108, NFR-ARC-0201, NFR-ARC-0202, NFR-ARC-0301, NFR-ARC-0303 ~ NFR-ARC-0306 |
| WP 1.2 | 프로파일 로더, development/release 기동 검증(빌드 종류별 기본값), 검증 증거 연결, 버전 협상 경로, 버전 인식 직렬화 골격 | `internal/profile` | FR-API-0301 ~ FR-API-0306, FR-VC-0207, FR-VC-0601 ~ FR-VC-0608, NFR-ARC-0105, NFR-ARC-0110, NFR-ARC-0112, TR-COV-0201, TR-COV-0202 |
| WP 1.3 | 기준 버전 조합 1개의 프로파일 데이터 (출처·수집 일자·근거 기록) | `data/compatibility-profiles` | FR-API-0305, FR-API-0307 ~ FR-API-0310 |
| WP 1.4 | vcsim 임베드 구성 (컨테이너 기반 VM 비활성화, 내장 보조 시뮬레이터 재사용 범위 설정) | `internal/vcenter` | FR-VC-0101, FR-VC-0102, FR-VC-0104, FR-VC-0107, FR-VC-0201, FR-VC-0212 |
| WP 1.5 | NSX 엔진 생성 (스펙 기반 범용 핸들러, 공통 필드) | `internal/nsx` | FR-NSX-0101, FR-NSX-0102, FR-NSX-0105 ~ FR-NSX-0107, FR-NSX-0209, FR-NSX-0210, FR-NSX-0301 ~ FR-NSX-0303 |
| WP 1.6 | 기본 인증과 세션 (vCenter SOAP/REST, NSX Basic/Session, 사용자·Credential 관리) | `internal/gateway`, `internal/vcenter`, `internal/nsx` | FR-API-0201 ~ FR-API-0203, FR-API-0205 ~ FR-API-0208, FR-API-0210, FR-API-0214 |
| WP 1.7 | 기본 API 조회 및 등록용 Compute Manager 관계 (Fixture의 vCenter 연결 정보·인증서 지문 포함) | `internal/vcenter`, `internal/nsx` | FR-VC-0205, FR-VC-0301 ~ FR-VC-0307, FR-NSX-0201 ~ FR-NSX-0205, FR-NSX-0601, NFR-ARC-0103 |
| WP 1.8 | 초기 Fixture 생성, Seed 적용 및 Environment Reset | `data/fixtures`, `internal/admin` | TR-SCN-0101 ~ TR-SCN-0103, TR-SCN-0201 ~ TR-SCN-0204, FR-BE-0210, NFR-0108 |
| WP 1.9 | API Logging (요청·작업·상태·오류 로그, 사람이 읽는 형식과 JSON) | `internal/gateway` | NFR-0401 ~ NFR-0403, NFR-0405, TR-SCN-0111 |
| WP 1.10 | 리소스 예산 측정을 CI에 추가 | `40-test/footprint` | NFR-0107, NFR-ARC-0111 |
| WP 1.11 | 설치·인증서·Admin 인증·마스킹·기본 로그 보존 한도 (MVP 설치 자동화는 Windows, macOS·Linux는 수동 절차 문서) | `50-deployment/install`, `internal/admin`, `internal/gateway`, `internal/store` | NFR-0301 ~ NFR-0305, NFR-0307 ~ NFR-0311, NFR-0313 ~ NFR-0315, NFR-0501 ~ NFR-0506, NFR-0508, NFR-ARC-0307 ~ NFR-ARC-0309 (조건부 항목은 D6에 따르며 NFR-0312는 D5·D13으로 제외) |
| WP 1.12 | 미지원 API 처리 (성공 위장 금지, 버전별 미지원 응답, Unsupported API Log·집계, deprecation 응답) | `internal/gateway`, `internal/vcenter`, `internal/nsx` | FR-API-0102, FR-API-0401 ~ FR-API-0404, FR-NSX-0231 |
| WP 1.13 | 검증 증거 생성 자동화 (CI가 테스트 결과를 프로파일·구현 식별자에 묶어 `verification.yaml`을 생성하고, 변경 시 영향받는 증거를 무효화) | CI, `internal/profile`, `data/compatibility-profiles` | FR-API-0302, TR-COV-0101, TR-COV-0102, TR-COV-0201, TR-COV-0202 |

- **종료 게이트:** Scenario 01(TR-E2E-0101 ~ TR-E2E-0104)의 vCenter·NSX 자체 연결·조회 시험 모두 통과, 기본 Fixture에서 리소스 예산(NFR-0109 ~ NFR-0112) 충족. Phase 1 실행 범위는 development 모드에서 통합 테스트 증거를 확보하고, WP 1.13으로 생성한 증거로 release 모드 기동과 잘못된 프로파일·검증 미달 거부를 확인한다. 최종 릴리스는 §17에서 별도 판정한다.
- **주요 위험:** R-09 (예산 초과 시 Phase 2a 진입 전 조치)

### Phase 2 — Stateful Resource Simulation

- **목표:** 기본 리소스 생명주기(VM, NSX Segment)를 상태 기반으로 재현하고, 승인된 시뮬레이션 기능 범위에서 MVP의 호환성을 판정한다.
- **세부 단계:** 생명주기 구현(2a)과 MVP 검증·호환성 판정(2b)을 나눈다. '승인 범위 자체 테스트 통과'의 MVP 판정은 Phase 2b에서 한다.

#### Phase 2a — Stateful 생명주기

- **진입 조건:** Phase 1 종료 게이트 통과

| WP | 작업 | 대상 모듈 | 관련 요구사항 |
|---|---|---|---|
| WP 2.11 | vCenter Behavior Adapter (요청 경로 내 동기 처리, 간접 변경 금지; WP 2.1, WP 2.5, WP 2.7의 선행 작업) | `internal/vcenter`, `internal/engine` | FR-BE-0101, FR-BE-0102, FR-BE-0401, FR-BE-0402, FR-BE-0404, FR-BE-0406 |
| WP 2.1 | vCenter VM CRUD 및 기본 전원 작업 (버전별 동작 규칙 포함) | `internal/vcenter` | FR-VC-0401, FR-VC-0404 ~ FR-VC-0406, FR-VC-0409, FR-VC-0418 |
| WP 2.2 | NSX Segment CRUD, PATCH 설정·헤더별 의미, Revision·페이지 조회·참조 검증, Tag | `internal/nsx` | FR-NSX-0108, FR-NSX-0211, FR-NSX-0219, FR-NSX-0401 ~ FR-NSX-0412, FR-NSX-0414 ~ FR-NSX-0416 |
| WP 2.3 | 상태 관리 | `internal/engine` | FR-BE-0201 ~ FR-BE-0207 |
| WP 2.4 | Resource ID Mapping (MVP의 ID 생성·유지·중복 방지) | `internal/engine`, `internal/store` | FR-DM-0201 ~ FR-DM-0206, FR-DM-0208, FR-DM-0210 |
| WP 2.5 | Constraint Validation (사전조건·용량) | `internal/vcenter`, `internal/engine` | FR-VC-0417, FR-BE-0701, FR-BE-0702 |
| WP 2.6 | SQLite 및 lite 저장소·무결성 절차 | `internal/store` | FR-DM-0301 ~ FR-DM-0303, FR-DM-0305, FR-DM-0307, NFR-0106, NFR-0201 ~ NFR-0204, NFR-ARC-0106 |
| WP 2.7 | 비동기 Task·PropertyCollector 및 실시간 스케줄러 | `internal/engine`, `internal/vcenter` | FR-BE-0501 ~ FR-BE-0503, FR-BE-0601, FR-BE-0610, FR-BE-0612, FR-VC-0501 ~ FR-VC-0511 |
| WP 2.8 | 기본 API 오류 처리 (인증 실패, 잘못된 입력, Not Found, 중복 생성, 버전 충돌, 미지원 기능, SOAP Fault; 정상 요청에 임의 오류 금지) | `internal/vcenter`, `internal/nsx` | FR-FLT-0101, FR-FLT-0104 ~ FR-FLT-0106, FR-FLT-0108, FR-FLT-0109, FR-FLT-0121, FR-FLT-0207 |
| WP 2.10 | 기본 NSX Realization (즉시·설정 지연 후 정상 완료, 삭제 후 조회, 상태 필드 매핑, 상태 조회 API) | `internal/nsx`, `internal/engine` | FR-NSX-0220, FR-NSX-0501 ~ FR-NSX-0503, FR-NSX-0508, FR-NSX-0509, FR-BE-0606 |

- **종료 게이트:** development 모드에서 Scenario 02~06과 기본 Negative 테스트(TR-COV-0303) 통과. Scenario 06은 즉시 완료와 설정 지연 후 완료를 모두 포함한다.
- **주요 위험:** R-04, R-06, R-15

#### Phase 2b — MVP 검증·호환성 판정

- **진입 조건:** Phase 2a 종료 게이트 통과

| WP | 작업 | 대상 모듈 | 관련 요구사항 |
|---|---|---|---|
| WP 2.9 | MVP 테스트 자동화 (Contract·Stateful·기본 Negative, 선정 SDK 및 범용 SDK, Scenario 01~06 회귀) | `40-test/contract`, `40-test/integration`, `40-test/simulator-e2e` | TR-COV-0301 ~ TR-COV-0305, TR-COV-0307, TR-SCN-0301, TR-SCN-0302, TR-COV-0103, NFR-ARC-0109, NFR-ARC-0113 |
| WP 2.12 | 승인된 시뮬레이션 기능 범위 확정과 MVP 호환성 판정 (D2 범위 확정, Coverage Matrix의 MVP 범위 상태 확정, 협상 버전 하향 세션의 버전 인식 직렬화 계약 테스트, MVP 범위 release 모드 기동, 검증 상한 표시) | `40-test/contract`, `data/compatibility-profiles`, `20-design/` | D2, FR-VC-0605 ~ FR-VC-0607, FR-API-0302, TR-COV-0304, TR-COV-0403, TR-COV-0409 |

- **종료 게이트:** 승인된 시뮬레이션 기능 범위가 확정되고, 그 범위의 Scenario 01~06 회귀 및 MVP Contract·Stateful·기본 Negative·SDK 테스트 통과(AC-17), 단일 프로파일의 협상 버전 하향 세션 계약 테스트 통과(AC-16의 MVP 범위), MVP 실행 범위의 release 모드 기동, 추적표의 적용 대상 MVP 요구사항(● 및 채택된 ○) 검증 증거 확보 → **MVP 마일스톤** (15.3). 현재 검증 상한은 `Integration Tested`이며 실제 CMP·VMware 호환 검증을 의미하지 않는다(TR-COV-0409, D13).
- **주요 위험:** R-01, R-10

### Phase 3 — Cross-System Behavior

- **목표:** vCenter와 NSX 사이의 관계·동기화·Realization을 재현하고, 비용이 낮은 전송 계층 장애 재현을 먼저 제공한다.
- **진입 조건:** MVP 마일스톤 달성

| WP | 작업 | 대상 모듈 | 관련 요구사항 |
|---|---|---|---|
| WP 3.1 | Phase 1의 등록 관계를 기반으로 Compute Manager·Fabric/Host 변경 동기화 확장 | `internal/relations` | FR-NSX-0601, FR-NSX-0602 |
| WP 3.2 | Network Mapping (Segment ↔ Network, 버전별 표현) | `internal/relations` | FR-NSX-0603, FR-NSX-0606, FR-NSX-0607 |
| WP 3.3 | VM vNIC 연계, NSX VM 인벤토리 파생 (Fabric VM·VIF·실현 VM 조회, NSX VM Tag 소유권, VM 관련 정책 참조) | `internal/relations`, `internal/nsx` | FR-NSX-0604, FR-NSX-0605, FR-NSX-0608, FR-NSX-0226 ~ FR-NSX-0228, FR-NSX-0609 (○ 항목은 D9에 따르며, MVP로 채택되면 필요한 범위를 Phase 2a에 선행 배정) |
| WP 3.4 | NSX Realization 확장 (변경 중 재수정·의존 리소스 누락 및 자연 발생 실패) | `internal/nsx` | FR-NSX-0504, FR-NSX-0505, FR-NSX-0507 (기본 동작은 WP 2.10 재사용) |
| WP 3.5 | Task/Event 통합 관리, 상태 변경 원인 추적 및 감사 로그 | `internal/engine`, `internal/admin` | FR-MET-0201 ~ FR-MET-0203, FR-BE-0209, FR-BE-0403, NFR-0306 |
| WP 3.6 | State Reconciliation 및 환경 간 Namespace | `internal/relations`, `internal/engine` | FR-DM-0401 ~ FR-DM-0404, FR-DM-0207 |
| WP 3.7 | 가상 메트릭·할당량 정합 (조회 시 결정론적 계산) | `internal/engine`, `internal/vcenter`, `internal/nsx` | FR-MET-0101 ~ FR-MET-0104, FR-BE-0703 (채택 시), FR-BE-0704, NFR-0507 |
| WP 3.8 | 전송 계층 Fault Injection 기본 (응답 지연, 지정 HTTP 상태, 일시적 503, N회 실패 후 복구; 공통 Fault Rule 모델과 Admin API 제어) | `internal/gateway`, `internal/fault`, `internal/admin` | FR-FLT-0117, FR-FLT-0120, FR-FLT-0123, FR-FLT-0125, FR-FLT-0205, FR-FLT-0206, TR-SCN-0109 (전송 계층 범위) |

- **종료 게이트:** Scenario 08(TR-E2E-0801 ~ TR-E2E-0804) 통과, 전송 계층 Fault Rule의 추가·제거·적용 시험 통과
- **주요 위험:** R-07

### Phase 4 — Fidelity 및 장애 재현

- **목표:** 자체 테스트 클라이언트의 오류·복구 흐름을 시험할 수 있도록 오류·장애·세션·권한을 재현하고(4a), MVP 이후 기능과 추가 버전 프로파일을 확보한다(4b).
- **세부 단계:** 장애 재현 기반(4a)을 먼저 완료한 뒤 기능 범위 확장(4b)을 진행한다.

#### Phase 4a — 장애·세션·권한 재현

- **진입 조건:** Phase 3 종료 게이트 통과
- **작업 순서:** WP 4.10(Virtual Clock)을 WP 4.1(Session Timeout)보다 먼저 수행한다(FR-BE-0607 공유). WP 4.3은 WP 3.8의 Fault Rule 모델을 확장하며, WP 4.5와 WP 4.8의 선행 작업이다.

| WP | 작업 | 대상 모듈 | 관련 요구사항 |
|---|---|---|---|
| WP 4.10 | Virtual Clock 가속·정지·진행 및 시간 제어 테스트 | `internal/engine`, `internal/admin` | FR-BE-0602 ~ FR-BE-0604, FR-BE-0607 ~ FR-BE-0609, FR-BE-0611, TR-SCN-0108 |
| WP 4.1 | Session Timeout | `internal/gateway`, `internal/engine` | FR-API-0204, FR-API-0209, FR-BE-0607 |
| WP 4.2 | RBAC | `internal/gateway` | FR-API-0211 ~ FR-API-0213 |
| WP 4.3 | Fault Injection 계층 확장 (vCenter 의미, NSX 의미, 세션 계층 및 테스트 코드용 Admin 제어; 전송 계층은 WP 3.8) | `internal/fault`, `internal/admin` | FR-FLT-0201 ~ FR-FLT-0206, TR-SCN-0109, TR-SCN-0303 |
| WP 4.4 | API Error Contract (전체 오류 유형) | `internal/vcenter`, `internal/nsx` | FR-FLT-0101 ~ FR-FLT-0125 |
| WP 4.5 | Task 지연 및 Realization Timeout·장애 제어 확장 (기본 Realization 지연은 WP 2.10) | `internal/engine`, `internal/nsx` | FR-BE-0605, FR-NSX-0506 |
| WP 4.8 | 권한·세션·Timeout·Task 실패 Negative 및 장애 E2E 확대 | `40-test/` | TR-COV-0308, TR-COV-0309, TR-SCN-0303 |

- **종료 게이트:** Scenario 09·10(TR-E2E-0901 ~ TR-E2E-0904, TR-E2E-1001 ~ TR-E2E-1003) 통과
- **주요 위험:** R-12

#### Phase 4b — 기능 확장·추가 버전

- **진입 조건:** Phase 4a 종료 게이트 통과

| WP | 작업 | 대상 모듈 | 관련 요구사항 |
|---|---|---|---|
| WP 4.6 | Event/PropertyCollector 개선 | `internal/vcenter` | FR-VC-0509, FR-MET-0202 |
| WP 4.7 | 추가 버전 프로파일 및 버전 간 Differential Test | `data/compatibility-profiles`, `40-test/contract` | FR-API-0311 |
| WP 4.9 | MVP 이후 NSX 라우팅·보안 리소스, Hierarchical API, Manager API 조회 및 Search 등 승인 범위 구현 | `internal/nsx` | FR-NSX-0206 ~ FR-NSX-0208, FR-NSX-0212 ~ FR-NSX-0218, FR-NSX-0221 ~ FR-NSX-0225, FR-NSX-0229, FR-NSX-0232, FR-NSX-0413 (채택 범위는 D9에 따르며, MVP로 채택된 ○ 항목은 Phase 2a에 선행 배정) |
| WP 4.11 | MVP 이후 vCenter 생명주기·보조 서비스의 채택 범위 구현 | `internal/vcenter` | FR-VC-0204, FR-VC-0206, FR-VC-0208 ~ FR-VC-0211, FR-VC-0402, FR-VC-0403, FR-VC-0407, FR-VC-0408, FR-VC-0410 ~ FR-VC-0416, NFR-ARC-0104 (MVP로 채택된 항목은 Phase 1~2에 선행 배정) |

- **종료 게이트:** Scenario 07(TR-E2E-0701 ~ TR-E2E-0704) 통과, 추가 버전 프로파일의 버전 간 Differential Test 통과(AC-16 확장)
- **주요 위험:** R-08

### Phase 5 — 자동화 및 품질 강화

- **목표:** 시뮬레이터를 재현 가능한 자동화 테스트 플랫폼으로 완성한다.
- **진입 조건:** Phase 4b 종료 게이트 통과

| WP | 작업 | 대상 모듈 | 관련 요구사항 |
|---|---|---|---|
| WP 5.1 | 기본·장애 시나리오를 관리 API Scenario Runner에 통합 | `internal/admin`, `data/scenarios` | TR-SCN-0106, TR-SCN-0107, TR-SCN-0110, TR-SCN-0301 ~ TR-SCN-0303, NFR-0404 |
| WP 5.2 | Snapshot/Restore·재기동 복구·참조 일관성 | `internal/store`, `internal/admin` | TR-SCN-0104, TR-SCN-0105, FR-DM-0209, FR-DM-0304, FR-DM-0306, FR-BE-0208, NFR-0205 ~ NFR-0208 |
| WP 5.3 | Regression Test (채택 시 Terraform·Ansible 자체 테스트 포함) | `40-test/`, CI | AC-11, TR-COV-0306 |
| WP 5.4 | **보류·현재 범위 제외(D5·D13):** 향후 Record 모드·비식별화·실환경 Trace 비교 | 후속 범위 재승인 시 배정 | TR-COV-0401, TR-COV-0402, TR-COV-0404 ~ TR-COV-0408, NFR-0312 |
| WP 5.5 | API Coverage Report | `internal/admin` | TR-SCN-0112, TR-COV-0101, TR-COV-0102 |
| WP 5.6 | Performance Benchmark | `40-test/performance` | NFR-0115 ~ NFR-0117 |
| WP 5.7 | CI 통합 및 빌드·배포 구조 조정 | CI | NFR-ARC-0111, NFR-ARC-0203 |
| WP 5.8 | 검증 범위·제약·추적 증거 문서화 | `20-design/` | TR-COV-0103, TR-COV-0403, TR-COV-0409, AC-14, AC-19 |
| WP 5.9 | full 저장소·로그·Trace 보존 한도 및 회전 검증 | `internal/store`, `40-test/performance` | NFR-0115, NFR-0501 ~ NFR-0506, NFR-0508 |
| WP 5.10 | macOS·Linux 설치 자동화 | `50-deployment/install` | NFR-ARC-0308 |

- **종료 게이트:** 적용 대상 AC-01 ~ AC-19 충족 → 자체 테스트 증거로 릴리스 판정 (AC-20 제외)
- **주요 위험:** R-09, R-10

## 15.3 MVP 마일스톤

MVP는 **Phase 2b 종료 게이트**로 정한다. MVP가 지원해야 할 범위(18장 Task 6)와 작업 패키지의 대응은 다음과 같다.

| MVP 범위 | 작업 패키지 |
|---|---|
| 자체 클라이언트의 vCenter·NSX 인증·연결 | WP 1.1, WP 1.6 |
| 가상 인벤토리 조회 | WP 1.7 |
| 미지원 API 식별·처리 | WP 1.12 |
| 기본 VM 생성·조회·변경·삭제 | WP 2.11, WP 2.1, WP 2.5 |
| 기본 NSX Segment 생성·조회·변경·삭제 | WP 2.2 |
| Task 및 상태 변경 | WP 2.3, WP 2.7 |
| NSX 기본 Realization 및 설정 지연 | WP 2.10 |
| 기본 API 오류 처리 | WP 2.8 |
| Seed 및 Reset | WP 1.8 |
| 테스트 자동화와 검증 증거 자동 생성 | WP 1.10, WP 1.13, WP 2.9 |
| 기준 버전 조합 1개의 버전 프로파일 및 버전 인식 직렬화 | WP 1.2, WP 1.3 |
| 승인된 시뮬레이션 기능 범위 확정과 호환성 판정 | WP 2.12 |
| Docker 없는 네이티브 실행 및 리소스 예산 충족 | WP 1.1, WP 1.10 |
| 설치(Windows 자동화)·Admin 인증·마스킹·인증서 지문 안정성·기본 보존 한도 | WP 1.11 |
| (D7·D9 결정에 따라) Clone·OVF/Content Library·Guest Customization·VM 재시작(Reset·Guest Reboot)·디스크 확장, NSX VM Tag·Hierarchical API | 결정 시 Phase 2a에 작업 패키지 추가 |

PRD-1~3의 MVP 열(●, ○)이 MVP의 요구사항 단위 범위이며, ○ 항목은 19장 결정 결과에 따라 포함 여부가 정해진다. CMP 참고 목록의 MVP 후보는 자동 채택하지 않는다. 채택된 ○와 후보 중 기존 후속 항목(예: Guest Customization FR-VC-0416, VM 재시작 중 Reset FR-VC-0407; Guest Reboot는 채택 시 FR 신설)의 MVP 표시·구현 WP·선행 기능·테스트를 함께 갱신하고 모두 Phase 2b 종료 이전에 배정한다. `호환성 목표범위` 문서 §3.1의 의존성 표를 따른다. 예를 들어 NSX VM Tag를 채택하면 WP 3.3의 VM 인벤토리 파생도 필요한 범위만큼 선행시킨다. 권고안 B 전용 항목은 D3에 따라 적용하지 않는다(PRD-B). 일부 E2E 통과만으로 미검증 MVP 요구사항을 완료 처리하지 않는다.

## 15.4 일정·공수 산정 원칙

- 대략적인 개발 규모는 산정하되, 실제 저장소 분석 전에는 근거 없는 일정이나 정확한 공수를 확정하지 않는다.
- 일정은 Phase 0b 종료 시점(PoC 결과와 D1~D13 확정 후)에 처음 수립하고, 각 단계 종료 게이트에서 갱신한다.
- 작업 패키지별 복잡도와 위험은 18장 Task 5의 형식으로 `DEVELOPMENT_PLAN.md`에 기록한다.
- 승인 판단을 돕기 위해 아래 단계별 상대 규모를 추정치로 둔다. 근거 있는 값이 아니며 Phase 0b 종료 시 일정과 함께 대체한다.

| 단계 | 상대 규모 (추정) | 규모를 좌우하는 요인 |
|---|---|---|
| Phase 0a | S~M (기간 상한 적용) | vcsim 확장 지점·직렬화 필터 PoC 결과 |
| Phase 0b | M | 계획 산출물 12종, Coverage Matrix 계약 근거·선정 SDK(D2, D11) |
| Phase 1 | M | 프로파일 로더·버전 협상, NSX 범용 핸들러, 설치·인증서 |
| Phase 2a | L | Behavior Adapter, PropertyCollector·Task, NSX PATCH·Revision 계약 |
| Phase 2b | M | 선정 SDK 계약 테스트, 승인 범위 확정 협의 |
| Phase 3 | M | 버전별 네트워크 표현(D8), 교차 시스템 일관성 |
| Phase 4a | L | Virtual Clock 시간 소스(R-12), 오류 계약 전체 |
| Phase 4b | L (변동 큼) | D9 NSX 기능 범위, 추가 버전 수(D1) |
| Phase 5 | M | 자체 회귀·복구·보존 한도·추적 증거 검증 |

## 15.5 요구사항 검증 추적 기준선

E2E 성공 기준에 직접 연결되지 않은 요구사항도 검증 대상이다. TR-COV-0103에 따라 `TEST_STRATEGY.md`에 아래 기준선을 **개별 요구사항 ID별 행**으로 전개하며, 전개 시점은 표 아래의 Phase 0·단계 진입 기준을 따른다. 이 표는 검증 계획이며 통과 증거가 아니다. 아직 실행하지 않은 시험·검토의 결과는 '미실행'으로 기록한다.

| 요구사항 범위 | 주 작업 패키지 | 검증 방법·판정 대상 | 검증 시점 | 완료 기준 |
|---|---|---|---|---|
| FR-VC 전체 | WP 0.1 ~ WP 0.5, WP 0.12, WP 0.13, WP 1.2, WP 1.4, WP 1.7, WP 2.1, WP 2.5, WP 2.7, WP 2.12, WP 4.6, WP 4.11 | PoC·Contract·Stateful·선정 SDK·Scenario 01~05; 버전 협상·직렬화, Task, PropertyCollector, 기본 전원 작업 포함 | Phase 0 조사, Phase 1~2b MVP, Phase 4b 후속 범위 | AC-02 ~ AC-04, AC-06, AC-07, AC-16, AC-17 |
| FR-NSX 전체 | WP 0.6, WP 1.5, WP 1.7, WP 1.12, WP 2.2, WP 2.10, WP 3.1 ~ WP 3.4, WP 4.5, WP 4.9 | Contract·Stateful·Scenario 01·06~08; PATCH 설정·헤더·배열·응답, Revision, Pagination, Realization 및 관계 검증 | Phase 1 인증·조회, Phase 2a 기본 기능, Phase 3 관계, Phase 4a·4b 확장 | AC-02 ~ AC-07, AC-09 |
| FR-BE 전체 | WP 0.9, WP 0.13, WP 0.18, WP 1.8, WP 2.3, WP 2.5 ~ WP 2.7, WP 2.10, WP 2.11, WP 3.5 ~ WP 3.7, WP 4.1, WP 4.5, WP 4.10, WP 5.2 | 설계 검토·Unit·동시성/Stateful·락 경계·시계 제어·Seed 재현·복구 시험; 권고안 A 상태 소유권 (B 전용 항목은 '적용 제외 — D3') | Phase 0 설계, Phase 2a·2b MVP, Phase 3~5 확장·복구 | AC-04 ~ AC-10, AC-19 |
| FR-DM 전체 | WP 0.9, WP 2.4, WP 2.6, WP 3.6, WP 5.2 | 모델 검토·ID 계약·동시성·참조 무결성·Snapshot/재기동 복구 시험 | Phase 0 설계, Phase 2a 기본 ID·저장소, Phase 3 관계, Phase 5 복구 | AC-06, AC-09, AC-10, AC-19 |
| FR-API 전체 | WP 0.8, WP 1.1 ~ WP 1.3, WP 1.6, WP 1.9, WP 1.12, WP 1.13, WP 2.8, WP 2.12, WP 4.1, WP 4.2, WP 4.7 | Contract·Negative·기동 모드/증거 미달 거부·지원 범위/버전·인증/권한·미지원 로그 검증 | Phase 1~2b MVP, Phase 4a·4b 확장 | AC-02, AC-03, AC-08, AC-13, AC-15 ~ AC-17 |
| FR-FLT 전체 | WP 2.8, WP 3.8, WP 4.3 ~ WP 4.5, WP 4.8 | MVP 기본 Negative, 전송 계층 Fault, 후속 장애 주입·재시도·복구 및 Scenario 09~10 | Phase 2a 기본 오류, Phase 3 전송 계층, Phase 4a 장애 | AC-05, AC-08, AC-13 |
| FR-MET 전체 | WP 3.5, WP 3.7, WP 4.6 | 메트릭 결정론·할당량 정합·Event/Alarm API Contract·이벤트 속성 시험 | Phase 3~4b | AC-03, AC-06, AC-07, AC-19 |
| TR-SCN 전체 | WP 1.8, WP 1.9, WP 2.9, WP 3.8, WP 4.3, WP 4.8, WP 4.10, WP 5.1, WP 5.2, WP 5.5 | Fixture·Seed/Reset 반복·기본 시나리오·Fault/Clock 제어·Runner·Snapshot·리포트 기능 검증 | Phase 1~2b 기본, Phase 3~4a 장애, Phase 5 관리 통합 | AC-10 ~ AC-12, AC-19 |
| TR-E2E 전체 | WP 1.6, WP 1.7, WP 2.9, WP 3.6, WP 4.8, WP 5.3 | Scenario 01~10의 각 성공 기준별 자체 테스트 결과·자체 API 실행 Trace 연결 | Phase 1: 01, Phase 2a·2b: 02~06, Phase 3: 08, Phase 4a: 09·10, Phase 4b: 07, Phase 5: 전체 회귀 | AC-02, AC-04, AC-05, AC-11, AC-17 |
| TR-COV 전체 | WP 0.7, WP 0.15, WP 0.17, WP 1.2, WP 1.13, WP 2.9, WP 2.12, WP 4.8, WP 5.3, WP 5.5, WP 5.8 | Coverage·추적표 정합성, 자체 테스트 실행, API별 증거·검증 상태·미검증 표시 검사. TR-COV-0401·0402·0404~0408은 적용 제외 — D5·D13 | Phase 0 기준선, 단계별 갱신, Phase 5 최종 판정 | AC-03, AC-11 ~ AC-17, AC-19 |
| NFR-01xx | WP 0.11, WP 0.14, WP 1.1, WP 1.8, WP 1.10, WP 2.6, WP 5.6, WP 5.9 | 네이티브 설치·실행, 기본 Fixture의 메모리·CPU·기동·설치 크기, full 규모 벤치마크 | Phase 0 예산 확정, Phase 1 이후 매 빌드, Phase 5 규모 | AC-01, AC-18 |
| NFR-02xx | WP 2.4, WP 2.6, WP 3.6, WP 5.2 | 동시성·중복·트랜잭션·ID·재기동 및 Snapshot 복구·재처리 시험 | Phase 2a 무결성, Phase 3 관계, Phase 5 복구 | AC-06, AC-09, AC-10, AC-19 |
| NFR-03xx | WP 0.19, WP 1.6, WP 1.9, WP 1.11, WP 3.5 | 바인딩·TLS·인증서 지문·Admin 인증·Credential·로그 마스킹·도메인/설치·접근 제한 검사. NFR-0312는 적용 제외 — D5·D13 | Phase 1 기본 보안, Phase 3 감사 | AC-08, AC-14, AC-18, AC-19 |
| NFR-04xx | WP 1.9, WP 2.7, WP 3.5, WP 3.8, WP 4.3, WP 5.1 | API/작업/상태 로그의 상관관계·형식·누락 및 테스트 실행 로그 검사 | Phase 1~2 기본, Phase 3~5 기능별 확장 | AC-07, AC-11 ~ AC-13, AC-19 |
| NFR-05xx | WP 0.14, WP 1.11, WP 3.7, WP 5.9 | 한도 경계·장시간 실행·회전·집계 키 상한·Trace 잘림/증거 삭제 처리·메트릭 미저장 검증 | Phase 0 설정 확정, Phase 1~2b lite, Phase 3 메트릭, Phase 5 full | AC-12, AC-14, AC-18, AC-19 |
| NFR-ARC 전체 | WP 0.11, WP 1.1, WP 1.2, WP 1.4, WP 1.7, WP 1.10, WP 1.11, WP 2.6, WP 2.9, WP 4.11, WP 5.7, WP 5.8, WP 5.10 | 아키텍처·디렉터리·의존성 검토, 권고안 A 실행·라우팅·토폴로지·설치·CI·SDK 시험 (B 전용 항목은 '적용 제외 — D3') | Phase 0 확인, Phase 1~2b MVP, Phase 5 배포·CI 회귀 | AC-01, AC-11, AC-17 ~ AC-19 |

`NFR-01xx` 등은 해당 절의 모든 기존 ID를 뜻하며, 새로운 요구사항 ID가 아니다. 개별 추적 행에는 요구사항 ID, 적용/제외 및 D 결정 근거, 주 WP, 목표 Phase, 테스트·검토 항목 ID, 검증 방법, 판정 기준, 연결 AC, 증거 위치, 실행 결과를 둔다. API 관련 행은 Coverage Matrix의 API 식별자에도 연결한다.

- **Phase 0 (0b 종료):** MVP 범위(● 및 채택된 ○)와 Phase 1 대상 ID는 개별 행으로 전개하고 시험 방법과 합격 조건을 정한다. 그 밖의 ID는 위 표의 요구사항 범위 단위로 주 WP·목표 단계를 배정해 두고, 개별 행은 해당 단계 진입 시 전개한다. 검토·선택 항목과 적용 제외 항목도 채택/제외 근거를 기록하고, 상세 WP가 없는 항목은 추가 배정한다.
- **각 단계 진입:** 그 단계에 배정된 ID를 개별 행으로 전개하고 시험 방법과 합격 조건을 정한다.
- **MVP:** 적용 대상 ●와 채택된 ○의 선행 작업·검증을 Phase 2b까지 완료한다. 표의 후속 단계 표시는 이를 뒤로 미루는 근거가 될 수 없다.
- **각 단계 종료:** 해당 단계까지 필요한 항목의 증거·결과를 확인한다. 자동 검사로 ID 누락·중복·미정의 참조, 작업/시험 미배정, 미검증 선행 요구사항을 찾는다.
- **릴리스:** 전체 적용 요구사항과 제외 근거를 AC-19로 확인한다. 비기능 요구사항은 E2E만으로 대체하지 않고 설치·보안·무결성·Footprint 등 해당 시험 증거를 제출한다.

---

# 16. 우선 분석해야 할 핵심 기술적 위험

다음 위험은 구현 전에 반드시 검토한다. '확인 시점'의 Phase 0a·0b, 2a·2b, 4a·4b는 §15.2의 세부 단계이다. '대응 방향'은 PRD-1~3에서 정한 설계 방침이며, 검증 결과에 따라 `RISK_REGISTER.md`에서 갱신한다. '영향 목표'는 PRD 개요 1장의 핵심 목표(A: 버전 호환, B: 경량 실행)이다.

| ID | 위험 | 영향 목표 | 대응 방향 | 검증 수단 | 확인 시점 |
|---|---|---|---|---|---|
| R-01 | vcsim의 실제 API 지원 범위 — vcsim의 SOAP·REST 기능과 승인 시뮬레이션 API 범위 간 Gap | A | Gap 분석 후 부족한 API는 확장 모듈로 보완하고 Coverage Matrix로 관리 (FR-VC-0103, FR-VC-0202) | WP 0.2, WP 0.3 | Phase 0b |
| R-02 | vcsim 내부 상태 변경과 행위 로직 연동 — 행위 로직을 요청 경로 안에 둘 수 있는가 | A, B | vcsim과 같은 프로세스의 Behavior Adapter. Hook이 부족하면 대안 비교 후 최소 Fork (FR-BE-0401, FR-BE-0407). 권고안 B 재검토 시 PRD-B §B7 | PoC-06, PoC-14 | Phase 0a |
| R-03 | PropertyCollector의 변경 이벤트 — 자체 클라이언트의 실시간 인벤토리 동기화 재현 | A | vcsim 구현 재사용, 공식 계약을 근거로 감시·갱신 패턴 자체 테스트 (FR-VC-0507 ~ FR-VC-0509) | PoC-05 | Phase 0b |
| R-04 | vCenter SOAP와 REST의 일관성 — 같은 VM을 다른 API로 조회할 때 같은 상태 | A | REST를 vcsim 인벤토리 뷰로 구현해 원본을 하나로 유지 (FR-VC-0205, FR-VC-0510) | PoC-03 | Phase 0b |
| R-05 | NSX API의 방대한 범위 | A, B | 스펙 기반 범용 핸들러 + 행위 리소스만 전용 로직 (FR-NSX-0105 ~ FR-NSX-0108) | PoC-17 | Phase 0a |
| R-06 | NSX Revision과 Realization — 낙관적 동시성 제어와 비동기 정책 상태 | A | `_revision` 비교와 계약별 충돌 응답, Virtual Clock 기반 Realization 상태 전이 (FR-NSX-0408, FR-NSX-0409, FR-NSX-0501) | PoC-09, PoC-10 | Phase 0b |
| R-07 | vCenter와 NSX 간 상태 동기화 — 교차 변경의 부분 실패와 재시도 | A | 단일 프로세스 락·트랜잭션으로 처리하고, 재기동 복구에만 최소한의 Outbox 사용 (FR-BE-0304, FR-DM-0401 ~ FR-DM-0404). 권고안 B 재검토 시 PRD-B §B6 | PoC-21, Scenario 08 | Phase 0a, Phase 3 |
| R-08 | 버전별 API 차이 — 한 버전의 동작을 다른 버전에 그대로 적용하는 오류 | A | 프로파일 데이터 출처 원칙, 빌드 간 무검증 복사 금지, 버전 간 Differential Test (FR-VC-0418, FR-API-0309) | WP 4.7 | Phase 4b |
| R-09 | 성능 및 동시성 — SQLite Lock, vcsim 인벤토리 처리, 비동기 작업 증가에 따른 병목 | B | 기본 예산 CI 측정, `full` 프로필 벤치마크 (NFR-0116, NFR-0201) | WP 1.10, WP 5.6 | Phase 1, Phase 5 |
| R-10 | 자체 테스트의 검증 한계 — 실제 CMP·VMware 결과로 오인할 위험 | A | 공식 계약·공개 SDK 기반 기대값을 사용하고 근거가 없는 동작은 가정·미검증으로 표시. 검증 상한 `Integration Tested`, 외부 비교는 현재 제외 (TR-COV-0403, TR-COV-0409) | WP 0.15, D5, D13 | Phase 0b 이후 |
| R-11 | 버전 인식 직렬화 — vcsim이 요청 버전별로 응답을 걸러내지 않을 가능성 | A | 메타데이터(WSDL, SDK 타입 정의) 정확성 검증, 직렬화 경로에 필터 삽입, 불가 시 최소 Fork (FR-VC-0604 ~ FR-VC-0607) | PoC-13 | Phase 0a |
| R-12 | vcsim의 시간 소스 — Virtual Clock을 Task·Event 시각, 세션 만료에 적용할 수 있는가 | A | 시간 소스 교체 확장 지점 확인, 없으면 최소 Fork (FR-BE-0610, FR-BE-0611) | PoC-15 | Phase 0a |
| R-13 | 로컬 실행 환경 — Windows 443 충돌, hosts·루트 CA 등록 권한, 루프백 별칭, 배포 토폴로지별 접근 경로 | B | 루프백 별칭 바인딩, 설치 자동화, 토폴로지 확정 (NFR-0301, NFR-0311, NFR-ARC-0307 ~ NFR-ARC-0309) | PoC-19, D6 | Phase 0a |
| R-14 | NSX 스펙 규모와 범용 핸들러의 정확도 — 스펙에 드러나지 않는 행위(서버 측 기본값, 암묵적 하위 리소스 생성) | A | 공식 문서·스펙으로 식별하고 근거가 없는 동작은 가정으로 구분해 프로파일 행위 규칙(`behavior-rules.yaml`)에 기록 (FR-NSX-0107, FR-NSX-0303) | PoC-17, WP 0.15 | Phase 0 ~ Phase 2b |
| R-15 | 교차 시스템 단일 락과 vcsim 내부 락의 상호작용 — vCenter·NSX 공통 락을 vcsim 객체 락과 겹칠 때의 교착 또는 전체 직렬화로 인한 처리량 저하 | A, B | 락 획득 순서와 범위를 정의하고, 교차 변경이 필요한 경로에만 공통 락을 적용. 불가 시 vcsim 변경 핸들러 기반 비동기 반영과 비교 (FR-BE-0304, FR-DM-0306) | PoC-21 | Phase 0a, Phase 2a |
| R-16 | 공식 스펙·공개 SDK 메타데이터의 라이선스·재배포 제약 — 프로파일 데이터를 저장소·배포물에 포함할 수 있는가 | A | 라이선스 조건 확인 후 저장소 포함, 사용자 측 다운로드 스크립트, 배포물 제외 중 선택 (FR-NSX-0103, FR-API-0307) | WP 0.19, D11 | Phase 0b |

---

# 17. 최종 완료 기준

현재 릴리스는 아래 AC-01 ~ AC-19의 승인 적용 범위를 자체 테스트로 충족해야 한다. D1~D13의 적용 범위·제외 근거를 추적표로 확인하며 검증 상한은 `Integration Tested`로 표시한다. 실제 CMP 연동·상품 제약 및 실환경 VMware 비교는 통과 조건이 아니며, 자체 검증만으로 '100% 호환'을 표시하지 않는다.

| ID | 완료 기준 | 검증 방법 | 확인 단계 |
|---|---|---|---|
| AC-01 | 실제 VMware 제품을 설치하지 않고 실행 가능하다. | 설치·실행 확인 (NFR-0101, NFR-0102) | Phase 1 |
| AC-02 | 자체 테스트 클라이언트가 시뮬레이터의 vCenter·NSX API에 인증하고 버전·인벤토리를 조회할 수 있다. | 자체 Scenario 01 | Phase 1 |
| AC-03 | 승인된 시뮬레이션 범위의 모든 VMware API 계약을 지원한다. | API Coverage Matrix (TR-COV-0101, TR-COV-0201) | Phase 5 |
| AC-04 | 모든 정상 생명주기 시나리오가 통과한다. | Scenario 02~06은 Phase 2a·2b, Scenario 08은 Phase 3, Scenario 07의 정상 변경 경로는 Phase 4b에서 확인 | Phase 4b (단계별 누적) |
| AC-05 | 모든 필수 오류·복구 시나리오가 통과한다. | Scenario 09~10 | Phase 4a |
| AC-06 | 동일 리소스에 대한 API 간 상태 일관성을 유지한다. | Stateful Test (FR-VC-0510, FR-DM-0401) | Phase 3 |
| AC-07 | 비동기 Task 및 NSX Realization 결과가 일치한다. | Stateful Test (FR-BE-0503, FR-NSX-0501) | Phase 3 |
| AC-08 | 인증과 Session 동작이 검증된다. | Contract·Negative Test (FR-API-0201 ~ FR-API-0214) | Phase 4a |
| AC-09 | Resource ID와 참조 관계가 정상적으로 유지된다. | Phase 2a 기본 ID, Phase 3 환경 Namespace·교차 관계, Phase 5 Snapshot 복원 검증 (FR-DM-0201 ~ FR-DM-0210, FR-BE-0203) | Phase 5 (단계별 누적) |
| AC-10 | 테스트 환경 초기화 및 재현이 가능하다. | Seed·Reset 반복 결과 비교 (TR-SCN-0101 ~ TR-SCN-0103, FR-BE-0210) | Phase 5 |
| AC-11 | 자동화된 Regression Test를 실행할 수 있다. | CI 실행 결과 (NFR-ARC-0111) | Phase 5 |
| AC-12 | API Coverage Report가 생성된다. | Report 산출물 (TR-SCN-0112) | Phase 5 |
| AC-13 | 지원하지 않는 API와 미검증 동작을 식별할 수 있다. | Unsupported API Log, Coverage 상태 (FR-API-0403, FR-API-0404, TR-COV-0409) | Phase 5 |
| AC-14 | 호환성 검증 범위 및 제약사항이 문서화된다. | 문서 산출물 (TR-COV-0403) | Phase 5 |
| AC-15 | 실행 시 지정 프로파일로 동작하며, 미등록 버전·잘못된 조합·검증 모드별 기준 미달 기동을 거부한다. | development 모드에서 초기 검증 후 release 모드 실행·거부 시험, 증거 무효화 및 미검증 표시 검사 (FR-API-0301, FR-API-0302) | Phase 1 (이후 프로파일 변경 시) |
| AC-16 | 버전별로 응답에 해당 버전에 없는 타입·속성·경로가 노출되지 않음을 계약 테스트로 확인한다. | Contract Test (FR-VC-0605, FR-VC-0606, FR-VC-0608, TR-COV-0301). Phase 2b는 기준 프로파일에서 협상 버전을 낮춘 세션(예: 7.0.3 기준 프로파일에 `urn:vim25/6.7.3` 요청)으로, Phase 4b는 추가 버전 프로파일 간 Differential Test로 확인 | Phase 2b (Phase 4b 확장) |
| AC-17 | D2에서 선정한 공개 SDK·버전으로 자체 계약 테스트가 통과한다. | SDK 계약 테스트 (TR-COV-0304), 승인 시뮬레이션 범위 기준 | Phase 2b |
| AC-18 | 기본 Fixture에서 리소스 예산을 충족하고 Docker 없이 네이티브로 실행되며, Reset·재기동·재설치 후에도 인증서 지문이 유지된다. | 예산 측정 (NFR-0104, NFR-0107 ~ NFR-0113), 인증서 지문 유지 확인 (NFR-0315) | Phase 1 (이후 매 빌드) |
| AC-19 | 적용 대상 요구사항 전체에 작업·검증 방법·통과 증거가 연결되고, 제외 항목에는 결정 근거가 있다. | TR-COV-0103 추적표와 PRD ID 집합의 대조, 기능·비기능 시험 결과, 채택 범위 내 API의 자체 테스트 증거 확인. 개발 모드 실행 이력만으로 통과시키지 않는다. | Phase 0 계획, 단계별 증거 갱신, Phase 5 최종 |

**향후 외부 검증 조건 — 현재 적용 제외(D5·D13)**

| ID | 표시 조건 | 검증 방법 | 확인 단계 |
|---|---|---|---|
| AC-20 | 향후 별도 승인한 외부 검증 범위에서 일반 릴리스 조건과 실제 CMP·지정 VMware 버전/빌드의 API·시나리오 전체 비교를 충족해 `Reference VMware Verified`에 도달한다. | 외부 CMP 시험과 실환경 Trace·정규화·Differential 비교 증거를 모두 연결하고 별도 판정한다. 현재 자체 시험으로 이 상태를 부여하지 않는다(TR-COV-0201, TR-COV-0202, TR-COV-0401 ~ TR-COV-0409). | 현재 제외, 향후 범위 재승인 시 |

현재는 '승인 범위 자체 테스트 통과, 외부 CMP·VMware 미검증'으로 표시하며 AC-20은 '적용 제외 — D5·D13'로 기록한다. 검증 범위·선정 SDK·제품 버전/빌드·증거 일자·제외 항목을 README와 Coverage Report에 공개한다. 테스트 통과율을 표시할 때는 승인 적용 API·시나리오의 분모와 제외 목록을 함께 제시한다.

---

# 18. AI 에이전트 개발계획 수립 지시

이 장은 AI 에이전트가 PRD(개요, PRD-1~4)를 기반으로 상세 개발계획을 수립하기 위한 작업 지시이다. 개발자가 직접 수행할 수도 있다.

## 공통 원칙

- 이번 작업에서는 전체 시스템을 즉시 구현하지 않는다. 기존 오픈소스의 구조와 지원 범위를 검증한 뒤 상세 설계와 단계별 개발계획을 작성한다.
- 근거 없는 코드 구현을 시작하지 않고, 실현 가능성과 아키텍처를 우선 검증한다. 불확실한 설계는 대안을 비교해 권고안을 제시한다.
- 공식 문서·저장소에서 확인한 사실과 추정한 내용을 구분해 기록한다.
- 모든 계획 산출물은 이 문서의 식별자(WP, R, AC, PoC, D)와 PRD-1~3의 요구사항 ID를 참조한다.
- Phase 0a에서는 Task 1의 vcsim 분석과 Task 4의 우선 PoC(PoC-12, PoC-13, PoC-14, PoC-17, PoC-19, PoC-20, PoC-21)를 먼저 수행한다. 나머지 Task는 Phase 0b에서 Task 1부터 Task 7까지 순서대로 진행한다.
- 프로세스 구성은 권고안 A(D3)를 기준으로 한다. 권고안 B는 PRD-B §B2의 재검토 조건에 해당할 때만 다룬다.

## Task 1 — 오픈소스 분석

다음 저장소와 문서를 분석한다.

- govmomi/vcsim, govmomi simulator, govmomi vapi/simulator
- govmomi의 lookup / sts / pbm / content library 시뮬레이터
- govmomi의 SOAP 직렬화 경로와 버전 협상 처리
- pyVmomi 타입 정의의 버전 메타데이터 (버전 인식 직렬화 데이터 소스 후보)
- Broadcom vSphere Web Services API, vSphere Automation API, NSX Manager/Policy API (Hierarchical API 포함)

분석 결과에는 재사용할 수 있는 기능, 미구현 기능, 확장 가능한 구조, Fork 필요 여부, 버전별 호환성, 개발 위험, 기술 검증이 필요한 항목을 포함한다.

**산출물:** `OPENSOURCE_ASSESSMENT.md` · **관련:** WP 0.1 ~ WP 0.3, R-01

## Task 2 — 상세 아키텍처 설계

다음 다이어그램을 가능하면 Mermaid로 작성한다.

1. 전체 시스템 Component Architecture
2. vCenter Simulator 내부 Component Diagram
3. NSX Simulator 내부 Component Diagram
4. Behavioral Engine Architecture
5. Virtual SDDC Data Model / ERD
6. vCenter–NSX Resource Relationship Diagram
7. VM Create Sequence Diagram
8. NSX Segment Create 및 Realization Sequence Diagram
9. Fault Injection Sequence Diagram
10. Deployment Diagram
11. 버전 협상 및 버전 인식 직렬화 Sequence Diagram
12. 자체 테스트 실행·검증 증거 생성 Sequence Diagram

권고안 A의 Component Diagram을 기준으로 각 모듈의 책임, 호출 방향, 상태 소유권, 락 경계, 내부 인터페이스를 명시한다. PoC-17·PoC-21 결과가 PRD-B §B2의 재검토 조건에 해당하는지 판단해 기록한다.

**산출물:** `ARCHITECTURE.md`, `DATA_MODEL.md`, `BEHAVIOR_SPEC.md` · **관련:** WP 0.9, WP 0.11, D3

## Task 3 — API Coverage 초안

공식 API 계약·공개 SDK와 CMP 참고 목록에서 시뮬레이션할 API 후보를 선정한다. CMP 소스 분석과 실제 CMP 실행·Trace 수집은 하지 않는다.

1. 지정 버전의 공식 WSDL/OpenAPI·API 문서와 공개 SDK에서 계약·의존 API를 확인한다.
2. CMP 기능 목록은 사용 사례 후보를 고르는 데만 참고한다. 상품 한도·과금·신청/승인·콘솔 상태는 재현 범위에서 제외한다.
3. 선정 기능을 정상·오류·조회·인증·비동기·교차 시스템 API까지 전개하고, 채택/제외 근거와 관련 FR·SDK·테스트를 Coverage Matrix에 기록한다.
4. 자체 테스트 실행 Trace와 Unsupported API Log로 구현 누락을 보완한다. 관측되지 않은 API는 자동 제외하지 않고 공식 계약·기능 의존성을 확인한다.

각 API에 대해 vcsim 재사용 가능 여부, 직접 구현 필요 여부, 복잡도, 의존관계, 검증 방법, 우선순위를 판단한다. 기대값은 공식 계약·공개 SDK 등의 독립 근거에서 작성한다. 근거가 없는 동작은 가정·미검증으로 표시하며, 자체 응답을 그대로 정답으로 삼지 않는다.

**산출물:** `API_COVERAGE_MATRIX.md` · **관련:** WP 0.2, WP 0.3, WP 0.7, D2

## Task 4 — 기술 검증 계획

본격 개발 전 수행할 PoC를 정의한다. 각 PoC에는 **검증 목적, 방법, 성공 조건, 실패 시 대안**을 기술한다. 최소 검증 항목은 다음과 같다.

| PoC | 검증 항목 | 관련 위험 | 관련 요구사항 |
|---|---|---|---|
| PoC-01 | vcsim 시작 및 SOAP 로그인 | R-01 | FR-VC-0201, FR-API-0201 |
| PoC-02 | vCenter REST Session 생성 | R-01 | FR-API-0202 |
| PoC-03 | 동일 VM의 SOAP/REST 조회 | R-04 | FR-VC-0205, FR-VC-0510 |
| PoC-04 | VM 생성 및 Task 완료 | R-01 | FR-VC-0401, FR-VC-0501 ~ FR-VC-0506 |
| PoC-05 | PropertyCollector 변경 감지 | R-03 | FR-VC-0509 |
| PoC-06 | Go 기반 vcsim 확장 가능성 | R-02 | FR-VC-0104, FR-VC-0106 |
| PoC-07 | Python ↔ Go 상태 동기화 (D3를 B로 재결정할 때만 수행, PRD-B §B7) | — | FR-BE-0405 |
| PoC-08 | NSX Segment CRUD | R-05 | FR-NSX-0211, FR-NSX-0401 ~ FR-NSX-0406 |
| PoC-09 | NSX Revision 충돌 처리 | R-06 | FR-NSX-0408, FR-NSX-0409 |
| PoC-10 | NSX Realization 상태 전이 | R-06 | FR-NSX-0501 |
| PoC-11 | Docker 기반 통합 실행 (선택) | — | NFR-0104 |
| PoC-12 | vcsim 임베드 + 단일 바이너리 내장 Gateway (SNI로 vCenter/NSX 분기) | R-02, R-13 | FR-VC-0102, NFR-ARC-0107, NFR-ARC-0304 |
| PoC-13 | 요청 버전별 응답 필터링 (예: 7.0.3 기준 프로파일에서 `urn:vim25/7.0.3.0` 세션과 `urn:vim25/6.7.3` 세션의 동일 객체 응답 비교) | R-11 | FR-VC-0605, FR-VC-0606 |
| PoC-14 | 메서드 오버라이드로 `CreateVM_Task` 사전조건 실패 시 Fault 반환 | R-02 | FR-BE-0402, FR-VC-0417 |
| PoC-15 | vcsim 시간 소스 교체 가능성 | R-12 | FR-BE-0611 |
| PoC-16 | NSX Hierarchical API(`PATCH /policy/api/v1/infra` + children) 처리 | R-05 | FR-NSX-0223 ~ FR-NSX-0225 |
| PoC-17 | NSX 스펙 기반 범용 핸들러의 기동 시간·메모리 | R-05, R-14 | FR-NSX-0105 ~ FR-NSX-0107, NFR-0109, NFR-0110 |
| PoC-18 | **보류·현재 제외(D5·D13):** 향후 Record 모드 프록시·비식별화 | R-10, R-14 | TR-COV-0405, TR-COV-0406 |
| PoC-19 | Windows 네이티브 실행 (루프백 별칭, hosts, CA, 443) 및 기본 Fixture 리소스 예산 측정 | R-09, R-13 | NFR-0104, NFR-0107 ~ NFR-0113, NFR-0301, NFR-0313, NFR-ARC-0307, NFR-ARC-0308 |
| PoC-20 | D2에서 선정한 공개 SDK로 자체 로그인·인벤토리 조회 | R-01, R-10 | TR-COV-0304 |
| PoC-21 | vCenter·NSX 교차 변경의 단일 락 경계와 vcsim 내부 락의 상호작용 (교착 여부, 동시 요청 처리량) | R-07, R-15 | FR-BE-0304, FR-DM-0306, NFR-0201 |

**산출물:** `POC_PLAN.md` · **관련:** WP 0.4, WP 0.5, WP 0.12, WP 0.13

## Task 5 — 단계별 개발계획

15장의 Phase 0~5와 작업 패키지를 기준으로 개발 작업을 상세 분해한다. 각 작업에는 다음 정보를 포함한다.

- Work Package ID, 기능명, 개발 목적, 선행 작업, 상세 작업 내용, 구현 대상 모듈, 산출물, 테스트 방법, 완료 기준, 복잡도, 위험, 우선순위, 관련 요구사항 ID

일정·공수는 15.4의 원칙을 따른다.

**산출물:** `DEVELOPMENT_PLAN.md` · **관련:** 15장 전체

## Task 6 — MVP 정의

15.3의 MVP 마일스톤을 기준으로 최초 실행 가능한 MVP 범위를 확정한다. MVP는 최소한 다음을 지원해야 한다.

- 자체 클라이언트의 vCenter·NSX 인증·연결, 가상 인벤토리 조회
- 기본 VM 생성·조회·변경·삭제, 기본 NSX Segment 생성·조회·변경·삭제
- Task 및 상태 변경, 기본 API 오류 처리, Seed 및 Reset, 테스트 자동화
- 기준 버전 조합 1개의 버전 프로파일 및 버전 인식 직렬화, 미지원 API 식별
- 승인된 시뮬레이션 기능 범위 확정과 그 범위의 호환성 판정 (Phase 2b)
- Docker 없는 네이티브 실행 및 리소스 예산 충족, Windows 설치 자동화
- (D7·D9 결정에 따라) Clone·OVF/Content Library·Guest Customization·VM 재시작(Reset·Guest Reboot)·디스크 확장, NSX VM Tag·Hierarchical API

기존 vcsim 기능을 최대한 재사용해 직접 구현할 코드를 줄이는 방향으로 설계한다.

**산출물:** `MVP_SCOPE.md` · **관련:** 15.3

## Task 7 — 계획 산출물 생성

최종적으로 다음 Markdown 문서를 `20-design/` 아래 해당 폴더에 작성하고, 문서 간 참조와 용어를 일관되게 유지한다.

| 문서 | 내용 | 작성 Task |
|---|---|---|
| `ARCHITECTURE.md` | 상세 시스템 설계서 | Task 2 |
| `OPENSOURCE_ASSESSMENT.md` | 오픈소스 분석 및 활용 방안 | Task 1 |
| `API_COVERAGE_MATRIX.md` | API Coverage 초안 | Task 3 |
| `DATA_MODEL.md` | 상태 모델 및 DB 설계 | Task 2 |
| `BEHAVIOR_SPEC.md` | 상태 전이·Task·이벤트·오류 명세 | Task 2 |
| `POC_PLAN.md` | 기술 검증 계획 | Task 4 |
| `DEVELOPMENT_PLAN.md` | 단계별 개발계획, 19장 결정 결과 | Task 5 |
| `MVP_SCOPE.md` | 최초 구현 범위 | Task 6 |
| `TEST_STRATEGY.md` | 검증 및 품질 계획, 요구사항별 작업·시험·완료 기준·증거 추적표(TR-COV-0103, §15.5) | Task 3, Task 4 |
| `RISK_REGISTER.md` | 기술 위험과 대응 방안 (16장 갱신본) | Task 1 ~ Task 4 |
| `VERSION_PROFILE_SPEC.md` | 프로파일 구조, 데이터 출처, 버전 인식 직렬화 설계 | Task 2, Task 4 |
| `RESOURCE_BUDGET.md` | 리소스 예산, 측정 방법, 실행 프로필, 보존 한도 | Task 4 |

마지막에는 다음을 요약한다.

- 최종 권장 아키텍처와 권고안 A 유지 여부(PRD-B §B2 재검토 조건 해당 여부) 및 그 근거
- 가장 위험한 기술적 문제 5개와 Phase 0a 우선 PoC의 결과
- 직접 개발해야 하는 핵심 모듈과 기존 오픈소스로 대체 가능한 부분
- MVP 개발 순서
- 개발 전 결정이 필요한 사항 (19장 갱신)
- 핵심 목표 A(버전 호환)·B(경량 실행) 각각의 달성 가능성 평가와 남은 위험

---

# 19. 개발 착수 전 결정 필요 사항

다음 항목은 Phase 0b 종료 전에 확정한다. D1 첫 기준 조합과 D3·D4는 Phase 0a에서 확인하고, D5·D13은 아래 현재 범위 결정을 적용한다. D2의 시뮬레이션 기능·API 범위는 Phase 0b에서 잠정 기준선으로 기록하고 공식 계약·자체 테스트 결과로 보완해 Phase 2b 종료 게이트(WP 2.12)에서 확정한다. CMP 참고 기능은 자동으로 MVP에 포함하지 않으며 범위 확대 시 FR·선행 WP·자체 테스트를 함께 갱신한다. 결정 결과는 아래 표와 `DEVELOPMENT_PLAN.md`에 기록한다.

| ID | 결정 항목 | 선택지 / 고려사항 | PRD 권고 | 영향 | 상태 | 결정 내용 |
|---|---|---|---|---|---|---|
| D1 | 대상 버전 목록과 첫 기준 조합 | vSphere 7.x / 8.x, VCF 9 계열 포함 여부, 각 버전의 빌드 단위 | 기준 조합 1개로 시작 (FR-API-0310) | 프로파일 데이터 범위, 검증 비용 | 결정 (2026-10-10) | **대상 범위: vSphere/vCenter 7.0 U2 이상(8.x 포함), NSX-T 3.x(Policy API 기반, NSX 4.x 전용 기능 미사용). 첫 기준 조합: vCenter 7.0 U3w(7.0.3.02500, 빌드 24927011) + NSX-T 3.2.4.3(빌드 24942842) — 선정된 기준 패치.** NSX 계약 근거는 Broadcom이 공개한 최신 3.2 계열 공식 스펙인 NSX-T 3.2.1(3.2.1.0.0) Manager·Policy OpenAPI로 한다. 3.2.4.x 스펙은 공개되지 않았으므로 3.2.4.3 프로파일에 3.2.1 스펙을 적용하고, 두 빌드 간 차이는 '미검증'으로 표시하며 3.2.2 ~ 3.2.4.3 릴리스 노트의 API 변경 사항으로 보완한다(FR-API-0309, 2026-10-10 결정). NSX-T 3.2.4.3은 Manager·Edge OVF 서명 인증서가 2026-01-03에 만료되어 신규 설치·재배포가 실패하는 알려진 문제(3636919)가 있으므로, 향후 실환경 검증을 별도 승인할 때 Broadcom 조치 방법을 확인한다(현재 구축 범위 제외 — D5·D13). vSphere 9(VCF 9 계열)는 범위에 포함하지 않음. vSphere 8.x × NSX-T 3.x 허용 조합은 VMware 상호운용성 매트릭스로 확인해 `compatibility-matrix.yaml`에 반영(vSphere 8.0은 NSX-T 3.2.2 이상과만 조합 가능한 것으로 파악, 확인 필요). NSX 4.x 전용 기능(Projects/VPC 등, FR-NSX-0230)은 '보류'·'적용 제외 — D1'. NSX Manager API는 인증·버전·상태 및 Policy API로 대체할 수 없는 채택 기능(Fabric 조회·VM Tag 등)에 한정하며 쓰기 범위는 D9에 기록 |
| D2 | 승인 시뮬레이션 기능·API 범위와 자체 테스트 SDK | 공식 API 계약, 공개 SDK·버전·언어, CMP 참고 기능의 채택/제외 | Phase 0b 잠정 기준선, Phase 2b 최종 확정. CMP 소스·실행 분석 불필요 | 구현 API 집합, 계약 테스트·MVP 판정 | 결정 (2026-10-10, Phase 2b 최종 확정) | **자체 테스트 SDK: govmomi v0.56.0 (Go).** vcsim과 같은 계열이라 관대할 수 있으므로(TR-COV-0305) SOAP 응답은 WSDL·타입 메타데이터 대조 검사로 보완한다. NSX는 govmomi 범위 밖이므로 Go 표준 HTTP 클라이언트로 OpenAPI 계약 테스트를 한다. 기능·API 범위는 D13 경계 내에서 선정하며 CMP 150개 참고 기능 전체는 의무 범위가 아님 **최종 범위(2026-10-10 확정):** 실행 범위 28개 API — VC-SOAP-001~013, VC-REST-001~004, NSX-001A, NSX-002~011(모두 Integration Tested). NSX-001B(`/api/session/create`·XSRF, 구현·시험 완료)와 NSX-012(스펙 제공, FR-NSX-0104)는 계약 근거(API Guide 본문) 확보 전까지 실행 범위 밖, VC-REST-005·006(P2) 제외. ○ 20건은 D6~D10에 따라 MVP 제외(`40-test/o-decisions.yaml`). 자체 테스트 SDK: 선정 govmomi v0.56.0(계약 시험·스키마 대조), 범용 govc(v0.56.0 태그 커밋)·pyVmomi 9.1.1.0. 결과는 시험한 SDK·버전에만 해당 **범위 확대(2026-10-10, Phase 3 종료 후):** Phase 3 추가 API 13건(NSX-013~021, VC-SOAP-014~016, VC-NET-001)을 실행 범위에 넣어 41개 API로 한다(release 기동 조건 포함). **범위 확대(2026-10-11, Phase 4b 종료 후):** Phase 4b 추가 API 20건(NSX-022~031, VC-SOAP-017~023, VC-LOOKUP-001, VC-PBM-001, VC-REST-007)을 실행 범위에 넣어 기준 조합 61개 API로 한다(추가 버전 vCenter 7.0.2는 SOAP 계열 26개). |
| D3 | 프로세스 구성안 | 권고안 A(단일 Go) / B(Go + Python, PRD-B) | 권고안 A | 리소스, 일관성, 설치·CI, 요구사항 적용 범위 | 결정 (2026-10-10), Phase 0a에서 PoC-17·PoC-21로 유지 확인 | 권고안 A 채택. B 전용 항목(FR-BE-0305, FR-BE-0405, NFR-ARC-0302)은 '보류'(MVP '–')·'적용 제외 — D3'. 재검토 조건·절차는 PRD-B §B2 |
| D4 | 리소스 예산·보존 한도 | PRD-3 §13.1(NFR-0107 ~ NFR-0113) 제안값과 §13.5의 기간·건수·총용량·집계 키·회전 정책 확정 | 유휴 ≤ 300 MB, 기동 ≤ 5초, 설치 ≤ 200 MB; 두 안 모두 전체 구성요소 합산, lite/full 모두 유한 보존 | 완료 기준 (AC-18), 장시간 실행 검증 | 리소스 예산·측정 경계·보존 한도 결정 (2026-10-10) | **리소스 예산은 제안값대로 확정: 유휴 메모리 ≤ 300 MB, 기동 ≤ 5초, 유휴 CPU 측정 오차 수준(10초간 ≤ 50 ms), 설치 크기 ≤ 200 MB.** Phase 0a 실측(유휴 Private 71 MB, 기동 약 1.2초, 설치 약 34 MB)이 근거다. 측정 경계(Windows는 Private·Working Set 중 큰 값, Linux·macOS는 RSS), CI 회귀 경고(20% 증가), §13.5 보존 한도 기본값(lite: Event 1,000건, Trace 메타데이터 10,000건, 완료 Task 200건 등 / full: 7일·용량 상한)은 `20-design/development-plan/RESOURCE_BUDGET.md` §2~§4로 확정 |
| D5 | 실환경 비교·Record 적용 여부 | 실제 VMware 접근, Trace 수집·비식별화·비교 | 현재 보류, 자체 테스트 근거·미검증 영역 기록 | WP 5.4·PoC-18·AC-20 제외, 검증 상한 | 결정 (2026-10-10) | 이번 범위는 시뮬레이션 구성·자체 테스트까지이므로 실환경 VMware 비교·Record 모드·Trace 확보는 제외. TR-COV-0401·0402·0404~0408, NFR-0312는 ID를 보존하고 보류. 향후 별도 승인 시 재검토 |
| D6 | 배포 토폴로지 | 같은 PC 네이티브 / 같은 PC 컨테이너·VM / 원격 서버 | — (NFR-ARC-0309) | 바인딩, 인증서, 보안 설정 | 결정 (2026-10-10) | 토폴로지 ① 같은 PC 네이티브 실행만 MVP 범위. ② 같은 PC 컨테이너·VM, ③ 원격 서버(NFR-0311 외부 바인딩)는 MVP 이후 |
| D7 | VM 생성 경로와 부가 동작의 MVP 포함 여부 | 기본 생성 / Clone / Content Library / OVF 업로드, Guest Customization, VM 재시작(Reset·Guest Reboot), 디스크 변경 | 선정한 경로만 채택. 참고 문서 1.2 §3.1의 선행 FR·WP·자체 테스트를 함께 배정 | ESXi 직접 경로, 업로드, 전원·게스트 동작, MVP 범위 | 결정 (2026-10-10) | MVP는 기본 `CreateVM_Task` 경로만 채택. Clone(FR-VC-0402), Template(FR-VC-0415), Content Library/OVF(FR-VC-0206, FR-VC-0211), PBM(FR-VC-0210), Guest Customization, VM 재시작(Reset·Guest Reboot), 디스크 변경은 MVP 이후(WP 4.11) |
| D8 | NSX 네트워크 표현 방식 | N-VDS(OpaqueNetwork) / VDS 7.0+(DVPG backingType=nsx) | — (FR-NSX-0607) | 매핑 규칙, Fixture | 결정 (2026-10-10) | 7.0.3 프로파일의 NSX 네트워크 표현은 VDS 7.0 DVPG(`backingType=nsx`, `logicalSwitchUuid`, `segmentId`). N-VDS(OpaqueNetwork)는 추가 프로파일에서 다룬다. 매핑 구현은 Phase 3(WP 3.2) |
| D9 | NSX 기능 범위 | Hierarchical API, Projects/VPC, Manager API, Policy Search 문법 범위 | H-API 권장 (FR-NSX-0223) | NSX 엔진 범위, MVP 범위 | 결정 (2026-10-10) | Projects/VPC는 D1에 따라 제외. MVP의 NSX 범위는 Segment·Realization·Tag·조회이며, Manager API는 node/version, cluster/status, fabric/compute-managers, transport-zones 조회로 한정한다. Hierarchical API(FR-NSX-0223~0225, PoC-16 확인), Fabric VM·VIF·VM Tag(FR-NSX-0226~0228, FR-NSX-0609), 인벤토리 파생(FR-NSX-0608), 기타 Manager API(FR-NSX-0208), Policy Search는 MVP 이후(Phase 3·4b) |
| D10 | 인증 방식 범위 | SOAP Login / SAML(STS) / REST Session / NSX Basic·Session·XSRF | — (FR-VC-0208, FR-VC-0209) | 보조 서비스 범위 | 결정 (2026-10-10) | MVP 인증은 SOAP `SessionManager.Login`, REST `/api/session`(구형 세션 저장소 어댑터), NSX Basic + `/api/session/create`·XSRF. SAML(STS `LoginByToken`, FR-VC-0209), Lookup Service(FR-VC-0208), 구형 REST 경로(FR-VC-0204), vcsim 보조 서비스(NFR-ARC-0104)는 MVP 이후 |
| D11 | 공식 스펙·공개 SDK의 라이선스와 보관·배포 방식 | vSphere WSDL/OpenAPI, NSX OpenAPI, SDK 타입 메타데이터. 저장소 포함 / 사용자 다운로드 / 배포물 제외 | 확정 전 저장소 커밋 보류 (WP 0.19, R-16) | 프로파일 데이터 배포, 설치 크기 | 결정 (2026-10-10) | govmomi는 go.mod 의존성(Apache-2.0, LICENSE·NOTICE 동봉). pyVmomi에서 추출한 `vim-typeinfo.json`은 출처·버전을 표시해 저장소에 포함한다(Apache-2.0). NSX OpenAPI와 vSphere Automation 스펙은 라이선스 표기가 없으므로 저장소·배포물에 포함하지 않고, 사용자 측 다운로드 스크립트로 확보한다 |
| D12 | 관리 대상 인스턴스 수 | 단일 vCenter·NSX / 다중 vCenter / 다중 NSX. 호스트 이름 체계(`vcenter-NN` 등)와 Fixture 구성 | — (FR-NSX-0601, NFR-0313) | 도메인·인증서 SAN, Fixture, 관계 모델, MVP 범위 | 결정 (2026-10-10) | MVP는 단일 vCenter + 단일 NSX. 호스트 이름은 `vcenter`, `nsx`, `esx-NN`(`<서비스>.<도메인>`). 다중 인스턴스(`vcenter-NN` 등)는 MVP 이후 |
| D13 | 프로젝트 범위 경계 | CMP 참고 자료 활용과 시뮬레이션·자체 테스트의 구분 | 시뮬레이터 구성 및 자체 테스트까지 | 전 문서의 범위·WP·완료/검증 기준 | 결정 (2026-10-10) | CMP는 범위 선정 참고 자료로만 사용. CMP 소스 분석, 실제 CMP 등록·연동 시험, CMP 상품 제약(할당량·과금·신청/승인·콘솔 상태) 재현은 제외. 공식 VMware API 계약상의 제약은 채택 범위에서 재현. 자체 E2E·SDK·상태·오류·리소스 예산 시험으로 완료 판정하며 검증 상한은 Integration Tested |
