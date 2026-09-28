# ordered_map

## 职责
`*OrderedMap`:插入序保序的 string-key map,Python dict(CPython 3.7+)的直接对应物。
它是本项目**所有解析/修复输出的对象载体**,保证对象键序与 Python `json.dumps`
1:1(D-006,2026-09-21 关闭)。
不做:不解析 JSON(parser 的事)、不做 schema 校验(schema_repair 的事);
纯数据结构 + 序列化入口(`MarshalJSON` 委托 `Dump`)。

## 对外接口
- `NewOrderedMap() *OrderedMap` — 唯一安全构造方式(零值不可用)。
- `Set(key, value)` — 镜像 `d[key]=value`:已有键**原地更新、位置不变**;新键追加末尾。
- `Get(key) (any, bool)` / `GetOrNil(key) any`(镜像 `d.get`)、`Has(key) bool`、
  `Delete(key)`(不存在时 no-op)、`Len() int`。
- `Keys() []string` / `Values() []any` — 插入序(镜像 `list(d.keys())`/`list(d.values())`)。
- `Clone() *OrderedMap` — 浅拷贝(键序/值容器独立,值本身共享)。
- `ToPlain() map[string]any` — 无序视图(一次性拷贝)。
- `MarshalJSON() ([]byte, error)` — 委托 `Dump(m, {})`,使 `encoding/json`
  消费者(gojsonschema 终验、用户 `json.Marshal` 返回值)也按 Python 键序输出。
- `OrderedFromPlain(map[string]any) *OrderedMap` — 导出;plain map 无序,取**排序键**。
- unexported:`toOrderedSchema`、`orderedMapFromPlainDeep`、`orderedFromPlainDeep`
  (plain→ordered 深转换,排序键)、`toPlainValue`(ordered→plain,供忽略键序的比较)。

## 依赖关系
- 依赖:`dump`(`MarshalJSON` 委托 `Dump`)。
- 被依赖:**全部核心模块**——parser(对象产出,含 `decodeOrderedStrict` 快速路径)、
  containers(对象/数组解析、多顶层合并)、schema_repair(schema 载体 + `repairObject`
  重建序 + `mapListToObject`)、api(`Dump` 分派)、utils(ObjectComparer)、
  全部测试(`ordSchema`/`orderedSchema` 助手)。
- 改本模块语义(尤其 Set/Delete 的位置语义)前:先对照 Python dict 语义,
  再跑 `ordered_map_test.go` + 差分对拍。

## 关键文件路径
- `ordered_map.go`(类型 + 转换助手)
- `ordered_map_test.go`(类型语义 + 12 条键序回归用例,期望值由 Python oracle 锁定)

## 已知约束与坑
- **零值不可用**:必须 `NewOrderedMap`(`values` map 为 nil,Get/Set 会 panic)。
- **plain `map[string]any` 输入没有键序**:边界处深转换为**排序键**(D-006 确定性
  约定)。要表达特定键序(如复刻 Python schema 字面量的属性序)必须构造
  `*OrderedMap`;Go map 字面量必然丢失 Python dict 字面量的顺序——差分对拍首跑
  的 478 条 schema 组合 mismatch 即由此而来(diff_parity_test.go 改用
  `diffSchemaOrdered()` 解决)。
- `Clone` 是浅拷贝,嵌套值共享;JSON 树深拷贝用 `deepCopyJSON`(schema_value.go)。
- `Delete` 是 O(n)(切片移除键);当前数据规模无问题,勿提前优化。
- `MarshalJSON` 用 Dump 默认形态(compact + ensure_ascii off),与
  `encoding/json` 的紧凑无空格默认不同——这是刻意的(Python 形态优先)。

## 实现细节记录
### 2026-09-21 D-006 关闭:键序回归测试锁定 + harness 升级 + 文档补齐
- 前一会话(2026-09-21 上午)已完成 `*OrderedMap` 全链路实现(见下条补记),
  但缺键序回归测试与 .pmemory 文档;本次补齐:
  - 新增 `ordered_map_test.go`:`TestOrderedMapSemantics`(Set/Get/Delete/
    Keys/Values/Clone/GetOrNil/ToPlain)+ 5 组 12 条键序回归用例
    (fast 路径 2、repair 路径 6[截断/未引号键/重复键/尾随合并/双顶层×2]、
    schema 重建 3[属性序重建/extra 追加/default 填充]、salvage list→object 1、
    对象返回形式 1)。所有期望字符串先由 Python oracle
    (`repair_json(return_objects=True)` + `json.dumps(v, ensure_ascii=False)`
    不带 sort_keys)实跑取得,再与 Go 实际输出核对后锁定。
  - 差分对拍升级:py_ref.py 去掉 `sort_keys`,两侧按**插入序**逐字节对比
    (键序从此是对拍的一部分);schema 组合改用 `diffSchemaOrdered()`
    (*OrderedMap,属性序与 py_ref.py 的 SCHEMA_PERF 字典字面量一致)。
    重跑 37014 条全绿(首跑曾 478 mismatch,全部为 schema 组合的键序分叉,
    根因即上述 plain-map 排序约定)。
  - 删除临时探针 `temp_order_probe_test.go`、`tools/diff_check/probe_keyorder.py`。
  - `parser_schema.go` 仅 gofmt 注释对齐(结构体字段注释漂移)。
- 涉及文件:`ordered_map_test.go`(新)、`diff_parity_test.go`、
  `tools/diff_check/{py_ref.py,py_ref.jsonl}`、`parser_schema.go`、
  `.pmemory/` 多处(D-002/D-005/D-006、INDEX、api/cli/containers/schema_repair/
  diff_check 模块文档)。

### 2026-09-21 (前一会话,补记) 全链路 *OrderedMap 实现
- 新增 `ordered_map.go`;解析器快速路径(`decodeOrderedStrict` 按 stdlib token
  流保序解码)、repair 路径(`parseObject` 产出、`completeObjectParse` 多顶层
  合并按 `obj.Set` 语义并入)、schema 路径(`ObjectSchemaConfig.Properties` 改
  `*OrderedMap` + `PropertyKeys` 切片;`repairObject` 重建序=schema properties
  插入序 + 剩余输入键输入序;`mapListToObject` 按 schema 属性插入序映射)、
  `Dump` 插入序输出,全部改为 `*OrderedMap`;plain map 输入在边界
  (`SchemaFromInput`/`NewSchemaRepairer`/`RepairValue`/`decodeOrderedStrict`
  入口)一次性归一化为排序键 ordered 形态。(当时漏记 CHANGELOG,本次补记。)

## 当前状态 / TODO
- **已关闭**。唯一相关遗留:gojsonschema 终验的嵌套实例消息与 Python jsonschema
  完整 repr 文本不同(2 条 INFO,见 schema_repair.md);其中 D-006 键序部分
  已不再是障碍,剩余是 Python repr 渲染器,刻意不追(D-005)。
