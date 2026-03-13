# TODO

## Message Formatting
- [ ] Improve Telegram message formatting — make endpoint lists, failure/recovery alerts visually clean and easy to scan

## Endpoint Names
- [x] Add `name` field to endpoints (DB migration + model) — made required, not optional
- [x] Update `/add` command syntax: `/add <name> <url> <interval>`
- [x] Allow `/delete`, `/interval`, and future commands to accept endpoint name (not just ID or URL)
- [x] Show both name and ID in `/list` and `/status` output

## Bot UX
- [ ] Trigger immediate health check when user adds a new endpoint (don't wait for first interval)
- [ ] Explore Telegram command menu (BotFather /setcommands or via API)
- [ ] Explore inline keyboards and other interactive Telegram features

## Observability
- [ ] Improve structured logging — consistent key-value fields (endpoint ID, URL, status code, duration), better log level usage
