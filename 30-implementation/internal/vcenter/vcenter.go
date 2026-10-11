// Package vcenter는 vcsim을 임베드하고 버전 프로파일·인증·REST 뷰를 적용한 vCenter 엔드포인트를 제공한다.
// 관련: FR-VC-0101~0107, FR-VC-0201·0205·0207, FR-VC-0604~0607, FR-API-0201~0205, FR-DM-0204.
package vcenter

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/vmware/govmomi/lookup/simulator" // Lookup Service(/lookupservice/sdk, FR-VC-0208)
	_ "github.com/vmware/govmomi/pbm/simulator"    // PBM(/pbm/sdk, FR-VC-0210)
	"github.com/vmware/govmomi/simulator"
	_ "github.com/vmware/govmomi/vapi/simulator" // vAPI: 구형 /rest 세션, Tagging, Content Library
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/clock"
	"cloudinfra-sim/internal/fault"
	"cloudinfra-sim/internal/fixture"
	"cloudinfra-sim/internal/profile"
	"cloudinfra-sim/internal/store"
)

// AuthFunc는 제품별 자격 증명 확인 함수다.
type AuthFunc func(product, user, password string) bool

// VCenter는 vCenter 시뮬레이션 인스턴스다. Reset으로 인벤토리를 다시 만든다.
type VCenter struct {
	db       *store.Store
	prof     *profile.Set
	fix      *fixture.Fixture
	auth     AuthFunc
	host     string // vcenter.<도메인>
	log      *slog.Logger
	mu       sync.RWMutex
	model    *simulator.Model
	tasks    *taskObserver
	handler  http.Handler
	created  time.Time                                       // 환경 생성 시각(appliance install_time)
	onChange []func(types.ManagedObjectReference)            // 관계 엔진 변경 신호(OnChange)
	perf     metrics                                         // 메트릭 덮어쓰기(perf.go)
	Clock    *clock.Clock                                    // Virtual Clock(WP 4.10). nil이면 실제 시간
	Faults   *fault.Store                                    // vCenter 의미 계층 Fault(WP 4.3)
	Role     func(product, user string) (role, scope string) // 역할·범위(RBAC, WP 4.2). nil이면 검사 안 함
	sess     *sessionTracker
	dsMu     sync.Mutex
	dsFree   map[string]int64   // datastore_full 규칙 적용 전 여유 공간
	dsAlarm  *dsAlarm           // 기본 알람 Datastore usage on disk(alarms.go)
	cert     *tls.Certificate   // Gateway 서버 인증서(Lookup Service가 알리는 SslTrust, WP 4.11)
	pbm      *pbmState          // VM ↔ 스토리지 정책 연결(pbm.go)
	versions *collectorVersions // PropertyCollector 데이터 버전(updates.go)

	cache       *FixtureCache  // Fixture 캐시(fixturecache.go, R-45 ②). nil이면 끔
	cacheStatus CacheStatus    // 마지막 Reset의 캐시 사용 결과
	removing    sync.WaitGroup // 바뀐 이전 모델의 파일 정리(백그라운드)

	// 이벤트 보관(events.go, R-44): epoch는 모델 세대(activate마다 증가), evLast는 그 세대에서 보관한 마지막 이벤트 키
	epoch     atomic.Int64
	evMu      sync.Mutex
	evEpoch   int64
	evLast    int32
	evDropped int64
}

// TaskRetention은 레지스트리에 남길 완료 Task 객체 수다(RESOURCE_BUDGET §4, lite 200).
var TaskRetention = 200

func New(prof *profile.Set, fix *fixture.Fixture, auth AuthFunc, host string, log *slog.Logger, db *store.Store, cert *tls.Certificate, opts ...Option) (*VCenter, error) {
	v := &VCenter{prof: prof, fix: fix, auth: auth, host: host, log: log, db: db, cert: cert, perf: metrics{overrides: map[string]MetricOverride{}}, sess: newSessionTracker(), dsFree: map[string]int64{}, pbm: newPBMState(), versions: newCollectorVersions()}
	for _, o := range opts {
		o(v)
	}
	return v, v.Reset()
}

