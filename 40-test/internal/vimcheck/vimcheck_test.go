package vimcheck

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func contract(t *testing.T) *Contract {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "..", "..", "..", "30-implementation", "data", "compatibility-profiles", "vcenter", "7.0.3.02500", "vim-typeinfo.json")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const envOpen = `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema"><soapenv:Body>`
const envClose = `</soapenv:Body></soapenv:Envelope>`

func rules(fs []Finding) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return strings.Join(out, ",")
}

// 검사기 자체 시험: 알려진 위반을 넣은 응답에서 각 규칙이 걸리고, 올바른 응답은 통과한다.
// 추적: T-TR-COV-0304
func TestCheckerDetectsViolations(t *testing.T) {
	c := contract(t)
	okTask := `<RetrievePropertiesExResponse xmlns="urn:vim25"><returnval><objects><obj type="Task">task-1</obj>` +
		`<propSet><name>info</name><val xsi:type="TaskInfo"><key>task-1</key><task type="Task">task-1</task>` +
		`<name>CreateVM_Task</name><descriptionId>Folder.createVm</descriptionId><entity type="Folder">group-v3</entity>` +
		`<state>error</state><cancelled>false</cancelled><cancelable>false</cancelable>` +
		`<error><fault xsi:type="DuplicateName"><name>vm1</name><object type="VirtualMachine">vm-1</object></fault><localizedMessage>dup</localizedMessage></error>` +
		`<reason xsi:type="TaskReasonUser"><userName>u</userName></reason><queueTime>2026-10-10T00:00:00Z</queueTime>` +
		`<eventChainId>1</eventChainId></val></propSet></objects></returnval></RetrievePropertiesExResponse>`
	if fs := c.CheckResponse("RetrievePropertiesEx", "7.0.3.0", []byte(envOpen+okTask+envClose)); len(fs) > 0 {
		t.Fatalf("valid response flagged: %v", fs)
	}
	for _, tc := range []struct {
		name, method, version, body, want string
	}{
		{"enum", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, "<state>error</state>", "<state>broken</state>", 1), "enum"},
		{"order", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, "<key>task-1</key><task type=\"Task\">task-1</task>", "<task type=\"Task\">task-1</task><key>task-1</key>", 1), "order"},
		{"required", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, "<cancelled>false</cancelled>", "", 1), "required"},
		{"unknown", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, "<eventChainId>1</eventChainId>", "<eventChainId>1</eventChainId><bogus>1</bogus>", 1), "unknown-property"},
		{"method-name", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, "CreateVM_Task", "CreateVm", 1), "method-name"},
		{"integer", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, "<eventChainId>1</eventChainId>", "<eventChainId>x</eventChainId>", 1), "integer"},
		{"moref", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, `<entity type="Folder">`, `<entity type="NoSuchType">`, 1), "moref"},
		{"anyType", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, `<val xsi:type="TaskInfo">`, `<val>`, 1), "xsi-type"},
		{"namespace", "RetrievePropertiesEx", "7.0.3.0", strings.Replace(okTask, `xmlns="urn:vim25"`, `xmlns="urn:other"`, 1), "namespace"},
		// 7.0.2.0 도입 속성(vmFailoverInProgress)을 6.7.3 세션에 노출
		{"property-version", "RetrievePropertiesEx", "6.7.3", `<RetrievePropertiesExResponse xmlns="urn:vim25"><returnval><objects><obj type="VirtualMachine">vm-1</obj>` +
			`<propSet><name>runtime</name><val xsi:type="VirtualMachineRuntimeInfo"><connectionState>connected</connectionState><powerState>poweredOn</powerState>` +
			`<vmFailoverInProgress>false</vmFailoverInProgress><faultToleranceState>notConfigured</faultToleranceState><toolsInstallerMounted>false</toolsInstallerMounted>` +
			`<numMksConnections>0</numMksConnections></val></propSet></objects></returnval></RetrievePropertiesExResponse>`, "property-version"},
		// CreateVM_Task가 선언하지 않은 비 RuntimeFault
		{"fault-declared", "CreateVM_Task", "7.0.3.0", `<soapenv:Fault><faultcode>ServerFaultCode</faultcode><faultstring>x</faultstring><detail>` +
			`<InvalidLoginFault xmlns="urn:vim25" xsi:type="InvalidLogin"></InvalidLoginFault></detail></soapenv:Fault>`, "fault-declared"},
	} {
		fs := c.CheckResponse(tc.method, tc.version, []byte(envOpen+tc.body+envClose))
		if !strings.Contains(rules(fs), tc.want) {
			t.Errorf("%s: want rule %s, got %v", tc.name, tc.want, fs)
		}
	}
	// RuntimeFault 계열은 선언 없이도 허용된다
	mo := `<soapenv:Fault><faultcode>ServerFaultCode</faultcode><faultstring>x</faultstring><detail>` +
		`<ManagedObjectNotFoundFault xmlns="urn:vim25" xsi:type="ManagedObjectNotFound"><obj type="VirtualMachine">vm-9</obj></ManagedObjectNotFoundFault></detail></soapenv:Fault>`
	if fs := c.CheckResponse("PowerOnVM_Task", "7.0.3.0", []byte(envOpen+mo+envClose)); len(fs) > 0 {
		t.Fatalf("runtime fault flagged: %v", fs)
	}
}
