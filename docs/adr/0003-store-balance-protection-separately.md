# Store balance protection separately

Balance Protection configuration and runtime state will live in a dedicated one-to-one channel table rather than the channel settings JSON. This prevents scheduled state updates from being overwritten by stale channel-editor submissions, supports atomic transitions and due-channel queries across SQLite, MySQL, and PostgreSQL, and keeps the channel API free to present the data as one cohesive editor experience.
