# 球员、球队与名单领域模块

## 覆盖路径

- `src/internal/domain/player`
- `src/internal/domain/team`
- `src/internal/domain/roster`

## 实体和值对象

`Player` 保存姓名、左右手位标志、多个守备位置、启用状态和乐观锁版本；`Team` 保存名称、启用状态和版本。两者在新建和从持久化恢复时都验证 ID、名称、标志、版本和时间戳，恢复到非法数据时返回损坏数据错误。

`Membership` 表达球员在球队的一段效力期，球衣号码限制为 0–99，结束日期必须晚于加入日期。`Date` 是不含时区和时刻的日历日期值对象。名单实体支持修改当前球衣和结束效力，并通过版本号参与乐观并发控制。

## 领域服务与端口

球员和球队服务通过各自 Repository 与 Clock 创建和修改实体。名单服务依赖 `UnitOfWork`，在事务中取得球队、球员、名单和关联自训数据 Repository：分配球员时要求双方启用、加入日期不在未来、同队无当前成员关系、球衣未占用且效力期不重叠；改号和离队会锁定球队与成员关系并校验归属和版本。删除球员时会在同一事务中显式软删除其名单关系与自训记录。

球队与名单分别提供只读 `QueryService`。球队查询返回经 Repository 恢复的 `Team`；名单查询按球队、日期和守备位置读取由 `Team`、`Player`、`Membership` 组成的 `Member`，并用三个领域对象计算指定日期的有效状态。

领域层只定义实体、业务规则、Repository/UnitOfWork/Clock 端口，不依赖 PostgreSQL、CLI、Eino 或 application 包。模型侧的 `team_modify` 直接适配写服务，`team_query` 通过只读领域服务取得领域对象后再转换成 Lua 数据。
