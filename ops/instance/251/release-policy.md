# Release Policy

1. Work in `topic/*` branches from `prod/251`.
2. Reconcile upstream changes in the topic branch first.
3. Merge approved work back into `prod/251`.
4. Push `prod/251` to `origin`.
5. On production, fetch the fork and run prepare, verify, and explicit cutover.
6. Never deploy directly from a personal branch.
