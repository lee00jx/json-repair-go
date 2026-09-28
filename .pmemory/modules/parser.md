# parser

## 职责
`JSONParser` 核心:维护字符游标 `index` 与对象成员上下文栈 `context`,
`ParseJSON` 递归下降分发循环(按首字符调 object/array/parenthesized/string/
number/comment),以及顶层解析(`parseTopLevel` 多值合并/更新、
`parseTopLevelSalvageWithSchema`)、`strict`/`streamStable` 行为、
`tryValidJsonSuffix`(用 `json.Decoder` 定点 raw_decode 式快速解码)。
数字(`parseNumber`)、注释(`parseComment`)、Python 括号/元组
(`parseParenthesized`)也合并在本文件实现(见 containers 文档说明)。
不做具体字符串/对象内部启发式(交给 parse_string/containers)。

## 对外接口
- `newJSONParser(jsonStr string, logging, streamStable, strict, allowValidJSONSuffix bool) *JSONParser`
- `(*JSONParser).Parse() any` — 顶层入口
- `(*JSONParser).ParseWithSchema(repairer *SchemaRepairer, schema any) any`
  — schema 引导顶层入口,内部 recover `schemaAbort`
- `(*JSONParser).parseJSON() any` / `parseJSONSchema(schema any, path string, finalizeSchema, recordTopLevelValue bool) any`
  — 按首字符 + schema 类型的分发(数字/注释/括号/字符串/对象/数组)
- `(*JSONParser).recordSchemaErr(err error)` — schema 错误经 `schemaAbort`
  panic 传递(直接调用 parseXxxSchema 的测试用 `catchSchemaPanic` 包一层)
- 内部方法(包内可见,供各 parse_*.go 调用):`getCharAt`、`skipWhitespaces`、
  `scrollWhitespaces`、`skipToCharacter`、`log`、`fail`(strict 错误)、`parseNumber`、
  `parseComment`、`parseParenthesized` 等。

## 依赖关系
- 依赖:`parse_string`、`containers`、`utils`(context/常量/游标)。
- 被依赖:`api`;`schema_repair` 在分发中被逐值回调。

## 关键文件路径
- src: `json_parser.go`
- 参考(Python): `../json_repair-main/src/json_repair/json_parser.py`

## 已知约束与坑
- 解析器重构对**上下文生命周期**与**启发式分支顺序**敏感;重构 `parseString`/
  `parseObject` 时必须保持畸形输入行为不变。
- 裸字面量只在与分隔符或输入结尾组成完整 token 时才识别;`none` 大小写不敏感
  当 null,带引号值与其它裸词保持字符串。
- 逗号分隔括号序列当 Python 元组归一为数组;单个括号值保持标量;Python 风格
  字面量仅在数组/对象/元组内支持,不作裸顶层值。
- 顶层原始解码仅在**非逗号垃圾**跟随时才丢弃合法前缀;保持完整输入启发式,
  逗号前缀的续接交给结构化对象修复。
- `strict=true`:遇到重复 key、缺失分隔符、空 key/value、多顶层值等结构问题
  抛错而非修复;仍遵守 `skipJsonLoads=true`。
- 性能回归常藏在对长畸形对象值上重复的 lookahead 扫描;保留含大量逗号/`}`
  且远端才有引号的用例,只保留有显著(约 ≥1s)基线降速的输入。
- 顶层 `RecursionError` 归一化为受控错误(Go 中注意递归深度/栈)。
- 多顶层值合并:重复相同对象按"更新"语义保留最新值。
- **panic 安全性(fuzz 验证,2026-09-24)**:解析器对任意输入**不得 panic**。
  所有字节访问走有边界检查的 `charAt/charAtIs/charAtOr/skipWhitespaces`;
  `robustness_test.go` 的 `TestHostileNoPanic`(恶意输入 recover 探针)+
  `FuzzRepairJSON`(原生 fuzz)是回归网,改解析器后应跑
  `go test -fuzz=FuzzRepairJSON -fuzztime=30s .`。
- **无顶层 recover**:`RepairJSON` 不设顶层 recover;唯一真实 panic 源是
  `recordSchemaErr` 抛 `schemaAbort`(模仿 Python raise),仅由 `ParseWithSchema`
  的 recover 封闭并转为 `schemaErr`。`RepairValue` 等 schema 方法只返回 error
  不 panic,stdlib 快速路径直接调它无需 recover。新增 panic 必须确保落在
  recover 覆盖范围内,否则会穿透给调用方(对"兜底"库是严重缺陷)。
- **超 int64 范围整数 → float64(2026-09-28)**:`parseNumber`(default 分支)与
  快速路径 `decodeOrderedValue` 的 `json.Number` 分支在 int64 溢出时都回落
  float64,保证 skip_json_loads 两路径"同输入同类型"一致。Python int 任意精度
  返回精确 int;Go 无 big.Int(D-002 刻意不引入),此为不可避免的语言级分叉。
  勿"修"成 string(那会重引入路径不一致),也勿引入 big.Int(违背 D-002,需动
  整套类型映射)。

