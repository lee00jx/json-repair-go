# api

## 职责
对外入口层:提供 `RepairJSON`/`Loads`/`Load`/`FromFile` 四个函数,负责
参数校验、stdlib 快速路径判定、调用解析器、以及**统一的输出收尾**。
不实现任何 token 级解析(那是 parser 及子模块的事),不做 CLI(那是 cli)。

## 对外接口
- `RepairJSON(jsonStr string, opts Options) (any, []RepairLog, error)` — 核心入口,
  修复并返回对象(return_objects 形式)。stdlib 快速路径 → 修复解析器;
  承载全部 flags:returnObjects、skipJsonLoads、logging、streamStable、strict、
  schema、schemaRepairMode、ensureAscii/pretty/indent(string 输出用)。
  `strict` 下结构违规返回 error(见 parser)。
- `Loads(jsonStr string, ...) (any, []RepairLog, error)` — `repair_json` 的 returnObjects=true 包装。
- `RepairJSONString(jsonStr string, opts Options) (string, []RepairLog, error)` —
  `repair_json(return_objects=False)` 的 string 输出:先走 `RepairJSON` 再 `Dump`;
  保留 Python 空串特判(结果为 `""` 时返回 `""` 而非带引号)。
- `Load(fd io.Reader, ...) (any, []RepairLog, error)` — 对标 `json.load`,从 fd 当前位置修复。
- `FromFile(filename string, ...) (any, []RepairLog, error)` — `Load` 的按文件名包装。
- `Dump(v any, opts DumpOptions) (string, error)` — 手写 `json.dumps` 等价序列化器
  (ensure_ascii 转义、float Python 规则、indent/pretty 分隔符);见 `dump.go`。
- `DecodeJSON(s string) (any, error)` — 严格单值解码(不修复),数字按
  D-002 映射(int64/uint64/float64);对标 Python `json.loads`,
  CLI 的 `--schema` 文件加载用它(与修复路径的 `stdlibDecode` 共用转换)。
- 选项结构 `Options` 承载 flags;`Options.Pretty`/`Indent`/`EnsureASCII` 供 string 输出;
  `logging=true` 时返回 `([]RepairLog)`。

## 依赖关系
- 依赖:`parser`(修复路径)、`schema_repair`(带 schema 时)、`ordered_map`
  (对象载体,D-006)。
- 被依赖:`cli`。

## 关键文件路径
- src: `json_repair.go`(四入口 + stdlib 快速路径 + 数字转换)、`dump.go`(`Dump`/`DumpOptions`/`pythonFloatString`/字符串转义)
- test: `api_test.go`(`RepairJSONString`、`FromFile` logging)、`dump_test.go`(float 规则、转义、indent)
- 参考(Python): `../json_repair-main/src/json_repair/json_repair.py`、`__init__.py`

## 已知约束与坑
- **对象键序(D-006,2026-09-21 已关闭)**:解析器/修复器输出对象均为
  `*OrderedMap`(插入序),`Dump` 与 `MarshalJSON` 都按 Python 键序输出;
  快速路径 `decodeOrderedStrict` 保序解码。多键对象字符串输出与 Python
  逐字节一致(ordered_map_test.go + 差分对拍验证)。plain map 输入仍按
  排序键归一化(Go 专属边界约定,见 ordered_map.md)。
- **float 格式(D-007)**:Go `strconv` 与 CPython `float_repr` 不同,已手写
  `pythonFloatString`(E<-4 或 E>=16 走科学计数)。勿改回 `strconv.FormatFloat`。
- **ensure_ascii 默认(D-008)**:`RepairJSONString` 透传 `opts.EnsureASCII`(零值
  false);库默认 True / CLI 默认 False 由调用点设定,勿在函数内硬编码。
- 合法 JSON 必须留在 stdlib 快速路径;`strict=true` 不得二次质疑 `encoding/json`
  已接受的输入。
- `skipJsonLoads=true` 是显式 opt-out,只用于已知非法输入;不得假设其对合法
  JSON 的行为与保留 stdlib 路径一致(强制合法 JSON 走修复器可能改变结构/值)。
- `load(fd)` 对标 `json.load(fd)`,从描述符当前位置开始修复,不捕获 IO 异常
  (由调用方处理)。
- 保持**单一共享的输出收尾路径**:logging、returnObjects、空串处理、
  `Dump` 都走这里(不要在各分支各写一份收尾)。
- 参数互斥校验:`salvage` 需 schema;`schema` 与 `strict` 互斥(已落地,
  阶段 4;CLI 的同名预检见 cli 模块)。
- 顶层 `RecursionError`(Go 里为过深递归/栈溢出风险)统一归一化为受控错误。
- 分块文件读:Go 侧 `Load`/`FromFile` 直接 `ReadAll` 整读,行为与 Python
  chunked 读等价(chunk_length 只影响内存,不改输出);`chunk_length` 参数
  暂不暴露,保持 1:1 行为即可。
