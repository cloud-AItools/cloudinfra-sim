# Phase 0a PoC

PRD-4 Phase 0a 기술 검증 코드다. 본 구현(`30-implementation/`)이 아니며, 결과는 `20-design/development-plan/POC_PLAN.md`에 기록한다.

| 파일 | PoC |
|---|---|
| `harness_test.go` | 공통: default-small Fixture, SNI Gateway, 자체 서명 인증서, govmomi 로그인 |
| `poc12_gateway_test.go` | PoC-12 단일 바이너리 SNI Gateway (+ PoC-19 루프백 별칭 바인딩) |
| `poc13_version_test.go`, `versionfilter_test.go` | PoC-13 버전 인식 응답 필터 |
| `poc14_precondition_test.go` | PoC-14 요청 경로 내 사전조건 Fault |
| `poc17_nsxspec_test.go`, `nsxspec/` | PoC-17 NSX 스펙 기반 범용 핸들러(지연 컴파일) |
| `poc21_lock_test.go` | PoC-21 교차 락과 vcsim 락 상호작용 |
| `poc0b_vcenter_test.go` | PoC-01~05: SOAP·REST 세션(`/api/session` 어댑터), REST VM 뷰, Task, PropertyCollector |
| `poc0b_nsx_test.go`, `nsxspec/handler.go` | PoC-08·09·10·16: Segment CRUD, `_revision` 412, Realization, H-API |
| `survey_test.go` | WP 0.2 vcsim SOAP 메서드 조사 → `20-design/api-coverage/soap-method-survey.csv` |
| `cmd/footprint` | PoC-19 기동 시간·메모리 측정용 실행파일 (`-nsxspec <dir>`로 NSX 포함) |
| `tools/extract_pyvmomi.py` | pyVmomi 타입 정보(core·query·vim)에서 버전 메타데이터(`vim-typeinfo.json`) 추출 |
| `../scripts/gen_trace.py` | 요구사항 검증 추적표 생성·자동 검사(TEST_STRATEGY §7) |

## 외부 데이터 (저장소 밖, D11 확정 전 커밋 금지)

| 데이터 | 기본 경로 | 환경변수 |
|---|---|---|
| NSX-T 3.2.1 스펙 | `D:/AISCP/tools/specs/nsx-t/3.2.1/` | `NSX_SPEC_DIR` |
| pyVmomi 추출 메타데이터 | `D:/AISCP/tools/specs/pyvmomi-9.1.1.0/vim-typeinfo.json` | `PYVMOMI_TYPEINFO` |

데이터가 없으면 PoC-17은 건너뛰고(skip), PoC-13은 govmomi 메타데이터만으로 실행한다.

## 실행

Go 1.27.2는 `D:\AISCP\tools\go`에 로컬로 설치되어 있다(시스템 PATH는 변경하지 않음).

```bash
export PATH=/d/AISCP/tools/go/bin:$PATH GOPATH=/d/AISCP/tools/gopath
go test -v -count=1 ./...
go build -trimpath -ldflags "-s -w" -o footprint.exe ./cmd/footprint
py -3 -I tools/extract_pyvmomi.py <_typeinfo_core.py> <_typeinfo_query.py> <_typeinfo_vim.py> <vim-typeinfo.json>
py -3 -I ../scripts/gen_trace.py ../../00-planning/v1.1 ../../20-design/development-plan/TEST_STRATEGY.md ""
```
