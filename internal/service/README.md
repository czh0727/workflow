# 服务层

服务层是 HTTP/Proto DTO 与业务层 DO 之间的适配边界。每个资源对应一个 service 文件，负责解析请求、校验输入、调用用例并组装响应。

## 依赖边界

服务层可以引用 `api/...` 和 `internal/biz`，不能引用 `internal/data`、PO 或 SQL 客户端。数据层细节不能通过服务接口泄漏到 API。

## 实现规则

- 使用 `convert<Resource>` 将输入 Proto 转换为 DO；返回响应时在处理方法中组装 Proto。
- 嵌入生成的 `Unimplemented<Resource>ServiceServer`。
- 列表请求使用项目已有的过滤、排序和分页适配器；部分更新使用字段掩码。
- 在服务边界校验请求参数，但把业务规则留在 biz 层。
- 直接返回 biz 错误，不在 service 层访问数据库或远程客户端。

新增 RPC 时先修改 Proto 并运行 `make api`，然后在这里实现生成的接口。服务层测试可以使用假的用例，不需要连接数据库。
