package profile

import (
	"strings"
	"testing"
	"time"
)

// T-FR-BE-0611 T-FR-BE-0608: 응답의 dateTime 필드만 가상 시각으로 바꾼다(문자열은 그대로).
func TestShiftTimes(t *testing.T) {
	plus := func(x time.Time) time.Time { return x.Add(time.Hour) }
	in := `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><soapenv:Body><CurrentTimeResponse xmlns="urn:vim25"><returnval>2026-10-10T12:00:00Z</returnval></CurrentTimeResponse></soapenv:Body></soapenv:Envelope>`
	out := string(ShiftTimes([]byte(in), plus))
	if !strings.Contains(out, "2026-10-10T13:00:00Z") {
		t.Fatalf("CurrentTime not shifted: %s", out)
	}
	in2 := `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><soapenv:Body><RetrievePropertiesExResponse xmlns="urn:vim25"><returnval><objects><obj type="Task">task-1</obj><propSet><name>info</name><val xsi:type="TaskInfo"><key>task-1</key><name>2026-10-10T12:00:00Z</name><queueTime>2026-10-10T12:00:00Z</queueTime></val></propSet><propSet><name>info.startTime</name><val xsi:type="xsd:dateTime">2026-10-10T12:00:00Z</val></propSet></objects></returnval></RetrievePropertiesExResponse></soapenv:Body></soapenv:Envelope>`
	out2 := string(ShiftTimes([]byte(in2), plus))
	if strings.Count(out2, "13:00:00Z") != 2 || !strings.Contains(out2, "<name>2026-10-10T12:00:00Z</name>") {
		t.Fatalf("typed shift: %s", out2)
	}
}

// T-FR-API-0211: 메서드 권한은 공식 타입 정보(SRC-VIM-TYPEINFO)에서 상속을 따라 찾는다.
func TestPrivileges(t *testing.T) {
	m, err := LoadVersionMeta("../../data/compatibility-profiles/vcenter/7.0.3.02500/vim-typeinfo.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][3]string{
		{"Folder", "CreateVM_Task", "VirtualMachine.Inventory.Create"},
		{"VirtualMachine", "PowerOnVM_Task", "VirtualMachine.Interact.PowerOn"},
		{"VirtualMachine", "ReconfigVM_Task", "VirtualMachine.Config.Settings"},
		{"VirtualMachine", "Destroy_Task", "VirtualMachine.Inventory.Delete"},
		{"PropertyCollector", "RetrievePropertiesEx", "System.Anonymous"},
	} {
		if got := m.Privilege(c[0], c[1]); got != c[2] {
			t.Errorf("%s.%s: %q want %q", c[0], c[1], got, c[2])
		}
	}
}
