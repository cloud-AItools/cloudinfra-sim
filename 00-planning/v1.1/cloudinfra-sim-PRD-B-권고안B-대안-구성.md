# CloudInfra Simulator
## PRD-B. 권고안 B 대안 구성 (Go + Python 2 프로세스)

**부제:** CMP-Oriented VMware SDDC API Simulator  
**저장소:** `cloudinfra-sim`  
**문서 버전:** 1.1  
**상위 문서:** `cloudinfra-sim-PRD.md`  
**범위:** 권고안 B의 프로세스 구성, 전용 요구사항의 상세, 재검토 조건과 전환 절차

---

## 목차

- B1. 문서의 지위
- B2. 재검토 조건과 전환 절차
- B3. 프로세스 구성
- B4. 전용 요구사항 상세
- B5. 공통 요구사항의 B 적용 조건
- B6. 교차 시스템 일관성
- B7. 기술 검증과 위험
- B8. 저장소 구성
- B9. 예상 리소스 사용량

---

## B1. 문서의 지위

PRD-4 §19 D3에서 **권고안 A(단일 Go 실행파일)를 채택**했다. 이 문서는 채택하지 않은 권고안 B의 설계 내용을 본문(PRD 개요, PRD-1~4)에서 분리해 보관한다. 본문은 권고안 A만 기준으로 기술하며, B가 필요한 지점에서 이 문서를 참조한다.

- 이 문서는 새 요구사항 ID를 정의하지 않는다. B 전용 요구사항(FR-BE-0305, FR-BE-0405, NFR-ARC-0302)의 ID 행은 원래 문서에 남아 있으며, 상태는 '보류'이고 상세는 이 문서를 가리킨다.
- 추적표(TR-COV-0103)에는 B 전용 항목을 '적용 제외 — D3'로 기록한다. 이는 요구사항 삭제가 아니다.
- B를 다시 채택하지 않는 한 이 문서의 내용은 구현·검증 의무가 아니다.

## B2. 재검토 조건과 전환 절차

다음 중 하나가 발생하면 D3를 재검토한다.

| 조건 | 판단 근거 |
|---|---|
| Go로 구현한 NSX 스펙 기반 범용 핸들러가 기동 시간·메모리 예산(NFR-0109, NFR-0110)을 충족하지 못하고, Go 안에서 개선할 방법이 없다 | PoC-17 |
| NSX 행위 로직의 Go 구현 비용이 Python 생태계를 활용할 때보다 현저히 크다는 근거가 있다 | PoC-08, PoC-16, `OPENSOURCE_ASSESSMENT.md` |

전환 절차는 다음과 같다.

1. D3 결정 내용을 B로 갱신하고 근거(PoC 결과)를 기록한다.
2. FR-BE-0305, FR-BE-0405, NFR-ARC-0302의 상태를 '정의'로 되돌리고, 추적표의 적용 여부를 '적용'으로 바꾼다. 권고안 A 전용 항목(FR-BE-0304, FR-DM-0306, NFR-ARC-0301)은 '적용 제외 — D3'로 기록한다.
3. PoC-07을 수행하고, R-07의 B 대응(§B6)을 `RISK_REGISTER.md`에 반영한다.
4. 리소스 예산의 측정 경계를 §B5에 맞춰 D4에서 다시 확정한다. B 선택이 예산 면제를 뜻하지는 않는다.
5. PRD 개정 이력에 전환을 기록한다.

## B3. 프로세스 구성

| 프로세스 | 언어 | 포함 구성 요소 |
|---|---|---|
| `cloudinfra-sim` | Go | 내장 HTTPS Gateway(SNI/Host 라우팅), vcsim 임베드, vAPI 확장, 보조 서비스, vCenter Behavior Adapter, Admin API |
| `cloudinfra-sim-nsx` | Python (FastAPI, SQLite) | NSX Manager/Policy API 엔진, Realization Controller |

