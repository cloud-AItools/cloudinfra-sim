package contract

import (
	"context"
	"testing"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-MET-0203: Task info.entityName은 엔터티 이름이다(vcsim은 MoRef 값을 넣었다, R-32). info.reason은 요청 사용자.
func TestTaskInfoEntityName(t *testing.T) {
	e := testenv.Start(t, nil)
	c := e.Login(testenv.VCenterVersion())
	ctx := context.Background()
	vms, err := find.NewFinder(c.Client).VirtualMachineList(ctx, "*")
	if err != nil || len(vms) == 0 {
		t.Fatalf("vms: %v", err)
	}
	vm := vms[0]
	name, _ := vm.ObjectName(ctx)
	tk, err := vm.PowerOff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = tk.Wait(ctx)
	var info mo.Task
	if err := c.RetrieveOne(ctx, tk.Reference(), []string{"info"}, &info); err != nil {
		t.Fatal(err)
	}
	if info.Info.EntityName != name || info.Info.Entity == nil || info.Info.Entity.Value != vm.Reference().Value {
		t.Fatalf("entity %v entityName %q, want %s %q", info.Info.Entity, info.Info.EntityName, vm.Reference().Value, name)
	}
	if r, ok := info.Info.Reason.(*types.TaskReasonUser); !ok || r.UserName != e.User("vcenter").Name {
		t.Fatalf("reason %#v", info.Info.Reason)
	}
}
