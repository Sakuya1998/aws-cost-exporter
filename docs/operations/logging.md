# Logging and Rotation

## Policy

aws-cost-exporter writes structured logs to the process standard stream. It
does not create, reopen, rotate, compress, or delete log files. Rotation and
retention belong to the container runtime or host service manager.

This policy preserves compatibility with read-only container filesystems,
non-root execution, centralized log collectors, and immutable Kubernetes pods.
It also prevents application replicas from competing over a shared log file.

The exporter supports JSON and text formatting, severity filtering, optional
source locations, and sensitive-attribute redaction. It intentionally has no
`log.path`, maximum file size, backup count, or retention-age settings.

## Docker

Configure the Docker daemon or the individual container log driver. The `local`
driver provides bounded storage and rotation without changing the exporter:

```json
{
  "log-driver": "local",
  "log-opts": {
    "max-size": "20m",
    "max-file": "5",
    "compress": "true"
  }
}
```

Daemon changes affect newly created containers. Existing containers must be
recreated. Production values must follow the host's capacity and compliance
requirements rather than treating this example as a universal default.

## Kubernetes

The container runtime writes pod logs and kubelet rotates them. Configure
`containerLogMaxSize` and `containerLogMaxFiles` in the kubelet configuration
for each node pool. A typical starting point is:

```yaml
containerLogMaxSize: 20Mi
containerLogMaxFiles: 5
```

The Helm chart must not modify node-level kubelet settings. A cluster logging
agent should ship stdout logs before local retention removes them. Monitor node
filesystem pressure and the logging agent's delivery failures independently.

## systemd

Run the binary as a service with `StandardOutput=journal` and
`StandardError=journal`. Configure journald retention centrally with settings
such as `SystemMaxUse`, `RuntimeMaxUse`, and `MaxFileSec` in `journald.conf`.
Do not redirect the service directly to an indefinitely growing regular file.

## Direct file redirection

Direct redirection is supported by the operating system but is not recommended.
If it is unavoidable, use an external logrotate policy with `copytruncate`,
restricted file permissions, explicit size and age limits, compression, and a
tested collection pipeline. `copytruncate` has a small copy/truncate race, so
journald or a container log driver is preferred.

## Security requirements

- Restrict access to logs because cost metadata may reveal account structure.
- Never log AWS credentials, authorization headers, or raw API responses.
- Do not place logs in credential, configuration, or projected-secret volumes.
- Apply retention and deletion policies required by the operating environment.
- Treat rotation failure and filesystem exhaustion as host-level alerts.