## 实现细节记录
### 2026-09-28 parseNumber 大整数 → float64(int64 溢出)
- **问题**:`parseNumber`(json_parser.go)default 分支在 `strconv.ParseInt` int64
  溢出时直接返回原始数字串(string);而 stdlib 快速路径 `decodeOrderedValue`
  (json_repair.go)的 `json.Number.Int64()` 失败时回落 `Float64()`(float64)。同一
  超 int64 整数(如 2^64=18446744073709551616)在快速路径(skipJsonLoads=false,
  合法 JSON)得 float64,修复路径(skipJsonLoads=true 或非法 JSON)得 string——
  两路径类型分叉,违反"同输入同类型",也与 D-002"数字为 int64/float64"不符。
- **修复**:default 分支 ParseInt 失败后先试 `strconv.ParseFloat(s, 64)`,成功则
  返回 float64(与快速路径一致);仅 ParseFloat 也失败(超 float64 范围,如 >1e308)
  才保留返回原串兜底。范围内整数仍返回 int64(1/42/MaxInt64 不变),小数仍
  float64,行为无回归。
- **与 Python 分叉边界**:Python `repair_json('{"n":2^64}')` 返回精确 int(任意
  精度);Go 降级 float64——语言级分叉,不追 1:1(见"已知约束与坑")。
- 测试:`regressions_test.go`(新)新增 `TestBigIntConsistencyAcrossPaths`(快速/
  慢速/非法 JSON 三路径均 float64 且相等)与 `TestInRangeIntsStayInt64`(范围内
  类型不变)。
- 涉及文件:`json_parser.go`(parseNumber)、`regressions_test.go`(新)。

### 2026-09-24 全库质量/bug 检查 + panic 回归网 + perf -race 修复
- 应大师兄要求做代码质量与 bug 检查。自动工具:`go vet`/`gofmt` 全清。
  精读解析器字节游标,确认所有下标访问走有界 `charAt*`。
- 新增 `robustness_test.go`:`TestHostileNoPanic`(100+ 恶意输入 × 13 选项
  组合,含 schema/salvage,每例 recover 捕获 panic)+ `FuzzRepairJSON`(原生
  fuzz target,7 种选项组合)。
- 验证结果:hostile 探针 0 panic;`go test -fuzz=FuzzRepairJSON -fuzztime=45s`
  228k 次执行(含 schema/salvage)0 panic;`go test -race ./...` 无数据竞争
  (排除 perf 后 `ok`);唯一 panic 源 `schemaAbort` 被 `ParseWithSchema` recover
  封闭。结论:核心代码 panic-free、race-free,**无功能 bug**。
- 唯一真实 bug(轻):`TestPerformanceRegression` 未在 `-race` 下跳过,~10x
  race 插桩打爆 10x 性能 cap,使 `go test -race ./...` 必然失败。修复:
  `perf_test.go` 加 `//go:build !race`(race 时整个文件排除,时序本无意义),
  文档注释同步。已确认 `perfSchema/perfCases/runPerfCase/medianDuration` 等
  辅助符号仅该文件引用,build tag 不影响其他文件编译。
- 复验:`go test ./... -count=1` 全绿(perf 正常跑)、`go test -race ./...
  -count=1` 全绿(perf 被排除)、`gofmt -l` 全清。
- 涉及文件:`robustness_test.go`(新)、`perf_test.go`(build tag + 注释)、
  `.pmemory/CHANGELOG.md`、`modules/parser.md`。

