"""pyVmomi 범용 SDK 통합 시험 (PRD-2 TR-COV-0305, NFR-ARC-0109).

Go 시험(sdk/pyvmomi_test.go)이 시뮬레이터를 띄우고 환경변수로 접속 정보를 넘겨 실행한다.
표준 unittest로 작성했으므로 pytest로도 실행할 수 있다. 인증서는 이름 없이 IP로 접속하므로 지문으로 검증한다.
pyVmomi는 vimServiceVersions.xml로 버전을 협상한다(FR-VC-0605). 통과가 다른 SDK의 통과를 보장하지 않는다.
"""
import hashlib
import os
import ssl
import sys
import unittest

sys.path.insert(0, os.environ["PYVMOMI_DIR"])
from pyVim.connect import SmartConnect, Disconnect  # noqa: E402
from pyVim.task import WaitForTask  # noqa: E402
from pyVmomi import vim, vmodl  # noqa: E402

HOST, PORT = os.environ["SIM_HOST"], int(os.environ["SIM_PORT"])
USER, PASSWORD = os.environ["SIM_USER"], os.environ["SIM_PASSWORD"]
THUMBPRINT = os.environ["SIM_THUMBPRINT"].replace(":", "").lower()


def connect(password=PASSWORD):
    return SmartConnect(host=HOST, port=PORT, user=USER, pwd=password, disableSslCertValidation=True)


def objects(si, kind):
    view = si.content.viewManager.CreateContainerView(si.content.rootFolder, [kind], True)
    try:
        return list(view.view)
    finally:
        view.DestroyView()


class PyVmomiTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        der = ssl.get_server_certificate((HOST, PORT))
        got = hashlib.sha256(ssl.PEM_cert_to_DER_cert(der)).hexdigest()
        if got != THUMBPRINT:
            raise RuntimeError("certificate thumbprint mismatch")
        cls.si = connect()

    @classmethod
    def tearDownClass(cls):
        Disconnect(cls.si)

    def test_01_about_and_version(self):
        about = self.si.content.about
        # 기대값은 활성 프로파일 값(SIM_ABOUT_*, Go 시험이 프로파일에서 넘긴다)
        self.assertEqual(about.version, os.environ["SIM_ABOUT_VERSION"])
        self.assertEqual(about.build, os.environ["SIM_ABOUT_BUILD"])
        self.assertEqual(about.apiVersion, os.environ["SIM_ABOUT_API"])
        # pyVmomi는 서버가 알리는 최신 버전(프로파일 api_version)으로 협상해야 한다
        self.assertEqual(self.si._stub.versionId, '"urn:vim25/%s"' % os.environ["SIM_ABOUT_API"])

    def test_02_inventory(self):
        self.assertEqual(len(objects(self.si, vim.VirtualMachine)), 20)
        self.assertEqual(len(objects(self.si, vim.HostSystem)), 3)
        self.assertEqual(len(objects(self.si, vim.Datastore)), 2)

    def test_03_vm_lifecycle(self):
        dc = self.si.content.rootFolder.childEntity[0]
        cluster = objects(self.si, vim.ClusterComputeResource)[0]
        ds = objects(self.si, vim.Datastore)[0]
        spec = vim.vm.ConfigSpec(name="sdk-pyvmomi", guestId="otherGuest64", numCPUs=1, memoryMB=512,
                                 files=vim.vm.FileInfo(vmPathName="[%s]" % ds.name))
        task = dc.vmFolder.CreateVM_Task(config=spec, pool=cluster.resourcePool)
        WaitForTask(task, si=self.si)
        vm = task.info.result
        self.assertIsInstance(vm, vim.VirtualMachine)
        self.assertEqual(vm.runtime.powerState, "poweredOff")

        WaitForTask(vm.ReconfigVM_Task(vim.vm.ConfigSpec(numCPUs=2, memoryMB=1024)), si=self.si)
        self.assertEqual((vm.config.hardware.numCPU, vm.config.hardware.memoryMB), (2, 1024))

        WaitForTask(vm.PowerOnVM_Task(), si=self.si)
        self.assertEqual(vm.runtime.powerState, "poweredOn")
        # 켜진 VM 삭제는 Task error(InvalidPowerState)
        with self.assertRaises(vim.fault.InvalidPowerState):
            WaitForTask(vm.Destroy_Task(), si=self.si)
        WaitForTask(vm.PowerOffVM_Task(), si=self.si)

        # 같은 폴더의 같은 이름은 DuplicateName
        with self.assertRaises(vim.fault.DuplicateName):
            WaitForTask(dc.vmFolder.CreateVM_Task(config=spec, pool=cluster.resourcePool), si=self.si)

        moid = vm._moId
        WaitForTask(vm.Destroy_Task(), si=self.si)
        with self.assertRaises(vmodl.fault.ManagedObjectNotFound):
            vim.VirtualMachine(moid, self.si._stub).name

    def test_04_invalid_login(self):
        with self.assertRaises(vim.fault.InvalidLogin):
            connect(password="wrong-" + PASSWORD)


if __name__ == "__main__":
    unittest.main(argv=[sys.argv[0], "-v"])
