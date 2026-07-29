# TODO

## Roadmap

### Batch 2 — Alert intelligence
- [ ] Degraded "slow" state (🟡) when latency exceeds a threshold
- [ ] "Still down" reminder re-alerts while an outage continues
- [ ] `/pause <name> <duration>` — auto-resume after a timed pause
- [ ] Threaded alerts: recovery replies to the original failure message

### Batch 3 — Ops features
- [ ] SSL certificate expiry warnings for https endpoints
- [ ] Expected status code per endpoint (default: any 2xx)
- [ ] Keyword/content check per endpoint
- [ ] `/rename <name> <new-name>`
- [ ] `/pause all` / `/resume all`

### Batch 4 — Stats
- [ ] Check history table → `/uptime` (24h/7d/30d %, avg/p95 latency)
- [ ] `/incidents` — outage history per endpoint
- [ ] Embeddable status badge SVG (`/badge/<name>.svg`)

## Future Ideas
- [ ] Multi-chat / multi-user support (currently a single configured chat)
- [ ] Webhook mode as an alternative to long polling
- [ ] Prometheus `/metrics` endpoint (needs dependency approval)
