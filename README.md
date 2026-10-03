# qsim — 本地量子线路仿真与态矢量查看工具

一个用 Go 编写的小规模态矢量（state vector）量子线路仿真器。输入是手写的
线路描述文本，工具在其上做确定性演化，输出**可复现**的态矢量、测量结果与
多次采样统计；同时支持 ASCII 线路图和逐门态矢量导出。

## 1. 构建

```bash
go build ./...
go test ./...
go build -o qsim ./cmd/qsim
```

## 2. 线路描述语法

文件为纯文本，每行一条指令；`#` 到行尾为注释，空行忽略，允许前导空白。
大小写不敏感的门名会被规范化为大写，比特索引从 `0` 开始。

### 2.1 比特声明（必须、且只能出现一次）

```
qubits <n>
```

- 必须是**第一条非注释指令**：在它之前出现任何门都会被拒绝。
- `n` 必须是正整数；重复声明会被拒绝。

### 2.2 门指令

```
<门名>[(参数)] <目标>[, <目标> ...]
```

操作数之间用逗号和/或空白分隔。受控门在基门名前加 `C`（一个控制位）或
`CC`（两个控制位），控制位是操作数列表最前面的比特。

| 类别 | 门 | 示例 |
|------|----|------|
| 单比特固定门 | `I X Y Z H S SDG T TDG` | `h 0` |
| 参数门（角度，单位**度**） | `RX RY RZ P` | `rx(90) 0`、`p(45) 2` |
| 受控门 | 任意固定/参数单比特门加 `C`/`CC` | `cx 0,1`、`ccx 0,1,2`、`crx(30) 0,1` |
| 多比特门 | `SWAP` | `swap 0,1` |
| 测量 | `measure` / `m` | `m 1`、`m 0,1`、`measure`（无参数 = 测全部） |

测量是**立即坍缩**的：执行到该指令即按概率投影、归一化并更新态矢量；
后续门作用在坍缩后的态上。`m 0,1` 等价于按顺序 `m 0` 再 `m 1`。

旋转矩阵约定（θ 为弧度，输入的度会自动换算）：

```
RX(θ)=exp(-iθX/2)  RY(θ)=exp(-iθY/2)  RZ(θ)=exp(-iθZ/2)  P(θ)=diag(1,e^{iθ})
```

### 2.3 示例：Bell 态

```
qubits 2
h 0        # 在比特 0 上叠加
cx 0,1     # 受控翻转：比特 0 为 1 时翻转比特 1
m 0,1      # 测量，两个结果必然相同
```

## 3. 命令行用法

```
qsim run      <file> [--seed N] [--shots K]   # 演化；--shots>1 时输出采样计数
qsim draw     <file>                          # ASCII 线路图
qsim snapshot <file> [--seed N] [--json]      # 每扇门后的态矢量
qsim sample   <file> --shots K [--seed N]     # 多次采样统计
```

`<file>` 传 `-` 时从标准输入读取线路。

```bash
$ qsim draw examples/bell.qc
     1   2   3   4
q0: -[H]--*--[M]-----
q1: -----[X]-----[M]-

$ qsim run examples/bell.qc --seed 42
$ qsim sample examples/bell.qc --shots 1000 --seed 42
$ qsim snapshot examples/bell.qc --seed 1          # 文本
$ qsim snapshot examples/bell.qc --seed 1 --json   # JSON
```

采样结果串的位序：线路含 `measure` 时按**测量声明顺序**排列被测比特；
不含测量时对全部 n 个比特测量，比特 0 在字符串最左端。

## 4. 确定性与可复现性

- 酉演化是纯函数：同一份线路重复仿真，态矢量**逐位一致**（测试
  `TestDeterminismRepeatRun` 覆盖）。
- 测量使用随机数；只要通过 `--seed N`（或库的 `NewSeededSimulator`）提供
  种子，测量坍缩结果与 `--shots` 采样序列都严格可复现
  （`TestSampleReproducible`、`TestSnapshotReproducible`）。
- 测量坍缩后的态矢量与记录的 outcome 严格对应：未测到的基态振幅全部清零，
  其余重新归一化（`TestMeasurementCollapse`）。

## 5. 拒绝策略：不静默、不猜测、不留半成品

解析到无法识别的内容时**立即报错并定位行、列**，绝不跳过或猜测意图；
解析失败时返回 `nil` 线路，调用方拿不到任何部分状态（`TestParseNoHalfState`）。
不同拒绝原因是**不同的错误类型**，可用 `errors.As` 区分：

| 错误类型 | 触发场景 |
|----------|----------|
| `*ParseError` | 未知指令、操作数个数不对、操作数不是整数、括号未闭合、列号定位等 |
| `*TargetOutOfRangeError` | 目标/控制比特不在 `[0, n)` 内 |
| `*MissingParamError` | `rx/ry/rz/p` 缺参数、参数不是数字、参数个数不对 |
| `*ConflictError` | 缺 `qubits` 声明、门出现在声明之前、重复声明 |
| `*DuplicateTargetError` | 同一条指令里同一比特出现多次 |

错误信息形如 `parse error at line 3, column 1 near "foobar": ...`。
CLI 在拒绝时以退出码 2 退出，并在 stderr 标注拒绝类别。

## 6. 规模与性能

- 态矢量稠密存储：内存 O(2^n)，长度 2^n 的 `complex128`。
- 每扇 1/2/3 比特（含受控）门 O(2^n)；自由比特块枚举为摊还 O(1) 增量推进，
  不随门数或控制位数量产生额外指数因子。
- 18 比特、35 扇门的线路单次演化 < 2s 有测试上限
  （`TestLargeScaleBoundedTime`，慢 CI 机器上约 0.1s 量级）。

## 7. 自动化测试

```bash
go test ./... -v
```

覆盖场景：

- 解析失败定位（行/列）、各类拒绝原因的类型区分、拒绝时无半成品；
- 越界目标、缺失参数、重复声明、重复目标；
- 重复仿真逐位一致、同种子独立仿真器一致；
- 已知态（Bell、H、RY、ZX、P）、酉演化概率归一；
- 测量坍缩态与 outcome 严格对应、采样可复现、采样位序；
- 16/18 比特较大规模输入的正确性、归一性与时间上限。

## 8. 作为库使用

```go
c, err := qsim.Parse(src)                 // 解析 + 校验
sim := qsim.NewSeededSimulator(c.NumQubits(), 42)
sim.Run(c)                                // 演化
state := sim.StateVector()                // 态矢量拷贝
recs  := sim.Measurements()               // 测量记录
counts, _ := sim.Sample(c, 1000)          // 多次采样
snaps, _  := qsim.Snapshot(c, ptr(int64(1))) // 逐门快照
qsim.DrawASCII(c, os.Stdout, qsim.DefaultDrawConfig())
```
