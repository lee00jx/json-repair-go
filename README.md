# json-repair-go

Go port of the Python library [json_repair](https://github.com/mangiucugna/json_repair).
Repairs malformed JSON produced by LLMs, APIs, logs and user input, targeting
1:1 behavioural parity with the Python original.

```
go get github.com/lee00jx/json-repair-go@latest
```

```go
import json_repair "github.com/lee00jx/json-repair-go"

fixed, logs, err := json_repair.RepairJSONString(`{'a': 1,}`, json_repair.Options{})
```

A CLI lives in `cmd/json_repair`.

## License

MIT. Original work Copyright (c) 2023 Stefano Baccianella; see [LICENSE](LICENSE).
