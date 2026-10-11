// 보존 한도·작업 추적 로그 (NFR-0402, NFR-0403, NFR-0502, NFR-0505, NFR-0506; Phase 1 → Phase 2a 이월 항목).
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) lines() []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for _, s := range strings.Split(strings.TrimSpace(l.b.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timeout waiting for %s", what)
}

// 완료 Task 객체는 보존 한도까지만 남고, 상태 전이는 저장소에 기록되며, 작업 추적·상태 전이·미지원 API 로그가 남는다.
// 추적: T-NFR-0505 T-NFR-0506 T-NFR-0402 T-FR-BE-0501 T-FR-BE-0502 T-NFR-0508
// NFR-0403의 상태 전이·미지원 API 로그도 여기서 시험하지만 Fault Injection Log(WP 3.8·4.3)가 남아 이월 유지.
func TestTaskRetentionAndLogs(t *testing.T) {

	buf := &lockedBuf{}
	// 보존 한도 설정(NFR-0508): 완료 Task 3건
	e := testenv.StartWithLogger(t, func(c *config.Config) { c.RetainTasks = 3 }, slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	ctx := context.Background()
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, _ := f.DefaultDatacenter(ctx)
	f.SetDatacenter(dc)
	vms, _ := f.VirtualMachineList(ctx, "*")
	var vm *object.VirtualMachine
	for _, v := range vms {
		if st, _ := v.PowerState(ctx); st == types.VirtualMachinePowerStatePoweredOn {
			vm = v
			break
		}
	}
	if vm == nil {
		t.Fatal("no powered-on VM in fixture")
	}

	var refs []types.ManagedObjectReference
	for i := 0; i < 3; i++ {
		for _, op := range []func(context.Context) (*object.Task, error){vm.PowerOff, vm.PowerOn} {
			tk, err := op(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := tk.Wait(ctx); err != nil {
				t.Fatal(err)
			}
			refs = append(refs, tk.Reference())
		}
	}
	last := refs[len(refs)-1]

	// 작업 기록(NFR-0506): 모든 Task가 저장소에 기록된다(lite 한도 5,000 안)
	waitFor(t, "operations recorded", func() bool {
		n, _ := e.Sim.Store.Count(ctx, "simulation_operations")
		return n >= len(refs)
	})
	if n, _ := e.Sim.Store.Count(ctx, "operation_transitions"); n < 3*len(refs) {
		t.Fatalf("transitions %d, want >= %d (QUEUED·RUNNING·SUCCESS)", n, 3*len(refs))
	}

	// 완료 Task 보존(NFR-0505): 한도(3)를 넘는 오래된 Task 객체는 조회되지 않는다
	pc := property.DefaultCollector(c.Client)
	waitFor(t, "old task purged", func() bool {
		var tk mo.Task
		return pc.RetrieveOne(ctx, refs[0], []string{"info.state"}, &tk) != nil
	})
	var tk mo.Task
	if err := pc.RetrieveOne(ctx, last, []string{"info"}, &tk); err != nil || tk.Info.State != types.TaskInfoStateSuccess {
		t.Fatalf("latest task must remain: %v %v", err, tk.Info.State)
	}
	// TaskManager.recentTask: 최근 Task를 담고, 정리된 Task를 가리키지 않는다(모든 참조가 조회 가능)
	var tm mo.TaskManager
	if err := pc.RetrieveOne(ctx, *c.ServiceContent.TaskManager, []string{"recentTask"}, &tm); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tm.RecentTask, last) || slices.Contains(tm.RecentTask, refs[0]) {
		t.Fatalf("recentTask must contain latest and not purged tasks: %v", tm.RecentTask)
	}
	var tasks []mo.Task
	if err := pc.Retrieve(ctx, tm.RecentTask, []string{"info.state"}, &tasks); err != nil || len(tasks) != len(tm.RecentTask) {
		t.Fatalf("recentTask has dangling references: %v (%d of %d)", err, len(tasks), len(tm.RecentTask))
	}

	// 미지원 API 로그(NFR-0403)
	nsxDo(t, e, http.MethodGet, "/policy/api/v1/infra/no-such-collection", "")

	// 작업 추적(NFR-0402: Task ID, Resource ID, 소요 시간)·상태 전이 로그(NFR-0403)
	waitFor(t, "task logs", func() bool {
		var completed, transition, unsupported bool
		for _, m := range buf.lines() {
			switch m["msg"] {
			case "task completed":
				if m["task"] == last.Value && m["resource"] == vm.Reference().Value && m["state"] == "SUCCESS" && m["duration_ms"] != nil {
					completed = true
				}
			case "task transition":
				if m["task"] == last.Value && m["from"] == "RUNNING" && m["to"] == "SUCCESS" {
					transition = true
				}
			case "unsupported api":
				if strings.Contains(fmt.Sprint(m["key"]), "no-such-collection") && m["level"] == "WARN" {
					unsupported = true
				}
			}
		}
		return completed && transition && unsupported
	})
}

// Event는 최근 1,000건 링버퍼로 보존된다(RESOURCE_BUDGET §4 lite, vcsim EventHistoryCollector 최대 페이지와 일치).
// 추적: T-NFR-0502
func TestEventRetention(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	c := e.Login("")
	em := event.NewManager(c.Client)
	const n = 1100
	for i := 0; i < n; i++ {
		ev := &types.GeneralUserEvent{GeneralEvent: types.GeneralEvent{Message: fmt.Sprintf("retention-%04d", i)}}
		if err := em.PostEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	events, err := em.QueryEvents(ctx, types.EventFilterSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) > 1000 {
		t.Fatalf("events retained %d, want <= 1000", len(events))
	}
	var msgs []string
	for _, ev := range events {
		if u, ok := ev.(*types.GeneralUserEvent); ok {
			msgs = append(msgs, u.Message)
		}
	}
	joined := strings.Join(msgs, "\n")
	if strings.Contains(joined, "retention-0000") || !strings.Contains(joined, fmt.Sprintf("retention-%04d", n-1)) {
		t.Fatalf("ring buffer must drop oldest and keep newest (%d events)", len(events))
	}
}
