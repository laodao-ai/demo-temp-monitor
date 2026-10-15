# planning-decisions

- `T10-choice` 复核: Task 2 5xx 日志脱敏修法 = 新建脱敏 slog.Handler 包装（NewHandler 签名不变），弃「NewHandler 加脱敏函数参数」 | 客观判据自动选（design「Component List」固定 `NewHandler(svc, logger) http.Handler`），未派对抗镜 | 系统：不改已定接口，脱敏逻辑一处实现供 main/testdb 复用；用户：无感知差异；开发循环：Task 3 只需在 main 包一层 logger。主次：系统镜为主。
