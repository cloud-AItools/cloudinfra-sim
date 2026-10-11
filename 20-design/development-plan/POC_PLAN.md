# POC_PLAN — Phase 0a 기술 검증 계획과 결과

> 관련: PRD-4 §15.2 Phase 0a, Task 4, WP 0.4·0.6·0.12·0.13·0.14·0.18 · 작성일 2026-10-10(같은 날 2차 갱신: NSX 스펙·pyVmomi 확보 후)  
> 환경: Windows 11 Home(관리자 권한 없음), 20코어, Go 1.27.2, govmomi v0.56.0, NSX-T 3.2.1 공식 OpenAPI, pyVmomi 9.1.1.0  
> PoC 코드: `.agentic/poc/` (본 구현 아님). 실행 방법: `.agentic/poc/README.md`

## 1. Phase 0a 우선 PoC 판정

| PoC | 검증 목적 | 방법 | 성공 조건 | 실패 시 대안 | 결과 |
|---|---|---|---|---|---|
| PoC-12 | vcsim 임베드와 단일 바이너리 내장 Gateway(SNI 분기) | TLS 리스너 1개에서 SNI `vcenter.*` → vcsim `ServeMux`, `nsx.*` → NSX 핸들러로 분기. govmomi로 로그인·인벤토리 조회 | 두 가상 호스트가 각각 분기되고 미등록 호스트는 거부된다 | 포트·주소를 분리한 다중 리스너 | **성공.** 로그인과 VM 20개 조회, NSX 응답, 미등록 호스트 421 거부 |
| PoC-13 | 요청 버전별 응답 필터링 (R-11) | Gateway에서 SOAPAction 버전과 프로파일 최대 버전 중 낮은 쪽을 세션 버전으로 정하고, 응답 XML을 타입 메타데이터로 따라가며 필드·타입을 제거. 양성 대조군: 7.0.0/7.0.1/7.0.3/8.0.0.1/8.0.1 도입 필드 | 7.0.3 프로파일에서 8.x 제거·7.x 유지, 8.0.3 요청은 7.0.3으로 하향, 6.7.3 세션에서는 7.x 필드도 제거 | vcsim 직렬화 경로 최소 패치 포크 | **성공.** govmomi 메타데이터만 쓰면 7.0.0.0 필드(`backingType`, `segmentId`)가 6.7.3 세션에 남는다. pyVmomi 메타데이터를 더하면 모든 대조군이 기대대로 동작한다(FR-VC-0606 수용 기준 예시 충족) |
| PoC-14 | `CreateVM_Task` 사전조건 실패를 요청 경로 안에서 Fault로 반환 (R-02) | A: `Registry.Handler` 훅. B: 레지스트리의 VM Folder를 래퍼로 교체해 메서드 오버라이드 | 위반 요청은 거부되고 VM이 생성되지 않으며, 정상 요청은 vcsim이 처리한다 | 최소 포크 (FR-BE-0407) | **성공.** A는 동기 SOAP Fault, B는 Task error(`InsufficientResourcesFault`) |
| PoC-17 | NSX 스펙 기반 범용 핸들러의 기동 시간·메모리 (R-05, R-14) | NSX-T 3.2.1 Manager·Policy 스펙을 RawMessage로 로드해 경로 트리만 색인하고, 스키마는 호출 시 지연 컴파일·캐시. 전체 디코드와 비교. Gateway 경유 Segment PATCH/GET, 검증 실패, 4.x 전용 경로 확인 | 기동·메모리가 예산 안이고 경로·스키마를 스펙에서 판단 | 사전 생성 색인, Python 대안(PRD-B §B2 재검토 조건) | **성공.** 결과는 §2 |
| PoC-19 | Windows 네이티브 실행과 기본 Fixture 리소스 예산 | `default-small`의 vCenter 부분 + NSX 스펙 핸들러 + Gateway를 단일 실행파일로 3회 실행하고 PowerShell로 측정 | NFR-0109~0112 제안값 이내 | 예산 조정(D4) | **성공.** 결과는 §3 |
| PoC-20 | 선정 SDK로 자체 로그인·인벤토리 조회 | govmomi로 수행(PoC-12에 포함) | 로그인·조회 성공 | — | **성공.** D2에서 govmomi로 선정(2026-10-10)되어 충족 |
| PoC-21 | 교차 단일 락과 vcsim 내부 락의 상호작용 (R-15) | Gateway에서 변경 요청 전체를 교차 락으로 감쌈. vcsim 변경 핸들러의 NSX 파생 반영을 (a) Outbox (b) 핸들러 내 교차 락 직접 획득으로 비교. VM 20개 × 전원 작업 200회 동시 실행 | 교착 없음, 처리량 저하가 허용 범위 | 변경 핸들러 기반 비동기 반영만 사용 | **성공(설계 규칙 포함).** (a)는 교착 0, 락이 없을 때와 처리량 차이 없음(수백 ops/s, 실행마다 편차). (b)는 콜백 400건 중 388건이 교착 상황 |

