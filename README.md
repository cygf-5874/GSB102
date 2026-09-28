# tlvkeep

TLV 编解码库（Go 1.24，仅标准库，`go.mod` 无 `require`）。

线格式是 `Tag(varint) | Length(varint) | Value(Length 字节)`。已知字段照旧编解码；
**本次要补的一层**是把「别人写进来的未知字段」原样透传，并支持一层嵌套 TLV。

```go
b := []byte{ /* Tag|Len|Value 序列 */ }
m, _ := tlvkeep.Decode(b)
out, _ := tlvkeep.Encode(m)   // 对规范输入逐字节等于 b
```

## 怎么跑

```bash
go build ./...             # 编译
go test ./...              # 既有用例（13 个）
go run ./check             # 验收场景（固定件，9 个）
go run ./check -list       # 列出全部场景
go run ./check --only keep
bash scripts/check.sh      # 同上，薄封装
```

前提：Go 1.24。不引入任何第三方依赖。

## 对外契约

`Message` 的字段名与类型已经定死（见 `tlv.go`），下面 9 条是语义。
**它们是契约，不是「当前行为」的转述。**

1. **线格式**：每一段是 `Tag(varint) | Length(varint) | Value(Length 字节)`（LEB128 无符号 varint）。
   `Tag` 必须**严格递增**；唯一的例外是 `Tag 4`（`Tags` 重复段），它可以连续出现多次。
   顺序回退、或除 `Tag 4` 之外的重复 Tag，都返回 `ErrOrder`。
2. **已知 Tag**：`1:Name(string)`、`2:Age(varint)`、`3:Email(string)`、`4:Tags([]string，重复段)`。
3. **既有行为（不得回归）**：解码已知 Tag、缺失字段取零值、顶层消息的 `Name` 必填
   （缺失返回 `ErrMissing`；嵌套层不强制）。`Encode` 的口径：`Name` 非空才写 `Tag 1`、
   `Age` 非 0 才写 `Tag 2`、`Email` 非空才写 `Tag 3`、`Tags` 每个元素各写一段 `Tag 4`，
   Length 用**最短** varint。
4. **本次新增**：解码时把**所有未知 Tag 的原始 `Tag|Len|Value` 字节**保留在 `Unknown [][]byte`
   里，按 Tag 升序排列（多段时按出现顺序）。「原始字节」指输入里那一段的**逐字节拷贝**。
5. **重新编码时**：未知字段必须按 Tag 升序**插回**到正确的位置，且**字节与原始完全一致**
   —— 包括 Length 的 varint 编码方式。
6. **对未知字段不得做规范化**：Length 用非最短 varint 编码时也要**原样保留**
   （这一条与第 5 条互相咬合：插回原位 ≠ 重编码）。
7. **嵌套**：`Tag 5` 是 `Nested`，其 Value 本身是一段完整的 TLV 字节流。
   嵌套深度上限 **4**（顶层记为深度 1）；超过返回 `ErrDepth`。嵌套内的未知字段同样要透传，
   嵌套内的 Tag 顺序规则与本层一致。
8. **往返性质**：`Decode(Encode(m)) == m`；且 `Encode(Decode(b))` 对任意**规范** `b`
   **逐字节等于 `b`**。规范 `b` 指：Tag 顺序合法、已知 Tag 用最短 varint 且按第 3 条的存在规则出现、
   未知 Tag 与嵌套内容可以是任意编码。非规范输入（例如已知 Tag 用非最短 varint）不保证逐字节往返。
9. **边界**：空输入、只有未知字段、Value 长度为 0、超长 Length 声明而实际字节不足
   （返回 `ErrTruncated`）都不得 panic。

## 本次要补的一层

`tlv.go` 是完整可编译的既有实现：已知字段的顺序编解码、`ErrOrder` / `ErrMissing` /
`ErrTruncated` 都已就位，既有用例 13/13 全绿。它目前**丢掉所有未知 Tag**，
也**不认识 `Tag 5`**。本次要补的就是：

- 未知字段的**保真透传**（含非最短 varint 的原样保留）；
- 嵌套 TLV（`Tag 5`），连同嵌套内的未知字段透传与深度上限。

## 验收

`check/` 是固定验收程序，**不要修改**。场景分四组，共 9 个：

| 组 | 场景 | 覆盖 |
| --- | --- | --- |
| `known` | `KN1_known_decode` | 契约 2、3：已知字段解码 |
| `known` | `KN2_missing_zero_and_name_required` | 契约 3：缺失取零值、顶层 `Name` 必填 |
| `keep` | `K1_unknown_reinserted_in_order` | 契约 4、5：未知字段按序插回、逐字节往返 |
| `keep` | `K2_nonminimal_varint_preserved` | 契约 6：非最短 varint 原样保留 |
| `keep` | `K3_byte_roundtrip` | 契约 5、7、8：混合消息逐字节往返 |
| `nested` | `N1_nested_unknown_passthrough` | 契约 7：嵌套内未知字段透传 |
| `nested` | `N2_depth_exceeded` | 契约 7：深度 5 返回 `ErrDepth`，深度 4 正常 |
| `edge` | `ED1_all_unknown_input` | 契约 3、9：全未知字段输入不 panic |
| `edge` | `ED2_truncated_input` | 契约 9：截断输入返回 `ErrTruncated` |

`--only known|keep|nested|edge` 可以只跑一组。9 个场景都在同一个进程里跑，
每个场景的 panic 会被单独捕获并记成一条 `FAIL`，**一个场景挂掉不会遮蔽其余场景**。
判据全部是硬编码字节串，不依赖墙钟、随机源或 `map` 迭代顺序。

## 目录

```
.
├── .gitattributes
├── .gitignore
├── go.mod               module tlvkeep
├── tlv.go               既有实现（已知字段编解码，未知字段被丢弃）+ 哨兵错误
├── tlv_test.go          既有用例（13 个）
├── PROMPT.md            本题的 User Prompt（逐字）
├── check/main.go        固定验收程序（勿改）
├── scripts/check.sh     薄封装：go run ./check "$@"
└── README.md
```

## User Prompt

```
现在的编解码只认识自己写出去的字段，别人写进来的字段会被丢掉。tlvkeep 是
Go 1.24 的 TLV 编解码库，只用标准库（无第三方依赖），构建走 go 工具链，
自检走 `scripts/check.sh`（`check/` 是固定验收程序，别改），既有用例走 `go test ./...`。

任务：按 README「本次要补的一层」，把未知字段的透传和嵌套做出来，让固定件全过。

验收：
- go test ./... 全绿，退出码 0；
- go run ./check 退出码 0，9 个场景全过（known 2 + keep 3 + nested 2 + edge 2）。

约束：
1. 不改 `check/`；可以新增文件与类型。
2. 对外 API 名与签名不许改；`tlv_test.go` 里的既有用例一条都不许删或改。
3. 不引入第三方依赖，`go.mod` 里不许出现 `require`。
4. 未知字段的原始字节要原样保留，别顺手规范化。
```
