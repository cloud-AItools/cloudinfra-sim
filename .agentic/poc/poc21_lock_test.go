package poc

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// crossLock은 vCenter·NSX 교차 변경을 보호하는 단일 락이다(FR-BE-0304).
type crossLock struct{ ch chan struct{} }

func newCrossLock() *crossLock { return &crossLock{ch: make(chan struct{}, 1)} }
func (l *crossLock) Lock()     { l.ch <- struct{}{} }
func (l *crossLock) Unlock()   { <-l.ch }

// tryLock은 교착을 무한 대기 대신 시간 초과로 탐지하기 위한 것이다.
func (l *crossLock) tryLock(d time.Duration) bool {
	select {
	case l.ch <- struct{}{}:
		return true
	case <-time.After(d):
		return false
	}
}

// nsxDerivation은 vcsim 변경 핸들러(Registry.AddHandler)로 NSX Fabric VM 인벤토리를 파생하는 자리표시자다(FR-NSX-0608).
type nsxDerivation struct {
	naive    bool // true: 핸들러 안에서 교차 락을 직접 획득 / false: Outbox에 넣고 별도 소비자가 반영
	lock     *crossLock
	outbox   chan string
	applied  atomic.Int64
	timeouts atomic.Int64
}

func (d *nsxDerivation) Reference() types.ManagedObjectReference {
	return types.ManagedObjectReference{Type: "NsxDerivation", Value: "poc21"}
}

func (d *nsxDerivation) on(kind string) {
	if d.naive {
		if !d.lock.tryLock(100 * time.Millisecond) {
			d.timeouts.Add(1) // 같은 고루틴이 이미 교차 락을 쥔 상태에서 호출됨 → 실제라면 교착
			return
		}
		d.applied.Add(1)
		d.lock.Unlock()
		return
	}
	d.outbox <- kind
}

func (d *nsxDerivation) PutObject(_ *simulator.Context, o mo.Reference) {
	if o.Reference().Type == "VirtualMachine" {
		d.on("put")
	}
}

func (d *nsxDerivation) UpdateObject(_ *simulator.Context, o mo.Reference, _ []types.PropertyChange) {
	if o.Reference().Type == "VirtualMachine" {
		d.on("update")
	}
}

func (d *nsxDerivation) RemoveObject(_ *simulator.Context, ref types.ManagedObjectReference) {
	if ref.Type == "VirtualMachine" {
		d.on("remove")
	}
}

// lockGateway는 변경 요청(SOAP POST) 전체를 교차 락 안에서 처리한다.
func lockGateway(next http.Handler, l *crossLock, enabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enabled && r.Method == http.MethodPost {
			l.Lock()
			defer l.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

func powerCycle(ctx context.Context, t *testing.T, vms []*object.VirtualMachine, rounds int) time.Duration {
	t.Helper()
	start := time.Now()
	var wg sync.WaitGroup
	for _, vm := range vms {
		wg.Add(1)
		go func(vm *object.VirtualMachine) {
			defer wg.Done()
			for range rounds {
				for _, op := range []func(context.Context) (*object.Task, error){vm.PowerOff, vm.PowerOn} {
					tk, err := op(ctx)
					if err != nil {
						t.Error(err)
						return
					}
					if err := tk.Wait(ctx); err != nil {
						t.Error(err)
						return
					}
				}
			}
		}(vm)
	}
	wg.Wait()
	return time.Since(start)
}

// PoC-21: 교차 시스템 단일 락 경계와 vcsim 내부 락의 상호작용(교착 여부, 동시 요청 처리량).
func TestPoC21_CrossSystemLock(t *testing.T) {
	cases := []struct {
		name       string
		globalLock bool
		naive      bool
	}{
		{"no-global-lock", false, false},
		{"global-lock+outbox", true, false},
		{"global-lock+naive-handler", true, true},
	}
	const rounds = 5
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, vc := newVCenterHandler(t, defaultSmall())
			l := newCrossLock()
			d := &nsxDerivation{naive: tc.naive, lock: l, outbox: make(chan string, 1024)}
			m.Map().AddHandler(d)

			// Outbox 소비자: 교차 락을 잡고 NSX 저장소에 반영한다(요청 경로 밖, 비동기 — FR-BE-0403).
			done := make(chan struct{})
			go func() {
				for {
					select {
					case <-d.outbox:
						l.Lock()
						d.applied.Add(1)
						l.Unlock()
					case <-done:
						return
					}
				}
			}()
			defer close(done)

			g := startGateway(t, "127.0.0.1", map[string]http.Handler{"vcenter": lockGateway(vc, l, tc.globalLock)})
			defer g.close()
			c := login(t, g)
			ctx := context.Background()
			vms, err := find.NewFinder(c.Client, true).VirtualMachineList(ctx, "/*/vm/*")
			if err != nil {
				t.Fatal(err)
			}

			elapsed := powerCycle(ctx, t, vms, rounds)
			time.Sleep(200 * time.Millisecond) // Outbox 소진
			ops := len(vms) * rounds * 2
			t.Logf("%d VMs x %d rounds: %d power ops in %v (%.0f ops/s); derivation applied=%d, lock-timeouts(would-deadlock)=%d",
				len(vms), rounds, ops, elapsed.Round(time.Millisecond), float64(ops)/elapsed.Seconds(), d.applied.Load(), d.timeouts.Load())
			if !tc.naive && d.timeouts.Load() != 0 {
				t.Fatal("outbox design must not time out")
			}
		})
	}
}
