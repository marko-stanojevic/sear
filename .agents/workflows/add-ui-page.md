---
description: how to add a new dashboard page (view) to the UI
---

1. Define the page name (e.g., `settings`).
2. **Update Navigation**: Open [internal/server/handlers/ui_templates.go](../../internal/server/handlers/ui_templates.go) and add a new `navItem` to the `buildNav` function.
3. **Add Data Type**: (Optional) Add a new struct for your page's data in `ui_templates.go` (e.g., `type settingsData struct`).
4. **Implement Handlers**: In [ui_templates.go](../../internal/server/handlers/ui_templates.go):
    - Add a main handler: `func (e *Handler) HandleSettingsUI(w http.ResponseWriter, r *http.Request)`. Use `renderPage(w, "settings", data)`.
    - Add a partial handler for HTMX updates: `func (e *Handler) HandlePartialSettings(w http.ResponseWriter, r *http.Request)`. Use `renderPartialTemplate(w, "settings-table", data)`.
5. **Register Routes**: Open [internal/server/server.go](../../internal/server/server.go) and register your handlers in `NewServer`:
    - `mux.Handle("/ui/settings", http.HandlerFunc(env.HandleSettingsUI))`
    - `mux.Handle("/ui/partials/settings", root(http.HandlerFunc(env.HandlePartialSettings)))`
6. **Create Templates**:
    - Create a file in [internal/server/handlers/ui/templates/pages/](../../internal/server/handlers/ui/templates/pages/) (e.g., `settings.html`).
    - Create a file in [internal/server/handlers/ui/templates/partials/](../../internal/server/handlers/ui/templates/partials/) (e.g., `settings-table.html`).
    - Use `{{define "layout"}}...{{end}}` in the page template and standard HTML in the partial.
7. **Update CSS**: If the new page includes a data list (table), open [internal/server/handlers/ui/assets/style.css](../../internal/server/handlers/ui/assets/style.css) and add a new grid layout rule (e.g., `.list-settings .list-head, .list-settings .list-row { display: grid; ... }`).
8. **Verify**: Run the server and navigate to `/ui/settings`.
