# parse_string

## 职责
字符串解析,是全库启发式最重的模块:处理引号(含智能引号)、转义序列归一化、
正则字符类里的裸引号、未闭合字符串、代码围栏(``` )、裸键/裸字面量、
对象成员边界的 lookahead。对应 Python `parse_string.py`(942 行)及其
`parse_string_helpers/` 子模块。

## 对外接口
- `parseString(p *JSONParser) any`(包内函数,由 parser 分发调用)。
- 一组 `_` 前缀辅助:外/活跃右引号判定、smart-quote 跨度栈、简单引号串尝试、
  正则字符类判定、未配对开括号重建、缓存 skipToCharacter、字符串体扫描、
  结果收尾等(具体签名随实现定)。

## 依赖关系
- 依赖:`utils`(游标 `skipToCharacter`、context、常量)、`parser`(状态)。
- 被依赖:`parser`(分发)。

## 关键文件路径
- src: `parse_string.go`(helpers 可能拆 `parse_string_helpers/`)
- 参考(Python): `../json_repair-main/src/json_repair/parse_string.py`、
  `parse_string_helpers/{parse_json_llm_block,object_value_context,parse_boolean_or_null}.py`

## 已知约束与坑
- 保留紧凑正则字符类里 `['"]`/`[^'"]` 的**裸引号**,不要让它把外层 JSON 值拆成
  多个顶层元素。
- 对象值以**双引号开头**时,仅当内部有非空白内容 + 明确外层收尾证据支持引号
  跨度时保留第二个引号;保持普通冗余前引号/重复收尾的归一化。
- 性能:长畸形值上重复 lookahead 扫描是回归高发区(见 parser 模块同款约束)。
- Python `parse_string_helpers` 里的 LLM block 识别、对象值上下文、布尔/null
  解析要一并对齐,注意它们与主解析的边界。
- `!p.streamStable` 分支(6 处)是**刻意的**:截断流式输入下跳过那些激进的对象值
  逗号/`}`/`]` 归属判定,保持结果稳定;EOF 分支里 `streamStable` 且串尾为悬空
  `\` 时丢弃该反斜杠(与 Python `parse_string.py:843-847` 一致)。见
  `stream_stable_test.go`。

## 实现细节记录
### 2026-09-18 补记:阶段 1+2 实现状态(此前未随做随记)
- `parse_string.go`(约 37KB)完整:Python `parse_string.py` 字符串状态机的
  全量移植——引号跨度判定、智能引号(smart quotes)、转义序列归一化、
  正则字符类裸引号保留、未闭合字符串、代码围栏、裸键/裸字面量、
  对象成员边界 lookahead、双引号开头值的引号保留规则等。
- 对应 Python `test_parse_string.py`(816 行)已翻译为
  `parity_string_test.go` 等测试并全绿;`parse_string_helpers/` 子模块的
  行为(LLM block、对象值上下文、布尔/null)合并在主文件内实现。
- 注:本条为状态补记,细节以代码与测试文件为准。

### 2026-09-17 建档(阶段 0)
- 建立模块文档;尚未写 Go 代码。

## 当前状态 / TODO
- 阶段 1/2:✅ 全部 corner case 完成,parity 测试全绿(见上方补记)。
- 无独立 TODO;D-006 键序缺口不属于本模块。
