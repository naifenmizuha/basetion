# 球员、球队与名单领域模块

## 覆盖路径

- `src/internal/domain/player`
- `src/internal/domain/team`
- `src/internal/domain/roster`

## 实体和值对象

`Player` 保存姓名、左右手位标志、多个守备位置、启用状态和乐观锁版本；`Team` 保存名称、启用状态和版本。两者在新建和从持久化恢复时都验证 ID、名称、标志、版本和时间戳，恢复到非法数据时返回损坏数据错误。

`Membership` 表达球员在球队的一段效力期，球衣号码限制为 0–99，结束日期必须晚于加入日期。`Date` 是不含时区和时刻的日历日期值对象。名单实体支持修改当前球衣和结束效力，并通过版本号参与乐观并发控制。

## 领域服务与端口

球员和球队服务通过各自 Repository 与 Clock 创建和修改实体。名单服务依赖 `UnitOfWork`，在事务中取得球队、球员和名单 Repository：分配球员时要求双方启用、加入日期不在未来、同队无当前成员关系、球衣未占用且效力期不重叠；改号和离队会锁定球队与成员关系并校验归属和版本。

领域层只定义实体、业务规则、Repository/UnitOfWork/Clock 端口，不依赖 PostgreSQL、CLI、Eino 或 application 包。当前这些写领域服务由基础设施集成测试直接覆盖，尚未提升为外部可调用的 application 用例；Agent 对名单的访问仅通过 TeamOps 只读投影。
