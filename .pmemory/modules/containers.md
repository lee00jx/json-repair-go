# containers

## 职责
容器解析:对象(`parseObject`/`parseObjectSchema`)与数组(`parseArray`/
`parseArraySchema`),含 schema 引导的属性/元素解析(properties →
patternProperties 安全匹配 → additionalProperties;items 单 schema/元组 →
additionalItems)与 required/default 收尾(`finalizeObject`)。
数字、注释、Python 括号/元组的解析实际合并在 `json_parser.go`
(见 parser 模块);字符串内部启发式见 parse_string。

## 对外接口
- `(*JSONParser).parseObject() any` — 无 schema 对象
- `(*JSONParser).parseObjectSchema(schema any, path string) any` — schema 引导对象
- `(*JSONParser).resolveObjectPropertySchema(repairer *SchemaRepairer, cfg *ObjectSchemaConfig, key string) (propSchema any, extraSchemas []any, dropProperty bool)`
- `(*JSONParser).parseObjectValueSchema(repairer *SchemaRepairer, propSchema any, keyPath string) any`
- `(*JSONParser).finalizeObject(obj, repairer, cfg, path) map[string]any` —
  required 校验(缺 → error,排序列出)+ 非 required 的 default 填充
- `(*JSONParser).parseArray(closing string) []any` /
  `parseArraySchema(schema any, path string, closing string) []any`
- `(*JSONParser).resolveArrayItemSchema(cfg *ArraySchemaConfig, idx int) (any, bool)`
- 空对象修复:`repairEmptyObjectResult`/`classifyEmptyObjectRepair`/
  `stripCommentsForEmptyObjectClassification`
- 重复 key:`shouldSplitDuplicateObject`/`splitObjectOnDuplicateKey`

## 依赖关系
- 依赖:`parse_string`(对象键/值)、`utils`、`schema_repair`(schema 配置与
  逐值 repair/validate)。
- 被依赖:`parser`。

## 关键文件路径
- src: `parse_object.go`、`parse_array.go`(number/comment/parenthesized
  实际在 `json_parser.go`)
- test: `parse_object_array_test.go`(parity 主体)、
  `schema_guided_parse_test.go`/`schema_parser_paths_test.go`(schema 路径)
- 参考(Python): `../json_repair-main/src/json_repair/parse_{object,array}.py`

