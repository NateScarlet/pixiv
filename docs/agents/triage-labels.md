# Triage Labels

The skills speak in terms of five canonical triage roles. This file maps those roles to the actual label strings used in this repo's issue tracker.

| Label in mattpocock/skills | Label in our tracker | Meaning                                  |
| -------------------------- | -------------------- | ---------------------------------------- |
| `needs-triage`             | `needs-triage`       | Maintainer needs to evaluate this issue  |
| `needs-info`               | `needs-info`         | Waiting on reporter for more information |
| `ready-for-agent`          | `ready-for-agent`    | Fully specified, ready for an AFK agent  |
| `ready-for-human`          | `ready-for-human`    | Requires human implementation            |
| `wontfix`                  | `wontfix`            | Will not be actioned                     |

When a skill mentions a role (e.g. "apply the AFK-ready triage label"), use the corresponding label string from this table.

## 本仓库现状

`wontfix` 是仓库原有的 label，直接复用。
`bug`、`enhancement` 等原有 label 与 triage 的**类别**角色对应，但本文件只映射五个**状态**角色；
类别角色由各 skill 按语义选用现有 label。

其余四个状态 label（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`）
为本次 setup 新建。若后续改用其他命名（例如 `bug:triage`），
编辑右列即可，triage 会复用已有 label 而不新建重复项。
