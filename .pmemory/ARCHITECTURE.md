# 架构

## 技术栈
- 语言:Go(go.mod 声明 1.24,本机工具链 go1.26.5)
- 依赖:唯一第三方库 `github.com/xeipuuv/gojsonschema`(schema 校验,
  阶段 4 引入,选型见 DECISIONS D-005);其余仅标准库(`encoding/json`
  用于 stdlib 快速路径)。
- 测试:标准库 `testing`,table-driven,逐条对齐 Python 测试。

## 整体结构
单一主包 `json_repair`(根目录),仅 `cli` 为独立子包 `cmd/json_repair`。
选择单包的原因:Python 版所有解析模块在同一包内大量访问 `self` 的 unexported
状态(游标 index、对象成员 context、lookahead 扫描);若拆成多个 Go 包,这些
内部方法就必须导出,破坏封装且偏离原实现。因此逻辑模块(见 INDEX)对应
同一包内的一组文件,而非物理子包。

文件 → 逻辑模块:
- api:            json_repair.go / dump.go
- parser:         json_parser.go(含 number/comment/parenthesized 解析)
- parse_string:   parse_string.go
- containers:     parse_object.go / parse_array.go
- schema_repair:  schema_repair.go / parser_schema.go / schema_value.go
- utils:          constants.go / json_context.go / object_comparer.go / pattern_properties.go
- cli:            cmd/json_repair/main.go / cli.go

## 数据流
1. `api.RepairJSON` 接收 `jsonStr` 与选项:
   - 参数校验(salvage 需 schema;schema 与 strict 互斥)。
   - **stdlib 快速路径**:`skipJsonLoads=false` 时先用 `encoding/json` 解析;
     带 schema 时快速路径还需 schema validate/repair,不通过才降级。
   - 失败则构建 `JSONParser` 走修复解析;带 schema 走 `ParseWithSchema`。
   - 统一输出口:处理 logging(repair log)、returnObjects、空串、`Dump`
     (手写序列化器,非 `json.Marshal`,见 D-003)。
2. `parser.ParseJSON` 为递归下降分发循环:按首字符分发到
   object/array/parenthesized/string/number/comment;维护 `index` 游标与
   `context`(对象成员状态栈)。
3. 顶层解析 `_parseTopLevel` 处理多个连续顶层 JSON 值的合并/更新语义;
   salvage 模式走 `_parseTopLevelSalvageWithSchema` 返回首个 schema 合法片段。
4. `schema_repair.SchemaRepairer` 在解析过程中被逐值回调(repair/validate)。

## 构建 / 测试 / 发布
- 构建:`go build ./...`(CLI:`go build ./cmd/json_repair`)
- 测试:`go test ./...`(每阶段翻译对应 Python 测试)
- 格式化/静态检查:`gofmt`、`go vet`(阶段 0 未配 CI,后续按需加)
- Python 原版发布走 PyPI;Go 版发布方式待定(见 DECISIONS / cli TODO)。

## 部署
库 + CLI 两种形态。CLI 由 `cmd/json_repair` 提供,对标 Python `json_repair` 命令
(阶段 6 已落地)。无服务端、无外部资源依赖。
