"""vSphere Automation SDK 바인딩(vapi_client_bindings, MIT)에서 REST 계약을 추출한다(R-20 대체 근거, 2026-10-10 결정 ②).

모듈을 import·실행하지 않고 ast로 파싱한다. 추출 대상:
  - structs: 정식 ID(예: com.vmware.vcenter.VM.summary) → 필드(타입, optional)
  - enums:   정식 ID(예: com.vmware.vcenter.vm.power.state) → 값 목록
  - operations: 서비스 ID.operation → HTTP 메서드, URL 템플릿(/api 기준), 출력 타입(정식 ID로 해석), 오류 딕셔너리 이름,
                입력 매개변수(input), 요청 본문 매개변수(body), 경로 변수(path_vars), 쿼리(query), 분기 매개변수(dispatch, 예: action=find)
사용법: py -3 -I extract_vapi.py <bindings 루트(com/ 상위)> <out.json> <모듈 경로> [...]
"""
import ast
import json
import sys
from pathlib import Path

SIMPLE = {"StringType": "string", "IntegerType": "integer", "BooleanType": "boolean", "DoubleType": "number",
          "DateTimeType": "datetime", "SecretType": "secret", "URIType": "uri", "BlobType": "blob",
          "VoidType": "void", "DynamicStructType": "dynamic", "OpaqueType": "opaque", "AnyErrorType": "error"}


def call_name(node):
    if isinstance(node, ast.Call):
        f = node.func
        if isinstance(f, ast.Attribute):
            return f.attr
        if isinstance(f, ast.Name):
            return f.id
    return None


def type_info(node, module):
    name = call_name(node)
    if name == "OptionalType":
        inner = type_info(node.args[0], module)
        inner["optional"] = True
        return inner
    if name in ("ListType", "SetType"):
        return {"kind": "list" if name == "ListType" else "set", "item": type_info(node.args[0], module)}
    if name == "MapType":
        return {"kind": "map", "key": type_info(node.args[0], module), "value": type_info(node.args[1], module)}
    if name == "ReferenceType":
        mod = node.args[0]
        mod_s = mod.value if isinstance(mod, ast.Constant) else module
        return {"kind": "ref", "alias": f"{mod_s}:{node.args[1].value}"}
    if name == "IdType":
        return {"kind": "id"}
    if name in SIMPLE:
        return {"kind": SIMPLE[name]}
    return {"kind": "other", "src": ast.unparse(node) if node is not None else None}


class Visitor(ast.NodeVisitor):
    """클래스 중첩을 따라가며 _set_binding_type·_set_values 호출을 정식 ID와 연결한다."""

    def __init__(self, module):
        self.module = module
        self.stack = []
        self.aliases = {}   # "module:Outer.Inner" → 정식 ID
        self.structs = {}
        self.enum_values = {}  # alias → 값

    def visit_ClassDef(self, node):
        self.stack.append(node.name)
        self.generic_visit(node)
        self.stack.pop()

    def visit_Call(self, node):
        f = node.func
        if isinstance(f, ast.Attribute) and f.attr in ("_set_binding_type", "_set_values"):
            alias = f"{self.module}:" + ".".join(self.stack + [ast.unparse(f.value)])
            arg = node.args[0] if node.args else None
            if f.attr == "_set_binding_type" and call_name(arg) in ("StructType", "ErrorType", "EnumType") and arg.args \
                    and isinstance(arg.args[0], ast.Constant):
                canonical = arg.args[0].value
                self.aliases[alias] = canonical
                if call_name(arg) in ("StructType", "ErrorType") and len(arg.args) > 1 and isinstance(arg.args[1], ast.Dict):
                    self.structs[canonical] = {k.value: type_info(v, self.module)
                                               for k, v in zip(arg.args[1].keys, arg.args[1].values) if isinstance(k, ast.Constant)}
            if f.attr == "_set_values" and isinstance(arg, ast.List):
                self.enum_values[alias] = [e.args[0].value for e in arg.elts
                                           if isinstance(e, ast.Call) and e.args and isinstance(e.args[0], ast.Constant)]
        self.generic_visit(node)


