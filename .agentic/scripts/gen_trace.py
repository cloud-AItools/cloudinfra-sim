"""요구사항 검증 추적표 생성기 (PRD-2 TR-COV-0103, PRD-4 §15.5, WP 0.17).

PRD-1~3의 요구사항 표와 PRD-4 작업 패키지 표를 읽어 다음을 만든다.
  - MVP(●, 채택된 ○)와 Phase 1 대상 ID의 개별 추적 행
  - 자동 검사: ID 중복, 미정의 참조, WP·검증 방법 미배정
사용법: py -3 -I gen_trace.py <PRD v1.1 폴더> <출력 .md> [채택된 ○ ID,...]
"""
import re
import sys
from collections import defaultdict
from pathlib import Path

IDPAT = r"(?:FR|TR|NFR)(?:-[A-Z0-9]+)?-\d{4}"


def rows(path):
    for line in path.read_text(encoding="utf-8").splitlines():
        m = re.match(r"^\|\s*(" + IDPAT + r")\s*\|", line)
        if m:
            yield m.group(1), [c.strip() for c in line.strip().strip("|").split("|")]


def expand(text):
    out = set()
    for a, b in re.findall("(" + IDPAT + r")\s*~\s*(" + IDPAT + r"|\d{4})", text):
        prefix = a[:-4]
        if re.fullmatch(r"\d{4}", b):
            b = prefix + b
        if b[:-4] == prefix:
            out.update(prefix + "%04d" % n for n in range(int(a[-4:]), int(b[-4:]) + 1))
    out.update(re.findall(IDPAT, text))
    # 축약 표기(FR-VC-0201, 0203 / TR-COV-0401·0402). 날짜(2026-10-10)처럼 뒤에 '-'가 오는 숫자는 제외한다.
    for m in re.finditer("(" + IDPAT + r")((?:\s*[·,]\s*\d{4}(?![\d-])(?:\s*~\s*\d{4})?)+)", text):
        prefix = m.group(1)[:-4]
        for a, b in re.findall(r"(\d{4})(?![\d-])(?:\s*~\s*(\d{4}))?", m.group(2)):
            rng = range(int(a), int(b or a) + 1)
            out.update(prefix + "%04d" % n for n in rng)
    return out


PHASE_0A = {"0.1", "0.4", "0.6", "0.8", "0.11", "0.12", "0.13", "0.14", "0.16", "0.18"}
PHASE_2B = {"2.9", "2.12"}
PHASE_4B = {"4.6", "4.7", "4.9", "4.11"}


def phase(wp):
    major, _ = wp.split(".")
    if major == "0":
        return "0a" if wp in PHASE_0A else "0b"
    if major == "2":
        return "2b" if wp in PHASE_2B else "2a"
    if major == "4":
        return "4b" if wp in PHASE_4B else "4a"
    return major


PHASE_ORDER = ["0a", "0b", "1", "2a", "2b", "3", "4a", "4b", "5"]
# CURRENT_PHASE는 진행이 끝난 마지막 단계다. 목표 단계가 이 단계 이내인 요구사항은 MVP가 아니어도 개별 행으로 추적한다.
CURRENT_PHASE = "5"

# 검증 방법과 AC는 PRD-4 §15.5 요구사항 범위 표를 따른다.
METHOD = [
    ("FR-VC", "Contract·Stateful·선정 SDK(govmomi)", "AC-02~04, AC-06, AC-07, AC-16, AC-17"),
    ("FR-NSX", "Contract·Stateful(OpenAPI 계약)", "AC-02~07, AC-09"),
    ("FR-BE", "Unit·동시성/Stateful·설계 검토", "AC-04~10, AC-19"),
    ("FR-DM", "모델 검토·ID 계약·무결성 시험", "AC-06, AC-09, AC-10, AC-19"),
    ("FR-API", "Contract·Negative·기동 모드 시험", "AC-02, AC-03, AC-08, AC-13, AC-15~17"),
    ("FR-FLT", "Negative·Fault 주입 시험", "AC-05, AC-08, AC-13"),
    ("FR-MET", "Contract·결정론 시험", "AC-03, AC-06, AC-07, AC-19"),
    ("TR-SCN", "관리 API 기능 시험", "AC-10~12, AC-19"),
    ("TR-E2E", "자체 E2E(성공 기준별)", "AC-02, AC-04, AC-05, AC-11, AC-17"),
    ("TR-COV", "추적표·Coverage 정합성 검사", "AC-03, AC-11~17, AC-19"),
    ("NFR-01", "Footprint·설치 시험", "AC-01, AC-18"),
    ("NFR-02", "동시성·무결성·복구 시험", "AC-06, AC-09, AC-10, AC-19"),
    ("NFR-03", "보안 검사(바인딩·TLS·인증·마스킹)", "AC-08, AC-14, AC-18, AC-19"),
    ("NFR-04", "로그 형식·상관관계 검사", "AC-07, AC-11~13, AC-19"),
    ("NFR-05", "보존 한도·장시간 실행 시험", "AC-12, AC-14, AC-18, AC-19"),
    ("NFR-ARC", "아키텍처 검토·설치·CI 시험", "AC-01, AC-11, AC-17~19"),
]