부수 확인:
- **PoC-15 (시간 소스):** 코드 분석만 했다(OPENSOURCE_ASSESSMENT §3.4). 교체 지점이 없으므로 MVP는 Virtual Clock을 자체 엔진에만 적용하고, 포크 여부는 WP 4.10에서 결정한다.
- **PoC-06 (Go 확장 가능성):** PoC-14로 확인했다.

## 2. PoC-17 결과 (NSX-T 3.2.1 공식 스펙)

| 항목 | 지연 방식(채택안) | 비교: 전체 디코드 |
|---|---|---|
| 스펙 규모 | Manager 974 경로·1,376 오퍼레이션·1,487 정의 / Policy 1,384 경로·2,295 오퍼레이션·2,010 정의 | 같음 |
| 로드 시간 | 26~28 ms | 96~100 ms |
| 힙 증가 | +10 MB | +36 MB |
| 요청 후 컴파일된 정의 | 3,497개 중 23개(Segment 검증 시) | — |
| 경로 색인 | 2,358/2,358 경로 해석 | — |

확인한 동작과 스펙 특성:
- Segment `PATCH` → 200 본문 없음, `GET` → `_revision` 0 → 1 증가.
- 스펙의 `readonly`(17개 필드)와 enum 위반은 400으로 거부했다. 오류 본문 형식은 가정이며, 프로파일의 `errors.yaml`로 대체해야 한다(FR-NSX-0416).
- NSX 4.x 전용 경로(`/orgs/default/projects`)는 404를 반환한다. FR-NSX-0230 '적용 제외 — D1'과 일치한다.
- **스펙 특성 ① 쿼리 조건부 경로:** 390개 경로 키에 쿼리가 포함되어 있다(`?action=start`, `?format=csv`, `?action=...&resource_type=EdgeNode`). 라우팅은 경로 모양과 쿼리 조건을 함께 매칭해야 한다.
- **스펙 특성 ② 같은 모양의 템플릿:** 5개 그룹은 매개변수 이름만 다른 두 템플릿에 메서드가 나뉘어 있다(예: `/transport-nodes/{transportnode-id}` POST와 `{transport-node-id}` GET/PUT/DELETE). 경로 모양 단위로 메서드를 합쳐야 한다.
- **스펙 특성 ③ H-API와 읽기전용:** `Segment.marked_for_delete`가 스펙상 읽기전용이지만 Hierarchical API는 이 필드로 삭제를 지시한다. H-API(FR-NSX-0223)를 구현할 때는 읽기전용 검사에 예외 규칙이 필요하다.

## 3. PoC-19 측정 결과 (vCenter + NSX, `lite` 상당)

| 항목 | vCenter만 | vCenter + NSX 스펙 핸들러 | 예산 제안값 | 판정 |
|---|---|---|---|---|
| 첫 API 응답(내부 / 외부) | 1.03~1.10초 / 1.16~2.46초 | 1.10~1.15초 / 1.23~2.14초 | ≤ 5초 (NFR-0110) | 충족 |
| 유휴 Private 메모리 | 58~59 MB | 71 MB | ≤ 300 MB (NFR-0109) | 충족 |
| 유휴 Working Set | 32~75 MB | 45 MB | — | 참고 |
| 유휴 CPU (10초) | 0 ms | 0 ms | 측정 오차 수준 (NFR-0111) | 충족 |
| 설치 크기 | 25.3 MB | 25.4 MB + 프로파일 데이터 8.5 MB(NSX 스펙 + vim-typeinfo) ≈ 34 MB | ≤ 200 MB (NFR-0112) | 충족 |
| 127.0.0.2:443, 127.0.0.3:443 바인딩 | 관리자 권한 없이 성공 | 같음 | NFR-ARC-0307 | 충족 |