## 已知约束与坑
- **移植转写坑(2026-09-20 实锤):带反斜杠/引号的字符串字面量必须逐字节核对**。
  `classifyEmptyObjectRepair` 的 escaped-object-key 分支,Python 条件是
  `'\\":' in body`(3 字符:`\` `"` `:`),Go 最初转写成
  `strings.Contains(body, "\\\":\"")` 实际是 **4 字符**(`\` `"` `:` `"`,多一个
  转义引号),分支从未命中,8 类 `\"` 转义输入塌成空对象。该 bug 逃过了所有
  parity 单测(Python 测试套件里没有这类输入),由差分对拍 harness 抓出。
  教训:① 移植含 `\\`/`\"` 的字面量时用拼接写法(如 `"\\\""+":"`)避免歧义;
  ② 怀疑字面量时 `od -c` 看源码字节 + 运行时打印 `%x` 看编译后值,别只信肉眼。
- 对象属性里"**未闭合的标量数组 + 其后带引号的 key:value 成员**"时,闭合数组
  并恢复外层对象;保持直接数组与标量前缺失对象的恢复
  (`["key": "value"]` → `[{"key": "value"}]`)。
- 空对象修复要区分"真空对象"与"注释包裹的空对象"(`stripCommentsForEmptyObjectClassification`)。
- 重复 key:按 strict 与否决定是否拆分对象/抛错(`shouldSplitDuplicateObject`)。
- 括号:单个括号值保持标量,逗号分隔括号序列归一为数组(见 parser 约束)。
- 数字:按字面量区分 int/float(见 DECISIONS D-002 类型映射)。
- 注释:支持 `#` 与 `/` 风格,识别顶层值记录。
- **注释终止字符必须按「整个上下文栈成员」判断,不能用栈顶 `currentIs`**:
  Python `parse_comment` 三处终止符判断都是 `ContextValues.X in self.context.context`
  (栈成员),不是 `self.context.current == X`。嵌套对象里栈同时含 OBJECT_VALUE 和
  OBJECT_KEY,所以 `#` 行注释的终止符会同时含 `}` 和 `:`;`# comment }`(无换行)时
  `#` 必须先停到更靠前的 `}` 而非 key3 之后的 `:`,这样外层 key3 才能被保留。
  若误用 `currentIs`(只看栈顶 OBJECT_KEY)则终止符缺 `}`,`#` 会一路吃到 key3 的 `:`,
  把 key3 吞掉、错误地把 value3 赋给空 key。`//` 行注释固定只认 `\n`/`\r`(无换行时会吃掉
  `}` 丢弃后续 key,这是 Python 的既定行为,Go 必须保持一致,勿"修好"它)。
  参见 DECISIONS 或下方实现记录。

## 实现细节记录
### 2026-09-20 修复 escaped-object-key 分支的转写 bug(差分对拍抓出)
- 现象:`{"a": {\"b\": 1}"}` 等 8 类输入(空对象 body 以 `\"key\":` 开头)Go 解析
  成空对象/丢值,Python 会规范化 `\"`→`"` 后重解析为对象(见 Python
  `_classify_empty_object_repair` 的 escaped-object-key 分支)。
- 根因:Go 的 Contains pattern 多写了一个转义引号,匹配 4 字符 `\"":` 而非
  Python 的 3 字符 `\":`,分支条件恒 false。排查过程:两侧 repair log 对比定位
  到分叉日志 → 临时 debug 打印 `bodyHex`/pattern hex 发现编译后 pattern 是
  `5c 22 3a 22`(4 字节)——源码字面量肉眼读不出来(转义歧义)。
- 改法:`parse_object.go` 条件改用拼接写法 `strings.Contains(body, "\\\""+":")`
  并加注释说明 pattern 必须是 3 字符;与第二个 disjunct(`\'`+`:`)风格统一。
  全库 grep 确认仅此一处同类字面量。
- 验证:8 输入 × 4 组合(skip_json_loads × stream_stable)probe 与 Python 逐条
  一致;新增 `escaped_object_key_test.go`(8 输入 × 4 组合 = 32 断言,期望值
  取自 Python 参考实现,corpus #5469/5473/5478/5502/5533/5535/5536/5544);
  差分对拍 harness 32 fatal → 0。
- 涉及文件:`parse_object.go`、新增 `escaped_object_key_test.go`。

### 2026-09-18 阶段 4:schema 引导的对象/数组解析路径
- `parseObjectSchema`:逐 key 走 `resolveObjectPropertySchema`(properties →
  patternProperties 安全匹配,不支持的正则打
  "Skipped unsupported patternProperties regex '…' while parsing object key '…'"
  日志(比 repairer 侧文案多后缀,与 Python parse_object.py 一致)→
  additionalProperties true/false/dict);值走 `parseObjectValueSchema` →
  `parseJSONSchema` 按 schema 类型分发;多 pattern 命中时 extraSchemas 逐个
  链式 `RepairValue`;additionalProperties=false → 丢弃属性(repairer 打
  "Dropped extra property not covered by schema")。
- `finalizeObject`:standard 缺 required 报 "Missing required properties at …"
  (键排序);非 required 带 default 的属性补值;salvage 模式放行缺失
  (repairer 层 `fillMissingRequiredForSalvage` 负责填)。
- `parseArraySchema`:items 单 schema/元组(按位)+ additionalItems
  (true/false/dict,超出的项按配置保留或丢弃);salvage 下 repair 失败的项
  丢弃(repairer 打 "Dropped invalid array item while salvaging")。
- 空对象启发式带 schema 化:`classifyEmptyObjectRepair`/
  `repairEmptyObjectResult`(set 字面量保数组、元组形态、空对象回落数组等,
  见 `schema_parser_paths_test.go` 对应用例)。
- schema 错误经 `recordSchemaErr` panic(`schemaAbort`)传递,由
  `ParseWithSchema` recover(直接调用 parseXxxSchema 的测试用
  `catchSchemaPanic` 包一层)。
- 涉及文件:`parse_object.go`、`parse_array.go`、`parser_schema.go`;
  测试:`schema_guided_parse_test.go`、`schema_parser_paths_test.go`。

### 2026-09-18 补记:阶段 1+2 实现状态(此前未随做随记)
- 对象/数组的基础解析与全部畸形修复 corner case 已完成,parity 测试
  (`parse_object_array_test.go` 等)全绿。
- 数字/注释/括号元组解析实现时合并进了 `json_parser.go`,没有独立
  `parse_number.go` 等文件(本文档旧版"关键文件路径"与实际不符,已按
  代码为准修正)。

### 2026-09-17 修复注释终止符按整个上下文栈判断(修 `#` 吞 key3)
- 现象:`{ "key": { "key2": "value2" # comment }, "key3": "value3" }`(无换行)
  被 Go 错误解析成 `{"key":{"key2":"value2"},"":"value3"}`(key3 丢失),而 Python
  基准保留 key3。
- 根因:`parseComment` 里 `#`/`//` 的终止符,对 OBJECT_VALUE / OBJECT_KEY 误用了
  `p.context.currentIs(...)`(只看栈顶当前值);Python 是 `ContextValues.X in
  self.context.context`(栈成员判断)。嵌套对象时栈同时含 OBJECT_VALUE 与 OBJECT_KEY,
  `currentIs` 只命中栈顶 OBJECT_KEY,导致终止符缺 `}`,`#` 一路吃到 key3 的 `:`。
- 改法:`json_parser.go` `parseComment` 中把 `OBJECT_VALUE`/`OBJECT_KEY` 两处由
  `currentIs` 改为 `in`(ARRAY 那处本就用 `in`,不动)。加注释说明为何必须用栈成员。
- 顺带清理:`parse_object.go` 里一行遗留的 `fmt.Printf("DBG-RET ...")` 调试输出删除
  (调试时留下,正式代码不应打印)。
- 验证:`TestParseComment` 全绿;与 Python 对 4 个用例(有/无换行 × `#`/`//`)逐一比对
  结果完全一致(无换行 `#` 保留 key3、无换行 `//` 丢弃 key3,有换行两者都保留)。
- 涉及文件:`json_parser.go`、`parse_object.go`。

### 2026-09-17 建档(阶段 0)
- 建立模块文档;尚未写 Go 代码。

## 当前状态 / TODO
- 阶段 1/2:✅ 基础 + 全部畸形 corner case,parity 测试全绿。
- 阶段 4:✅ schema 引导对象/数组路径(见上方记录)。
- D-006 键序缺口:✅ 已关闭(2026-09-21,解析器产出 `*OrderedMap`,见
  modules/ordered_map.md)。容器模块无剩余 TODO。
