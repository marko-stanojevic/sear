---
description: how to add a new REST or WebSocket API endpoint to the server
---

1. Define the endpoint path (e.g., `/api/v1/my-feature`).
2. Create or update a handler file in [internal/server/handlers/](../../internal/server/handlers/) (e.g., `my_feature.go`).
3. Implement the handler function (see [admin.go](../../internal/server/handlers/admin.go) for examples of JSON CRUD).
4. Register the route in [internal/server/server.go](../../internal/server/server.go) within the `New` function.
5. If persistent state is needed, update [internal/server/store/store.go](../../internal/server/store/store.go):
    - Add a `CREATE TABLE` statement to the `schema` constant.
    - Implement the necessary `Get`/`Save`/`List` methods on the `Store` struct.
6. If it's a WebSocket message, update [internal/common/types.go](../../internal/common/types.go) with a new `WSMessageType` and message struct.
7. Register the WS message handler in [internal/server/handlers/ws.go](../../internal/server/handlers/ws.go)'s `handleWSMessage`.
8. Add integration tests in [internal/server/](../../internal/server/) (e.g., `integration_auth_test.go`).
