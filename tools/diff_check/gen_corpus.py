#!/usr/bin/env python3
"""Generate the differential-test corpus (deterministic).

Writes corpus.jsonl: one JSON-encoded input string per line. Both the Python
reference runner (py_ref.py) and the Go test (diff_parity_test.go) read this
same file, so the two implementations are always fed byte-identical inputs.

Composition:
  - seed documents: testdata/valid.json, testdata/invalid.json, a sample
    document matching schema_perf
  - ~50 hand-picked "nasty" strings covering the heuristic surface
    (comments, tuples, smart quotes, code fences, regex char classes,
    bare literals, unterminated strings, escapes, far quotes, ...)
  - systematic mutations per seed: truncations, single-char deletions,
    insertions and replacements from a junk alphabet, whitespace variants

Run:  python tools/diff_check/gen_corpus.py
"""
import json
import pathlib
import random

SEED = 20260920
ROOT = pathlib.Path(__file__).resolve().parents[2]

JUNK = (
    list('{}[]",:')
    + list(' \\n\t\'`')
    + list('@#$%^&*()=+~/;|<>?')
)

# A sample document that validates against schema_perf.
SCHEMA_SAMPLE = json.dumps(
    [
        {
            "_id": "655b66256574f09bdae8abe8",
            "index": 0,
            "guid": "31082ae3-b0f3-4406-90f4-cc450bd4379d",
            "isActive": False,
            "balance": "$2,562.78",
            "tags": ["a,b", "c}d", "e\"f"],
            "friends": [{"id": 1, "name": "Glover Rivas"}, {"id": 2, "name": "x"}],
        },
        {
            "_id": "655b66256574f09bdae8abe9",
            "index": 1,
            "guid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
            "isActive": True,
            "balance": "$1,000.00",
            "tags": [],
            "friends": [],
        },
    ],
    indent=2,
)

NASTY = [
    # comments
    '{// comment\n"a": 1}',
    '{/* block */ "a": 1}',
    '["a", # hash comment\n1]',
    '{ "a": 1 // trailing\n }',
    '{"a": 1} // after',
    '# only a comment',
    '/* unterminated comment "a": 1',
    # python-style
    "{'a': 1}",
    "{'a': 1, 'b': 2}",
    '{"a": (1, 2, 3)}',
    '{"a": (1)}',
    "(1, 2, 3)",
    "(1)",
    '{"a": None}',
    '{"a": none}',
    '{"a": TRUE, "b": False}',
    "[None, True, null, true]",
    "{'a': 'single quoted'}",
    # bare / mixed delimiters
    '{a: 1}',
    '{a: 1, b: 2}',
    '{"a": 1, b: 2}',
    '{"a": "x", "b"',
    '{"a": "x" "b": 2}',
    '{"a": 1 "b": 2}',
    '{"a":1,"b":2}',
    '{,}',
    '{"a":,}',
    '{"a" : 1 }',
    # strings
    '"unterminated',
    '"unterminated \\',
    '"a \\" b"',
    '{"a": "x", "b": "y',
    '{"a": "he said \"hi\""}',
    '{"a": "line1\nline2"}',
    '{"a": "tab\there"}',
    '{"a": "\u00e9\u4e2d\u6587"}',
    '{"a": "smart \u201cquotes\u201d here"}',
    '{"a": \u201csmart value\u201d}',
    '{"a": ["x", "y"]}',
    '{"a": "v}alue"}',
    '{"a": "v,alue"}',
    '{"a": "v\\"alue"}',
    # regex char class / fences
    '{"a": "pattern [\'\\\"]+ end"}',
    '{"a": "regex [^\'\\\"]* end"}',
    '{"a": "```python\ncode ````"}',
    '```json\n{"a": 1}\n```',
    # structure
    '[]',
    '[1, 2, 3',
    '[1, 2, 3,',
    '{"a": [1, 2,]',
    '{"a": {"b": {"c": 1}}}',
    '{"a": {"b": [1, {"c": 2}]}',
    '123',
    '"just a string"',
    '42 and a half',
    '{"a": 1} {"b": 2}',
    '{"a": 1} trailing junk',
    'garbage before {"a": 1}',
    '{"a": 1} {"a": 2}',
    # double serialization
    '{"a": "{\\"b\\": 1}"}',
    '{"a": "[1, 2]"}',
    # empty-ish / whitespace
    '',
    '   ',
    '\n\t\n',
    '{"a": }',
    '{"a": 1,}',
    '{"": 1}',
    '{"a": }',
    '{ }',
    '[[[]]]',
    # far quote (small versions of the perf payloads)
    '{"a": "' + "x," * 200 + '" tail',
    '{"a": "' + "x}" * 100 + '" tail',
    '{"a": "' + ",".join("fragment" for _ in range(100)),
]


def mutate(seed: str, seed_index: int) -> list[str]:
    out: list[str] = []
    n = len(seed)
    if n < 8:
        # tiny seed: just a few truncations
        out.extend(seed[:k] for k in range(1, n))
        return out
    rng = random.Random(SEED * 1000 + seed_index)
    # truncations at ~40 evenly spaced positions + midpoint + near-end
    cuts = sorted({int(n * k / 40) for k in range(1, 40)} | {n // 2, n - 1, n - 2})
    out.extend(seed[:k] for k in cuts if 0 < k < n)
    # single-char deletions
    for _ in range(30):
        i = rng.randrange(n)
        out.append(seed[:i] + seed[i + 1:])
    # single-char insertions from the junk alphabet
    for _ in range(40):
        i = rng.randrange(n + 1)
        c = rng.choice(JUNK)
        out.append(seed[:i] + c + seed[i:])
    # single-char replacements
    for _ in range(30):
        i = rng.randrange(n)
        c = rng.choice(JUNK)
        out.append(seed[:i] + c + seed[i + 1:])
    # global whitespace variants
    out.append(seed.replace(" ", "\t"))
    out.append("\n" + seed)
    out.append(seed + " \n ")
    return out


def main() -> None:
    random.seed(SEED)
    seeds: list[str] = [
        (ROOT / "testdata" / "valid.json").read_text(),
        (ROOT / "testdata" / "invalid.json").read_text(),
        SCHEMA_SAMPLE,
    ]
    seeds.extend(NASTY)

    seen: set[str] = set()
    corpus: list[str] = []
    for idx, seed in enumerate(seeds):
        candidates = [seed] + mutate(seed, idx)
        for c in candidates:
            if c not in seen:
                seen.add(c)
                corpus.append(c)

    out = ROOT / "tools" / "diff_check" / "corpus.jsonl"
    out.parent.mkdir(parents=True, exist_ok=True)
    with out.open("w", encoding="utf-8") as f:
        for s in corpus:
            f.write(json.dumps(s, ensure_ascii=False) + "\n")
    print(f"corpus: {len(corpus)} inputs -> {out}")


if __name__ == "__main__":
    main()