- 두 프로세스 사이에는 내부 Control API 하나만 둔다(NFR-ARC-0302).
- 별도 Reverse Proxy, vCenter Bridge, Simulation Controller 서비스는 두지 않는다(NFR-ARC-0303, 공통).
- Gateway는 NSX 경로를 `cloudinfra-sim-nsx`로 전달한다. 외부 노출 주소·포트는 권고안 A와 같다(PRD 개요 §2.2).

**참고 — 프로세스 구성안 비교**

| 항목 | 권고안 A — 단일 Go 바이너리 (채택) | 권고안 B — Go + Python 2 프로세스 | (참고) 다중 서비스 구성 |
|---|---|---|---|
| 프로세스 수 | 1 | 2 | 5~6 |
| 언어 | Go | Go, Python | Go, Python |
| vCenter SOAP/REST | Go + vcsim 임베드 + vAPI 확장 | 동일 | vcsim 별도 서비스 |
| vCenter 행위 로직 | Go (요청 경로 내) | Go (요청 경로 내) | Python (요청 경로 밖) |
| NSX API | Go, 스펙 기반 범용 핸들러 | Python + FastAPI, 스펙 기반 지연 검증 | Python + FastAPI + 전체 Pydantic 모델 |
| Gateway | 내장 (Go TLS + SNI 라우팅) | Go 프로세스에 내장 | Caddy/Nginx 별도 |
| 교차 시스템 일관성 | 단일 프로세스 락/트랜잭션 | 내부 Control API 1개 + Outbox | Bridge + Outbox + Saga |
| 필수 런타임 | 없음 | Python 런타임 | Docker |
| 예상 메모리 | 가장 낮음 | 중간 | 가장 높음 |
| 단점 | NSX를 Go로 구현해야 함, Python 생태계 활용 제한 | 2개 언어, 프로세스 간 동기화 일부 필요 | 리소스·동기화 비용 큼, 행위 로직이 요청 거부 불가 |

## B4. 전용 요구사항 상세

| ID (원래 문서) | 요구사항 | B 채택 시 의무 |
|---|---|---|
| FR-BE-0305 (PRD-1 §5.3) | 교차 시스템 절차 | PRD-1 §6.4의 교차 시스템 일관성 절차(FR-DM-0401 ~ FR-DM-0404)를 프로세스 경계에 그대로 적용한다. 구체 방식은 §B6. |
| FR-BE-0405 (PRD-1 §5.4) | 권고안 B 연동 | NSX 프로세스는 vCenter Behavior Adapter 기능(FR-BE-0404)을 `cloudinfra-sim`의 내부 Control API로 사용한다. Adapter가 vCenter 변경(VM 생성·변경·삭제, vNIC 변경)을 NSX 프로세스에 통지하는 방법(Outbox 전달, 순서 보장, 재전송)을 설계한다. |
| NFR-ARC-0302 (PRD-3 §14.3) | 프로세스 구성 (권고안 B) | §B3의 2개 프로세스로 구성하고, 두 프로세스 사이에는 내부 Control API 하나만 둔다. |

FR-NSX-0101(구현 언어)은 B 채택 시 "Python(FastAPI, SQLite)으로 별도 프로세스 하나에 구현"으로 적용한다. FR-NSX-0105 ~ FR-NSX-0107(단일 버전 스펙, 지연 컴파일, 범용 핸들러)은 Python 구현에도 동일하게 적용하며, 전체 스펙을 정적 Pydantic 모델로 생성하지 않는다.

## B5. 공통 요구사항의 B 적용 조건

다음 공통 요구사항은 면제되지 않으며, B에서는 아래 조건으로 충족한다.

| ID | B 적용 조건 |
|---|---|
| NFR-0104 네이티브 실행 | Go 실행파일과 Python 프로세스의 2개 프로세스로 배포한다. Docker는 필요하지 않다. |
| NFR-0105 외부 의존성 | NSX 프로세스용 Python 런타임과 잠금된 패키지 의존성만 허용한다. 배포·설치 방법(동봉 또는 설치)은 D3 재결정 시 확정한다. |
| NFR-0109 유휴 메모리 | 두 프로세스의 RSS를 합산한다. |
| NFR-0112 설치 크기 | 전용 Python 런타임·패키지의 설치 크기를 합산한다. D4에서 확정한 동일 측정 경계로 검증한다. |
| NFR-0113 사전 설치 조건 | D3에서 확정한 Python 런타임·패키지 설치 또는 동봉만 예외로 허용한다. 설치 문서와 CI에서 이 조건을 검증한다. |

