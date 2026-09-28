# 决策记录(ADR 风格)

> 只记录**跨模块的关键技术决策**与从 Python 原版继承的**移植不变量**。
> 模块级细节/踩坑放对应 `modules/*.md` 的"已知约束与坑",此处不重复。

## D-001 单主包结构
- 状态:已定(阶段 0)
- 决策:除 `cli` 子包外,全部解析逻辑放同一 Go 包 `json_repair`。
- 原因:Python 各解析模块在同一包内大量访问 unexported 解析器状态;拆多包会
  迫使内部方法导出,破坏封装并偏离原实现。逻辑模块用文件分组 + .pmemory 文档表达。
- 后果:根目录 .go 文件较多;新增模块只加文件,不加子包。

## D-002 JSON 值类型映射
- 状态:已定(阶段 0;dict 一项被 D-006 于 2026-09-21 更新)
- 决策:整数 → `int64`/`uint64`,小数 → `float64`,`None`→`nil`,
  `dict`→`*OrderedMap`(保序,见 D-006),`list`→`[]interface{`,`bool`→`bool`。
- 原因:`encoding/json` 默认数字全 `float64`,会把整数 `1` 变成 `1.0`,
  无法 1:1 对齐 Python `json` 的 int/float 区分。
- 后果:解析数字时需按字面量判断 int/float 并选对应 Go 类型;测试断言要按此写。

## D-003 ensure_ascii 序列化
- 状态:已实现(阶段 3,`dump.go`)
- 决策:手写 JSON 序列化器以复刻 Python `json.dumps(ensure_ascii=True)` 的
  `\uXXXX` 转义;Go `json.Marshal` 默认输出 UTF-8 不满足 1:1。
