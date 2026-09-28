# json_repair-go

Go 版 `json_repair`:修复 LLM/API/日志/用户输入里常见的坏 JSON(缺引号/逗号/括号、
注释、散文、截断值、Python 风格元组),作为 `encoding/json` 的兜底替代。
目标是与 Python 原版(隔壁 `json_repair-main`)行为 **1:1 完全等价**,Python 测试套件是权威行为规格。

## 模块清单
| 模块 | 文档 | 职责(一句话) |
|------|------|--------------|
| api | modules/api.md | `RepairJSON`/`Loads`/`Load`/`FromFile` 入口:参数校验、stdlib 快速路径、输出统一化 |
| ordered_map | modules/ordered_map.md | `*OrderedMap` 保序 dict 类型:解析器/修复器输出载体,键序 1:1 对齐 Python dict(D-006) |
| parser | modules/parser.md | `JSONParser` 核心:字符游标、上下文栈、`ParseJSON` 分发、多顶层值合并、strict、streamStable |
| parse_string | modules/parse_string.md | 字符串解析,启发式最重:引号/正则字符类/未闭合/裸字面量/双引号/代码围栏 |
| containers | modules/containers.md | 对象/数组解析(含 schema 引导属性/元素解析、required/default 收尾) |
| schema_repair | modules/schema_repair.md | `SchemaRepairer` + standard/salvage 模式 + patternProperties |
| utils | modules/utils.md | 常量、JsonContext、ObjectComparer、patternProperties 匹配 |
| cli | modules/cli.md | 命令行入口 `cmd/json_repair`(阶段 6 已落地) |
| diff_check | modules/diff_check.md | 差分对拍 harness:与 Python 参考实现逐字节对拍(DIFF_CHECK=1 门控) |

## 模块依赖关系(文字)
- `api` 是对外入口,依赖 `parser`(修复路径)与 `schema_repair`(带 schema 时);其它模块的起点。
- `parser` 依赖 `parse_string`、`containers`、`utils`;是启发式解析的中枢。
- `parse_string` / `containers` 依赖 `utils`(游标、上下文、常量)。
- `schema_repair` 在 `parser` 分发时被回调(逐值 repair/validate),依赖 `utils`(patternProperties)。
- `ordered_map` 是被全部核心模块共用的对象载体类型(D-006),只依赖 `dump`(MarshalJSON)。
- `cli` 依赖 `api`。
- 全部核心解析逻辑在**同一个 Go 包** `json_repair` 内(启发式大量跨文件引用 unexported 状态),
  仅 `cli` 是独立子包 `cmd/json_repair`。

## 类型映射约定(Go 侧,1:1 对齐 Python json)
- 整数 → `int64`/`uint64`,小数 → `float64`(不全用 float64,避免 `1` 变 `1.0`)
- `None`→`nil`、`dict`→`*OrderedMap`(插入序,D-006)、`list`→`[]interface{}`、`bool`→`bool`
- `ensure_ascii` 需手写序列化器(Python 默认转 `\uXXXX`,Go 默认输出 UTF-8)

## 当前开发阶段
**全部 6 阶段完成(2026-09-18)**:阶段 0(骨架)+ 1(核心解析)+ 2(parse_string/strict)
+ 3(string 序列化/flags)+ 4(schema standard)+ 5(schema salvage)+ 6(CLI)。
Python 测试套件对应的 Go 测试全部翻译且 `go test ./...` 全绿;pydantic-model/
module-import 用例无 Go 对应,刻意省略(D-005)。
遗留:尚无 git 仓库(建议先本地 `git init` 不推远端)。
已定(2026-09-20):CI 暂缓(之后上 GitHub 时用 GitHub Actions,
`TestPerformanceRegression` 已按 `CI` 环境变量自动跳过);发布方式为
**本地先用占位模块名 `json_repair`(D-004),确认无误后再推 GitHub**——
届时改模块 path 为 `github.com/<user>/<repo>`(go.mod 一行 + cli.go 一处
import)+ 打 semver tag 即为正规发布,无需注册模块代理。
(已完成:性能回归网 perf_test.go 移植 + streamStable/tryValidJsonSuffix 实现验证,见 CHANGELOG 2026-09-20。
2026-09-20:差分对拍 harness(diff_check)首跑 37014 条全绿,
抓出并修复 escaped-object-key 转写 bug;遗留 2 条 INFO 差异为
gojsonschema 终验消息分叉,已知且刻意不追 1:1。
2026-09-21:**D-006 保序 map 改造关闭**——`*OrderedMap` 全链路落地,
ordered_map_test.go 12 条键序回归用例(期望值由 Python oracle 实跑锁定);
diff_check 对拍从 sorted-key 升级为**插入序逐字节**对比,37014 条全绿,
仅剩原 2 条 INFO。)

## 分阶段验收网
每个阶段把对应 Python 测试翻译成 Go table-driven test,跑绿才算完成。
Python 测试参考:
- 核心:tests/test_json_repair.py, test_parse_{number,comment,array,object,string}.py
- 模式:test_strict_mode.py, test_performance.py, test_repair_json_from_file.py, test_repair_json_cli.py
- schema:test_schema_repairer.py, test_schema_guided_parse.py, test_schema_parser_paths.py

**范围说明(Python 测试全集的去留)**:
- `test_type_inference.py`(1 例):mypy 类型推断测试,Python typing 专属,
  无 Go 对应,**刻意不移植**。
- `test_docs_app_schema.py`(11 例):文档站 Flask 示例 app 的端点测试,
  不属于"库 + CLI"移植范围,**不移植**。
- `test_performance.py`(16 例):性能回归网,已移植为 `perf_test.go`
  (16 个 `testing.B` benchmark + `TestPerformanceRegression` 阈值测试,
  CI/`-short` 下跳过,阈值取 CPython 的 10 倍;病态负载与 Python 逐字节一致)。
