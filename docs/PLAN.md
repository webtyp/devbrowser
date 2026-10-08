---
PLAN: "fix(devbrowser): Chrome se cierra ordenadamente y el aviso «Restore pages?» no vuelve a aparecer"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 15705319383734565078
PR: https://github.com/webtyp/devbrowser/pull/16
---

# Plan — El aviso «Restore pages?» en cada arranque del navegador de desarrollo

> Reportado por el usuario el 2026-10-08, durante la ola del login de mjosefa-cms
> (`/home/cesar/.claude/plans/si-la-ui-se-drifting-treasure.md`). Es un defecto aparte, por eso tiene
> su propio plan.

## 1. El problema

Cada vez que el daemon abre el navegador de un proyecto, Chrome muestra *«Restore pages? Chrome
didn't shut down correctly»* encima de DevTools. Hay que cerrarlo a mano en cada arranque y tapa
la herramienta mientras se desarrolla.

## 2. Evidencia

- El perfil persistente del proyecto (`~/.cache/webtyp/devbrowser/profiles/7c13c7e09225`, de
  `ProfileDir`) tiene en `Default/Preferences`: `profile.exit_type = "Crashed"`. Chrome escribe
  `"Normal"` solo cuando se cierra él mismo; al arrancar con `"Crashed"` muestra el aviso.
- `CloseBrowser.go:17-28` cierra así:
  ```go
  h.Cancel()      // cancela el contexto de la pestaña
  h.AllocCancel() // cancela el allocator → mata el proceso de Chrome
  ```
  Nunca le pide a Chrome que se cierre. Es un corte en seco, que Chrome registra como caída.
- El chromedp vendorizado ya tiene el cierre ordenado: `chromedp.Cancel(ctx)`
  (`chromedp/chromedp.go:245`): *«If the context allocated a browser, the browser will be closed
  gracefully by Cancel»*. Envía `Browser.close` y espera a que Chrome termine.
- Aunque el cierre normal se arregle, un daemon que muere sin cerrar (SIGKILL, corte de luz,
  `kill -QUIT`) deja el perfil igual. Para ese caso hacen falta dos redes de seguridad.

## 3. La corrección (tres capas, sin API pública nueva)

Solo se tocan `CloseBrowser.go`, `context.go`, `profile_dir.go` y tests. No se exporta ningún
símbolo nuevo (AGENTS.md: los tests usan solo la API pública; una función privada se prueba con un
test en la raíz, justificado en un comentario).

### Capa 1 — causa raíz: cerrar Chrome ordenadamente (`CloseBrowser.go`)

Hoy (líneas 17–28):

```go
if h.Cancel != nil {
	h.Cancel()
	h.Cancel = nil
}
if h.AllocCancel != nil {
	h.AllocCancel()
	h.AllocCancel = nil
}
```

Queda:

```go
// Ask Chrome to close itself first (Browser.close): a process killed by the
// allocator records exit_type "Crashed" in the profile, and the next start
// shows the "Restore pages?" bubble over DevTools. The allocator cancel below
// stays as the last step, so a hung Chrome is still killed and no second
// window survives (the about:blank "double window" bug).
if h.Ctx != nil {
	closeCtx, closeCancel := context.WithTimeout(h.Ctx, gracefulCloseTimeout)
	_ = chromedp.Cancel(closeCtx)
	closeCancel()
}
if h.Cancel != nil {
	h.Cancel()
	h.Cancel = nil
}
if h.AllocCancel != nil {
	h.AllocCancel()
	h.AllocCancel = nil
}
```

con `const gracefulCloseTimeout = 3 * time.Second` en el mismo archivo. `chromedp` es el paquete
vendorizado `webtyp.com/devbrowser/chromedp` (`chromedp.Cancel` está en `chromedp/chromedp.go:245`).
`CloseBrowser` tiene `h.Mu` tomado: `chromedp.Cancel` no toma `h.Mu`, así que no hay bloqueo.

`CreateBrowserContext` (`context.go:89-96`) también corta un contexto anterior con `h.Cancel()`.
Aplicar el mismo patrón ahí, con una función privada compartida, por ejemplo
`func (h *DevBrowser) shutdownChrome()` que hace los tres pasos y deja `Ctx`, `Cancel` y
`AllocCancel` en nil. **Una sola implementación**, llamada desde los dos lugares.

### Capa 2 — red de seguridad: el perfil queda "Normal" antes de lanzar (`profile_dir.go`)