Docker 없는 실행, API 호환, 보안, 데이터 무결성 의무는 공통이다.

## B6. 교차 시스템 일관성

권고안 A는 vCenter·NSX 상태가 한 프로세스에 있어 단일 락·트랜잭션으로 교차 변경을 처리한다(FR-BE-0304). B에서는 저장소가 프로세스 단위로 나뉘므로 다음을 적용한다.

- **Outbox:** 원본 변경과 같은 트랜잭션에서 Outbox 이벤트를 기록하고, 상대 프로세스에 전달한 뒤 확인 시 제거한다.
- **Reconciliation:** 전달 실패·재기동 후에는 양쪽 상태를 비교해 불일치를 검출하고 재처리한다(FR-DM-0402 ~ FR-DM-0404, NFR-0207, NFR-0208).
- **동기화 전 상태:** 전달 완료 전 자체 테스트 클라이언트에 보이는 상태를 정의하고 Scenario 08(TR-E2E-0803)의 검증 대상에 포함한다.
- **Snapshot:** 두 프로세스의 상태를 같은 시점으로 맞추는 절차(정지 후 덤프 또는 Outbox 위치 기록)를 FR-DM-0304 설계에 포함한다.

## B7. 기술 검증과 위험

| 식별자 | 내용 | B 채택 시 조치 |
|---|---|---|
| PoC-07 | Python ↔ Go 상태 동기화 | 검증 목적·방법·성공 조건·실패 시 대안을 `POC_PLAN.md`에 추가하고 수행한다. |
| R-02 | vcsim 내부 상태 변경과 행위 로직 연동 | Go → Python 변경 전달 방법을 함께 검증한다. |
| R-07 | vCenter와 NSX 간 상태 동기화 | Outbox + Reconciliation(§B6)으로 대응하고 Phase 3에서 Scenario 08로 확인한다. |

## B8. 저장소 구성

- NSX Python 패키지는 `30-implementation/` 아래 별도 패키지로 두고 자체 `pyproject.toml`을 가진다(NFR-ARC-0202).
- `30-implementation/internal/nsx/`는 Python 패키지로 대체한다. 나머지 디렉터리 구성은 PRD-3 §14.2와 같다.

## B9. 예상 리소스 사용량

README '예상 리소스 요구사항'의 권고안 A 추정과 같은 조건에서 B를 구성했을 때의 설계 단계 추정치이다. Phase 0 측정으로 확정하지 않은 값이다.

| 시나리오 | 권고안 B 예상 메모리 (RSS 합계) | CPU |
|---|---|---|
| 기동 직후 유휴, `lite`, 기본 Fixture (VM 20, Segment 10) | 약 250~450 MB | 유휴 시 거의 0% |
| 중규모: VM 1,000, NSX Segment 200 | 약 400~700 MB | 요청 처리 시 1~2코어 순간 사용 |
| 대규모 `full`: VM 5,000, Segment 1,000, 동시 요청 100 | 약 1.0~2.0 GB | 2~4코어 |

- **A보다 큰 이유:** Python 런타임 자체의 기본 메모리와, 큰 JSON 스펙을 Python 객체로 적재할 때의 오버헤드가 더해진다.
- **예산과의 관계:** 유휴 추정 상단(450 MB)이 D4에서 확정한 기본 예산(NFR-0109, 300 MB)을 넘는다. B를 채택하면 예산 충족 방안 또는 D4의 수치 조정 근거를 함께 제시해야 한다.
- **필수 소프트웨어:** D3 재결정 시 정한 Python 런타임·패키지가 추가로 필요하다.
