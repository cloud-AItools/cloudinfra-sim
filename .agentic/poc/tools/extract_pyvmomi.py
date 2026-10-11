"""pyVmomi _typeinfo_vim.py에서 타입·속성·메서드별 도입 버전을 추출한다(FR-VC-0604, PoC-13 보완).

모듈을 import하지 않고 텍스트로만 파싱한다. 사용법:
    py -3 -I extract_pyvmomi.py <typeinfo.py> [<typeinfo.py> ...] <out.json>
    (vim 타입은 _typeinfo_vim.py, PropertyCollector 등 vmodl 타입은 _typeinfo_query.py·_typeinfo_core.py)
"""
import ast
import json
import re
import sys

ADD_VERSION = re.compile(r'^AddVersion\("([^"]+)", "([^"]+)", "([^"]+)"')
ADD_PARENT = re.compile(r'^AddVersionParent\("([^"]+)", "([^"]+)"\)')
CREATE = re.compile(r'^Create(DataType|ManagedType|EnumType)\((.*)\)\s*$')
FLAG = re.compile(r'\bF_[A-Z_]+(?:\s*\|\s*F_[A-Z_]+)*\b')  # F_OPTIONAL | F_SECRET 등 조합 포함


def parse_args(text):
    # F_OPTIONAL 등 플래그 상수를 문자열 리터럴로 바꿔 ast.literal_eval로 안전하게 읽는다.
    text = FLAG.sub(lambda m: repr(m.group(0)), text)  # 플래그는 문자열로 보존(F_OPTIONAL 판정)
    return ast.literal_eval("(" + text + ",)")


def props_of(props):
    # 선언 순서(WSDL sequence 순서)를 보존한 [이름, vmodl 타입, 도입 버전, 선택 여부, 링크 여부] 목록(스키마 대조 검사기용).
    # F_LINK 속성은 대상 객체 대신 그 key 문자열로 직렬화된다(pyVmomi SoapAdapter).
    return [[p[0], p[1], p[2], "F_OPTIONAL" in str(p[3]), bool(re.search(r"\bF_LINK\b", str(p[3])))] for p in (props or [])]


def main(srcs, out):
    versions, types, parents = {}, {}, {}
    lines = []
    for src in srcs:
        with open(src, encoding="utf-8") as f:
            lines.extend(f)
    if True:
        for line in lines:
            line = line.rstrip("\n")
            m = ADD_VERSION.match(line)
            if m and m.group(1).startswith("vim.version."):  # version1(2.0)은 vim2 네임스페이스로 선언됨
                versions[m.group(1)] = m.group(3)
                continue
            m = ADD_PARENT.match(line)
            if m:
                parents.setdefault(m.group(2), set()).add(m.group(1))  # parent 버전 → 이를 포함하는 버전
                continue
            m = CREATE.match(line)
            if not m:
                continue
            kind, args = m.group(1), parse_args(m.group(2))
            if kind == "DataType":
                name, wsdl, parent, ver, props = args[:5]
                types[wsdl] = {"kind": "data", "name": name, "parent": parent, "since": ver,
                               "fields": {p[0]: p[2] for p in (props or [])}, "props": props_of(props)}
            elif kind == "ManagedType":
                name, wsdl, parent, ver, props, methods = args[:6]
                types[wsdl] = {"kind": "managed", "name": name, "parent": parent, "since": ver,
                               "fields": {p[0]: p[2] for p in (props or [])}, "props": props_of(props),
                               "methods": {mm[1]: mm[2] for mm in (methods or [])},
                               # 반환 [타입, Task 결과 타입]·선언 Fault(스키마 대조 검사기용)
                               "results": {mm[1]: [mm[4][1], mm[4][2]] for mm in (methods or [])},
                               "faults": {mm[1]: list(mm[6] or []) for mm in (methods or [])},
                               # 메서드 권한(RBAC, FR-API-0211): vSphere 권한 ID(예: VirtualMachine.Interact.PowerOn)
                               "privileges": {mm[1]: mm[5] for mm in (methods or []) if mm[5]}}
            else:
                name, wsdl, ver, values = args[:4]
                types[wsdl] = {"kind": "enum", "name": name, "since": ver, "values": list(values)}

    # wire 버전이 없는 vmodl 버전은 그것을 포함하는 가장 이른 vim 버전의 wire 버전으로 정한다.
    def key(s):
        return [int(x) if x.isdigit() else 0 for x in re.split(r"[.u]", s)]
    for vid, children in parents.items():
        if versions.get(vid):
            continue
        wires = [versions[c] for c in children if versions.get(c) and re.fullmatch(r"[\d.]+", versions[c])]
        if wires:
            versions[vid] = min(wires, key=key)

    def wire(v):
        return versions.get(v, v)

    for t in types.values():
        t["since"] = wire(t["since"])
        for p in t.get("props", []):
            p[2] = wire(p[2])
        for k in ("fields", "methods"):
            if k in t:
                t[k] = {n: wire(v) for n, v in t[k].items()}

    with open(out, "w", encoding="utf-8") as f:
        json.dump({"source": [s.replace("\\", "/").rsplit("/", 1)[-1] for s in srcs], "versions": versions, "types": types}, f, ensure_ascii=False, indent=0, sort_keys=True)
    n_fields = sum(len(t.get("fields", {})) for t in types.values())
    n_methods = sum(len(t.get("methods", {})) for t in types.values())
    print(f"versions={len(versions)} types={len(types)} fields={n_fields} methods={n_methods}")


if __name__ == "__main__":
    main(sys.argv[1:-1], sys.argv[-1])
