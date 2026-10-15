# 重建演示库时只显式删除本项目的表，不删 schema、不删库

**Status: Accepted** · 来源 change：`temp-monitor-baseline`（2026-10-15）

演示库 `temp_monitor_demo` 需要一个可重复的重建动作，用来清掉测试残留并重装表结构。

远端库由 `pg-dev-init` 建立。owner 角色与库同名，默认不带 CREATEDB 权限。
本 change 开始时远端库还未接入，下面的权限前提无法实测。

## Decision

重建 = 显式删除本项目的表，再顺序执行迁移和种子。

- `db/drop.sql` 只有一条语句：`DROP TABLE IF EXISTS readings, devices CASCADE;`。
- `scripts/db-reset.sh` 把 drop、`db/migrations/*.sql`（按字典序）、`db/seed.sql` 放进一个事务（`--single-transaction`）。
- 脚本先查 `current_database()`，不等于 `temp_monitor_demo` 就退出 3，不执行任何删除。
- 新增表时，必须同步修改 `db/drop.sql`。文件头注释已写明这条要求。

不删 schema，不删库。该方案只依赖「建表者即表 owner」。

## Considered Options

- **DROP SCHEMA public CASCADE**：否决。PG 文档 ddl-schemas 页没有正面说明库 owner 能删 `public`，远端库又无法实测。
- **DROP OWNED BY CURRENT_USER**：否决。它会收回服务器授予该角色的权限。
- **DROP DATABASE**：否决。owner 角色默认无 CREATEDB，没有权限。
- **显式删表（采用）**：权限前提最弱，且在单个事务内，失败时库保持原样。

## Consequences

- 系统镜：drop 清单与迁移要同步，多一个维护点。不依赖未验证的权限，失败可整体回滚。
- 用户镜：重建只清本项目的表，不碰库里其他对象。库名守卫避免误清别的库。
- 开发循环镜：新增表时容易漏改 `db/drop.sql`。重跑 `db-reset.sh` 和契约测试会暴露遗漏。
- 主次：系统镜为主，不依赖未验证前提优先于省一个维护点。
- 前提若变化（远端库实测确认库 owner 能删 schema），本决策仍成立，只是多了备选。

## 附录：修订历史

- 暂无。
