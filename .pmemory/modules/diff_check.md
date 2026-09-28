# diff_check(差分对拍 harness)

## 职责
用真实 Python 参考实现(`../json_repair-main`)对 Go 移植做**逐字节**行为对拍,
抓单测结构上抓不到的转写错误(典型:转写时手滑改错一个转义字面量)。
明确不做:不替代单测(默认 `go test` 保持快),不做模糊测试(fuzz 另有其事)。

## 对外接口(运行方式)
```
.venv-oracle/bin/python tools/diff_check/gen_corpus.py   # 生成 corpus.jsonl(6169 条)
.venv-oracle/bin/python tools/diff_check/py_ref.py      # 生成 py_ref.jsonl(37014 行=6169×6 组合)
DIFF_CHECK=1 go test -run TestDiffParity -v .           # 跑对拍,写 report.txt
```
- 6 组合:`skip_json_loads × stream_stable` 的 4 种 + 带 schema 的 2 种
  (Go 侧用 `diffSchemaOrdered()` 构造 *OrderedMap,属性序与 py_ref.py 的
  SCHEMA_PERF 字典字面量一致——schema 属性序是输入状态,见 D-006)。
- 归一化输出:Go `Dump(v, {EnsureASCII:false})` vs Python
  `json.dumps(v, ensure_ascii=False)`(两侧都按 **dict 插入序**输出键,
  D-006 落地后的真 1:1 对比形态;键序本身是对拍的一部分)。
- 异常形态:Python `ERR:<ExceptionType>:<msg>`,Go `ERR:<msg>`。

## 结果分桶(diff_parity_test.go)
- `output` 双方都成功但值不同 — **fatal**
- `status` 仅一方报错 — **fatal**
- `errtext` 双方都报错(非 schema 组合)消息不同 — **fatal**
- `int64` 值不同且涉及 >int64 整数 — 已知类型映射限制(D-002)
- `schemaerr` schema 组合双方都报错、消息不同 — **INFO**(归一化后仍不同才算)
fatal 总数 >0 时测试失败;report.txt 落 `tools/diff_check/`。

## 归一化规则(schemaerr 桶,normalizeSchemaErr)
只消除"伪差异",不掩盖真差异:
1. Python 侧剥掉 `ValueError:` 异常类型前缀(py_ref.py 记录格式造成)。
2. `Missing required properties at <path>: <list>` 的 list 两侧都排序
   (Python 迭代 `set[str]`,顺序随 PYTHONHASHSEED 随机;Go 本已排序)。

## 依赖关系
- 依赖:api(`RepairJSON`/`Dump`)、Python 参考仓库(同级目录
  `json_repair-main`)+ `.venv-oracle`(含 jsonschema/pydantic)。
- 被依赖:无(纯验证工具)。

## 关键文件路径
- `tools/diff_check/{gen_corpus.py,py_ref.py,corpus.jsonl,py_ref.jsonl,report.txt}`
- `diff_parity_test.go`(测试入口,DIFF_CHECK=1 门控,缺参考文件时 skip)

## 已知约束与坑
- **2026-09-20 首跑战绩**:6169×6=37014 条对拍,首跑 32 条 fatal(8 输入 ×
  4 组合),全部是 `classifyEmptyObjectRepair` escaped-object-key 分支的
  转写 bug(见 containers.md);修复后 output/status/errtext/int64 全 0。
- **遗留 2 条 INFO 差异**(corpus #336 × schema 两组合):schema 引导解析把
  每个顶层对象包成 `[obj]`(满足 `type:array`),顶层合并成 `[[obj1],[obj2]]`,
  终验在 items 位置报类型错误。Python 消息=失败实例的完整 Python repr
  (含嵌套 dict 插入序),Go 消息=gojsonschema 原文。D-006 保序 map 已落地
  (2026-09-21 重跑确认这 2 条仍在、无新增),剩余缺口是完整 Python repr
  渲染器,刻意不做(见 schema_repair.md 已知约束)。
- corpus 与 py_ref 是**生成物**:改了 Python 参考实现或组合定义后必须重新
  生成两侧;测试会校验 `len(refs) == len(corpus)*6` 防陈旧。
- `report.txt` 每跑必重写,截断显示 120 字符/行(diffExcerpt)。

## 实现细节记录
### 2026-09-21 D-006 落地:对比形态升级为插入序逐字节
- py_ref.py 去掉 `sort_keys`(两侧都是插入序,键序从此是对拍的一部分);
  重新生成 py_ref.jsonl。
- 首跑出现 478 条 output mismatch,全部为 schema 组合:Go 侧原先把
  perfSchema() 的 plain map 直接传入(边界归一化=排序键),而 Python 的
  SCHEMA_PERF 是有序 dict 字面量,`repairObject` 按 schema 属性序重建,
  两侧属性序不同 → 键序分叉。修复:新增 `diffSchemaOrdered()`(*OrderedMap,
  属性序逐一对齐 Python 字面量),schema 组合改用它。
- 重跑:`output=0 status=0 errtext=0 int64=0 schemaerr=2`,PASS,0.67s;
  37014 条输出(含键序)与 Python 逐字节一致。
- 涉及文件:`diff_parity_test.go`、`tools/diff_check/{py_ref.py,py_ref.jsonl}`。

### 2026-09-20 建立 harness 并首跑
- 建 `gen_corpus.py`(从 Python 测试套件/常见坏 JSON 模式采样 6169 条)、
  `py_ref.py`(真实 Python 实现跑 6 组合产出参考)、`diff_parity_test.go`
  (DIFF_CHECK=1 门控 + 缺文件 skip)。
- 首跑抓出 32 fatal(escaped-object-key 转写 bug,containers 模块修复)。
- 加 `normalizeSchemaErr`(ValueError 前缀 + required 列表排序归一),
  schemaerr 从 11736 降到 2(剩余为 gojsonschema 终验消息分叉,已知差异)。
- 最终:`output=0 status=0 errtext=0 int64=0 schemaerr=2`,PASS,0.6s。

## 当前状态 / TODO
- 可用。对比形态已是**插入序逐字节**(D-006 落地后升级),键序分叉会被直接
  抓出;后续:corpus 可扩充(更多 schema 形状/更多截断模式、salvage 组合)。