- 原因:无第三方捷径;这是 1:1 的必然要求,不是可选项。
- 落地:`Dump(v, DumpOptions)` 支持 `EnsureASCII`/`Pretty`/`Indent`;字符串转义
  与 Python 一致(`"`/`\`/`\n`/`\t`/`\r`/`\b`/`\f` + 控制字符 `\u00XX`;astral 用
  surrogate pair `\uD8xx\uDCxx`)。

## D-004 module path
- 状态:已定(阶段 0)
- 决策:module 名为 `json_repair`(无组织前缀)。
- 原因:本地重构,无既定仓库域名。若后续要发布到模块代理,再改为带域名 path。

## D-005 schema 校验库选型
- 状态:已定并已落地(阶段 4,2026-09-18)
- 决策:`github.com/xeipuuv/gojsonschema`,作为 `RepairValue` 修复完成后的最终校验
  器(修复启发式本身仍是 Python 逻辑的手工移植,不走第三方)。
- 原因:纯 Go 生态里唯一成熟的 JSON Schema 校验库;校验层与修复层分离,
  修复行为以 Python 测试套件为准,校验兜底交给库。
- 后果:go.mod 新增 gojsonschema 依赖。**Pydantic v2 模型输入无 Go 对应**:
  `SchemaFromInput` 对非 dict/非 bool 输入直接报 "Schema must be a JSON Schema
  dict or boolean schema.";Python 测试套件中 pydantic-model/module-import 用例
  在 Go 侧刻意省略(Go 测试文件头部有注释说明)。**CLI 侧(阶段 6)**:
  `--schema-model` 保留在参数表面(保证互斥校验的顺序与文案与 Python 一致),
  但单独使用时恒定报 "Error: --schema-model is not supported by the Go port; ..."
  退出 1。**终验错误文本(2026-09-20 差分对拍确认,2026-09-21 复核)**:根类型不匹配时 Go 会模仿
  Python jsonschema 文案,但嵌套位置的类型违规走 gojsonschema 原文,与 Python
  的"失败实例完整 repr"消息不同;D-006 保序 map 已落地(2026-09-21),repr 键序
  障碍消除,剩余缺口是完整 Python repr 渲染器,仍刻意不追(见 modules/schema_repair.md)。

## D-006 对象键序:sorted-key 输出的 1:1 缺口
- 状态:**已关闭(2026-09-21)**——`ordered_map.go` 的 `*OrderedMap` 全链路落地。
- 原缺口(阶段 3 时):解析器产出 `map[string]any`(丢插入顺序),`Dump` 输出
  对象时按键排序。Python `json.dumps` 保留 dict 插入顺序;Go map 迭代随机,
  排序是唯一确定性选择。多键对象的字符串输出与 Python 非逐字节一致。
- 关闭方案:
  1. `*OrderedMap`(`keys []string + values map[string]any`)复刻 Python dict
     语义:已有键 Set 原地更新(位置不变)、新键 Set 追加末尾、Delete 后
     重新 Set 追加末尾;构造用 `NewOrderedMap`(零值不可用)。
  2. 全解析链路改为产出/消费 `*OrderedMap`:stdlib 快速路径
     (`decodeOrderedStrict` 保序解码)、repair 路径(`parseObject`/
     `completeObjectParse` 多顶层合并)、schema 引导路径
     (`repairObject` 重建序=schema properties 插入序 + 剩余输入键输入序,
     与 Python `_repair_object` 逐段一致)、salvage `mapListToObject`
     (按 schema properties 插入序映射)。`Dump` 按插入序输出;
     `MarshalJSON` 委托 `Dump`,故 encoding/json 消费者也保序。
  3. Go 专属边界约定:plain `map[string]any` 输入(Go map 字面量本身无序)
     在边界一次性深转换为 `*OrderedMap`,键取**排序序**——这是"无序输入"
     的确定性约定,不是行为分叉;要表达键序的 schema/值必须构造
     `*OrderedMap`(`orderedSchema`/`NewOrderedMap`+`Set`)。
- 验证(2026-09-21):
  1. `ordered_map_test.go`:12 条键序回归用例(快速路径/repair 路径/
     schema 重建/default 填充/salvage list→object/对象返回形式),期望值
     全部由 Python oracle(`json_repair-main` + `.venv-oracle`,
     `json.dumps(v, ensure_ascii=False)` 不带 sort_keys)实际跑出后锁定。
  2. 差分对拍(D-009)同步升级:py_ref.py 去掉 `sort_keys`,两侧都按
     **插入序**逐字节对比(键序从此是对拍的一部分);schema 组合改用
     `diffSchemaOrdered()`(*OrderedMap,属性序与 py_ref.py 的 SCHEMA_PERF
     字典字面量完全一致——schema 属性序是输入状态,repairObject 会按它
     重建,两侧必须同序才是真 1:1)。6169 条 corpus × 6 组合 = 37014 条
     全绿,仅剩 2 条已知 INFO(见 D-005)。
- 后果:多键对象的字符串输出与 Python 逐字节一致;`RepairJSON`/`Loads`
  对象返回形式的键序也 1:1(返回 `*OrderedMap`)。

## D-007 float 字符串格式规则
- 状态:已定(阶段 3,`dump.go:pythonFloatString`)
- 决策:复刻 Python `str()/repr(float)`:最短往返数字,**当且仅当十进制数量级
  `E < -4` 或 `E >= 16` 时用科学计数**;否则定点(整数补 `.0`)。
- 原因:Go `strconv` 的 float 格式(`1` 而非 `1.0`、`1e+16` 阈值不同)与 CPython
  `float_repr` 不一致;这是 1:1 的必然要求。
- 落地:用 `FormatFloat(f,'e',-1,64)` 取最短数字,按 `E` 判定点/科学,再渲染;
  已用 68 个随机+边界值对照 Python 逐字验证(`TestPythonFloatString`)。

## D-008 string 输出路径的 ensure_ascii 默认值
- 状态:已定(阶段 3)
- 决策:`RepairJSONString` 直接透传 `opts.EnsureASCII`(零值 false=保留 UTF-8);
  **不**在函数内硬编码 Python 默认。
- 原因:Python 库 `repair_json(return_objects=False)` 默认 ensure_ascii=True,
  但 CLI 默认 ensure_ascii=False,两处默认相反。交由调用点显式设定,避免函数
  内猜测语义。文档已在 `RepairJSONString` 注释标明。

## D-009 差分对拍 harness(对真实 Python 实现逐字节验证)
- 状态:已落地(2026-09-20,`tools/diff_check/` + `diff_parity_test.go`)
- 决策:除"把 Python 测试翻译成 Go 单测"外,再加一层**差分对拍**:同一批
  输入(6169 条 corpus)分别喂给真实 Python 实现(oracle venv)与 Go 移植,
  归一化输出逐字节对比,`DIFF_CHECK=1` 门控(默认 `go test` 保持快)。
- 原因:翻译单测的期望值本身也是人工转写的,会继承同样的转写错误
  (escaped-object-key 字面量 bug 即由此暴露:Python 测试套件无此输入,
  翻译单测结构上不可能抓出);差分对拍以 Python 运行时为唯一真相源。
- 归一化边界:只消除伪差异(Python 异常类型前缀、`set` 迭代顺序),
  不掩盖真差异;gojsonschema 终验嵌套消息分叉记录为已知 INFO 差异(见 D-005)。
- 后果:本地开发依赖同级 `json_repair-main` 仓库 + `.venv-oracle`;
  corpus/py_ref 为生成物,Python 参考实现变更后需重新生成(测试校验 6× 行数防陈旧)。

## 移植不变量(继承自 Python 原版 AGENTS.md,Go 版必须遵守)
这些是 Python 版用 TDD 反复踩坑沉淀下来的行为约束,1:1 移植必须逐条保持,
否则会在边界用例上与原版行为分叉。各条归属模块:
- [api/parser] 合法 JSON 必须留在 stdlib 快速路径;`strict=true` 不得二次质疑
  `json.loads` 已接受的输入。
- [api] `skipJsonLoads=true` 是显式 opt-out,只用于已知非法输入;不得假设
  其对合法 JSON 的行为与保留 stdlib 路径一致。
- [api] `load(fd)` 行为对标 `json.load(fd)`,从描述符当前位置开始修复。
- [parser] 新增修复启发式时保持 `strict=true` 保守,自动纠正要打 parser log。
- [schema_repair] 提供 schema 时,合法与非法 JSON 都要应用 schema 修复与校验。
- [parser] schema 引导分发集中在 `ParseJSON(schema, path)`。
- [schema_repair] `patternProperties` 匹配刻意限制在安全子集;**不执行用户正则**。
- [schema_repair] `resolveSchema` 尽量保持 schema dict 身份,让校验缓存有效。
- [schema_repair] 校验分离的 anyOf/oneOf 分支时,保留原 root schema 的解析作用域,
  使 root 相对的 `$ref` 能解析 `$defs`。
- [schema_repair] `schemaRepairMode` 仅支持 `standard` 与 opt-in `salvage`;
  salvage 可恢复强证据的结构错配,但**不得静默强转或推断语义属性重命名**
  (如 `results`→`patterns`)。
- [schema_repair] 顺序顶层片段,salvage 可跳过 schema 修复/校验失败的候选,
  返回第一个 schema 合法片段;**绝不从真正的顶层 JSON 数组里挑选元素**。
- [全部] 用户提供的 schema 视为攻击者可控输入:schema 归一化/校验/修复路径
  的深层嵌套需要显式深度限制或受控 `ValueError`,而非未捕获的 `RecursionError`。
- [全部] 顶层 `RecursionError` 统一归一化为 `ValueError`。
- [api] `repair_json` 保持**单一共享的输出收尾路径**(logging、returnObjects、
  空串处理、`json.dumps` 都走这里)。
- [parser] 解析器重构对上下文生命周期与启发式分支顺序敏感;重构 `parseString`/
  `parseObject` 时必须保持畸形输入行为不变。
- [parse_string] 裸字面量只在与分隔符或输入结尾组成完整 token 时才识别;
  `none` 大小写不敏感当 null,但带引号值与其它裸词保持为字符串。
- [parser] 逗号分隔的括号序列当 Python 元组归一为数组;单个括号值保持标量;
  Python 风格字面量仅在数组/对象/元组内支持,不作裸顶层值。
- [parse_string] 保留紧凑正则字符类里 `['"]`/`[^'"]` 的裸引号,
  不要让它把外层 JSON 值拆成多个顶层元素。
- [parse_string] 对象值以双引号开头时,仅当内部非空白内容 + 明确的外层收尾
  证据支持引号跨度时保留第二个引号;保持普通冗余前引号/重复收尾的归一化。
- [parse_object] 对象属性里"未闭合的标量数组 + 其后带引号的 key:value 成员"
  时,闭合数组并恢复外层对象;保持直接数组与标量前缺失对象的恢复
  (`["key": "value"]` → `[{"key": "value"}]`)。
- [parser] 顶层原始解码仅在非逗号垃圾跟随时才丢弃合法前缀;保持完整输入启发式,
  逗号前缀的续接交给结构化对象修复。
- [parser] 性能回归常藏在对长畸形对象值上重复的 `parseString` lookahead 扫描;
  保留含大量逗号或 `}` 且远端才有引号的用例,只保留有显著(约 ≥1s)基线降速的输入。