def operations(tree, module):
    ops = {}
    for cls in [c for c in ast.walk(tree) if isinstance(c, ast.ClassDef) and c.name.endswith("Stub")]:
        rest, meta, errdicts, inputs, service = {}, {}, {}, {}, None
        for node in ast.walk(cls):
            if isinstance(node, ast.Assign) and isinstance(node.targets[0], ast.Name):
                name = node.targets[0].id
                if name.endswith("_error_dict") and isinstance(node.value, ast.Dict):
                    errdicts[name] = sorted(k.value for k in node.value.keys if isinstance(k, ast.Constant))
                if name.endswith("_rest_metadata") and call_name(node.value) == "OperationRestMetadata":
                    kw = {k.arg: k.value for k in node.value.keywords}
                    r = {"method": kw["http_method"].value, "url": kw["url_template"].value}
                    if isinstance(kw.get("request_body_parameter"), ast.Constant):
                        r["body"] = kw["request_body_parameter"].value
                    for key, out in (("path_variables", "path_vars"), ("query_parameters", "query"), ("dispatch_parameters", "dispatch")):
                        d = kw.get(key)
                        if isinstance(d, ast.Dict) and d.keys:
                            r[out] = {k.value: v.value for k, v in zip(d.keys, d.values)
                                      if isinstance(k, ast.Constant) and isinstance(v, ast.Constant)}
                    rest[name[: -len("_rest_metadata")]] = r
                if name.endswith("_input_type") and call_name(node.value) == "StructType" and len(node.value.args) > 1                         and isinstance(node.value.args[1], ast.Dict):
                    inputs[name[: -len("_input_type")]] = {k.value: type_info(v, module)
                                                           for k, v in zip(node.value.args[1].keys, node.value.args[1].values)
                                                           if isinstance(k, ast.Constant)}
                if name == "operations" and isinstance(node.value, ast.Dict):
                    for k, v in zip(node.value.keys, node.value.values):
                        if isinstance(v, ast.Dict):
                            d = {kk.value: vv for kk, vv in zip(v.keys, v.values) if isinstance(kk, ast.Constant)}
                            meta[k.value] = {
                                "output": type_info(d["output_type"], module) if "output_type" in d else None,
                                "errors_ref": ast.unparse(d["errors"]) if "errors" in d else None,
                            }
            if isinstance(node, ast.Call):
                for kw in node.keywords:
                    if kw.arg == "iface_name" and isinstance(kw.value, ast.Constant):
                        service = kw.value.value
        for op, r in rest.items():
            m = dict(meta.get(op, {}))
            m["errors"] = errdicts.get(m.get("errors_ref") or "", [])
            if inputs.get(op):
                m["input"] = inputs[op]
            ops[f"{service}.{op}"] = {"service": service, "operation": op, **r, **m}
    return ops


def resolve(t, aliases):
    if isinstance(t, dict):
        if t.get("kind") == "ref":
            t["type"] = aliases.get(t["alias"], "UNRESOLVED")
        for v in t.values():
            resolve(v, aliases)
    elif isinstance(t, list):
        for v in t:
            resolve(v, aliases)


def http_map(root):
    """vapi_runtime의 vapi_to_http_error_map을 읽는다(VAPI_RUNTIME 환경변수 = 런타임 압축 해제 경로)."""
    import os
    rt = os.environ.get("VAPI_RUNTIME")
    if not rt:
        return {}
    tree = ast.parse((Path(rt) / "vmware/vapi/rest/lib.py").read_text(encoding="utf-8"))
    codes, out = {}, {}
    for node in ast.walk(tree):
        if isinstance(node, ast.ClassDef) and node.name == "HTTPStatusCodes":
            for a in node.body:
                if isinstance(a, ast.Assign) and isinstance(a.value, ast.Constant):
                    codes[a.targets[0].id] = a.value.value
        if isinstance(node, ast.Assign) and isinstance(node.targets[0], ast.Name) and node.targets[0].id == "vapi_to_http_error_map":
            for k, v in zip(node.value.keys, node.value.values):
                out[k.value] = codes[v.attr]
    return out


def main(root, out, modules):
    root = Path(root)
    doc = {"source": "vapi_client_bindings 3.8.0 (vsphere-automation-sdk-python v7.0.3.2, MIT)",
           "structs": {}, "enums": {}, "operations": {}}
    aliases, enum_values = {}, {}
    for m in modules:
        module = m.replace("/", ".").removesuffix(".py")
        tree = ast.parse((root / m).read_text(encoding="utf-8"))
        v = Visitor(module)
        v.visit(tree)
        aliases.update(v.aliases)
        enum_values.update(v.enum_values)
        doc["structs"].update(v.structs)
        doc["operations"].update(operations(tree, module))
    for alias, values in enum_values.items():
        if alias in aliases:
            doc["enums"][aliases[alias]] = values
    # vAPI 표준 오류 → HTTP 상태(vapi_runtime 2.34.0 vmware/vapi/rest/lib.py vapi_to_http_error_map)
    doc["http_status"] = http_map(root)
    resolve(doc["structs"], aliases)
    resolve(doc["operations"], aliases)
    unresolved = json.dumps(doc).count("UNRESOLVED")
    Path(out).write_text(json.dumps(doc, ensure_ascii=False, indent=1, sort_keys=True), encoding="utf-8")
    print(f"structs={len(doc['structs'])} enums={len(doc['enums'])} operations={len(doc['operations'])} unresolved_refs={unresolved}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3:])
