#!/usr/bin/env python3
"""Produce the Python reference outputs for the differential test.

Runs the REAL Python json_repair (../json_repair-main/src) over every corpus
input under 6 flag combinations and writes py_ref.jsonl (one JSON object per
line: {"i": corpus index, "c": combo name, "o": normalized output}).

Normalized output = json.dumps(value, ensure_ascii=False), i.e. object keys
in Python dict insertion order — matching Go's Dump(v, {EnsureASCII: false}),
which emits *OrderedMap (repairer output) keys in the same insertion order
(D-006). Both sides use the ", " / ": " separators. On exception:
"ERR:<ExceptionType>:<message>".

Run:  .venv-oracle/bin/python tools/diff_check/py_ref.py
"""
import json
import pathlib
import sys
import time

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT.parent / "json_repair-main" / "src"))
from json_repair import repair_json  # noqa: E402

SCHEMA_PERF = {
    "type": "array",
    "items": {
        "type": "object",
        "properties": {
            "_id": {"type": "string"},
            "index": {"type": "integer"},
            "guid": {"type": "string"},
            "isActive": {"type": "boolean"},
            "tags": {"type": "array", "items": {"type": "string"}},
            "friends": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {"id": {"type": "integer"}, "name": {"type": "string"}},
                    "required": ["id", "name"],
                },
            },
        },
        "required": ["_id", "index", "guid", "isActive", "tags", "friends"],
        "additionalProperties": True,
    },
}

COMBOS = [
    ("skipF_ssF", dict(skip_json_loads=False, stream_stable=False)),
    ("skipT_ssF", dict(skip_json_loads=True, stream_stable=False)),
    ("skipF_ssT", dict(skip_json_loads=False, stream_stable=True)),
    ("skipT_ssT", dict(skip_json_loads=True, stream_stable=True)),
    ("schema_skipF", dict(skip_json_loads=False, schema=SCHEMA_PERF)),
    ("schema_skipT", dict(skip_json_loads=True, schema=SCHEMA_PERF)),
]


def main() -> None:
    corpus_path = ROOT / "tools" / "diff_check" / "corpus.jsonl"
    out_path = ROOT / "tools" / "diff_check" / "py_ref.jsonl"
    corpus = [json.loads(line) for line in corpus_path.open(encoding="utf-8")]

    n_err = 0
    t0 = time.time()
    with out_path.open("w", encoding="utf-8") as f:
        for i, s in enumerate(corpus):
            for name, kw in COMBOS:
                try:
                    v = repair_json(s, return_objects=True, **kw)
                    out = json.dumps(v, ensure_ascii=False)
                except Exception as e:  # noqa: BLE001
                    out = f"ERR:{type(e).__name__}:{e}"
                    n_err += 1
                f.write(json.dumps({"i": i, "c": name, "o": out}, ensure_ascii=False) + "\n")
            if (i + 1) % 1000 == 0:
                print(f"  {i + 1}/{len(corpus)} inputs, {time.time() - t0:.1f}s", flush=True)
    print(f"done: {len(corpus)} inputs x {len(COMBOS)} combos, {n_err} exceptions, {time.time() - t0:.1f}s")
    print(f"-> {out_path}")


if __name__ == "__main__":
    main()
