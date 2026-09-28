#!/usr/bin/env python3
"""Extract black-box repair behaviour assertions from a Python test file and
emit a Go table-driven parity test.

Robust to @pytest.mark.parametrize (expanded via the decorator literals) and to
`repaired, logs = repair_json(...); assert repaired == expected` bindings.
No hand-escaping: inputs become Go string literals, expected objects become
JSON (json.dumps) so Go unmarshals them for comparison.
"""
import ast
import json
import sys


def go_quote(s: str) -> str:
    if "\n" not in s and "`" not in s and "\x00" not in s:
        return "`" + s + "`"
    out = []
    for ch in s:
        if ch == "\\":
            out.append("\\\\")
        elif ch == '"':
            out.append('\\"')
        elif ch == "\n":
            out.append("\\n")
        elif ch == "\t":
            out.append("\\t")
        elif ch == "\r":
            out.append("\\r")
        elif ord(ch) < 0x20:
            out.append("\\u%04x" % ord(ch))
        else:
            out.append(ch)
    return '"' + "".join(out) + '"'


def lit(node):
    return ast.literal_eval(node)


def eval_node(node, env):
    if isinstance(node, ast.Constant):
        return node.value
    if isinstance(node, ast.Name):
        if node.id in env:
            return env[node.id]
        raise ValueError("free variable: " + node.id)
    if isinstance(node, (ast.List, ast.Tuple)):
        return [eval_node(e, env) for e in node.elts]
    if isinstance(node, ast.Dict):
        return {eval_node(k, env): eval_node(v, env)
                for k, v in zip(node.keys, node.values)}
    if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.Not):
        return not eval_node(node.operand, env)
    if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.USub):
        return -eval_node(node.operand, env)
    raise ValueError("unsupported node: %r" % ast.dump(node)[:60])


def is_repair_call(node):
    return (isinstance(node, ast.Call) and isinstance(node.func, ast.Name)
            and node.func.id == "repair_json")


def repair_call_info(call, env):
    """Return (raw, skip, strict) for a repair_json call, or None."""
    if not call.args:
        return None
    try:
        raw = eval_node(call.args[0], env)
    except Exception:
        return None
    if not isinstance(raw, str):
        return None
    skip = strict = False
    for kw in call.keywords:
        try:
            v = eval_node(kw.value, env)
        except Exception:
            continue
        if kw.arg == "skip_json_loads":
            skip = bool(v)
        elif kw.arg == "strict":
            strict = bool(v)
    return raw, skip, strict


def expected_obj(node, env):
    try:
        expected = eval_node(node, env)
    except Exception:
        return None
    if isinstance(expected, str):
        try:
            expected = json.loads(expected)
        except Exception:
            return None
    try:
        return json.dumps(expected, ensure_ascii=False)
    except Exception:
        return None


def parse_argnames(node):
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return [x.strip() for x in node.value.split(",")]
    if isinstance(node, (ast.List, ast.Tuple)):
        return [eval_node(e, {}) for e in node.elts]
    return None


def parametrize_sets(func):
    sets = [{}]
    for dec in func.decorator_list:
        if (isinstance(dec, ast.Call) and isinstance(dec.func, ast.Attribute)
                and dec.func.attr == "parametrize" and len(dec.args) == 2):
            names = parse_argnames(dec.args[0])
            if not names:
                continue
            try:
                values = lit(dec.args[1])
            except Exception:
                continue
            newsets = []
            for s in sets:
                for av in values:
                    s2 = dict(s)
                    if len(names) == 1:
                        s2[names[0]] = av
                    else:
                        for n, v in zip(names, av):
                            s2[n] = v
                    newsets.append(s2)
            sets = newsets
    return sets


def extract(path, exclude=()):
    tree = ast.parse(open(path).read())
    cases = []

    def add(raw, exp, skip, strict):
        if exp is None or raw is None:
            return
        cases.append((raw, exp, bool(skip), bool(strict)))

    funcs = [n for n in ast.walk(tree) if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef))]
    for func in funcs:
        if func.name in exclude:
            continue
        for env in parametrize_sets(func):
            bindings = {}  # varname -> (raw, skip, strict)
            for stmt in ast.walk(func):
                # track: repaired = repair_json(...)  /  repaired, logs = ...
                if isinstance(stmt, ast.Assign) and is_repair_call(stmt.value):
                    info = repair_call_info(stmt.value, env)
                    if info:
                        for tgt in stmt.targets:
                            if isinstance(tgt, ast.Name):
                                bindings[tgt.id] = info
                            elif isinstance(tgt, ast.Tuple) and tgt.elts:
                                if isinstance(tgt.elts[0], ast.Name):
                                    bindings[tgt.elts[0].id] = info
                if not isinstance(stmt, ast.Assert):
                    continue
                test = stmt.test
                if not (isinstance(test, ast.Compare) and len(test.ops) == 1
                        and isinstance(test.ops[0], ast.Eq)):
                    continue
                a, b = test.left, test.comparators[0]
                if is_repair_call(a) or is_repair_call(b):
                    call = a if is_repair_call(a) else b
                    expnode = b if call is a else a
                    if isinstance(expnode, ast.Tuple):
                        continue  # (result, logs) logging-form assert; not a plain value
                    info = repair_call_info(call, env)
                    if info:
                        raw, skip, strict = info
                        add(raw, expected_obj(expnode, env), skip, strict)
                elif (isinstance(a, ast.Name) and a.id in bindings
                      and a.id not in env):
                    if isinstance(b, ast.Tuple):
                        continue
                    raw, skip, strict = bindings[a.id]
                    add(raw, expected_obj(b, env), skip, strict)

    seen, uniq = set(), []
    for c in cases:
        if c in seen:
            continue
        seen.add(c)
        uniq.append(c)
    return uniq


def main():
    path = sys.argv[1]
    fnname = sys.argv[2] if len(sys.argv) > 2 else "TestRepairParity"
    exclude = set()
    if len(sys.argv) > 3:
        exclude = set(sys.argv[3].split(","))
    cases = extract(path, exclude)
    print("// Code generated from " + path + " by tools/gen_parity_test.py. DO NOT EDIT BY HAND.")
    print("package json_repair")
    print()
    print("import (")
    print("\t\"testing\"")
    print(")")
    print()
    print("func " + fnname + "(t *testing.T) {")
    print("\tcases := []struct {")
    print("\t\tin     string")
    print("\t\twant   string")
    print("\t\tskip   bool")
    print("\t\tstrict bool")
    print("\t}{")
    for raw, exp, skip, strict in cases:
        # The trailing comment is a Go block comment `/* ... */` which does not
        # nest. If `raw` itself contains `/*` or `*/` (e.g. a comment test), the
        # comment would close early and leak code. Strip `*` and `/` to be safe.
        comment = raw[:48].replace("\n", "\\n").replace("\t", " ")
        comment = comment.replace("*", "").replace("/", "")
        print("\t\t{in: " + go_quote(raw) + ", want: " + go_quote(exp)
              + ", skip: " + ("true" if skip else "false")
              + ", strict: " + ("true" if strict else "false") + " /* " + comment + " */},")
    print("\t}")
    print("\tfor _, c := range cases {")
    print("\t\tt.Run(c.in, func(t *testing.T) {")
    print("\t\t\tassertRepair(t, c.in, c.want, c.skip, c.strict)")
    print("\t\t})")
    print("\t}")
    print("}")


if __name__ == "__main__":
    main()
