### relevant environment variables

> [!note]
>
> `.env` also works

#### required

- `LISTEN_PORT` this server's listening port
- `DB_USERNAME`
- `DB_PASSWORD`[^1]
- `DB_NAME`
- `AUTH_KEY`[^1] client's Authorization header

#### optional

- `FILL_ITEMS` initialize items table

### how to run

`go run .`

[^1]: required for docker compose, unlike other required variables
