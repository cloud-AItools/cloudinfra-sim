# OPENSOURCE_ASSESSMENT — govmomi/vcsim 분석 (Phase 0a)

> 관련: PRD-4 WP 0.1, WP 0.12, WP 0.13, Task 1, R-01·R-02·R-11·R-12·R-15 · 작성일 2026-10-10  
> 분석 대상: `github.com/vmware/govmomi` **v0.56.0** (Apache-2.0), Go 1.27.2 / Windows 11  
> 표기: **확인** = 소스·실행으로 확인 / **추정** = 아직 검증하지 않음

## 1. 요약

| 항목 | 결론 | 근거 |
|---|---|---|
| vcsim 임베드 | **확인.** 자체 리스너 없이 `Service.ServeMux`를 내장 Gateway에 연결해 사용할 수 있다 | PoC-12 |
| 요청 경로 내 행위 로직 (R-02) | **확인.** 포크 없이 가능. 두 가지 확장 지점이 있다(§3.1) | PoC-14 |
| 버전 인식 직렬화 (R-11) | **확인: vcsim은 버전 필터링을 하지 않는다.** Gateway에서 응답을 필터링하면 포크 없이 대응할 수 있다. 다만 메타데이터에 공백이 있다(§3.3) | PoC-13 |
| 시간 소스 (R-12) | **확인: 교체 지점이 없다.** 20개 파일에 `time.Now()` 호출 54곳이 있다 | §3.4 |
| 락 (R-15) | **확인.** vcsim은 객체 단위 재진입 락을 쓴다. 교차 락은 Gateway에서 잡고, 변경 핸들러는 Outbox만 써야 한다 | PoC-21 |
| 기본 About | vcsim VPX 모델의 기본값은 `VMware vCenter Server 6.5.0`, API `6.5`이므로 프로파일이 반드시 덮어써야 한다 | PoC-12 |
| 내장 Fault Injection | `simulator.FaultInjector`가 있다(메서드·객체·속성 필터, 확률, 지연, Max/Skip 횟수). 난수 시드는 시각 기반이고 외부에서 설정할 수 없다 | §3.5 |

## 2. 구조

| 경로 | 내용 | 재사용 |
|---|---|---|
| `simulator/` (파일 122개) | SOAP Managed Object 구현(Folder, VM, Host, Task, PropertyCollector, EventManager, SessionManager 등), `Registry`, `Model`(Fixture 생성), `Service`(HTTP/SOAP 디스패치) | 핵심 엔진으로 임베드 (FR-VC-0101, 0102) |
| `simulator/vpx`, `simulator/esx` | vCenter·ESXi의 기본 ServiceContent·설정 데이터 | 프로파일 값으로 덮어씀 |
| `vapi/simulator` | vSphere Automation(REST) 시뮬레이터(Tagging, Content Library 등) | WP 0.3에서 범위 조사 |
| `lookup/simulator`, `sts/simulator`, `pbm/simulator` | Lookup Service, SSO STS, PBM | FR-VC-0212 (D10 결정에 따라) |
| `vim25/types` | vim25 타입(Go struct), 일부 필드의 `vim:"<버전>"` 태그, 타입·enum 최소 버전 맵 | 버전 메타데이터 일부로 사용 |

## 3. 확장 지점 검증 (FR-VC-0106, PRD-1 '참고 — vcsim 확장 지점' 표 갱신)

### 3.1 요청 경로 내 Hook

| 확장 지점 | 상태 | 용도와 한계 |
|---|---|---|
| `Registry.Handler func(*Context, *Method) (mo.Reference, BaseMethodFault)` | **확인** | 디스패치 전에 실행된다. Fault를 반환하면 **동기 SOAP Fault**로 응답한다(사전조건·권한·의미적 Fault). 대체 핸들러를 반환해도 세션이 있으면 `Session.Get`이 덮어쓰므로 **메서드 교체 용도로는 쓸 수 없다** |
| 레지스트리 객체 교체 `Map().Put(&Wrapper{obj})` (타입 임베딩 + 메서드 오버라이드) | **확인** | vcsim 공식 예제 방식이다. `CreateVM_Task`를 오버라이드해 **Task error**로 실패시켰다. vcsim 내부의 `*Folder` 타입 단언 경로가 래퍼와 충돌하는지는 메서드별로 확인해야 한다 |
| `RegisterObject` 변경 핸들러(`Registry.AddHandler`) | **확인** | 객체의 Put/Update/Remove 통지를 받는다. **요청 고루틴 안에서 동기로 호출**되므로, 여기서 교차 락을 잡으면 교착한다(PoC-21) |
| `DelayConfig` | 존재 확인(동작 미검증) | 메서드별 지연 설정 |
| `FaultInjector` | 존재 확인 | 디스패치 직후, 지연 전에 평가된다. §3.5 참조 |

### 3.2 HTTP·임베드

- `Model.Create()` 다음에 `Service.NewServer()`를 호출하면 `ServeMux`에 `/sdk`, `/sdk/vimServiceVersions.xml`, `/about`, `/folder`, NFC 경로가 등록된다. NewServer는 자체 리스너도 만든다. PoC는 이 리스너를 쓰지 않았다. 본 구현에서는 리스너를 만들지 않는 초기화 방법을 따로 확인해야 한다.
- `vimServiceVersions.xml`은 About.ApiVersion 하나만 넣고, priorVersions는 고정값이다. 프로파일 기반으로 제공해야 한다(FR-VC-0207).

### 3.3 버전 메타데이터 (FR-VC-0604)