// Reset은 Fixture로 인벤토리를 새로 만든다(TR-SCN-0102). 기존 세션은 모두 무효가 된다.
// Fixture 캐시가 켜져 있으면 같은 입력으로 만든 저장본을 적재하고, 없으면 만든 뒤 저장한다(fixturecache.go).
func (v *VCenter) Reset() error {
	start := time.Now()
	st := CacheStatus{Mode: "off"}
	defer func() {
		st.Duration = time.Since(start).Round(time.Millisecond).String()
		v.mu.Lock()
		v.cacheStatus = st
		v.mu.Unlock()
	}()
	if v.cacheEnabled() {
		var ok bool
		if ok, st = v.resetFromCache(); ok {
			v.log.Info("Fixture 캐시로 vCenter 인벤토리를 적재했다", "key", st.Key, "duration", time.Since(start).Round(time.Millisecond))
			return nil
		}
	}
	if err := v.create(); err != nil {
		return err
	}
	if st.Mode == "miss" && st.Key != "" {
		t := time.Now()
		if err := v.saveCache(st.Key); err != nil {
			v.log.Warn("Fixture 캐시를 저장하지 못했다(다음 기동에 다시 만든다)", "err", err)
			st.Error = err.Error()
		} else {
			v.log.Info("Fixture 캐시를 저장했다", "key", st.Key, "duration", time.Since(t).Round(time.Millisecond))
		}
	}
	return nil
}

// create는 Fixture로 vcsim 모델을 새로 만들어 활성화한다.
func (v *VCenter) create() error {
	m := v.fix.Model()
	about := &m.ServiceContent.About
	a := v.prof.VCenter.About
	about.Name, about.FullName, about.Vendor = a["name"], a["fullName"], a["vendor"]
	about.Version, about.Build = a["version"], a["build"]
	about.LocaleVersion, about.LocaleBuild, about.OsType = a["localeVersion"], a["localeBuild"], a["osType"]
	about.ProductLineId, about.ApiType, about.ApiVersion = a["productLineId"], a["apiType"], a["apiVersion"]
	about.LicenseProductName, about.LicenseProductVersion = a["licenseProductName"], a["licenseProductVersion"]
	// VM은 vcsim이 무작위 호스트에 두므로 직접 만든다(같은 요청·순서, 호스트만 결정론적, placement.go)
	content := deferContent(m) // VM·로컬 데이터스토어는 결정론적 순서로 직접 만든다(placement.go)
	if err := m.Create(); err != nil {
		return fmt.Errorf("vcsim 모델 생성: %w", err)
	}
	if err := content.create(m); err != nil {
		m.Remove()
		return fmt.Errorf("vcsim VM 생성: %w", err)
	}
	// vcsim 로컬 데이터스토어의 형식은 OTHER로, vAPI Datastore.Type 열거형(VMFS, NFS, ...)에 없다.
	// Fixture 데이터스토어를 VMFS로 둔다(SOAP·REST가 같은 값을 보도록 원본을 바꾼다, SRC-VAPI-SDK-7032).
	for _, obj := range m.Map().AllReference("Datastore") {
		if ds, ok := obj.(*simulator.Datastore); ok && strings.EqualFold(ds.Summary.Type, "OTHER") {
			ds.Summary.Type = "VMFS"
		}
	}
	// 스키마 대조 검사기(TR-COV-0304)가 찾은 vcsim 계약 위반 보정(R-27): 필수 열거 속성 빈 값, vcsim 전용 옵션.
	for _, obj := range m.Map().AllReference("VirtualMachine") {
		if vm, ok := obj.(*simulator.VirtualMachine); ok {
			// 환경 Namespace 분리(FR-DM-0207): Fixture VM의 BIOS·인스턴스 UUID를 Namespace·Seed·MoRef로 결정론적으로 만든다.
			vm.Config.Uuid = v.nsUUID("bios/" + vm.Self.Value)
			vm.Config.InstanceUuid = v.nsUUID("instance/" + vm.Self.Value)
			vm.Summary.Config.Uuid, vm.Summary.Config.InstanceUuid = vm.Config.Uuid, vm.Config.InstanceUuid
			fixVMRuntime(vm)
		}
	}
	for _, obj := range m.Map().AllReference("ClusterComputeResource") {
		if c, ok := obj.(*simulator.ClusterComputeResource); ok {
			fixClusterConfig(c)
		}
	}
	for _, obj := range m.Map().AllReference("HostSystem") {
		if h, ok := obj.(*simulator.HostSystem); ok && h.Config != nil {
			// 가상 용량(FR-BE-0701)을 호스트 하드웨어 정보에 반영한다: Adapter 용량 검사·메트릭·조회가 같은 값을 쓴다(FR-BE-0704).
			if c := v.fix.VCenter.HostCPUCores; c > 0 {
				if hw := h.Summary.Hardware; hw != nil {
					hw.NumCpuCores, hw.NumCpuThreads, hw.NumCpuPkgs = int16(c), int16(c), 1
				}
				if h.Hardware != nil {
					h.Hardware.CpuInfo.NumCpuCores, h.Hardware.CpuInfo.NumCpuThreads, h.Hardware.CpuInfo.NumCpuPackages = int16(c), int16(c), 1
				}
			}
			if gb := v.fix.VCenter.HostMemoryGB; gb > 0 {
				if hw := h.Summary.Hardware; hw != nil {
					hw.MemorySize = int64(gb) << 30
				}
				if h.Hardware != nil {
					h.Hardware.MemorySize = int64(gb) << 30
				}
			}
			h.Config.OptionDef = slices.DeleteFunc(h.Config.OptionDef, func(d types.OptionDef) bool { return d.OptionType == nil })
		}
	}
	v.pbmImport(nil)
	return v.activate(m, v.Now().UTC())
}

