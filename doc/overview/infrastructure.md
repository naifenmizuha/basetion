# Infrastructure 模块

## 覆盖路径

- `src/internal/infra/session`
- `src/internal/infra/teamops`

## Session 文件存储

`FileStore` 将每个已完成 Session 保存为独立 JSON 快照。文件名是 Session ID 的 SHA-256 十六进制摘要，避免直接使用外部 ID 作为路径。加载时会校验文件中的 ID 与请求 ID 一致，并把缺失文件映射为应用层的 `ErrSessionNotFound`。

保存流程会以 0700 创建目录，在同目录创建 0600 临时文件，写入格式化 JSON、同步并关闭后通过 rename 原子替换目标快照。提交前失败会清理临时文件；Context 在读取、写入和提交前均会检查。

## TeamOps Lua 运行时

`LuaExecutor` 使用 GopherLua 为每次查询创建独立 State，只开放裁剪后的 base、`table`、`string` 和 `math` 能力。文件、系统、模块加载、动态代码、调试、协程、channel、打印、随机数和元表修改均不可用。`teamops` 以只读 userdata 代理暴露 `array()` 和 `null`，脚本无法替换宿主能力。

默认限制为 2 秒、32 KiB 源码、32 层返回深度、10,000 个 table 元素和 256 KiB JSON 结果。返回转换支持 nil、布尔、有限数值、UTF-8 字符串与 table；连续正整数键映射为数组，字符串键映射为对象，并拒绝循环、稀疏或混合键及不可序列化值。当前运行时不注册真实 TeamOps 数据模块。
