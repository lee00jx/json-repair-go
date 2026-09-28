# schema_repair

## 职责
schema 引导修复:`SchemaRepairer` 按 JSON Schema / 布尔 schema,在解析过程中
逐值 repair/validate——填默认/必填字段、安全标量强转(`"1"`→`1`、`"yes"`→`true`)、
丢弃 schema 不允许的属性/数组项。支持 `standard`(默认)与 `salvage`(更激进的
数组/对象抢救)两种模式。含 `patternProperties` 匹配与 root `$ref`/`$defs`
解析作用域管理。**不做 Pydantic 模型输入**(无 Go 对应,`SchemaFromInput` 拒绝)。
**此模块在 Python 原版中标注为 beta,已知有 bug;移植需以测试为准对齐。**

## 对外接口
- `NewSchemaRepairer(schema any, log *[]RepairLog, mode *string) (*SchemaRepairer, error)`
- `(*SchemaRepairer).RepairValue(value, schema any, path string) (any, error)`
- `(*SchemaRepairer).isValid(value, schema any) (bool, error)`、
  `.validate(value, schema any) error`(gojsonschema 终验,validator 按 root 缓存)
- `(*SchemaRepairer).ResolveSchema(schema any) (any, error)`(解析 `$ref`/`$defs`/组合键)
- `(*SchemaRepairer).IsObjectSchema/IsArraySchema(schema any) bool`
- `SchemaFromInput(schema any) (any, error)`、`normalizeSchemaRepairMode`、
  `normalizeMissingValues`(missing 哨兵 → `""`)、`SchemaDefinitionError`
- `pattern_properties.go`:`matchPatternProperties(pp map[string]any, key string)
  ([]any, []string)`,返回(按 pattern 顺序匹配到的 schema, 不支持的正则串)
- `parser_schema.go`:`objectSchemaConfig`/`arraySchemaConfig`/
  `resolveParserObjectSchema`/`resolveParserArraySchema`

## 依赖关系
- 依赖:`utils`(pattern_properties);`parser`(在 `ParseJSON` 中被回调)。
- 被依赖:`api`、`parser`。
- 外部库:`github.com/xeipuuv/gojsonschema`(已落地,DECISIONS D-005)。

## 关键文件路径
- src: `schema_repair.go`(1289 行,repairer 主体)、`parser_schema.go`、
  `pattern_properties.go`、`schema_value.go`(missing 哨兵/错误类型/校验前归一)
- test: `schema_repairer_test.go`、`schema_guided_parse_test.go`、
  `schema_parser_paths_test.go`(分别对应 Python 同名测试)
- 参考(Python): `../json_repair-main/src/json_repair/schema_repair.py`、`parser_schema.py`

## 已知约束与坑
- 提供 schema 时,**合法与非法 JSON 都要**应用 schema 修复与校验(不只是非法)。
- schema 与 `strict=true` **互斥**。
- `patternProperties` 匹配刻意限制在**安全子集;不执行用户正则**。
- `resolveSchema` 尽量保持 schema dict 身份,让校验缓存有效。
- 校验分离的 anyOf/oneOf 分支时,保留原 root schema 的解析作用域,使 root
  相对的 `$ref` 能解析 `$defs`。
- `schemaRepairMode` 仅 `standard` 与 opt-in `salvage`;salvage 可恢复强证据的
  结构错配,但**不得静默强转或推断语义属性重命名**(如 `results`→`patterns`)。
- 顺序顶层片段,salvage 可跳过 schema 失败候选,返回第一个 schema 合法片段;
  **绝不从真正的顶层 JSON 数组里挑选元素**。
- 用户 schema 视为**攻击者可控输入**:归一化/校验/修复的深层嵌套需显式深度
  限制或受控 `ValueError`,而非未捕获 RecursionError。
- 无法修复到满足 schema 时抛 `ValueError`。
- **schema 的 properties 已保序(D-006,2026-09-21 关闭)**:schema 在边界
  (`SchemaFromInput`/`NewSchemaRepairer`)归一化为 `*OrderedMap`,`mapListToObject`
  与 `repairObject` 都按 schema 属性**插入序**迭代,与 Python 一致。
  仅 plain `map[string]any` 输入(Go map 字面量本身无序)按排序键归一化——
  Go 专属边界约定;要复刻 Python dict 字面量的顺序必须构造 `*OrderedMap`
  (差分对拍的 `diffSchemaOrdered()` 即为此而设)。