// activate는 만든(Create) 또는 적재한(Load, snapshot.go) 모델에 인증·Adapter·관찰자를 붙이고 현재 모델로 바꾼다.
// 기존 세션은 모두 무효가 된다.
func (v *VCenter) activate(m *simulator.Model, created time.Time) error {
	v.dsMu.Lock()
	v.dsFree = map[string]int64{}
	v.dsMu.Unlock()
	if v.versions != nil {
		v.versions = newCollectorVersions() // 세션이 모두 무효가 되므로 버전 상태도 지운다
	}
	v.created = created
	// 기본 vcsim은 비어 있지 않은 아무 계정이나 허용한다. 기본값이 아닌 사용자로 바꾸고 ValidLogin으로 검증한다.
	m.Service.Listen = &url.URL{Host: "127.0.0.1:0", User: url.UserPassword("cloudinfra-sim", "unused")}
	m.Service.RegisterEndpoints = true
	m.Map().SessionManager().ValidLogin = func(l *types.Login) bool { return v.auth("vcenter", l.UserName, l.Password) }
	pbmOwners.Store(m.Map(), v) // PBM 래퍼 등록(NewServer의 엔드포인트 등록 시점, pbm.go)
	s := m.Service.NewServer()
	// vcsim이 만든 내부 리스너는 쓰지 않는다. Gateway만 외부에 노출한다.
	s.Server.Close()
	m.Map().SessionManager().ServiceHostName = v.host
	v.advertiseGateway(m)
	v.installAdapter(m)
	installPerf(v, m)
	v.installDatastoreAlarm(m)
	obs := newTaskObserver(m, v.db, v.log, TaskRetention)
	v.mu.RLock()
	if len(v.onChange) > 0 {
		m.Map().AddHandler(&changeWatcher{fns: append([]func(types.ManagedObjectReference){}, v.onChange...)})
	}
	v.mu.RUnlock()

	h := v.wrap(m)
	v.epoch.Add(1) // 이벤트 키가 1부터 다시 시작하는 새 모델(events.go)
	v.mu.Lock()
	old, oldObs := v.model, v.tasks
	v.model, v.handler, v.tasks = m, h, obs
	v.mu.Unlock()
	if oldObs != nil {
		oldObs.Close()
	}
	if old != nil {
		pbmOwners.Delete(old.Map())
		// 이전 모델의 데이터스토어 파일 삭제는 VM 수에 비례해 오래 걸린다(Windows) → 백그라운드로 지운다(R-45 ②).
		// 새 모델은 다른 임시 디렉터리를 쓰므로 겹치지 않는다. Close가 끝날 때까지 기다린다.
		v.removing.Add(1)
		go func() {
			defer v.removing.Done()
			old.Remove()
		}()
	}
	return nil
}

func (v *VCenter) Model() *simulator.Model {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.model
}

func (v *VCenter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	v.mu.RLock()
	h := v.handler
	v.mu.RUnlock()
	h.ServeHTTP(w, r)
}

func (v *VCenter) Close() {
	v.mu.Lock()
	if v.tasks != nil {
		v.tasks.Close()
		v.tasks = nil
	}
	if v.model != nil {
		v.model.Remove()
		v.model = nil
	}
	v.mu.Unlock()
	v.removing.Wait()
}

func (v *VCenter) wrap(m *simulator.Model) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sdk/vimServiceVersions.xml", v.serviceVersions)
	mux.HandleFunc("/sdk", func(w http.ResponseWriter, r *http.Request) { v.soap(m, w, r) })
	mux.Handle("/api/", v.rest(m))
	mux.Handle("/rest/", v.legacyREST(m.Service.ServeMux))
	mux.Handle("/", m.Service.ServeMux) // /folder, /nfc, /about 등 vcsim 기본 경로
	return mux
}