# Phase 0a/0b PoC 근거 (POC_PLAN.md). 최종 통과 증거가 아니라 실현 가능성 근거다.
POC = {
    "PoC-01": "FR-VC-0201, FR-API-0201", "PoC-02": "FR-API-0202", "PoC-03": "FR-VC-0205, FR-VC-0510, FR-DM-0204",
    "PoC-04": "FR-VC-0401, FR-VC-0501 ~ FR-VC-0506", "PoC-05": "FR-VC-0507 ~ FR-VC-0509",
    "PoC-08": "FR-NSX-0211, FR-NSX-0401 ~ FR-NSX-0406", "PoC-09": "FR-NSX-0408, FR-NSX-0409",
    "PoC-10": "FR-NSX-0501 ~ FR-NSX-0503, FR-NSX-0220", "PoC-12": "FR-VC-0102, NFR-ARC-0107, NFR-ARC-0304",
    "PoC-13": "FR-VC-0604 ~ FR-VC-0606", "PoC-14": "FR-BE-0401, FR-BE-0402, FR-VC-0417",
    "PoC-16": "FR-NSX-0223 ~ FR-NSX-0225", "PoC-17": "FR-NSX-0105 ~ FR-NSX-0107, NFR-0109, NFR-0110",
    "PoC-19": "NFR-0104, NFR-0107 ~ NFR-0113, NFR-ARC-0307", "PoC-20": "TR-COV-0304",
    "PoC-21": "FR-BE-0304, FR-DM-0306, NFR-0201",
}


def method_of(rid):
    for prefix, m, ac in METHOD:
        if rid.startswith(prefix + "-") or (prefix.startswith("NFR-0") and rid.startswith(prefix)):
            return m, ac
    return "", ""


def e2e_phase(rid):
    n = int(rid[-4:-2])
    return {1: "1", 2: "2a", 3: "2a", 4: "2a", 5: "2a", 6: "2a", 7: "4b", 8: "3", 9: "4a", 10: "4a"}[n]


def test_tags(repo):
    """테스트 코드의 T-<요구사항 ID> 표기를 테스트 함수별로 모은다(직전 주석 포함)."""
    tags = defaultdict(set)  # 요구사항 ID → 테스트 이름
    for root in ("30-implementation", "40-test"):
        for p in (repo / root).rglob("*_test.go"):
            pending = set()
            current = None
            for line in p.read_text(encoding="utf-8").splitlines():
                m = re.match(r"^func (Test\w+)\(", line)
                if m:
                    current = m.group(1)
                    for rid in pending:
                        tags[rid].add(current)
                    pending = set()
                    continue
                found = set()
                for a, b in re.findall(r"T-(" + IDPAT + r")(?:\s*~\s*(\d{4}))?", line):
                    if b:
                        found.update(a[:-4] + "%04d" % n for n in range(int(a[-4:]), int(b) + 1))
                    else:
                        found.add(a)
                if line.startswith("//"):
                    pending |= found
                elif current:
                    for rid in found:
                        tags[rid].add(current)
                if line.startswith("}"):
                    current = None
    return tags


def test_results(repo):
    """go test -json 결과(40-test/results*.json)에서 최상위 테스트의 통과·실패를 읽는다."""
    import json
    passed, failed = set(), set()
    for p in (repo / "40-test").glob("results*.json"):
        for line in p.read_text(encoding="utf-8", errors="replace").splitlines():
            if not line.startswith("{"):
                continue
            try:
                e = json.loads(line)
            except ValueError:
                continue
            name = e.get("Test", "")
            if not name or "/" in name:
                continue
            if e.get("Action") == "pass":
                passed.add(name)
            elif e.get("Action") == "fail":
                failed.add(name)
    return passed, failed