- **终验错误文本的已知分叉(gojsonschema vs Python jsonschema,2026-09-20
  差分对拍确认)**:Go `formatValidationError` 对**根类型不匹配**会模仿 Python
  jsonschema 文案(`<value> is not of type '<t>'`,但 value 仅标量级
  `pythonReprScalar`);对**嵌套位置**的类型违规则回落到 gojsonschema 原文
  (如 `0: Invalid type. Expected: object, given: array`),而 Python jsonschema
  输出的是 `<失败实例的 Python repr> is not of type '<t>'`(repr 含完整嵌套
  结构 + 单引号 + dict 插入序)。D-006 保序 map 已落地(2026-09-21),repr 的
  键序障碍消除,剩余缺口是完整 Python repr 渲染器,仍刻意不做。
  差分对拍中此类仅剩 2 条(corpus #336 × schema 两组合),状态(报错与否)
  一致,仅文本不同,记为 INFO 差异。
- **pythonReprScalar 标量 repr 已对齐(2026-09-28)**:`None`/`True`/`False`/
  float64(复用 `pythonFloatString`)/int64/int/uint64 均输出与 Python `repr`
  一致文本;仅 string 分支保留简单单引号包裹(`'s'`,不切双引号/不转义——属
  上方"完整 repr 渲染器刻意不做"缺口范围)。该函数仅在**根类型不匹配**时到达,
  且标量不匹配通常先被 `RepairValue` 修复(如 `true`→`[True]`),故 nil/bool/
  float 分支实际可达性低(本次为潜在正确性修复,非用户可见 bug)。
- **schema 错误消息的 required 列表顺序**:Python `schema_config.required`
  是 `set[str]`,多属性缺失时报错顺序随 PYTHONHASHSEED 随机(已用 seed 0/1/2
  实测);Go 用 `sortedStrings` 确定性排序。差分对拍对两侧均做排序归一。
- **日志切片是值拷贝**:`JSONParser.log` 内部 `append` 只改 parser 自己的
  `p.logger` 字段,调用方提前 `p.logger = logs` 拿到的外层变量不会更新;
  生产路径 `RepairJSON` 靠结尾 `logs = p.logger` 读回兜底。直接调 parser
  方法的测试必须解析后自行读回 `p.logger` 再断言。
- Pydantic 模型输入无 Go 对应:`SchemaFromInput` 只接受 dict/bool,
  其余报 "Schema must be a JSON Schema dict or boolean schema.";
  Python 测试的 pydantic-model/module-import 用例刻意省略。

## 实现细节记录
### 2026-09-28 pythonReprScalar 标量格式化对齐 Python repr
- **问题**:`pythonReprScalar`(schema_repair.go)此前仅对 string 特判(单引号
  包裹),其余一律 `fmt.Sprintf("%v")`:`nil` 输出 `<nil>`(Python `None`)、
  `bool` 输出 `true`/`false`(Python `True`/`False`)、`float64(1.0)` 输出 `1`
  (Python `1.0`)。这些用于根类型不匹配错误文案 `<value> is not of type '<t>'`,
  与 Python jsonschema 措辞分叉。
- **修复**:改为 type switch——nil→`None`、bool→`True`/`False`、float64→
  `pythonFloatString`(复用 api/dump.go 的 Python float 规则)、int64/int/
  uint64→十进制、string 保持单引号包裹。仅对齐标量分支;嵌套容器(dict/list)
  仍回落 `%v`(完整 repr 渲染器是刻意不做缺口,见"已知约束与坑")。
- **边界**:标量类型不匹配通常先被 `RepairValue` 修复(如 `true`→`[True]`),
  nil/bool/float 分支实际可达性低;本次为潜在正确性修复,非用户可见 bug。
- 测试:`regressions_test.go`(新)新增 `TestPythonReprScalar`(直接断言本函数
  11 个标量用例)。
- 涉及文件:`schema_repair.go`(pythonReprScalar)、`regressions_test.go`(新)。

### 2026-09-21 D-006 关闭(schema 侧)
- schema 载体改 `*OrderedMap`:`ObjectSchemaConfig.Properties`/
  `PatternProperties` 为 `*OrderedMap` + `PropertyKeys`/`PatternKeys` 切片
  (插入序,供消费者按 Python dict 序迭代);`repairObject` 重建序=
  schema properties 插入序(仅 value 中存在或声明 default 的键)+ 剩余
  输入键的输入序,与 Python `_repair_object` 逐段一致;`mapListToObject`
  按 `pm.Keys()`(schema 属性插入序)映射,2026-09-18 的 sorted-key 临时
  方案移除。
- salvage 缺失必填填充仍按 `sortedStrings` 迭代:Python 迭代 `set[str]`
  (顺序随 PYTHONHASHSEED 随机),Go 取排序这一确定性约定(非行为分叉)。
- plain `map[string]any` 输入(测试字面量/直接调用方)在 `RepairValue`/
  `NewSchemaRepairer` 边界深转换为排序键 ordered 形态(D-006 约定)。
- 验证:ordered_map_test.go 的 3 条 schema 键序用例(schema 重建/extra
  追加/default 填充)+ salvage list→object 用例全绿;差分对拍 37014 条
  (含键序)全绿。
- 涉及文件:`schema_repair.go`、`parser_schema.go`、`schema_value.go`
  (前一会话实现,本次补记 + 验证)。

### 2026-09-18 修复 mapListToObject 无序 flaky + 测试日志读回
- `TestSchemaSalvageModeMappingRejectsTypeMismatch` 时过时挂:Go map 迭代随机,
  `mapListToObject` 取到的属性顺序不定;Python 按插入序(name 先)会让映射失败
  回退到 "Expected object",而 Go 随机取到 tags 先时映射成功,测试 flaky。
- 修复:`mapListToObject` 的 keys 用 `sortedStrings` 排序(与 D-006 排序约定一致),
  本用例排序序恰等于 Python 插入序,行为对齐且确定。
- 同批修复 `TestParseObjectSchemaSkipsUnsupportedPatternRegexWithLog`:测试
  `p.logger = logs` 后 parser 内部 append 不回传外层 `logs`(切片值拷贝),
  断言前补 `logs = append(logs, p.logger...)`,镜像 `RepairJSON` 的读回逻辑。
  实现本身(日志文案 "…while parsing object key '…'" 后缀)与 Python 一致,未动。
- 涉及文件:`schema_repair.go`、`schema_parser_paths_test.go`;`go test -count=5` 全绿。

### 2026-09-18 阶段 4+5 落地(standard + salvage,补记)
- `schema_repair.go` 完整移植 Python `SchemaRepairer`:标量强转(含 Python
  float→int/bool 字符串规则)、default/const/enum 填充、required/minItems/
  minProperties、oneOf/anyOf/allOf 与 type union、`$ref`/`$defs`、salvage 模式
  (数组→对象映射 `mapListToObject`、单元素根数组解包、缺失必填填充、
  非法数组项丢弃、JSON 字符串容器解包/修复)。
- 校验层用 `gojsonschema`(D-005 落地),修复完成后终验;validator 按 root
  schema 缓存。`SchemaDefinitionError` 区分 schema 定义错误与数据 ValueError。
- `pattern_properties.go` 实现 Python `match_pattern_properties` 安全子集
  (字面量 + `^`/`$` 锚点,遇不支持 token 记 unsupported 并打日志,不执行用户正则)。
- `parser_schema.go` 提供 parser 侧 `ObjectSchemaConfig`/`ArraySchemaConfig`
  与 `resolveParserObjectSchema/ArraySchema`,schema 引导分发仍在 `parseJSONSchema`。
- 翻译 3 份 Python schema 测试为 Go table-driven test;pydantic-model/
  module-import 用例无 Go 对应,刻意省略(测试文件头注释已说明)。
- 涉及文件:`schema_repair.go`、`parser_schema.go`、`pattern_properties.go`、
  `schema_value.go`、`schema_repairer_test.go`、`schema_guided_parse_test.go`、
  `schema_parser_paths_test.go`、`go.mod`(新增 gojsonschema)。

### 2026-09-17 建档(阶段 0)
- 建立模块文档;尚未写 Go 代码;外部库选型(gojsonschema)与 Pydantic 等价方案待阶段 4 前确认。

## 当前状态 / TODO
- 阶段 4(standard)+ 阶段 5(salvage)**已完成**:3 份 Python schema 测试
  全部翻译且 `go test` 全绿(2026-09-18)。
- D-006 键序缺口:✅ 已关闭(2026-09-21,schema 侧保序见"已知约束与坑"更新,
  类型本体见 modules/ordered_map.md)。
- 阶段 6(CLI)已完成(2026-09-18,见 modules/cli.md);本模块无剩余 TODO,
  唯一相关遗留是终验嵌套实例消息的 repr 渲染器(见"已知约束与坑",刻意不追)。