// advertiseGateway는 vcsim이 자기 내부 리스너(닫힘)로 알리는 서비스 URL·인증서를 Gateway 주소·인증서로 바꾼다.
// Lookup Service 등록 정보(서비스 끝점 URL, SslTrust)가 클라이언트가 실제로 접속할 vCenter 호스트를 가리키게 한다(FR-VC-0208).
func (v *VCenter) advertiseGateway(m *simulator.Model) {
	base := &url.URL{Scheme: "https", Host: v.host}
	om := m.Map().OptionManager()
	var changed []types.BaseOptionValue
	for _, o := range om.Setting {
		opt := o.GetOptionValue()
		val, ok := opt.Value.(string)
		if !ok {
			continue
		}
		switch {
		case opt.Key == "vcsim.server.url":
			changed = append(changed, &types.OptionValue{Key: opt.Key, Value: base.String()})
		case strings.HasSuffix(opt.Key, ".uri"):
			if u, err := url.Parse(val); err == nil && u.Host != "" {
				u.Scheme, u.Host = base.Scheme, base.Host
				changed = append(changed, &types.OptionValue{Key: opt.Key, Value: u.String()})
			}
		}
	}
	om.UpdateOptions(&types.UpdateOptions{ChangedValue: changed})
	if v.cert != nil {
		cfg := &tls.Config{Certificates: []tls.Certificate{*v.cert}}
		m.Map().SessionManager().TLS = func() *tls.Config { return cfg }
	}
}

// serviceVersions는 프로파일 버전으로 버전 협상 문서를 만든다(FR-VC-0207).
func (v *VCenter) serviceVersions(w http.ResponseWriter, _ *http.Request) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" ?>` + "\n<namespaces version=\"1.0\">\n <namespace>\n  <name>urn:vim25</name>\n")
	fmt.Fprintf(&b, "  <version>%s</version>\n  <priorVersions>\n", v.prof.VCenter.APIVersion)
	for _, p := range v.prof.VCenter.PriorVersions {
		fmt.Fprintf(&b, "   <version>%s</version>\n", p)
	}
	b.WriteString("  </priorVersions>\n </namespace>\n</namespaces>\n")
	w.Header().Set("Content-Type", "text/xml")
	_, _ = io.WriteString(w, b.String())
}

