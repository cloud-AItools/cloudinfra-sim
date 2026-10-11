// Content Library /api 계약(WP 4.11, FR-VC-0206, NFR-ARC-0104). 근거: SRC-VAPI-SDK-7032(content 모듈 추출).
package contract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/vmware/govmomi/find"

	"cloudinfra-sim/test/internal/testenv"
)

func (r *restClient) doBody(method, path, body string) (int, []byte) {
	r.t.Helper()
	req, _ := http.NewRequest(method, "https://vcenter."+testenv.Domain+path, strings.NewReader(body))
	req.Header.Set("vmware-api-session-id", r.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.e.Client.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

// T-FR-VC-0206 T-NFR-ARC-0104
func TestContentLibraryContract(t *testing.T) {
	c := loadContract(t)
	e := testenv.Start(t, nil)
	r := login(t, e, c)
	check := func(op string, code, want int, b []byte) {
		t.Helper()
		if code != want {
			t.Fatalf("%s: status %d, want %d: %s", op, code, want, b)
		}
		if len(b) > 0 {
			if errs := c.CheckOperation(op, b); len(errs) > 0 {
				t.Fatalf("%s output: %v\n%s", op, errs, b)
			}
		}
	}
	vc := e.Login("")
	f := find.NewFinder(vc.Client, true)
	ds, err := f.DefaultDatastore(context.Background())
	if err != nil {
		dc, _ := f.DefaultDatacenter(context.Background())
		f.SetDatacenter(dc)
		dss, _ := f.DatastoreList(context.Background(), "*")
		ds = dss[0]
	}

	// 로컬 라이브러리 생성(201 + ID) → 목록·조회
	code, b := r.doBody("POST", "/api/content/local-library?client_token=t-1", `{"name":"lib-a","type":"LOCAL","storage_backings":[{"type":"DATASTORE","datastore_id":"`+ds.Reference().Value+`"}]}`)
	check("com.vmware.content.local_library.create", code, http.StatusCreated, b)
	var lib string
	_ = json.Unmarshal(b, &lib)
	code, b = r.do("GET", "/api/content/library", true)
	check("com.vmware.content.library.list", code, 200, b)
	if !strings.Contains(string(b), lib) {
		t.Fatalf("library list: %s", b)
	}
	code, b = r.do("GET", "/api/content/local-library", true)
	check("com.vmware.content.local_library.list", code, 200, b)
	code, b = r.do("GET", "/api/content/library/"+lib, true)
	check("com.vmware.content.library.get", code, 200, b)
	var model map[string]any
	_ = json.Unmarshal(b, &model)
	if model["name"] != "lib-a" || model["type"] != "LOCAL" {
		t.Fatalf("library model: %s", b)
	}
	code, b = r.do("GET", "/api/content/local-library/"+lib, true)
	check("com.vmware.content.local_library.get", code, 200, b)
	// find(action=find)
	code, b = r.doBody("POST", "/api/content/library?action=find", `{"name":"lib-a"}`)
	check("com.vmware.content.library.find", code, 200, b)
	if !strings.Contains(string(b), lib) {
		t.Fatalf("find: %s", b)
	}
	// 갱신(void → 200 본문 없음)
	code, b = r.doBody("PATCH", "/api/content/local-library/"+lib, `{"description":"updated"}`)
	if code != 200 || len(b) != 0 {
		t.Fatalf("update: %d %s", code, b)
	}
	// 항목 생성·목록·조회·찾기·삭제
	code, b = r.doBody("POST", "/api/content/library/item", `{"library_id":"`+lib+`","name":"item-a","type":"ovf"}`)
	check("com.vmware.content.library.item.create", code, http.StatusCreated, b)
	var item string
	_ = json.Unmarshal(b, &item)
	code, b = r.do("GET", "/api/content/library/item?library_id="+lib, true)
	check("com.vmware.content.library.item.list", code, 200, b)
	if !strings.Contains(string(b), item) {
		t.Fatalf("item list: %s", b)
	}
	code, b = r.do("GET", "/api/content/library/item/"+item, true)
	check("com.vmware.content.library.item.get", code, 200, b)
	code, b = r.doBody("POST", "/api/content/library/item?action=find", `{"name":"item-a","library_id":"`+lib+`"}`)
	check("com.vmware.content.library.item.find", code, 200, b)
	code, b = r.do("DELETE", "/api/content/library/item/"+item, true)
	if code != http.StatusNoContent {
		t.Fatalf("item delete: %d %s", code, b)
	}
	// 없는 항목 → 선언 오류 NOT_FOUND(404, Error 구조)
	code, b = r.do("GET", "/api/content/library/item/"+item, true)
	if errs := c.CheckError("com.vmware.content.library.item.get", "NOT_FOUND", code, b); len(errs) > 0 {
		t.Fatalf("item get not found: %v (%d %s)", errs, code, b)
	}
	code, b = r.do("DELETE", "/api/content/local-library/"+lib, true)
	if code != http.StatusNoContent {
		t.Fatalf("library delete: %d %s", code, b)
	}
	code, b = r.do("GET", "/api/content/library/"+lib, true)
	if errs := c.CheckError("com.vmware.content.library.get", "NOT_FOUND", code, b); len(errs) > 0 {
		t.Fatalf("library get not found: %v (%d %s)", errs, code, b)
	}
	// 인증 없이 → 401
	if code, _ := r.do("GET", "/api/content/library", false); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", code)
	}
}
