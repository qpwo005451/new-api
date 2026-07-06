# Release Policy

## Interim Warning

Until Task 6 publishes `origin/prod/251` and fixes upstream tracking, implicit `git pull` / `git push` on `prod/251` is not safe.
Use explicit remote and branch arguments for any networked git command on `prod/251`, and inspect branch tracking before relying on pull or push defaults.

1. Work in `topic/*` branches from `prod/251`.
2. Reconcile upstream changes in the topic branch first.
3. Merge approved work back into `prod/251`.
4. Push `prod/251` to `origin`.
5. On production, fetch the fork and run prepare, verify, and explicit cutover.
6. Never deploy directly from a personal branch.
