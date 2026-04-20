# pretend compat matrix

<!-- compat-harness:deviations:start -->
```yaml
deviations:
  - path: ini.opcache.jit_buffer_size
    kind: ignore
    reason: "v2 sets 256M; we set 0 pending JIT support"
    fixtures: ["*"]
  - path: extensions
    kind: allow
    reason: "reset fixture produces different exact set"
    fixtures: ["none-reset"]
```
<!-- compat-harness:deviations:end -->