| 소스 | 범위 | 접근 방법 |
|---|---|---|
| 필드 태그 `vim:"x.y.z.w"` | **7.0.0.2 ~ 9.1.0.0**에서 도입된 필드 약 390개 | 리플렉션(공개) |
| `minAPIVersionForType` | 타입 323개(7.x 이후) | 비공개 맵. PoC는 소스를 파싱했고, 본 구현에서는 코드 생성으로 추출한다 |
| 7.0.0.0 이전 도입 필드 | **태그 없음.** 예: `DVPortgroupConfigInfo.backingType`, `segmentId`(7.0) | pyVmomi로 보완(아래) |
| pyVmomi 9.1.1.0 `_typeinfo_vim.py` | 2.0~9.1의 모든 타입·필드·메서드 도입 버전(타입 3,566, 필드 8,014, 메서드 963) | `.agentic/poc/tools/extract_pyvmomi.py`가 모듈을 실행하지 않고 텍스트로 파싱해 `vim-typeinfo.json`으로 추출한다 |

결론: 7.0.3 기준 프로파일에서 8.x/9.x 필드를 제거하는 것은 govmomi 메타데이터만으로도 된다. **7.0 미만으로 협상한 세션까지 다루려면 pyVmomi 추출본이 필요하며, PoC-13에서 이를 확인했다.** 두 소스의 겹치는 값은 일치했다(예: `rebootPowerOff` 8.0.0.1).

### 3.4 시간 소스 (FR-BE-0611, R-12)

- 20개 파일에 `time.Now()` 직접 호출이 54곳 있다(task.go, task_manager.go, event_manager.go, session_manager.go, virtual_machine.go 등). 시계를 주입할 지점은 없다.
- 대안:
  - ① 최소 패치 포크: `time.Now`를 패키지 변수로 치환하는 기계적 변경이다.
  - ② Virtual Clock을 자체 엔진(Task 지연, Realization, 세션 만료 판정)에만 적용하고, vcsim이 기록하는 시각은 실제 시간을 그대로 둔다.
- 권고: MVP는 ②로 진행하고(Virtual Clock은 MVP 이후 항목), Phase 4a(WP 4.10)에서 ①을 결정한다.

### 3.5 내장 Fault Injection (FR-FLT-0205 참고)

- 규칙 필드: MethodName, ObjectType, ObjectName, 속성 포함·제외 필터, Probability, FaultType(인증·권한·잘못된 인자·Not Found·InvalidState·ResourceInUse·InsufficientResources·Custom·Network·Timeout), Delay, MaxCount, SkipCount.
- 제약:
  - 규칙별 난수 생성기를 `time.Now()`로 시드하고 외부에서 바꿀 수 없다. 따라서 FR-FLT-0202의 '확률 + Seed' 재현성을 직접 만족하지 못한다. 확률 0/1과 Skip/Max 횟수 조합은 결정론적이다.
  - 동기 SOAP Fault만 만들고 Task error 전환은 하지 않는다.
- 권고: vCenter 의미 계층 Fault Rule(WP 4.3)은 자체 Fault Rule 모델에서 이 기능을 감싸 재사용하고, Seed와 Task error는 Adapter에서 보완한다.

## 4. 라이선스 (WP 0.19, D11 입력)

| 자료 | 버전·출처 | 라이선스 | 현재 보관 |
|---|---|---|---|
| govmomi (vcsim, vim25 타입·버전 태그) | v0.56.0, Go 모듈 | **Apache-2.0**. 임베드·재배포 가능, LICENSE·NOTICE 포함 필요 | Go 모듈 캐시 |
| pyVmomi | 9.1.1.0, PyPI wheel (SHA-256 `76af3cba…e2d2`) | **Apache-2.0**. 추출 메타데이터(`vim-typeinfo.json`)도 출처를 표시하면 저장소 포함 가능할 것으로 판단(D11에서 확정) | `D:\AISCP\tools\specs\pyvmomi-9.1.1.0\` |
| NSX-T Manager/Policy OpenAPI | 3.2.1.0.0, developer.broadcom.com Related Resources (SHA-256 `acd5b1ba…fb22` / `f813fa42…abdb`) | **파일에 라이선스 표기가 없다.** 재배포 가능 여부가 불명확하므로 D11 확정 전에는 저장소·배포물에 넣지 않는다. 사용자가 직접 내려받는 스크립트 방식이 유력 | `D:\AISCP\tools\specs\nsx-t\3.2.1\` |
| vSphere WSDL / Automation 스펙 | 미확보 | — | — |

모두 D11 확정 전에는 저장소에 커밋하지 않고 저장소 밖에 둔다.

## 5. Phase 0b 조사 결과

- **WP 0.2 (SOAP):** 주요 34개 타입의 7.0.3 메서드 398개 중 vcsim 구현은 152개. MVP에 필요한 메서드(로그인, PropertyCollector, ContainerView, CreateVM·Reconfig·Power·Destroy, Task)는 모두 구현되어 있다(`20-design/api-coverage/soap-method-survey.csv`).
- **WP 0.3 (REST/vAPI):** vcsim vAPI는 구형 `/rest` 세션, Tagging, Content Library, OVF/VMTX, VM data-sets, cluster modules를 제공한다. **`/api/session`과 `/api/vcenter/vm`·`datacenter`·`cluster`·`host`·`datastore`·`network`는 없다**(R-17). 자체 구현 방식은 PoC-02·03으로 확인했다.
- **WP 0.5 (Task·PropertyCollector):** PoC-04·05. `WaitForUpdatesEx` 버전 토큰이 항상 `"-"`이고, 완료 Task의 progress는 0이다. 계약 확인 대상이다.
- **남은 확인:** 레지스트리 객체 교체 방식과 vcsim 내부 타입 단언이 충돌하는 메서드 목록(Phase 2a, WP 2.11).
