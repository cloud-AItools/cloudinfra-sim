package poc

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

)

// WP 0.2: vcsim SOAP 지원 메서드 조사.
// pyVmomi 메타데이터의 관리 객체 메서드(상속 포함, 도입 버전 포함)와 vcsim 객체의 실제 Go 메서드를 대조해
// CSV로 출력한다. 출력: SURVEY_OUT(기본 20-design/api-coverage/soap-method-survey.csv)
func TestSurvey_SOAPMethods(t *testing.T) {
	b, err := os.ReadFile(pyVmomiTypeinfo())
	if err != nil {
		t.Skipf("pyVmomi metadata unavailable: %v", err)
	}
	var doc struct {
		Types map[string]struct {
			Kind    string            `json:"kind"`
			Name    string            `json:"name"`
			Parent  string            `json:"parent"`
			Since   string            `json:"since"`
			Methods map[string]string `json:"methods"`
		} `json:"types"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{} // 내부 이름(vim.VirtualMachine) → WSDL 이름
	for wsdl, ty := range doc.Types {
		byName[ty.Name] = wsdl
	}
	// 상속을 따라 메서드 → 도입 버전 수집
	methodsOf := func(wsdl string) map[string]string {
		out := map[string]string{}
		for cur := wsdl; cur != ""; {
			ty, ok := doc.Types[cur]
			if !ok {
				break
			}
			for m, v := range ty.Methods {
				if _, seen := out[m]; !seen {
					out[m] = v
				}
			}
			cur = byName[ty.Parent]
		}
		return out
	}

	// 대상: VPX 모델(+ default-small)로 생성되는 모든 관리 객체 타입
	m := defaultSmall()
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	defer m.Remove()
	impl := map[string]reflect.Type{}
	for _, kind := range []string{"ServiceInstance", "SessionManager", "PropertyCollector", "ViewManager", "SearchIndex",
		"Folder", "Datacenter", "ClusterComputeResource", "ComputeResource", "HostSystem", "ResourcePool", "VirtualMachine",
		"Datastore", "Network", "DistributedVirtualSwitch", "VmwareDistributedVirtualSwitch", "DistributedVirtualPortgroup",
		"TaskManager", "Task", "EventManager", "PerformanceManager", "CustomFieldsManager", "AuthorizationManager",
		"OptionManager", "LicenseManager", "VirtualDiskManager", "FileManager", "OvfManager", "UserDirectory",
		"CustomizationSpecManager", "GuestOperationsManager", "ContainerView", "ListView", "DistributedVirtualSwitchManager",
		"HttpNfcLease", "StorageResourceManager", "ExtensionManager", "AlarmManager", "VirtualMachineSnapshot"} {
		for _, obj := range m.Map().AllReference(kind) {
			impl[kind] = reflect.TypeOf(obj)
			break
		}
	}

	out := os.Getenv("SURVEY_OUT")
	if out == "" {
		out = filepath.Join("..", "..", "20-design", "api-coverage", "soap-method-survey.csv")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"managed_object", "method", "since_api_version", "in_7.0.3_profile", "vcsim_object_present", "vcsim_implements"})

	kinds := make([]string, 0)
	for _, k := range []string{"ServiceInstance", "SessionManager", "PropertyCollector", "ViewManager", "SearchIndex",
		"Folder", "Datacenter", "ClusterComputeResource", "ComputeResource", "HostSystem", "ResourcePool", "VirtualMachine",
		"Datastore", "Network", "DistributedVirtualSwitch", "VmwareDistributedVirtualSwitch", "DistributedVirtualPortgroup",
		"TaskManager", "Task", "EventManager", "PerformanceManager", "CustomFieldsManager", "AuthorizationManager",
		"ContainerView", "VirtualMachineSnapshot", "OvfManager", "HttpNfcLease", "VirtualDiskManager", "FileManager",
		"CustomizationSpecManager", "GuestOperationsManager", "OptionManager", "LicenseManager", "UserDirectory"} {
		if _, ok := doc.Types[k]; ok {
			kinds = append(kinds, k)
		}
	}
	total, inProfile, implemented := 0, 0, 0
	summary := map[string][2]int{}
	for _, k := range kinds {
		ms := methodsOf(k)
		names := make([]string, 0, len(ms))
		for n := range ms {
			names = append(names, n)
		}
		sort.Strings(names)
		rt, present := impl[k]
		for _, n := range names {
			ver := ms[n]
			in := !newer(ver, profileMax) && vimVersion(ver)[0] > 0
			goName := strings.Title(n)
			if strings.HasSuffix(goName, "_Task") {
				goName = strings.TrimSuffix(goName, "_Task") + "Task"
			}
			has := present && func() bool { _, ok := rt.MethodByName(goName); return ok }()
			total++
			s := summary[k]
			if in {
				inProfile++
				s[0]++
				if has {
					implemented++
					s[1]++
				}
			}
			summary[k] = s
			_ = w.Write([]string{k, n, ver, yn(in), yn(present), yn(has)})
		}
	}
	w.Flush()
	t.Logf("SOAP methods: %d total on %d types; %d available in 7.0.3; vcsim implements %d of those", total, len(kinds), inProfile, implemented)
	for _, k := range kinds {
		s := summary[k]
		t.Logf("  %-32s 7.0.3 methods %3d, vcsim %3d", k, s[0], s[1])
	}
	t.Logf("written: %s", out)
}

func yn(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}