Un daemon que muere sin cerrar (SIGKILL, reinstalación de `webtyp`) deja el perfil en
`"Crashed"` aunque la capa 1 esté bien. Agregar en `profile_dir.go`:

```go
// markProfileExitedCleanly rewrites <dir>/Default/Preferences so Chrome
// believes its last run closed normally. Without it, a daemon killed without
// closing the browser leaves exit_type "Crashed" and every start shows
// "Restore pages?". A missing file is fine (fresh profile); an unreadable or
// invalid one is left untouched and reported — the profile holds the
// developer's logins and is never deleted or rewritten blindly.
func markProfileExitedCleanly(dir string) error
```

- Leer `filepath.Join(dir, "Default", "Preferences")`. Si no existe → `return nil`.
- `json.Unmarshal` en `map[string]any`. Si falla → devolver el error, sin escribir.
- Poner `profile.exit_type = "Normal"` y `profile.exited_cleanly = true` (crear el mapa `profile`
  si no existe). Conservar todas las demás claves.
- Escribir con `os.WriteFile` en un temporal del mismo directorio y `os.Rename` encima (escritura
  atómica: Chrome no debe leer un JSON a medio escribir). Permisos `0o600`.

Llamarla en `buildAllocatorOptions` (`context.go`), dentro de `if h.ProfileDir != ""`, en la rama
`else` (perfil **no** bloqueado), justo antes de `chromedp.UserDataDir`. Si devuelve error:
`h.Logger("browser profile: could not mark clean exit:", err)` y seguir (es cosmético, no bloquea).
Nunca tocar un perfil bloqueado: lo está usando otro Chrome.

### Capa 3 — flag de Chrome (`context.go`)

En `buildAllocatorOptions`, junto a los demás `chromedp.Flag`:

```go
// Chrome 113+: never show the "Restore pages?" bubble. Third layer only:
// older builds or distro Chromium may not know it, and Chrome ignores
// unknown switches.
chromedp.Flag("hide-crash-restore-bubble", true),
```

**[inv]** El string aparece en el binario de Google Chrome 154 (`/opt/google/chrome/chrome`), pero
no en `/usr/bin/chromium` 154 (puede estar en una librería). Por eso no es la única capa.

### De paso: comentario obsoleto

`CloseBrowser.go:11-14` dice «The selection history stays: … nextSeq keeps counting so a badge
number is never reused». Desde v0.6.3 el historial se vacía y la numeración vuelve a 1 en cada
navegación del frame principal (`selection_capture.go`, `EventFrameNavigated`). Reemplazar por:
«The next browser context needs its listeners again; the history is cleared by the first
navigation of the new context.»

## 4. Tests (rojo primero)

1. **Raíz, justificado (función privada):** `profile_dir_internal_test.go`, con comentario
   `// Root test: markProfileExitedCleanly is unexported; tests/ can only reach the public API.`
   - Un `Preferences` con `{"profile":{"exit_type":"Crashed","exited_cleanly":false},"other":1}`
     → queda `exit_type "Normal"`, `exited_cleanly true` y `other` sigue en `1`.
   - Sin archivo → `nil`, y no se crea nada.
   - JSON inválido → error, y el archivo queda byte a byte igual.
2. **`tests/`, extremo a extremo (headless, se salta si no hay Chrome, como
   `tests/mcp_selected_test.go`):** apuntar `XDG_CACHE_HOME` a `t.TempDir()`, crear el navegador con
   la opción pública `devbrowser.WithProfile(root)` en modo headless, abrirlo en un `httptest.Server`,
   `CloseBrowser()`, y leer `<ProfileDir(root)>/Default/Preferences`: `profile.exit_type == "Normal"`.
   Hoy da `"Crashed"` (rojo). Si `Preferences` no existe tras cerrar, el test falla con ese mensaje
   (Chrome siempre lo escribe al cerrar ordenadamente).
3. `gotest` verde (vet, race, tests).

## 5. Criterios de aceptación

- `grep -n 'hide-crash-restore-bubble' context.go` → 1 resultado.
- `grep -n 'chromedp.Cancel' *.go` → solo dentro de la función compartida de la capa 1.
- `grep -rn 'nextSeq keeps counting' .` → vacío.
- Ningún símbolo exportado nuevo: `git diff | grep '^+func [A-Z]'` → vacío.
- `gotest` verde.

## 6. Fuera de alcance

- Por qué el daemon a veces muere sin cerrar el navegador (reinstalación de `webtyp` por `gopush`).
  Las capas 2 y 3 cubren ese caso; la causa merece su propia investigación.
