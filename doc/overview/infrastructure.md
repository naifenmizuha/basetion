# Infrastructure 模块

## 覆盖路径

- `internal/infra/session`
- `internal/infra/knowledge`

## Session 文件存储

`FileStore` 将每个已完成 Session 保存为独立 JSON 快照。文件名是 Session ID 的 SHA-256 十六进制摘要，避免直接使用外部 ID 作为路径。加载时会校验文件中的 ID 与请求 ID 一致，并把缺失文件映射为应用层的 `ErrSessionNotFound`。

保存流程会以 0700 创建目录，在同目录创建 0600 临时文件，写入格式化 JSON、同步并关闭后通过 rename 原子替换目标快照。提交前失败会清理临时文件；Context 在读取、写入和提交前均会检查。

## 内存知识检索

`MemoryRetriever` 持有构造时文档切片的副本。默认语料包含架构、Session/Checkpoint 和 AgenticModel 三条开发期知识。

检索将查询按空白和标点分词，对标题与内容执行不区分大小写的词频计分，只返回正分文档；结果按分数降序、ID 升序稳定排序，再裁剪到 limit。该实现是确定性的开发适配器，不提供持久化、语义向量或远程数据源。