// soap은 세션 버전을 정하고, 세션 버전에 없는 메서드를 거부하며(FR-VC-0607), 응답을 필터링한다(FR-VC-0606).
func (v *VCenter) soap(m *simulator.Model, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		m.Service.ServeMux.ServeHTTP(w, r) // GET /sdk/vimService.wsdl 등
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	ver := profile.SessionVersion(r, v.prof.VCenter.APIVersion)
	method := profile.SOAPMethod(body)
	if since, ok := v.prof.Meta.MethodSince(method); ok && profile.Newer(since, ver) {
		// 가정: 세션 버전에 없는 메서드는 MethodNotFound Fault(근거 확보 전, BEHAVIOR_SPEC §6)
		writeFault(w, "MethodNotFound", fmt.Sprintf("%s is not available in API version %s", method, ver))
		return
	}
	var collector string
	if updateMethods[method] {
		collector = collectorKey(r, body)
		if ver, ok := v.versions.check(collector, body); !ok {
			// API 문서: 같은 세션·같은 Collector가 돌려준 버전이 아니면 InvalidCollectorVersion(updates.go)
			writeFault(w, "InvalidCollectorVersion", fmt.Sprintf("Invalid data version %q for this property collector", ver))
			return
		}
	}
	if timeFilterOps[method] && v.Clock != nil && !v.Clock.Identity() {
		body = v.shiftTimeFilter(body) // 가상 시각 범위 → 실제 시각 범위(FR-BE-0611)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	rec := httptest.NewRecorder()
	m.Service.ServeMux.ServeHTTP(rec, r)
	if rec.Code == http.StatusInternalServerError && rewriteAdapterFault(w, method, rec.Body.Bytes()) {
		return // Adapter가 거부한 요청: 실패 Task를 담은 정상 응답(FR-BE-0402)
	}
	if rec.Code == http.StatusInternalServerError && rewriteAdapterResult(w, method, rec.Body.Bytes()) {
		return // vcsim 미구현 메서드에 Adapter가 만든 응답(clusterusage.go)
	}
	if rec.Code == http.StatusOK {
		switch method {
		case "MarkAsTemplate", "MarkAsVirtualMachine":
			v.afterTemplateChange(m, body) // vcsim이 알리지 않는 속성 변경(lifecycle.go)
		case "CreateVM_Task", "ReconfigVM_Task", "CloneVM_Task", "RelocateVM_Task":
			v.observePBM(method, body, rec.Body.Bytes()) // 스토리지 정책 지정(pbm.go)
		}
	}
	out, st := v.prof.Meta.FilterSOAP(rec.Body.Bytes(), ver)
	if method == "RetrievePropertiesEx" || method == "ContinueRetrievePropertiesEx" {
		// 일치하는 객체가 없으면 결과는 null이다(API 문서: "null if there are no matching objects"). vcsim은 빈
		// RetrieveResult(필수 objects 없음)를 돌려준다(R-27).
		out = emptyReturnval.ReplaceAll(out, nil)
	}
	if v.Clock != nil && !v.Clock.Identity() {
		out = profile.ShiftTimes(out, v.Clock.ToVirtual) // vcsim 실제 시각 → 가상 시각(FR-BE-0611)
	}
	if collector != "" && rec.Code == http.StatusOK {
		out = v.versions.issue(collector, out)
	}
	if st.Fields+st.Types > 0 {
		v.log.Debug("soap response filtered", "method", method, "session_version", ver, "fields", st.Fields, "types", st.Types)
	}
	for k, vals := range rec.Header() {
		w.Header()[k] = vals
	}
	w.Header().Del("Content-Length")
	w.WriteHeader(rec.Code)
	_, _ = w.Write(out)
}

func writeFault(w http.ResponseWriter, fault, msg string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><soapenv:Body><soapenv:Fault><faultcode>ServerFaultCode</faultcode><faultstring>%s</faultstring><detail><%sFault xmlns="urn:vim25" xsi:type="%s"></%sFault></detail></soapenv:Fault></soapenv:Body></soapenv:Envelope>`,
		xmlEscape(msg), fault, fault, fault)
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xmlEscapeTo(&b, s)
	return b.String()
}

// fixClusterConfig: vcsim은 vSAN을 켜지 않은 클러스터의 configurationEx.vsanConfigInfo를 비워 두지만 실제 vCenter는
// vSAN 미사용 클러스터에도 enabled=false로 채운다. 이를 전제로 하는 클라이언트가 있다(community.vmware
// vmware_cluster_info가 vsanConfigInfo.enabled를 바로 읽음, TR-COV-0306 실행에서 발견). 값 선택은 가정.
func fixClusterConfig(c *simulator.ClusterComputeResource) {
	cfg, ok := c.ConfigurationEx.(*types.ClusterConfigInfoEx)
	if !ok || cfg.VsanConfigInfo != nil {
		return
	}
	cfg.VsanConfigInfo = &types.VsanClusterConfigInfo{
		Enabled:       types.NewBool(false),
		DefaultConfig: &types.VsanClusterConfigInfoHostDefaultInfo{AutoClaimStorage: types.NewBool(false)},
	}
}

// fixVMRuntime은 vcsim이 비워 두는 필수 열거 속성(VirtualMachineRuntimeInfo.faultToleranceState·recordReplayState)을
// FT·Record/Replay를 쓰지 않는 VM의 값으로 채운다(SRC-VIM-TYPEINFO: 필수, 빈 문자열은 열거값이 아님; 값 선택은 가정).
func fixVMRuntime(vm *simulator.VirtualMachine) {
	for _, r := range []*types.VirtualMachineRuntimeInfo{&vm.Runtime, &vm.Summary.Runtime} {
		if r.FaultToleranceState == "" {
			r.FaultToleranceState = types.VirtualMachineFaultToleranceStateNotConfigured
		}
		if r.RecordReplayState == "" {
			r.RecordReplayState = types.VirtualMachineRecordReplayStateInactive
		}
	}
}

var emptyReturnval = regexp.MustCompile(`<returnval(?:\s[^>]*)?>\s*</returnval>|<returnval(?:\s[^>]*)?/>`)

// nsUUID는 환경 Namespace·Seed 기반 결정론적 UUID다(FR-DM-0207·0210).
func (v *VCenter) nsUUID(key string) string {
	h := func(s string) uint64 {
		var x uint64 = 14695981039346656037
		for i := 0; i < len(s); i++ {
			x ^= uint64(s[i])
			x *= 1099511628211
		}
		return x
	}
	ns := fmt.Sprintf("%s/%d/", v.fix.Namespace, v.fix.Seed)
	b := fmt.Sprintf("%016x%016x", h(ns+key), h(key+"/"+ns))
	return b[0:8] + "-" + b[8:12] + "-4" + b[13:16] + "-a" + b[17:20] + "-" + b[20:32]
}

// Now는 현재 가상 시각이다.
func (v *VCenter) Now() time.Time {
	if v.Clock != nil {
		return v.Clock.Now()
	}
	return time.Now()
}
