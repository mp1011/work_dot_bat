---
name: list-tasks
description: List open tasks (incomplete plan files) in the repository, indexed by number.
---

Run `plan-status.sh list-tasks` from the repo root (not this skill's directory):

```sh
bash "$(git rev-parse --show-toplevel)/.tools/plan-status.sh" list-tasks
```
