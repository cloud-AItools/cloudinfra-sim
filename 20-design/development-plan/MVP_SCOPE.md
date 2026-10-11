# MVP_SCOPE — 최초 구현 범위

> 관련: PRD-4 §15.3·Task 6, PRD-1~3 MVP 열, `API_COVERAGE_MATRIX.md` §3 · 작성일 2026-10-10 (Phase 0b)  
> 상태: ○ 항목은 D6~D12 결정(2026-10-10, 권고안 채택)으로 확정. 승인 시뮬레이션 범위는 Phase 2b(WP 2.12)에서 최종 확정한다.

## 1. MVP 정의

MVP는 **Phase 2b 종료 게이트**다. 자체 테스트 클라이언트(govmomi + Go `net/http`)로 Scenario 01~06을 통과하고, 기준 버전 조합 하나(vCenter 7.0 U3w + NSX-T 3.2.4.3, NSX 계약 근거는 3.2.1 스펙)의 승인 범위가 `Integration Tested`에 도달한 상태다.

| 범위 | 내용 | 요구사항 | 작업 패키지 |
|---|---|---|---|
| 연결·인증 | vCenter SOAP 로그인, REST `/api/session`(어댑터), NSX Basic·세션 | FR-API-0201~0203, 0205~0208, 0210, 0214 | WP 1.1, 1.6 |
| 인벤토리 조회 | Datacenter·Cluster·Host·Datastore·Network·VM(SOAP·REST), NSX 버전·클러스터·Compute Manager·Transport Zone·Infra | FR-VC-0301~0307, FR-NSX-0201~0205, 0209~0210, 0601 | WP 1.7 |
| VM 생명주기 | 생성·변경(CPU·메모리)·전원·삭제, 사전조건·용량 | FR-VC-0401, 0404~0406, 0409, 0417, 0418 | WP 2.11, 2.1, 2.5 |
| Task·변경 감시 | Task 계약, PropertyCollector, `WaitForUpdatesEx` | FR-VC-0501~0511, FR-BE-0501~0503 | WP 2.3, 2.7 |
| NSX Segment | CRUD, PATCH/PUT 의미, Revision 412, Pagination, Tag | FR-NSX-0211, 0219, 0401~0412, 0414~0416 | WP 2.2 |
| Realization | 즉시·설정 지연 완료, 상태 조회, 삭제 후 조회 | FR-NSX-0220, 0501~0503, 0508, 0509, FR-BE-0606 | WP 2.10 |
| 기본 오류 | 인증 실패, 잘못된 입력, Not Found, 중복, Revision 충돌, 미지원, SOAP Fault | FR-FLT-0101, 0104~0106, 0108, 0109, 0121, 0207 | WP 2.8 |
| 버전 프로파일 | 7.0.3 + 3.2.4.3 프로파일, 기동 검증 모드, 버전 협상, 응답 필터(6.7.3 하향 세션 포함) | FR-API-0301~0310, 0312, FR-VC-0601~0608 | WP 1.2, 1.3, 2.12 |
| 미지원 처리 | 성공 위장 금지, 미지원 응답·로그·집계 | FR-API-0401~0404, FR-NSX-0231 | WP 1.12 |
| Seed·Reset | default-small Fixture, 결정론적 ID | TR-SCN-0101~0103, 0201~0204, FR-BE-0210 | WP 1.8 |
| 운영 기반 | 설치(Windows 자동화), 인증서·지문 유지, Admin 인증, 마스킹, lite 보존 한도, 리소스 예산 CI | NFR-01xx·03xx·05xx의 ● 항목 | WP 1.1, 1.9~1.11, 1.13 |
| 시험·증거 | Contract·Stateful·Negative·govmomi 계약, Scenario 01~06 회귀, 추적표 증거 | TR-COV-0101~0103, 0201~0202, 0301~0305, 0307, 0403, 0409 | WP 2.9, 2.12 |

요구사항 단위로는 PRD-1~3의 MVP ● 274건(FR 165, TR 48, NFR 61)이다. 개별 추적 행은 `TEST_STRATEGY.md` §7에 있다.

## 2. ○ 항목 처리 (D6~D10 결정, 2026-10-10)

| 결정 | ○ 항목 | 결정 내용 | MVP 포함 |
|---|---|---|---|
| D7 | Clone(FR-VC-0402), Template(FR-VC-0415), Content Library/OVF(FR-VC-0206, 0211), PBM(FR-VC-0210) | 기본 `CreateVM_Task`만 MVP. 나머지는 WP 4.11 | 제외 |
| D8 | 네트워크 표현(FR-NSX-0607) | VDS 7.0 DVPG(`backingType=nsx`)를 7.0.3 프로파일 기본값으로 | 제외(Phase 3) |
| D9 | H-API(FR-NSX-0223~0225), Fabric VM·VIF·실현 VM(0226~0228), VM Tag 소유권(0609), 기타 MP(0208), 인벤토리 파생(0608) | MVP 제외. H-API는 PoC-16으로 가능성 확인, Phase 4b | 제외 |
| D10 | 구형 REST(FR-VC-0204), Lookup(0208), STS(0209), vcsim 보조 서비스(NFR-ARC-0104) | SOAP Login + `/api/session` + NSX Basic·세션만 MVP | 제외 |
| D6 | 외부 바인딩(NFR-0311), 배포 토폴로지(NFR-ARC-0309) | 토폴로지 ① 같은 PC 네이티브만 MVP | ①만 포함 |

채택된 ○는 NFR-ARC-0309의 토폴로지 ①뿐이며, 나머지 ○ 20건은 MVP 이후 단계에 둔다(추적표에 결정 ID와 함께 기록).

## 3. MVP 밖 (명시)

- Fault Injection, Virtual Clock 제어, Session Timeout, RBAC(Phase 3~4a)
- vCenter–NSX 교차 관계, Fabric 파생, Scenario 07~10(Phase 3~4b)
- Snapshot/Restore, Scenario Runner, Coverage Report, `full` 프로필 검증(Phase 5)
- CMP 연동·상품 제약, 실환경 비교·Record(D5·D13)

## 4. 구현 순서 (MVP)

1. WP 1.1 실행 골격·Gateway → WP 1.2·1.3 프로파일(필터·메서드 검사 포함) → WP 1.4 vcsim 임베드
2. WP 1.6 인증·세션(REST 어댑터 포함) → WP 1.7 조회(REST 뷰, NSX 범용 핸들러) → WP 1.5 NSX 엔진 정리
3. WP 1.8 Fixture·Reset → WP 1.9·1.11·1.12 로그·보안·미지원 → WP 1.10·1.13 CI 예산·증거 → **Phase 1 게이트(Scenario 01)**
4. WP 2.11 Adapter → WP 2.1·2.5 VM 생명주기 → WP 2.7 Task·PC → WP 2.3·2.4·2.6 상태·ID·저장소
5. WP 2.2 Segment → WP 2.10 Realization → WP 2.8 기본 오류 → **Phase 2a 게이트(Scenario 02~06)**
6. WP 2.9 시험 자동화 → WP 2.12 범위 확정·하향 세션 계약·release 기동 → **MVP**
