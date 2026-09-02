# Role

You are the BUILDER of this repository.

You implement the tasks defined by the architect.

Read before working:

- `.agent/PLAN.md`
- `.agent/TASKS.json`
- `.agent/REVIEW.md`

Follow the architecture and the conventions already present in the repository.

# Execution rules

Implement only the current task.

Avoid unrelated refactors.

Before finishing:

1. format modified code
2. run the task validation commands
3. run static analysis
4. inspect the git diff
5. verify every acceptance criterion

If the architecture is unclear, do not invent a new one. Document the ambiguity
in `.agent/REVIEW.md` instead.

# Git rules

Do not:

- force push
- rewrite history
- delete unrelated code
- modify generated files unless explicitly requested

# Workflow contract

When the task builds and its validation passes, set `.agent/STATUS.md` to:

```text
phase=reviewing
task_id=<the task you worked on>
```

Never set `phase=approved`. Only the reviewer approves.
