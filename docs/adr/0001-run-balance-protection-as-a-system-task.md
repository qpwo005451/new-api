# Run balance protection as a system task

Balance Protection will run through the existing scheduled system-task framework instead of the legacy process-local balance loop. This provides database-leased execution across multiple instances, durable run history, and one shared evaluation path for scheduled and manual balance checks while preserving the existing balance endpoints for compatibility.
