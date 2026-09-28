# cli

## 职责
命令行入口,对标 Python `json_repair` 命令(由 `cmd/json_repair` 独立子包提供)。
支持文件/ stdin 输入、inline/output 写回、ensure_ascii、indent、
skip-json-loads、schema/schema-model、strict、schema-repair-mode 等参数。
只做参数解析与调用 `api`,不含解析逻辑。

## 对外接口
- `main()`(os.Exit(cli(os.Args[1:])))
- `cli(args []string) int` — 可测试入口(args 注入);无 filename 时读
  `os.Stdin`,写 `os.Stdout`/`os.Stderr`(测试可替换这三个包级变量捕获)。
- `parseCLIArgs(args []string) (*cliFlags, error)` — 手写 argparse 等价解析
  (位置参数 filename 可省;-i/--inline、-o/--output TARGET、--ensure_ascii、
  --indent N 默认 2、--skip-json-loads、--schema PATH、--schema-model MODEL、
  --strict、--schema-repair-mode {standard,salvage} 默认 standard;
  值参数支持 `--flag value` 与 `--flag=value` 两种形态)。
- `loadSchemaFile(path string) (any, error)` — `--schema` 文件加载,
  走 `json_repair.DecodeJSON`(严格解码 + D-002 int/float 映射,**不**走修复器)。

## 依赖关系
- 依赖:`api`(`FromFile`/`Load`/`Dump`/`Options`/`DecodeJSON`)。
- 被依赖:无(终端入口)。

## 关键文件路径
- src: `cmd/json_repair/main.go`、`cmd/json_repair/cli.go`
- test: `cmd/json_repair/cli_test.go`(从 Python 同名测试翻译)
- 参考(Python): `../json_repair-main/src/json_repair/__main__.py`、`json_repair.py: cli()`

## 已知约束与坑
- 参数互斥校验**顺序**与 Python 一致(1 内联缺文件名 → 2 内联+输出 →
  3 schema+schema-model → 4 strict+schema → 5 salvage 缺 schema),
  stderr 文案逐字对齐(测试断言依赖文案)。
- `--schema-model` 保留在 CLI 表面(让互斥校验与 Python 同序同文案),
  但 Go 无 Pydantic 对应(D-005),单独使用时恒定报
  "Error: --schema-model is not supported by the Go port; ..." 退出 1;
  Python 的 pydantic 测试用例(`test_cli_schema_model_guides_repair`)
  刻意省略,测试文件头注释已说明。
- 退出码约定:参数预检/运行时错误(文件 IO、修复失败、解码失败)→ 1
  (对应 Python `sys.exit(1)`/except 分支);**用法错误**(未知参数、缺值、
  非法 int/choice,对应 argparse)→ 2。
- 输出路径:stdout 用 `fmt.Fprintln`( dumps 结果 + 换行);`-o`/`-i` 写文件
  **不带**结尾换行(`os.WriteFile` 裸字节)。
- D-006 键序(2026-09-21 已关闭):CLI string 输出走 `Dump`,多键对象键序
  现为 Python 插入序(ordered_map_test.go + 差分对拍验证)。
- Python 的 `OSError`/`JSONDecodeError` 错误文案与 Go 不同(如缺文件:
  "[Errno 2] ..." vs "open ...: no such file or directory");这些分支在
  Python 测试里是 `# pragma: no cover`,不追求文案逐字。
- 测试捕获 stdout/stderr 靠替换 `os.Stdout`/`os.Stderr` 包级变量,
  因此实现里必须用 `fmt.Fprintf(os.Stderr, ...)` 这类调用时取值的形式,
  不能缓存 `log` 包的默认 writer。

## 实现细节记录
### 2026-09-18 阶段 6:CLI 实现(阶段 6 完成 = 整个移植的最后一个阶段)
- `cmd/json_repair/cli.go` 移植 Python `cli()`:argparse 等价的手写参数解析
  (flag 包会停在第一个位置参数,无法支持 "filename 在前、flags 在后" 的
  交错形态,故手写);五条参数预检按 Python 顺序逐字对齐 stderr 文案;
  文件输入走 `FromFile`、stdin 走 `Load(os.Stdin)`;结果统一 `Dump`
  (Pretty + indent + ensure_ascii),stdout 加换行、写文件不加。
- `--schema` 文件经新增的 `api.DecodeJSON` 严格解码(保持 int/float 区分,
  避免 schema 里 `{"minimum": 1}` 变 1.0 后影响 minItems/minimum 比较)。
- `--schema-model` 保留表面、恒定报不支持(D-005 无 Pydantic 对应)。
- 测试:`cli_test.go` 翻译 Python 9 个用例(pydantic 1 个刻意省略),
  用管道替换 os.Stdout/os.Stderr/os.Stdin 捕获,断言 stdout 逐字 +
  stderr 文案 + 退出码;`TestCLISchemaModelNotSupported` 为 Go 侧新增。
- 冒烟验证:真实二进制跑通文件/stdin/inline/-o/schema fill/salvage/
  三类错误路径。
- 涉及文件:`cmd/json_repair/{main,cli,cli_test}.go`(新)、`json_repair.go`
  (新增 `DecodeJSON`)、`api_test.go`(`TestDecodeJSON`)。

### 2026-09-17 建档(阶段 0)
- 建立模块文档;尚未写 Go 代码。

## 当前状态 / TODO
- 阶段 6:✅ CLI 完成,`test_repair_json_cli.py` 除 pydantic 用例外全部翻译跑绿。
- 无剩余 TODO。整个 6 阶段移植至此完成;遗留缺口集中为 D-006(键序,
  保序 map 大改造)与 parser 的可选性能项(streamStable/tryValidJsonSuffix)。