### 2026-09-20 streamStable/tryValidJsonSuffix 验证 + 性能回归网移植
- **streamStable/tryValidJsonSuffix**:文档此前标"尚未实现",但代码早已接进
  (`parse_string.go` 6 处 `!p.streamStable` 分支 + `json_parser.go`
  `tryParseValidJsonValue`)。用 Python oracle 逐例核对 10 个截断输入 ×
  两标志(共 20 组,紧凑 JSON 逐字节 diff),**全部一致**。唯一可观察差异:
  未闭合字符串值以悬空 `\` 结尾(不完整转义的开头)且到输入末尾时,
  `streamStable=true` 丢弃该 `\`(`parse_string.go` 对应 Python
  `parse_string.py:843-847` 的 EOF 分支),`false` 保留。新增
  `stream_stable_test.go` 锁定该行为。
  - 踩坑:首次核对报"Go 保留了反斜杠"的假差异,实为**探针 bug**——Go 侧
    病态串误写成两个 `\`(`\`` 结尾 raw string 又 `+ "\\"`),构成合法 `\\`
    转义;Python 侧只有一个 `\`。修正探针后 20 组全一致。教训:跨语言 diff
    先核对输入字节(MD5),再怀疑被测代码。
- **性能回归网**:`perf_test.go` 移植 `test_performance.py` 的 16 例。
  `BenchmarkRepairPerformance`(16 个子 benchmark,供 `go test -bench` 看
  ns/op/allocs 趋势)+ `TestPerformanceRegression`(本地跑、CI/`-short`
  跳过,取 1 次热身 + 5 次中位数,超上限即 fail)。上限取 CPython 阈值的 10 倍
  ——Go 解析器是线性的(本地最慢 far-quote 仅 ~1.6ms),10 倍裕量只为抓
  **算法级**(数量级)回归,不抓常数因子抖动。字符串输出用例设
  `EnsureASCII:true` 对齐 Python `return_objects=False` 默认。
  - 4 个病态负载(unclosed 3000 逗号 / mixed-quote 35KB / far-quote
    10000 逗号 / far-quote 5000 花括号)的构造公式与 Python 逐一对齐,
    MD5 核对一致。fixture `valid.json`/`invalid.json` 拷入 `testdata/`。
  - 涉及文件:`perf_test.go`、`testdata/{valid,invalid}.json`。
- 修正 `parser.md`/`INDEX.md` 中 streamStable"尚未实现"、性能网"未移植"的
  过时标注(以代码为准)。

### 2026-09-18 阶段 4:schema 引导分发与 schema 错误通道
- `parseJSONSchema` 按首字符 + schema 类型分发:对象 → `parseObjectSchema`、
  数组 → `parseArraySchema`、括号 → `parseParenthesizedSchema`、标量/字符串 →
  解析后交给 `SchemaRepairer.RepairValue`(repairer 侧强转/填充/丢弃)。
- `ParseWithSchema`:顶层入口,`defer recover` 把 `recordSchemaErr` 触发的
  `schemaAbort` panic 转成 `p.schemaErr`(api 层 `RepairJSON` 读取返回);
  salvage 模式走 `parseTopLevelSalvageWithSchema`(顺序顶层片段,跳过 schema
  失败候选,返回第一个合法片段;绝不从真顶层数组挑元素)。
- 涉及文件:`json_parser.go`(`parseJSONSchema`/`ParseWithSchema`/
  `recordSchemaErr`/`parseTopLevelSalvageWithSchema`);
  测试:`schema_guided_parse_test.go`、`schema_parser_paths_test.go`。

### 2026-09-17 实现 strict mode 错误返回
- **做了什么**:给 `JSONParser` 加 `strictErr error` 字段和 `fail(msg)` 方法;
  在 `parseTopLevel`(多顶层值)、`parseObject`(重复 key / 缺失冒号 / 空值 /
  空对象带多余字符 / 空 key)、`parseString`(双引号后跟引号)的 strict 分支
  调用 `p.fail()`;`RepairJSON` 返回 `p.strictErr`。
- **为什么**:Python 原版 strict 模式遇到结构问题直接 `raise ValueError`,
  Go 侧需要等价地通过 error 通道返回。
- 涉及文件:`json_parser.go`(strictErr 字段 + fail 方法 + parseTopLevel 检查)、
  `parse_object.go`(5 处 strict 分支加 fail)、`parse_string.go`(2 处)、
  `json_repair.go`(返回 p.strictErr)。
- 新增测试:`parse_number_comment_test.go` 中 `TestStrictMode`(14 个用例)、
  `TestCommentBracketSafety`、`TestCommentInArrayItems`。

### 2026-09-17 修复 classifyEmptyObjectRepair 切片越界 panic
- **问题**:截断输入(如 `[{"key": "value", "key`)使 `parseObject` 循环结束时
  `p.index++` 将游标推到 `len(jsonStr)+1`。随后 `classifyEmptyObjectRepair` 执行
  `p.jsonStr[startIndex-1 : p.index+1]` 触发 "slice bounds out of range" panic。
- **修复**:在 `classifyEmptyObjectRepair` 和 `repairEmptyObjectResult` 中对
  `p.index+1` 做 `min(end, len(p.jsonStr))` 钳位。
- 涉及文件:`parse_object.go`

### 2026-09-17 建档(阶段 0)
- 建立模块文档;尚未写 Go 代码。

## 当前状态 / TODO
- 阶段 1/2/4:✅ 核心解析、strict、schema 引导分发均完成,测试全绿。
- `streamStable` + `tryValidJsonSuffix`:✅ 已实现并验证 1:1(2026-09-20,见
  下方实现记录 + `stream_stable_test.go`)。`tryValidJsonSuffix` 即
  `!SkipJsonLoads` 的 stdlib 快速路径,已被全量测试覆盖;`streamStable` 为
  截断流式输入下的保守修复模式。
- 性能回归网:✅ 已移植为 `perf_test.go`(2026-09-20)。
