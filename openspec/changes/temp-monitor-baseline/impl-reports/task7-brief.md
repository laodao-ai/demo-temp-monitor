### Task 7: 实现验证（收尾，不计入 3–6 预算）

**Blocked-by:** 1, 2, 3, 4, 5, 6
**R-ID:** all

按「聚合套件发现契约」运行本 change 命中域的全部层并全部通过。报告头以管道直贴（不手工转录）`suite-scope` 输出的 JSON（`base` / `head` / `domains_configured` / `hit_domains` / `layers` / `skipped_layers` / `mapping`）。证据落 `impl-reports/task7-verify.md`，每层一行 `<层> | <命令原文> | <退出码> | <SHA>`。

- [ ] 命中域各层证据齐全并通过（或记未覆盖 / 不在命中域）