- 외부 측정의 첫 실행 값이 큰 것은 디스크 캐시 영향이다.
- NSX Fixture 객체(Segment 10 등)는 아직 채우지 않았다. 객체당 메모리가 작아 예산에 주는 영향은 미미할 것으로 예상하지만, 본 구현에서 다시 측정한다.
- 시사점: Windows 11에서는 `127.0.0.0/8` 전체가 루프백이라 별칭 설정이 필요 없다. 설치 시 관리자 권한이 필요한 단계는 hosts 파일과 머신 단위 루트 CA 등록뿐이다. macOS는 별칭 설정이 필요하므로 따로 확인한다.

## 4. 요구사항·위험 영향

| 대상 | 영향 | 제안 |
|---|---|---|
| R-02 / FR-BE-0401~0402 | 해소 | 동기 Fault는 `Registry.Handler`, Task error는 래퍼 객체로 처리한다. 포크는 불필요 |
| R-11 / FR-VC-0604~0606 | 해소 | 필터는 Gateway 계층에 둔다. 메타데이터는 pyVmomi 추출본(`vim-typeinfo.json`, 타입 3,566·필드 8,014·메서드 963)을 기본으로 하고 govmomi 태그로 교차 확인한다. 하위 호환 타입 대체 규칙(상위 타입 대체)과 메서드 버전 검사(FR-VC-0607)는 미구현 |
| R-05·R-14 / FR-NSX-0105~0107 | 해소 | 지연 컴파일 방식으로 확정을 제안한다. 쿼리 조건부 경로, 같은 모양 템플릿 병합, H-API 읽기전용 예외를 설계에 반영 |
| R-12 / FR-BE-0611 | 확인(위험 유지) | MVP에서 vcsim 시각은 실제 시간을 쓴다. WP 4.10에서 최소 포크 여부를 결정 |
| R-15 / FR-BE-0304 | 해소(설계 규칙) | **규칙:** 교차 락은 Gateway·Adapter 진입점에서만 획득한다. vcsim 변경 핸들러는 Outbox에만 기록하고 락을 잡지 않는다 |
| FR-API-0306 | 확인 | vcsim 기본 About(6.5)을 프로파일 값(7.0.3, 빌드 24927011, API 7.0.3.0)으로 덮어써야 한다 |
| FR-FLT-0202 | 주의 | 내장 FaultInjector의 시드는 외부에서 정할 수 없으므로 자체 Fault Rule 모델로 감싼다 |
| R-13 / NFR-ARC-0308 | 완화 | Windows에서 443·루프백 바인딩에 관리자 권한이 필요 없다 |
| **D1 / FR-API-0309** | **결정됨: 선택지 ①** | Broadcom은 NSX-T 3.2 API 참조를 **3.2.0과 3.2.1만** 공개한다. 첫 기준 조합의 NSX-T 3.2.4.3에 해당하는 공식 스펙이 없다. §6 참조 |

## 5. Phase 0a 종료 게이트 현황

| 조건 | 상태 |
|---|---|
| 우선 PoC 판정 (12, 13, 14, 17, 19, 20, 21) | 모두 성공(20은 D2 govmomi 선정으로 충족) |
| D1 첫 기준 조합 | 결정됨. NSX 계약 근거는 공개 스펙 3.2.1로 결정(§6 ①) |
| D3 권고안 A 유지 확인 | **유지 근거 충족.** PoC-17(Go 지연 핸들러 +10 MB, 28 ms)이 PRD-B §B2의 재검토 조건(예산 미충족)에 해당하지 않는다. PoC-21도 단일 프로세스 일관성 설계를 뒷받침한다 |
| D4 리소스 예산 확정 | **확정(2026-10-10, 제안값대로).** 측정 경계는 `RESOURCE_BUDGET.md`. 보존 한도는 Phase 0b에서 확정 |
| 기간 상한 | `DEVELOPMENT_PLAN.md`에 기록 필요(미정) |

