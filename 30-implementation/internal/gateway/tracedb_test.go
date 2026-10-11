package gateway

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// T-NFR-0504 T-NFR-0302: 본문 마스킹·64 KB 자르기
func TestMaskBody(t *testing.T) {
	cases := map[string]string{
		`<Login><userName>u</userName><password>s3cr3t!</password></Login>`: `<password>***</password>`,
		`{"user":"a","password":"p@ss"}`:                                    `"password":"***"`,
		`j_username=admin&j_password=Secret1`:                               `j_password=***`,
		`<returnval><key>52b1f7a3-0d6e-4b6a-9f4c-112233445566</key>`:        `<key>***</key>`,
	}
	for in, want := range cases {
		got, tr := maskBody([]byte(in), false)
		if !strings.Contains(got, want) || tr || strings.Contains(got, "s3cr3t") || strings.Contains(got, "p@ss") || strings.Contains(got, "Secret1") {
			t.Errorf("mask %q → %q", in, got)
		}
	}
	if got, _ := maskBody([]byte(`"vmware-api-session-token-value"`), true); got != `"***"` {
		t.Errorf("session response: %q", got)
	}
	big := strings.Repeat("x", MaxTraceBody+10)
	if got, tr := maskBody([]byte(big), false); len(got) != MaxTraceBody || !tr {
		t.Errorf("truncate: %d %v", len(got), tr)
	}
}

// T-NFR-0504 T-NFR-0501 T-NFR-0508: 건수·용량·기간 한도 중 먼저 닿는 조건으로 회전
func TestTraceDBRotation(t *testing.T) {
	db, err := sql.Open("sqlite", "file:tracedb-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxIdleConns(1)
	defer db.Close()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	td, err := NewTraceDB(db, 50, 20<<10, 7*24*time.Hour, clock)
	if err != nil {
		t.Fatal(err)
	}
	// 건수 한도
	for i := 0; i < 80; i++ {
		td.Add(TraceEntry{Path: "/x", Status: 200})
	}
	if u := td.Usage(); u["rows"] != 50 || u["dropped"] != 30 {
		t.Fatalf("row limit: %v", u)
	}
	// 용량 한도(20 KB): 2 KB 본문 30건
	for i := 0; i < 30; i++ {
		td.Add(TraceEntry{Path: "/big", RequestBody: strings.Repeat("b", 2048)})
	}
	if u := td.Usage(); u["bytes"] > 20<<10 || u["rows"] == 0 {
		t.Fatalf("byte limit: %v", u)
	}
	got, dropped := td.Recent(0)
	if len(got) == 0 || got[len(got)-1].Path != "/big" || dropped <= 30 {
		t.Fatalf("recent after byte rotation: %d %d", len(got), dropped)
	}
	// 기간 한도(7일): 8일 뒤에는 이전 기록이 모두 지워진다
	now = now.Add(8 * 24 * time.Hour)
	td.Add(TraceEntry{Path: "/new"})
	td.Prune()
	got, _ = td.Recent(0)
	if len(got) != 1 || got[0].Path != "/new" {
		t.Fatalf("age limit: %d entries", len(got))
	}
	// 재개(기존 파일): 건수·용량을 다시 읽는다
	td2, err := NewTraceDB(db, 50, 20<<10, 7*24*time.Hour, clock)
	if err != nil || td2.Usage()["rows"] != 1 {
		t.Fatalf("reopen: %v %v", td2.Usage(), err)
	}
	td2.Reset()
	if r, _ := td2.Recent(0); len(r) != 0 {
		t.Fatal("reset")
	}
}
