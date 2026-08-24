# TODO

## Roadmap

## Future Ideas

## Deferred: Multi-tenancy
Single-user by design for now. Multi-chat/multitenancy (per-chat endpoint isolation, onboarding, quotas) is postponed until the feature set is complete; revisit if hosting access for others becomes a goal. Multi-tenant would require: `chat_id` on endpoints, chat-scoped queries, removing the `guarded` single-chat middleware, abuse/quota controls.
