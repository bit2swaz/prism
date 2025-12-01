# Prism (Ephemeral DB Proxy)

prism is a self hosted, protocol aware reverse proxy for postgresql.

it sits between your application and your database storage. by intercepting the postgresql wire protocol, prism detects branch contexts (e.g., via `postgres@feature-1`) and uses a **polymorphic storage engine** to spin up an isolated, read-write clone of the production database in milliseconds using filesystem-level Copy-on-Write (CoW)