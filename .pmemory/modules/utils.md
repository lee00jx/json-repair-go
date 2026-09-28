# utils

## 职责
跨解析模块的基础设施:常量与类型别名、对象成员上下文栈 `JsonContext`、
对象相等比较 `ObjectComparer`、`patternProperties` 安全匹配、schema 辅助
(missing 哨兵/`SchemaDefinitionError`/校验前归一,`schema_value.go` 实际
挂在 schema_repair 模块)。不含任何 token 解析逻辑。
(注:Go 侧未实现 Python 的 `StringFileWrapper` 分块包装——`Load`/`FromFile`
直接 `ReadAll` 整读,行为等价,见 api 模块。)

## 对外接口
- `constants.go`:`STRING_DELIMITERS`、`JSONReturnType`(= `any`)及常用常量。
- `JsonContext`:`enter(value) (context, error)`(上下文进入,配合 defer 退出)、
  `empty` 状态、`clear()` 等;`ContextValue` 类型。
- `ObjectComparer`:`IsSameObject(a, b) bool`(顶层"更新"语义用)。
- `pattern_properties.go`:`matchPatternProperties(patternProperties map[string]any, key string) ([]any, []string)`
  ——安全子集匹配,返回(匹配到的 schema, 不支持的正则串)。

## 依赖关系
- 依赖:仅标准库。
- 被依赖:`parser`、`parse_string`、`containers`、`schema_repair` 全部依赖 utils。
  **改动前必看:任何 utils 接口变更影响面=整个解析层。**

## 关键文件路径
- src: `constants.go`、`json_context.go`、`object_comparer.go`、`pattern_properties.go`
- 参考(Python): `../json_repair-main/src/json_repair/utils/*.py`

## 已知约束与坑
- `patternProperties` 匹配限制在安全子集,**不执行用户正则**(与 schema_repair 约束一致)。
- `JsonContext` 的进入/退出要配合 Go 的 defer 语义正确管理栈;Python 用 context
  manager + ExitStack,移植时注意延迟上下文(`deferredContexts`)的重放逻辑。
- `ObjectComparer.IsSameObject` 用于顶层重复对象判定"是否更新",语义要
  与 Python 版严格一致。

## 实现细节记录
### 2026-09-17 修复 JsonContext.reset() 栈顶 off-by-one
- **问题**:`reset()` 在栈深度 >1 时设置 `c.current = c.stack[n-1]`(即正在被弹出的元素),
  而非 `c.stack[n-2]`(弹出后的新栈顶)。导致 `parseObjectValue` 返回后 `currentIs(ContextObjectValue)`
  仍为 true,`completeObjectParse` 因此跳过了"跳过多余 `}`"的逻辑,
  使数组解析器在遇到 stray `}` 时提前退出,产生多余嵌套数组。
- **修复**:`json_context.go` 中 `reset()` 的 default 分支改为 `c.current = c.stack[n-2]`。
- 涉及文件:`json_context.go`
- 测试覆盖:`TestParseObjectAndArrayParity/[{"key":"value"}},{"key":"value"}]` 等。

### 2026-09-17 建档(阶段 0)
- 建立模块文档;尚未写 Go 代码。

## 当前状态 / TODO
- ✅ 全部完成:constants/JsonContext/ObjectComparer/patternProperties 均已落地,
  随阶段 1/2/4 验证通过。
- `StringFileWrapper` 无 Go 对应(`ReadAll` 整读等价),不再列为 TODO。