## 6. NSX 스펙 버전 — 결정: ① (2026-10-10)

| 선택지 | 내용 | 장단점 |
|---|---|---|
| ① 3.2.1 스펙을 3.2.4.3 프로파일의 계약 근거로 사용(권장) | 프로파일에 "스펙 출처 3.2.1.0.0, 3.2.4.3과의 차이는 미검증"으로 기록. 3.2.2~3.2.4.x 릴리스 노트의 API 변경 항목으로 차이를 보완 | 첫 기준 조합을 유지한다. 3.2.4는 버그 수정 릴리스로 안내되어 차이가 작을 것으로 보이나, FR-API-0309에 따라 '미검증'으로 표시해야 한다 |
| ② D1의 NSX 기준을 3.2.1로 변경 | 공식 스펙과 프로파일 버전이 일치 | 실제 운영 사이트가 쓰는 3.2.4.3과 달라진다 |
| ③ 3.2.4.3 NSX Manager의 `/api/v1/spec/openapi/*`에서 스펙 확보 | 정확한 계약 | 실환경 접근이 필요하므로 D5 보류와 충돌한다 |

pyVmomi 9.1.1.0 메타데이터는 각 타입·필드의 '도입 버전'만 사용하므로 최신판을 써도 7.0.3 필터링에 문제가 없다.

## 7. Phase 0b PoC 결과 (2026-10-10)

| PoC | 검증 항목 | 결과 | 비고 |
|---|---|---|---|
| PoC-01 | vcsim SOAP 로그인 | 성공 | Gateway 경유 govmomi |
| PoC-02 | vCenter REST 세션 | **조건부 성공** | vcsim은 구형 `/rest/com/vmware/cis/session`만 제공한다. `/api/session`은 어댑터로 같은 세션 저장소에 연결하고 응답을 신형 계약(201 + 토큰 문자열)으로 변환했다 |
| PoC-03 | 같은 VM의 SOAP/REST 조회 | 성공 | REST 뷰(자체 구현)에서 REST ID = MoRef(`vm-57`), SOAP 전원 변경이 REST에 즉시 반영 |
| PoC-04 | VM 생성과 Task | 성공 | state success, start·complete 시각, result = VM MoRef. 완료 Task의 progress는 0, recentTask 무제한(보존 한도 필요) |
| PoC-05 | PropertyCollector 변경 감시 | 성공(주의) | 초기 상태 수신, 변경 없을 때 MaxWaitSeconds 후 nil, 전원 변경 수신. **버전 토큰이 항상 `"-"`** |
| PoC-07 | Python ↔ Go 동기화 | 대상 아님 | D3 = A |
| PoC-08 | NSX Segment CRUD | 성공 | PATCH 200 본문 없음, GET `_revision`, DELETE 후 404 |
| PoC-09 | NSX Revision 충돌 | 성공 | PUT에 `_revision` 없음 → 412, 오래된 값 → 412(스펙 근거). PATCH + 오래된 `_revision` → 412는 가정 |
| PoC-10 | NSX Realization 상태 전이 | 성공 | `consolidated_status` IN_PROGRESS → SUCCESS(설정 지연) |
| PoC-11 | Docker 통합 실행 | 미수행 | 선택 항목이고 이 PC에 Docker가 없다. 네이티브 실행으로 대체(NFR-0104) |
| PoC-15 | vcsim 시간 소스 | 분석 완료 | 교체 지점 없음 → WP 4.10 |
| PoC-16 | NSX Hierarchical API | 성공 | ChildTier1·ChildSegment 일괄 생성, `marked_for_delete` 삭제, 잘못된 자식 하나 → 400·전체 미반영(검증 단계 실패만 확인, 적용 중 실패 롤백은 미검증) |
| PoC-18 | Record 모드 | 대상 아님 | D5·D13 |

WP 0.2·0.3 조사 결과는 `API_COVERAGE_MATRIX.md` §2에 있다. vcsim은 SOAP MVP 메서드를 모두 구현하지만 REST `/api/vcenter/*`는 제공하지 않는다(R-17).