- **负 indent 钳到 0(2026-09-28)**:`dumpIndent`(dump.go)在调 `strings.Repeat`
  前把 `Indent*depth` 钳到 ≥0,负 indent(如 `RepairJSONString(s,
  Options{Pretty:true, Indent:-2})`)不再触发 "strings: negative Repeat count"
  panic。行为上负 indent 等价 indent=0(换行、无前导空格),与 Python
  `json.dumps(indent<0)` 逐字节一致(实测)。勿把 `Indent*depth` 直接喂
  `strings.Repeat`。

## 实现细节记录
### 2026-09-28 Dump 负 indent 不再 panic(钳到 0)
- **问题**:`dumpIndent`(dump.go)执行 `strings.Repeat(" ", opts.Indent*depth)`;
  `Indent` 为负时 Repeat 收到负 count,直接 panic "strings: negative Repeat count"。
  这是可被外部参数触发(`RepairJSONString(s, Options{Pretty:true, Indent:<0})`)的
  panic,对"兜底"库是严重缺陷(见 parser.md"无顶层 recover"约束:panic 不得穿透
  给调用方)。
- **修复**:`dumpIndent` 先算 `n := opts.Indent*depth`,若 `n<0` 钳到 0 再 Repeat。
  负 indent 由此等价 indent=0(换行、无前导空格),与 Python `json.dumps(indent<0)`
  逐字节一致(实测 indent=-2 与 indent=0 输出相同)。
- 测试:`regressions_test.go`(新)新增 `TestNegativeIndentNoPanic`(indent=-2 等于
  indent=0 + 嵌套层无前导空格)。
- 涉及文件:`dump.go`(dumpIndent)、`regressions_test.go`(新)。

### 2026-09-21 D-006 关闭(api 侧)
- stdlib 快速路径改用 `decodeOrderedStrict`(json_repair.go:215 附近):按
  stdlib token 流严格解码整个输入,对象键保插入序(对标 Python
  `json.JSONDecoder.raw_decode`),数字仍走 D-002 映射;快速路径输出的
  对象即 `*OrderedMap`。`RepairJSON`/`Loads` 对象返回形式的键序自此 1:1。
- 本条为前一会话实现的补记 + 本次回归验证(ordered_map_test.go 的
  fast-path 用例、差分对拍插入序升级后的 37014 条全绿)。

### 2026-09-18 新增 DecodeJSON(CLI 前置)
- 导出 `DecodeJSON(s string) (any, error)`:严格单值解码 + `convertJSONNumbers`
  (D-002 int/float 映射),尾部多余数据报错。动机:CLI 的 `--schema` 文件
  加载需要"严格解码但不修复"的公共原语(Python cli 用 `json.load`),
  在 api 层导出可避免 CLI 包复制 D-002 转换逻辑(该转换是已知坑区,
  不宜两处维护)。纯新增导出,不影响既有调用。
- 测试:`api_test.go` 新增 `TestDecodeJSON`(类型映射 + 尾随数据 + 非法 JSON)。
- 涉及文件:`json_repair.go`、`api_test.go`。

### 2026-09-17 阶段 3:string 输出序列化器 + flags + from_file/logging
- 新增 `dump.go`:`Dump(v, DumpOptions)` 复刻 `json.dumps`——ensure_ascii 转义
  (`"`/`\`/`\n\t\r\b\f` + 控制字符 `\u00XX` + astral surrogate pair)、
  `pythonFloatString`(Python `E<-4||E>=16` 科学计数规则)、compact/pretty 分隔符
  (compact `", "`/`": "`;pretty 换行 + 缩进,`indent=0` 也走 pretty)。
- 新增 `RepairJSONString`(string 输出,含空串特判);`Options` 加 `Pretty` 字段。
- 新增测试:`dump_test.go`(68 个 float 值逐字对照 Python、转义、indent 0/2/4)、
  `api_test.go`(`RepairJSONString` 对照 Python、`FromFile` 6 条日志逐字一致)。
- 修复 `tools/gen_parity_test.py`:行尾 Go 块注释 `/* … */` 不嵌套,当输入含
  `*/` 时会提前闭合注释泄漏代码;已在生成注释前剔除 `*` 和 `/`。
- 涉及文件:`dump.go`(新)、`dump_test.go`(新)、`api_test.go`(新)、
  `json_repair.go`、`tools/gen_parity_test.py`。

### 2026-09-17 建档(阶段 0)
- 建立模块文档,确定入口四函数与"统一输出收尾"约束;尚未写 Go 代码。

## 当前状态 / TODO
- 阶段 1:✅ `RepairJSON`/`Loads`/`Load`/`FromFile` + stdlib 快速路径 + 简单修复路径。
- 阶段 3:✅ ensure_ascii 手写序列化、logging repair log、string 输出路径、
  from_file 行为;`streamStable`/`tryValidJsonSuffix` 为纯性能优化(非行为差异),
  尚未实现,列为可选。
- 阶段 4/5:✅ schema standard/salvage(见 schema_repair 模块)。
- 阶段 6:✅ CLI(见 cli 模块)。api 层无剩余 TODO。
