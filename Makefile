# cloudinfra-sim 작업 진입점 (NFR-ARC-0202). Windows에서는 Git Bash + make, 또는 아래 명령을 직접 실행한다.
GO      ?= go
IMPL    := 30-implementation
TEST    := 40-test
BIN     := $(IMPL)/bin/cloudinfra-sim
GOVC    ?= D:/AISCP/tools/govc/govc.exe
DIGEST   = $(shell cd $(IMPL) && $(GO) run ./cmd/srcdigest)
LDFLAGS  = -s -w -X cloudinfra-sim/internal/sim.BuildCommit=$(DIGEST)
DIST    := dist
# 배포 대상(NFR-ARC-0203, WP 5.7): 순수 Go(cgo 없음)라 교차 빌드한다
PLATFORMS := windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build release test test-short evidence trace run clean govc scenario regression perf soak dist cross

build:            ## 개발 빌드(기본 검증 모드 development)
	cd $(IMPL) && $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/cloudinfra-sim ./cmd/cloudinfra-sim

release:          ## 배포 패키지 빌드(기본 검증 모드 release, FR-API-0302)
	cd $(IMPL) && $(GO) build -trimpath -ldflags "$(LDFLAGS) -X cloudinfra-sim/internal/config.BuildKind=release" -o bin/cloudinfra-sim-release ./cmd/cloudinfra-sim

test:             ## 단위·계약·E2E·Footprint 시험
	cd $(IMPL) && $(GO) vet ./... && $(GO) test ./...
	cd $(TEST) && $(GO) vet ./... && $(GO) test ./...

test-short:       ## Footprint·벤치마크 제외
	cd $(TEST) && $(GO) test -short ./...

evidence:         ## 시험 결과로 verification.yaml 생성(WP 1.13). 구현·프로파일이 바뀌면 다시 실행한다.
	cd $(IMPL) && $(GO) test -json ./... > ../$(TEST)/results-impl.json; cd $(TEST) && $(GO) test -json ./... > results.json; $(GO) run ./cmd/evidence -results results.json

govc:             ## 범용 SDK 시험용 govc(govmomi v0.56.0 태그 커밋, 40-test/tools/govc 모듈) 빌드
	cd $(TEST)/tools/govc && $(GO) build -o $(GOVC) github.com/vmware/govmomi/govc

scenario:         ## 선언형 시나리오 전체 회귀, 리포트·Trace·실행 로그를 40-test/reports에 저장(TR-SCN-0301·0303)
	cd $(TEST) && SCENARIO_REPORT_DIR=$(CURDIR)/$(TEST)/reports $(GO) test -count=1 -run TestDeclarativeScenarios ./simulator-e2e

regression:       ## 전체 회귀(AC-11, WP 5.3): vet·단위·계약·E2E·시나리오·Footprint, 추가 버전 프로파일, 증거 재생성
	cd $(IMPL) && $(GO) vet ./... && $(GO) test -count=1 ./...
	cd $(TEST) && $(GO) vet ./... && $(GO) test -count=1 -timeout 30m ./...
	cd $(TEST) && SIM_TEST_VCENTER_VERSION=7.0.2.00000 $(GO) test -count=1 -timeout 30m -short ./contract ./simulator-e2e ./sdk
	$(MAKE) evidence

perf:             ## Performance Benchmark(WP 5.6, NFR-0116). PERF_FULL=1이면 VM 5,000·동시 100 포함
	cd $(TEST) && PERF_RUN=1 PERF_REPORT_DIR=$(CURDIR)/$(TEST)/performance/results $(GO) test -count=1 -timeout 60m -run TestBenchmark -v ./performance

soak:             ## full 보존 한도 수렴(WP 5.9)
	cd $(TEST) && PERF_RUN=1 PERF_REPORT_DIR=$(CURDIR)/$(TEST)/performance/results $(GO) test -count=1 -timeout 60m -run TestSoakRetention -v ./performance

cross:            ## 교차 빌드 확인(배포 대상 전체)
	@for p in $(PLATFORMS); do os=$${p%/*}; arch=$${p#*/}; echo "build $$os/$$arch"; \
	  (cd $(IMPL) && CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -o /dev/null ./cmd/cloudinfra-sim) || exit 1; done

dist:             ## 배포 패키지(release 빌드 + 프로파일·Fixture·시나리오·설치 스크립트, SHA256SUMS). NSX 스펙·vAPI 계약 제외(D11)
	bash 50-deployment/dist.sh $(DIST)

trace:            ## 요구사항 검증 추적표 재생성(TR-COV-0103)
	py -3 -I .agentic/scripts/gen_trace.py 00-planning/v1.1 20-design/development-plan/TEST_STRATEGY.md "NFR-ARC-0309"

run: build        ## 기본 프로파일로 실행
	cd $(IMPL) && ./bin/cloudinfra-sim --vcenter-version 7.0.3.02500 --nsx-version 3.2.4.3

clean:
	rm -rf $(IMPL)/bin $(TEST)/results.json $(DIST)
