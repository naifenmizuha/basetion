# Knowledge 领域模块

## 覆盖路径

- `internal/domain/knowledge`

## 模型与抽象

`Document` 是与存储无关的检索结果，包含 ID、标题、内容和相关度分数。`Retriever` 接口定义查询与数量上限，不暴露向量库或数据库细节。

## 领域规则

`Service.Search` 去除查询首尾空白并拒绝空查询。limit 为 0 时使用默认值 5；负数或大于 20 时返回错误。通过校验后委托 Retriever，并为基础设施错误补充领域操作上下文。

领域模块只依赖 Context 和自身抽象，不依赖 Eino 工具协议、CLI 或具体存储。
