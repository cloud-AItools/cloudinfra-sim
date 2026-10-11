package poc

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// maxVMMemoryMB는 PoC용 용량 규칙이다(FR-BE-0701의 예시 값).
const maxVMMemoryMB = 32 * 1024

// adapterFolder는 Behavior Adapter 방식 B다. vcsim Folder를 임베드하고 CreateVM_Task만 오버라이드해
// 사전조건 실패를 Task error로 돌려준다(FR-BE-0402, FR-VC-0417).
type adapterFolder struct {
	*simulator.Folder
}

func (f *adapterFolder) CreateVMTask(ctx *simulator.Context, c *types.CreateVM_Task) soap.HasFault {
	if c.Config.MemoryMB > maxVMMemoryMB {
		t := simulator.CreateTask(f.Folder, "createVm", func(*simulator.Task) (types.AnyType, types.BaseMethodFault) {
			return nil, &types.InsufficientResourcesFault{}
		})
		return &methods.CreateVM_TaskBody{Res: &types.CreateVM_TaskResponse{Returnval: t.Run(ctx)}}
	}
	return f.Folder.CreateVMTask(ctx, c)
}

func createVM(ctx context.Context, t *testing.T, c *object.Folder, pool *object.ResourcePool, memMB int64) error {
	t.Helper()
	spec := types.VirtualMachineConfigSpec{
		Name: "poc14-vm", GuestId: string(types.VirtualMachineGuestOsIdentifierOtherGuest),
		NumCPUs: 1, MemoryMB: memMB,
		Files: &types.VirtualMachineFileInfo{VmPathName: "[LocalDS_0]"},
	}
	tk, err := c.CreateVM(ctx, spec, pool, nil)
	if err != nil {
		return err // 동기 SOAP Fault
	}
	_, err = tk.WaitForResult(ctx, nil)
	return err // Task error
}

// PoC-14: 요청 경로 안에서 CreateVM_Task 사전조건 실패를 Fault로 반환할 수 있는가.
// 방식 A: Registry.Handler 훅(포크 없이 dispatch 전에 실행) → 동기 SOAP Fault.
// 방식 B: 레지스트리의 VM Folder를 래퍼(adapterFolder)로 교체해 메서드 오버라이드 → Task error.
func TestPoC14_PreconditionFault(t *testing.T) {
	for _, mode := range []string{"A-sync-fault", "B-task-error"} {
		t.Run(mode, func(t *testing.T) {
			m, vc := newVCenterHandler(t, defaultSmall())
			ctx := context.Background()

			m.Map().Handler = func(sc *simulator.Context, method *simulator.Method) (mo.Reference, types.BaseMethodFault) {
				if method.Name != "CreateVM_Task" {
					return nil, nil
				}
				req := method.Body.(*types.CreateVM_Task)
				if mode == "A-sync-fault" {
					if req.Config.MemoryMB > maxVMMemoryMB {
						return nil, &types.InsufficientResourcesFault{}
					}
					return nil, nil
				}
				return nil, nil // 방식 B는 아래에서 레지스트리 객체를 교체한다
			}
			if mode == "B-task-error" {
				// Registry.Handler의 대체 핸들러는 세션 조회(Session.Get)에 덮어써지므로,
				// vcsim 예제(example_extend_test.go)와 같이 레지스트리 객체 자체를 래퍼로 교체한다.
				for _, obj := range m.Map().All("Folder") {
					f := obj.(*simulator.Folder)
					if f.Name == "vm" {
						m.Map().Put(&adapterFolder{Folder: f})
					}
				}
			}

			g := startGateway(t, "127.0.0.1", map[string]http.Handler{"vcenter": vc})
			defer g.close()
			c := login(t, g)

			finder := find.NewFinder(c.Client, true)
			dc, err := finder.DefaultDatacenter(ctx)
			if err != nil {
				t.Fatal(err)
			}
			finder.SetDatacenter(dc)
			folders, _ := dc.Folders(ctx)
			pool, err := finder.DefaultResourcePool(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// 사전조건 위반: 거부되어야 하며 VM이 생성되지 않아야 한다.
			err = createVM(ctx, t, folders.VmFolder, pool, maxVMMemoryMB*2)
			if err == nil {
				t.Fatal("expected precondition fault")
			}
			var te task.Error
			isTaskErr := errors.As(err, &te)
			isSoapFault := soap.IsSoapFault(err)
			t.Logf("rejected: taskError=%v soapFault=%v err=%v", isTaskErr, isSoapFault, err)
			if mode == "A-sync-fault" && !isSoapFault {
				t.Fatal("mode A should return a synchronous SOAP fault")
			}
			if mode == "B-task-error" {
				if !isTaskErr {
					t.Fatal("mode B should fail the task")
				}
				if _, ok := te.Fault().(*types.InsufficientResourcesFault); !ok {
					t.Fatalf("unexpected task fault %T", te.Fault())
				}
			}
			if _, err := finder.VirtualMachine(ctx, "poc14-vm"); err == nil {
				t.Fatal("VM must not exist after rejected request")
			}

			// 정상 요청은 그대로 vcsim 동작으로 처리된다.
			if err := createVM(ctx, t, folders.VmFolder, pool, 1024); err != nil {
				t.Fatalf("valid request failed: %v", err)
			}
		})
	}
}