def main(prd_dir, out, adopted):
    prd = Path(prd_dir)
    reqs, dups = {}, []
    for f in sorted(prd.glob("cloudinfra-sim-PRD-[123]-*.md")):
        for rid, cells in rows(f):
            if rid in reqs:
                dups.append(rid)
            title = cells[1]
            if rid.startswith("TR-E2E"):  # 시나리오 표는 '구분' 열이므로 시나리오 번호와 함께 표시
                title = f"Scenario {rid[-4:-2]} {cells[1]}"
            reqs[rid] = {"title": title, "mvp": cells[-2], "status": cells[-1], "doc": f.name[15:20]}

    p4 = (prd / "cloudinfra-sim-PRD-4-개발계획-검토.md").read_text(encoding="utf-8")
    wp_of = defaultdict(list)
    for line in p4.splitlines():
        m = re.match(r"^\| WP (\d+\.\d+) \|", line)
        if m:
            for rid in expand(line):
                wp_of[rid].append(m.group(1))
    poc_of = defaultdict(list)
    for poc, ids in POC.items():
        for rid in expand(ids):
            poc_of[rid].append(poc)

    # 자동 검사
    undefined = set()
    for f in list(prd.glob("*.md")):
        for rid in expand(f.read_text(encoding="utf-8")):
            if rid not in reqs:
                undefined.add((f.name, rid))
    no_wp = [r for r, v in reqs.items() if v["status"] != "보류" and not wp_of.get(r) and not r.startswith("TR-E2E")]

    # ○ 항목 MVP 채택 결과(PRD-4 §15.3, D2 확정): 40-test/o-decisions.yaml — "ID: 채택|제외 — 근거"
    odec = {}
    od = prd.parent.parent / "40-test" / "o-decisions.yaml"
    if od.exists():
        for line in od.read_text(encoding="utf-8").splitlines():
            m = re.match(r"^\s*(" + IDPAT + r"):\s*(.+)$", line)
            if m:
                odec[m.group(1)] = m.group(2).strip().strip('"')
    adopted = set(adopted) | {r for r, t in odec.items() if t.startswith("채택")}

    def row_phase(rid):
        """완료 목표 단계: MVP ●는 Phase 2b 이내 WP 중 가장 늦은 단계, 그 외는 배정 WP 중 가장 늦은 단계."""
        if rid.startswith("TR-E2E"):
            return e2e_phase(rid)
        ps = [phase(w) for w in wp_of.get(rid, [])]
        if not ps:
            return "—"
        if reqs[rid]["mvp"] == "●" or rid in adopted:
            mvp_ps = [p for p in ps if PHASE_ORDER.index(p) <= PHASE_ORDER.index("2b")]
            ps = mvp_ps or ps
        return max(ps, key=PHASE_ORDER.index)

    def start_phase(rid):
        ps = [phase(w) for w in wp_of.get(rid, [])]
        return min(ps, key=PHASE_ORDER.index) if ps else row_phase(rid)

    repo = prd.parent.parent
    tags = test_tags(repo)
    passed, failed = test_results(repo)

    reviews = {}
    rv = repo / "40-test" / "review-evidence.yaml"
    if rv.exists():
        for line in rv.read_text(encoding="utf-8").splitlines():
            m = re.match(r"^\s*(" + IDPAT + r"):\s*(.+)$", line)
            if m:
                reviews[m.group(1)] = m.group(2).strip().strip('"')

    carry = {}
    co = repo / "40-test" / "carryover.yaml"
    if co.exists():
        for line in co.read_text(encoding="utf-8").splitlines():
            m = re.match(r"^\s*(" + IDPAT + r"):\s*(.+)$", line)
            if m:
                carry[m.group(1)] = m.group(2).strip().strip('"')

    def result(rid):
        tests = sorted(tags.get(rid, ()))
        if not tests and rid in carry:
            return "이월: " + carry[rid]
        if not tests and rid in reviews:
            return "자체 검토: " + reviews[rid]
        if not tests:
            return "미실행"
        if any(x in failed for x in tests):
            return "실패: " + ", ".join(x for x in tests if x in failed)
        done = [x for x in tests if x in passed]
        if not done:
            return "미실행(시험 작성됨)"
        return "통과: " + ", ".join(done)

    selected = []
    for rid, v in reqs.items():
        if v["status"] == "보류":
            continue
        is_mvp = v["mvp"] == "●" or (v["mvp"] == "○" and rid in adopted)
        done_phase = PHASE_ORDER.index(row_phase(rid)) <= PHASE_ORDER.index(CURRENT_PHASE) if row_phase(rid) in PHASE_ORDER else False
        if is_mvp or start_phase(rid) in ("0a", "0b", "1") or done_phase:
            selected.append(rid)

    def sortkey(rid):
        return (["FR", "TR", "NFR"].index(rid.split("-")[0]), rid)

    lines = []
    lines.append("| 요구사항 ID | 요구사항 | MVP | 적용 | 주 WP | 완료 목표 단계 | 검증 방법 | 시험·검토 ID | 연결 AC | Phase 0 근거 | 결과 |")
    lines.append("|---|---|---|---|---|---|---|---|---|---|---|")
    for rid in sorted(selected, key=sortkey):
        v = reqs[rid]
        m, ac = method_of(rid)
        wps = ", ".join("WP " + w for w in sorted(set(wp_of.get(rid, [])), key=lambda s: [int(x) for x in s.split(".")]))
        if rid.startswith("TR-E2E"):
            wps = "시나리오 게이트"
        adopt = "적용" if v["mvp"] != "○" else "적용(○ 채택)"
        poc = ", ".join(poc_of.get(rid, [])) or "—"
        lines.append(f"| {rid} | {v['title']} | {v['mvp']} | {adopt} | {wps} | Phase {row_phase(rid)} | {m} | T-{rid} | {ac} | {poc} | {result(rid)} |")

    held = sorted((r for r, v in reqs.items() if v["status"] == "보류"), key=sortkey)
    excl = {"FR-NSX-0230": "D1", "FR-BE-0305": "D3", "FR-BE-0405": "D3", "NFR-ARC-0302": "D3"}
    summary = {
        "total": len(reqs), "selected": len(selected), "held": len(held),
        "mvp": sum(1 for v in reqs.values() if v["mvp"] == "●"),
        "open_o": sorted(r for r, v in reqs.items() if v["mvp"] == "○" and r not in adopted and r not in odec and v["status"] != "보류"),
    }
    text = Path(out).read_text(encoding="utf-8") if Path(out).exists() else ""
    begin, end = "<!-- TRACE:BEGIN -->", "<!-- TRACE:END -->"
    body = [begin, ""]
    body.append(f"자동 생성: `.agentic/scripts/gen_trace.py` · 요구사항 {summary['total']}건 중 개별 행 {summary['selected']}건"
                f"(MVP ● {summary['mvp']}건 + Phase 0·1 대상 + 목표 단계 Phase {CURRENT_PHASE} 이내), 보류 {summary['held']}건.")
    body.append("")
    body.append("**자동 검사 결과**")
    body.append("")
    body.append(f"- ID 중복: {len(dups)}건{(' — ' + ', '.join(dups)) if dups else ''}")
    body.append(f"- 미정의 참조: {len(undefined)}건{(' — ' + ', '.join(sorted(r for _, r in undefined))) if undefined else ''}")
    body.append(f"- WP 미배정(보류·TR-E2E 제외): {len(no_wp)}건{(' — ' + ', '.join(sorted(no_wp))) if no_wp else ''}")
    body.append(f"- 결정 대기 ○ 항목: {len(summary['open_o'])}건 — {', '.join(summary['open_o'])}")
    n_pass = sum(1 for r in selected if result(r).startswith("통과"))
    n_fail = sum(1 for r in selected if result(r).startswith("실패"))
    n_rev = sum(1 for r in selected if result(r).startswith("자체 검토"))
    n_carry = sum(1 for r in selected if result(r).startswith("이월"))
    body.append(f"- 시험 결과(개별 행 기준): 시험 통과 {n_pass}건, 실패 {n_fail}건, 자체 검토 {n_rev}건, 이월 {n_carry}건, 미실행 {len(selected) - n_pass - n_fail - n_rev - n_carry}건 "
                f"(근거: 40-test/results*.json, 테스트 코드의 T-<ID> 표기, 40-test/review-evidence.yaml). "
                f"'자체 검토'는 AI 에이전트의 문서·코드 검토이며 사람의 검토 승인을 대신하지 않는다")
    body.append("")
    body.append("**보류·적용 제외**")
    body.append("")
    body.append("| 요구사항 ID | 요구사항 | 제외 근거 |")
    body.append("|---|---|---|")
    for rid in held:
        body.append(f"| {rid} | {reqs[rid]['title']} | 적용 제외 — {excl.get(rid, 'D5·D13')} |")
    body.append("")
    if odec:
        body.append("**○ 항목 MVP 채택 결과** (`40-test/o-decisions.yaml`, PRD-4 §15.3·§19)")
        body.append("")
        body.append("| 요구사항 ID | 요구사항 | 결과 |")
        body.append("|---|---|---|")
        for rid in sorted(odec, key=sortkey):
            body.append(f"| {rid} | {reqs[rid]['title'] if rid in reqs else '(미정의)'} | {odec[rid]} |")
        body.append("")
    body.append("**개별 추적 행**")
    body.append("")
    body.extend(lines)
    body.append("")
    body.append(end)
    block = "\n".join(body)
    if begin in text:
        text = text[: text.index(begin)] + block + text[text.index(end) + len(end):]
    else:
        text = text.rstrip("\n") + "\n\n" + block + "\n"
    Path(out).write_text(text, encoding="utf-8")
    print(f"rows={len(selected)} dup={len(dups)} undefined={len(undefined)} no_wp={len(no_wp)} open_o={len(summary['open_o'])}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], set(sys.argv[3].split(",")) if len(sys.argv) > 3 and sys.argv[3] else set())
