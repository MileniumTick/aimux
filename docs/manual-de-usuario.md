# Manual de Usuario — aimux

> **aimux** (AI Provider Multiplexer) es un TUI + CLI de un solo binario que centraliza credenciales de proveedores AI y te permite intercambiar entre proveedores para tus CLIs de desarrollo (Claude Code, OpenCode, Codex, GitHub Copilot y pi).

---

## Índice

1. [¿Qué es aimux?](#qué-es-aimux)
2. [Instalación](#instalación)
3. [Primeros pasos (TUI)](#primeros-pasos-tui)
4. [Primeros pasos (CLI)](#primeros-pasos-cli)
5. [El Dashboard](#el-dashboard)
6. [Gestión de Proveedores](#gestión-de-proveedores)
   - [Agregar proveedor](#agregar-proveedor)
   - [Editar proveedor](#editar-proveedor)
   - [Eliminar proveedor](#eliminar-proveedor)
   - [Re-intentar fetch de modelos](#re-intentar-fetch-de-modelos)
   - [Probar conectividad](#probar-conectividad)
7. [Switch Flow — Vincular Proveedores a CLIs](#switch-flow--vincular-proveedores-a-clis)
   - [Flujo completo paso a paso](#flujo-completo-paso-a-paso)
   - [Multi-proveedor: Varios proveedores por CLI](#multi-proveedor-varios-proveedores-por-cli)
   - [Mapeo de modelos por variable de entorno](#mapeo-de-modelos-por-variable-de-entorno)
   - [Selección de modelos](#selección-de-modelos)
   - [Revisión de configuración avanzada](#revisión-de-configuración-avanzada)
   - [Dry-run y aplicación](#dry-run-y-aplicación)
8. [Gestión de CLIs](#gestión-de-clis)
9. [Restaurar Backups](#restaurar-backups)
10. [Referencia CLI](#referencia-cli)
11. [Sistema de Backups](#sistema-de-backups)
12. [Atajos de Teclado](#atajos-de-teclado)
13. [Solución de Problemas](#solución-de-problemas)
14. [Ejemplos Completos](#ejemplos-completos)
    - [Claude Code + Bifrost (Anthropic)](#ejemplo-1-claude-code--bifrost-anthropic)
    - [OpenCode + proveedor OpenAI](#ejemplo-2-opencode--proveedor-openai)
    - [GitHub Copilot + proveedor local](#ejemplo-3-github-copilot--proveedor-local)
    - [pi + Gemini via Google AI](#ejemplo-4-pi--gemini-via-google-ai)

---

## ¿Qué es aimux?

![imagen_dashboard]

**aimux** resuelve un problema concreto: si usas múltiples CLIs de AI (Claude Code, OpenCode, Codex, GitHub Copilot, pi) con proveedores propios o alternativos, terminas editando archivos de configuración a mano constantemente.

Con aimux:

- **Centralizas credenciales**: Tus API keys y tokens viven en un solo lugar (SQLite local).
- **Descubres modelos automáticamente**: aimux consulta `GET /v1/models` de cada proveedor y te muestra los modelos disponibles.
- **Vinculas proveedores a CLIs**: Eliges qué proveedor usa cada CLI y qué modelos mapear.
- **Muta configs automáticamente**: aimux escribe la configuración en el archivo correspondiente (`settings.json`, `config.json`, `config.toml`, `.zshrc`, `models.json`).
- **Backups centralizados**: Antes de cada cambio, aimux respalda tu configuración actual en `~/.config/aimux/backups/`.
- **Multi-proveedor**: Un mismo CLI puede tener múltiples proveedores vinculados (OpenCode, pi, Copilot).

### CLIs Soportados

| CLI | Archivo de Config | Mutador |
|-----|-------------------|---------|
| **Claude Code** | `~/.config/claude/settings.json` | `claude-settings-json` |
| **OpenCode** | `~/.config/opencode/config.json` | `opencode-provider-json` |
| **Codex** | `~/.codex/config.toml` | `codex-config-toml` |
| **GitHub Copilot** | Shell profile (`~/.zshrc`, `~/.bashrc`, `~/.config/fish/config.fish`) | `copilot-shell-profile` |
| **pi** | `~/.pi/agent/models.json` | `pi-dual-json` |

### Tipos de Proveedor

| Tipo | Autenticación | Discovery de Modelos |
|------|--------------|---------------------|
| **OpenAI / OpenAI-compatible** | Bearer token | `GET /v1/models` |
| **Anthropic** | `x-api-key` header | `GET /v1/models` |
| **Google AI (Gemini)** | API key query param | `GET /v1beta/models` |

---

## Instalación

### Desde el código fuente

```bash
git clone https://github.com/MileniumTick/aimux.git
cd aimux
go build -o aimux .
sudo mv aimux /usr/local/bin/
```

### Desde Go install

```bash
go install github.com/MileniumTick/aimux@latest
```

### Verificar

```bash
aimux version
# → aimux 0.2.0
```

### Dependencias

- **SQLite** — embebido via `modernc.org/sqlite` (100% Go, sin CGO). No necesitas instalar SQLite.
- **Ninguna otra**: aimux es un solo binario estático.

---

## Primeros Pasos (TUI)

```bash
# Lanzar la TUI (sin argumentos)
aimux
```

![imagen_bienvenida]

La primera vez verás una pantalla de bienvenida. aimux detecta que no hay proveedores configurados y te guía al menú **Manage Providers**.

El flujo básico es:

1. **Agregar un proveedor** con su URL, API key y tipo
2. aimux **descubre modelos** automáticamente
3. **Iniciar Switch Flow** para vincular el proveedor a un CLI
4. **Elegir modelos** y confirmar la aplicación
5. ¡El CLI ya usa tu proveedor!

Cada paso se detalla en las secciones siguientes.

---

## Primeros Pasos (CLI)

```bash
# Ver multiplex activos (qué CLI usa qué proveedor)
aimux list

# Re-aplicar configuración para un CLI
aimux apply claude-code

# Listar backups centralizados
aimux backups claude-code

# Restaurar último backup
aimux restore claude-code

# Versión y actualizaciones
aimux version
aimux update
```

---

## El Dashboard

Al ejecutar `aimux` sin argumentos, entras al **Dashboard**, la vista principal de la TUI.

![imagen_dashboard]

### Secciones del Dashboard

| Sección | Descripción |
|---------|-------------|
| **Logo** | Cabecera con identidad visual de aimux |
| **Summary** | Resumen numérico: proveedores activos/en error, CLIs activos/inactivos |
| **Tabla de estado** | (Integrada en la lista de proveedores) Muestra los 5 CLIs con proveedor actual, modelos y estado ACTIVO/INACTIVO |
| **Menú** | Navegación entre acciones: Switch, Manage Providers, Manage CLIs, Restore Backup, Exit |
| **Barra de notificaciones** | Mensajes verde/rojo en la parte inferior para éxito/error |

### Pantalla de bienvenida

Si no hay proveedores configurados, el dashboard muestra un mensaje de bienvenida con instrucciones rápidas.

### Navegación

| Tecla | Acción |
|-------|--------|
| `↑` / `↓` o `k` / `j` | Navegar entre opciones del menú |
| `Enter` | Seleccionar opción |
| `?` | Mostrar/ocultar ayuda completa |
| `q` o `Ctrl+C` | Salir |

---

## Gestión de Proveedores

Desde el Dashboard, selecciona **Manage Providers** y presiona Enter.

![imagen_provider_list]

### Lista de Proveedores

Cada proveedor se muestra con:

- **Nombre** del proveedor
- **Estado**: OK (verde) o ERROR (rojo)
- **Badge "in use"**: si está vinculado a algún CLI activo
- **URL base**
- **Cantidad de modelos** descubiertos y tipo de API

### Atajos en la lista de proveedores

| Tecla | Acción |
|-------|--------|
| `↑` / `↓` | Navegar entre proveedores |
| `a` | **Agregar** nuevo proveedor |
| `e` | **Editar** proveedor seleccionado |
| `d` | **Eliminar** proveedor seleccionado |
| `r` | **Re-intentar** fetch de modelos |
| `t` | **Probar** conectividad |
| `Enter` | Iniciar **Switch Flow** con este proveedor |
| `Esc` | Volver al Dashboard |

---

### Agregar Proveedor

Presiona `a` en la lista de proveedores para abrir el formulario.

![imagen_add_provider]

El formulario pide:

1. **Name** — Identificador amigable (ej: "Bifrost", "Mi OpenAI", "Bifrost Anthropic")
2. **Base URL** — URL completa incluyendo esquema (ej: `https://api.openai.com/v1`)
3. **API Key** — Tu clave de API (se muestra como contraseña)
4. **Auth Token** — Opcional si es igual al API Key (algunos proveedores usan un token diferente)
5. **Discovery URL (opcional)** — URL separada para discovery de modelos. Si se deja vacía, usa la Base URL.
6. **API Type** — `OpenAI`, `Anthropic`, o `Google AI (Gemini)`

> **Discovery URL**: Útil cuando el endpoint de modelos está en una URL diferente al endpoint de chat/completions. Por ejemplo, si tu Base URL es `https://api.bifrost.local/v1/chat/completions` pero los modelos se descubren en `https://api.bifrost.local/v1/models`, pones la primera como Base URL y la segunda como Discovery URL.

Al enviar, aimux:

1. Crea el proveedor en la base de datos
2. Llama a `GET /v1/models` (o `/v1beta/models` para Google) para descubrir modelos
3. Analiza la respuesta y extrae metadata (ventana de contexto, límites de tokens, etc.)
4. Muestra el proveedor como **active** (check verde) o **error** (rojo)

Si el fetch falla (red, autenticación, timeout), el proveedor se crea con estado `error` y puedes usar `r` (retry) después.

---

### Editar Proveedor

Selecciona un proveedor y presiona `e`.

![imagen_edit_provider]

El formulario se prellenan con los valores actuales. Puedes cambiar:

- **Base URL**
- **API Key**
- **Auth Token**
- **Discovery URL**
- **API Type**

> El nombre es de solo lectura — si necesitas renombrar, elimina y crea de nuevo.

Al guardar, aimux re-ejecuta el fetch de modelos con las nuevas credenciales. Modelos nuevos se agregan, modelos que ya no existen en la respuesta se eliminan.

### Eliminar Proveedor

Selecciona un proveedor y presiona `d`. Aparece un diálogo de confirmación.

![imagen_delete_confirm]

- **Yes**: Elimina el proveedor, todos sus modelos, y limpia cualquier binding activo.
- **No**: Cancela.

> ⚠️ Eliminar un proveedor que está en uso por uno o más CLIs dejará esos CLIs sin configuración. aimux regenera el archivo de configuración automáticamente.

### Re-intentar Fetch de Modelos

Selecciona un proveedor y presiona `r`.

Útil cuando:

- El primer fetch falló (proveedor en estado error)
- El proveedor agregó nuevos modelos y quieres actualizar la lista
- La conexión de red se restableció

aimux muestra un resumen de cambios: `+N agregados, -N eliminados, N total`.

### Probar Conectividad

Selecciona un proveedor y presiona `t`.

Hace un `GET /v1/models` sin almacenar resultados. Muestra:

- **"Connectivity OK"** si todo funciona
- **"Connectivity: <error>"** con el mensaje de error (autenticación, rate limit, timeout, etc.)

---

## Switch Flow — Vincular Proveedores a CLIs

El **Switch Flow** es el corazón de aimux: vincula uno o más proveedores a un CLI y muta su archivo de configuración.

Puedes iniciarlo de dos formas:

1. Desde el menú principal: **Switch**
2. Desde la lista de proveedores: selecciona un proveedor y presiona **Enter**

### Flujo completo paso a paso

![imagen_switch_stepper]

El Switch Flow tiene 5 pasos, visibles como un **stepper** (indicador de progreso) en la parte superior:

```
Step 1/5: Select Target CLI
  ● ◉ ○ ○ ○
```

#### Paso 1: Seleccionar CLI

![imagen_select_cli]

Elige el CLI destino:

- `claude-code` — Claude Code (usa variables de entorno)
- `opencode` — OpenCode (proveedor en JSON)
- `codex` — Codex (usa variables de entorno)
- `github-copilot` — GitHub Copilot (shell profile)
- `pi-ai` — pi (models.json)

> Puedes filtrar escribiendo mientras el selector está abierto.

**Comportamiento según el tipo de CLI:**

| CLI | ¿Soporta multi-proveedor? | ¿Usa mapeo env→model? |
|-----|--------------------------|----------------------|
| claude-code | ❌ No (reemplaza) | ✅ Sí |
| codex | ❌ No (reemplaza) | ✅ Sí |
| opencode | ✅ Sí (agrega) | ❌ No (selección de modelos) |
| github-copilot | ✅ Sí (agrega) | ❌ No (modelo único) |
| pi-ai | ✅ Sí (agrega) | ❌ No (selección de modelos) |

#### Paso 2: Seleccionar Proveedor

![imagen_select_provider]

Elige el proveedor a vincular. Los proveedores con estado `error` se muestran con la etiqueta `[ERROR]`.

#### Paso 3: Mapear Modelos o Seleccionar Modelos

El paso 3 depende del tipo de CLI:

| CLI | Interfaz |
|-----|----------|
| **Claude Code** | Formulario de mapeo: cada variable de entorno → un modelo |
| **Codex** | Formulario de mapeo: cada variable de entorno → un modelo |
| **pi** | Multi-select: elige qué modelos incluir en `models.json` |
| **OpenCode** | Multi-select: elige qué modelos registrar |
| **GitHub Copilot** | Single-select: elige un modelo (solo `COPILOT_MODEL`) |

##### Para Claude Code / Codex (mapeo env→model)

![imagen_map_models]

Cada variable de entorno que espera el CLI aparece como un selector individual:

```
ANTHROPIC_DEFAULT_HAIKU_MODEL  → deepseek-v4-flash
ANTHROPIC_DEFAULT_SONNET_MODEL → deepseek-v4-pro
ANTHROPIC_DEFAULT_OPUS_MODEL   → (Not Selected)
```

Puedes usar **"(Apply to all)"** en los campos 2+ para propagar la selección del primer campo.

##### Para pi / OpenCode (selección múltiple)

![imagen_select_models]

Se muestra una lista con todos los modelos disponibles del proveedor, **todos pre-seleccionados por defecto**. Usa la barra espaciadora para des/marcar modelos individuales.

##### Para GitHub Copilot (selección única)

![imagen_select_single_model]

Un solo selector: el modelo que se asignará a `COPILOT_MODEL`.

#### Paso 4: Revisión de Configuración Avanzada

![imagen_advanced_config]

Antes de aplicar, aimux muestra un resumen de la metadata de cada modelo seleccionado:

```
  • deepseek-v4-flash | ctx: 131072 | max: 4096 | cost: $0.15/$0.60
  • deepseek-v4-pro | ctx: 65536 | max: 4096 | reasoning | cost: $3.00/$15.00
```

Esta metadata se usará para configurar aspectos avanzados como:

- **Context window**: límite de tokens de entrada
- **Max tokens**: límite de tokens de salida
- **Reasoning**: si el modelo soporta pensamiento extendido
- **Cost**: precios de input/output/cache para referencia
- **Context suffix**: sufijos como `[1m]` para indicar ventanas de contexto grandes

Desde aquí puedes presionar **Enter** para proceder a la confirmación, o **Esc** para regresar a la selección de modelos.

#### Paso 5: Confirmación y Dry-run

![imagen_dry_run]

La vista de confirmación muestra un **diff visual** lado a lado:

```
┌─ Current ─────────────────┐   ┌─ New ─────────────────────┐
│ {                          │   │ ANTHROPIC_BASE_URL = ...  │
│   "env": {}                │   │ ANTHROPIC_AUTH_TOKEN = .. │
│ }                          │   │ ANTHROPIC_HAIKU_MODEL = ..│
│                            │   │ ANTHROPIC_SONNET_MODEL = .│
└────────────────────────────┘   └───────────────────────────┘
```

**Enter** = Aplicar · **Esc** = Cancelar

Al aplicar, aimux:

1. ✅ Crea backup del archivo de configuración actual en `~/.config/aimux/backups/`
2. ✅ Escribe la nueva configuración
3. ✅ Limpia backups antiguos (conserva los 5 más recientes)
4. ✅ Muestra notificación verde de éxito

Después de aplicar, puedes presionar **Enter** o **Esc** para volver al Dashboard.

### Multi-proveedor: Varios proveedores por CLI

![imagen_manage_bindings]

Para CLIs que soportan multi-proveedor (pi, OpenCode, Copilot), al seleccionar un CLI que ya tiene bindings activos, aimux muestra la vista **Manage Bindings** en lugar del selector de proveedor.

Desde aquí puedes:

| Tecla | Acción |
|-------|--------|
| `↑` / `↓` | Navegar entre bindings |
| `a` | **Agregar** otro proveedor |
| `d` | **Eliminar** binding seleccionado |
| `e` | **Editar** modelos del binding seleccionado |
| `Enter` | Aplicar **todos** los bindings |
| `Esc` | Volver al Dashboard |

Esto te permite tener, por ejemplo, **OpenCode con dos proveedores**: uno para modelos rápidos y otro para modelos razonadores.

### Mapeo de modelos por variable de entorno

Los CLIs **Claude Code** y **Codex** usan variables de entorno para configurar modelos específicos. Cada variable se mapea individualmente.

**Ejemplo para Claude Code:**

| Variable de Entorno | Propósito |
|--------------------|-----------|
| `ANTHROPIC_BASE_URL` | URL base de la API (se setea automáticamente) |
| `ANTHROPIC_AUTH_TOKEN` | Token de autenticación (se setea automáticamente) |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` | Modelo rápido/barato por defecto |
| `ANTHROPIC_DEFAULT_SONNET_MODEL` | Modelo balanceado por defecto |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` | Modelo premium/capaz por defecto |

> **Nota**: aimux usa `ANTHROPIC_AUTH_TOKEN` (no `ANTHROPIC_API_KEY`) porque Claude Code tiene un flujo de OAuth que interfiere con `API_KEY`. El token se escribe en el bloque `env` del `settings.json` para no contaminar variables de entorno globales.

### Selección de modelos

Para CLIs como **pi** y **OpenCode**, aimux usa un formulario **multi-select** donde puedes elegir exactamente qué modelos del proveedor incluir en la configuración.

Todos los modelos aparecen **pre-seleccionados** por defecto. Usa:

- **Espacio** para marcar/desmarcar
- **Tab** para mover el foco
- **Enter** para confirmar

Para **GitHub Copilot**, la selección es single-select: solo un modelo a la vez en `COPILOT_MODEL`.

Para **OpenCode**, además del multi-select, hay un campo de texto para **modelos personalizados** que no aparecen en la lista del proveedor. Esto es útil si el endpoint de modelos no devuelve todos los modelos disponibles, o si quieres usar un alias.

---

## Gestión de CLIs

Desde el Dashboard, selecciona **Manage CLIs**.

![imagen_manage_clis]

Este menú te permite **editar la ruta de configuración** de cada CLI. Es útil si tu CLI usa una ubicación no estándar o si usas un worktree de git.

Al seleccionar un CLI, puedes cambiar su **Config Path**. Por ejemplo, si Claude Code está configurado en `~/workspace/proyecto/.config/claude/settings.json`, escribes esa ruta aquí.

**Comportamiento especial:**

- **Copilot**: No muestra campo de ruta porque usa detección automática del shell profile según `$SHELL`. Muestra solo una nota informativa:
  - `zsh` → `~/.zshrc`
  - `bash` → `~/.bashrc`
  - `fish` → `~/.config/fish/config.fish`

---

## Restaurar Backups

Desde el Dashboard, selecciona **Restore Backup**.

![imagen_restore_cli]

### Paso 1: Seleccionar CLI

Elige el CLI del cual quieres restaurar.

### Paso 2: Seleccionar backup

![imagen_restore_backup]

Se muestra una lista de backups disponibles, ordenados del más reciente al más antiguo:

```
Select Backup to Restore
  2026-06-18T03-21-00Z
  2026-06-18T02-15-00Z
  2026-06-17T22-00-00Z
```

### Confirmación

Al seleccionar un backup, aimux:

1. ✅ Lee el archivo de backup de `~/.config/aimux/backups/`
2. ✅ Sobrescribe el archivo de configuración del CLI con el backup
3. ✅ Muestra notificación de éxito

> ⚠️ La restauración **sobrescribe** la configuración actual. Asegúrate de tener un backup reciente antes de restaurar.

### Undo rápido (tecla Z)

Después de aplicar un switch, puedes presionar **Z** (mayúscula) desde el Dashboard para restaurar automáticamente el backup más reciente del último CLI modificado.

```
✅ Profile activated successfully · Z to undo
```

---

## Referencia CLI

```
Uso:
  aimux                    Lanzar TUI (por defecto)
  aimux apply <cli-name>   Aplicar binding activo para un CLI
  aimux list               Mostrar multiplex activos
  aimux backups <cli-name> Listar backups centralizados
  aimux restore <cli-name> Restaurar backup más reciente
  aimux version            Mostrar versión y buscar actualizaciones
  aimux update             Auto-actualizar aimux

Ejemplos:
  aimux apply claude-code
  aimux backups claude-code
  aimux restore claude-code
```

### `aimux` (sin argumentos)

Lanza la TUI Bubbletea con el Dashboard.

### `aimux apply <cli-name>`

Re-aplica el binding activo del proveedor para el CLI dado. Crea backup y muta el archivo de configuración.

```bash
aimux apply claude-code
# → Applied. Backup saved to: /Users/tu/.config/aimux/backups/settings.json-abc123/settings.json.2026-06-18T03-21-00Z
```

### `aimux list`

Muestra todos los multiplex activos — qué CLI está vinculado a qué proveedor.

```bash
$ aimux list
Active multiplexes:
  claude-code     → Bifrost (Anthropic)   (2026-06-18 11:07:51)
  opencode        → Bifrost               (2026-06-18 09:41:42)
  pi-ai           → Bifrost               (2026-06-18 11:01:20)
```

### `aimux backups <cli-name>`

Lista backups centralizados para un CLI, del más reciente al más antiguo.

```bash
$ aimux backups claude-code
Backups for 'claude-code' (newest first):
  [0] 2026-06-18T03-21-00Z
  [1] 2026-06-18T02-15-00Z
```

### `aimux restore <cli-name>`

Restaura el backup más reciente. Sobrescribe la configuración actual con el backup.

```bash
$ aimux restore claude-code
Restored latest backup: /Users/tu/.config/aimux/backups/settings.json-abc123/settings.json.2026-06-18T03-21-00Z
```

### `aimux version`

Muestra la versión y verifica actualizaciones via GitHub Releases.

```bash
$ aimux version
aimux 0.2.0
Update available: v0.2.0 → v0.3.0
```

### `aimux update`

Auto-actualiza el binario desde la última release de GitHub. Detecta instalaciones via Homebrew y delega a `brew upgrade aimux`.

```bash
$ aimux update
✓ Updated to v0.3.0
```

---

## Sistema de Backups

Aimux hace **backups centralizados** antes de cada mutación de configuración. Los backups se almacenan en `~/.config/aimux/backups/` — NO junto al archivo de configuración del CLI.

### Estructura

```
~/.config/aimux/backups/
├── settings.json-abc123def4/       ← hash de la ruta absoluta del archivo
│   ├── settings.json.2026-06-18T03-21-00Z
│   ├── settings.json.2026-06-18T02-15-00Z
│   └── ...
├── config.json-987fedcba0/
│   ├── config.json.2026-06-18T04-00-00Z
│   └── ...
└── ...
```

### Retención

- aimux conserva los **5 backups más recientes** por archivo de configuración.
- Los backups más antiguos se eliminan automáticamente después de cada aplicación.

### ¿Por qué centralizados?

El enfoque anterior creaba archivos como `settings.json.aimux-backup-2026-06-18T03:21:00Z` dentro de `~/.config/claude/`, contaminando el directorio del CLI.

Ahora todos los backups viven en `~/.config/aimux/backups/`, organizados por un hash de la ruta absoluta del archivo original, para que archivos con el mismo nombre (ej. múltiples `settings.json`) no colisionen.

### Variable de entorno

Puedes sobrescribir la raíz de backups para testing o ubicaciones personalizadas:

```bash
export AIMUX_BACKUP_ROOT=/mnt/backups/aimux
aimux apply claude-code
```

---

## Atajos de Teclado

### Dashboard

| Tecla | Acción |
|-------|--------|
| `↑` / `↓` | Navegar menú |
| `k` / `j` | Navegar menú (alternativo) |
| `Enter` | Seleccionar opción |
| `?` | Mostrar/ocultar ayuda |
| `q` | Salir |
| `Ctrl+C` | Salir (alternativo) |
| `Z` | Undo: restaurar último backup |

### Lista de Proveedores

| Tecla | Acción |
|-------|--------|
| `↑` / `↓` | Navegar lista |
| `a` | Agregar proveedor |
| `e` | Editar proveedor |
| `d` | Eliminar proveedor |
| `r` | Re-intentar fetch de modelos |
| `t` | Probar conectividad |
| `Enter` | Iniciar Switch Flow |
| `Esc` | Volver al Dashboard |

### Switch Flow

| Tecla | Acción |
|-------|--------|
| `Enter` | Confirmar paso / Aplicar |
| `Esc` | Retroceder / Cancelar |
| `Espacio` | (Multi-select) Marcar/desmarcar modelo |
| `Tab` | (Formularios) Siguiente campo |

### Manage Bindings

| Tecla | Acción |
|-------|--------|
| `↑` / `↓` | Navegar bindings |
| `a` | Agregar proveedor |
| `d` | Eliminar binding |
| `e` | Editar modelos |
| `Enter` | Aplicar todos |
| `Esc` | Volver al Dashboard |

---

## Solución de Problemas

### "Model fetch failed"

Posibles causas:

1. **URL incorrecta**: Verifica que la Base URL sea correcta. Debe ser la URL base de la API, ej. `https://api.openai.com/v1`
2. **API Key inválida**: Verifica que la key sea correcta y tenga permisos para `GET /v1/models`
3. **Timeout**: El proveedor puede estar caído o lento. Usa `r` para reintentar.
4. **Formato de respuesta**: Si el endpoint devuelve HTML en lugar de JSON, aimux lo detecta y muestra el error.

### "Provider not active"

El proveedor se creó pero el fetch de modelos falló. Usa:

- `e` para editar y corregir credenciales
- `r` para reintentar fetch
- `t` para probar conectividad primero

### "No active binding"

El CLI no tiene ningún proveedor vinculado. Inicia el **Switch Flow** desde el menú o desde la lista de proveedores.

### "CANNOT LINK" — problema con el mutador

Si encuentras errores al aplicar, verifica:

1. Que el archivo de configuración del CLI exista y tenga permisos de escritura
2. Que la ruta del archivo sea correcta (puedes cambiarla en **Manage CLIs**)
3. Que el CLI no esté en ejecución (algunos CLIs bloquean el archivo de configuración)

### El archivo de configuración de Copilot no se actualiza

Copilot lee variables de entorno del proceso, no de archivos `.env`. aimux escribe las variables en tu **shell profile** (`~/.zshrc`, `~/.bashrc`, etc.).

Después de aplicar con Copilot:

1. **Reinicia tu terminal** o ejecuta `source ~/.zshrc` (o el archivo que corresponda)
2. Copilot ahora usará el proveedor configurado

### Quiero deshacer un cambio

1. Presiona **Z** (mayúscula) en el Dashboard para undo rápido
2. O usa **Restore Backup** desde el menú
3. O desde CLI: `aimux backups claude-code` → `aimux restore claude-code`

### "Connectivity test returned no models"

El endpoint responde pero no devuelve modelos en el formato esperado. Verifica:

1. Que el API type sea correcto (OpenAI, Anthropic, Google)
2. Que el endpoint de modelos exista
3. Si es un proveedor compatible con OpenAI, el formato esperado es `{"data": [{"id": "..."}]}`

---

## Ejemplos Completos

### Ejemplo 1: Claude Code + Bifrost (Anthropic)

Configurar **Claude Code** para usar un proveedor Anthropic-compatible (Bifrost) en `https://ai.intranet.istmocenter.com`.

#### Paso 1: Lanzar aimux

```bash
aimux
```

#### Paso 2: Agregar proveedor

Desde el Dashboard, selecciona **Manage Providers** → presiona `a`:

```
Name:          Bifrost (Anthropic)
Base URL:      https://ai.intranet.istmocenter.com
API Key:       <tu-api-key-anthropic>
Auth Token:    <mismo o dejar vacío>
Discovery URL: (vacío — usa Base URL)
API Type:      Anthropic
```

Presiona Enter. aimux llama a `GET /v1/models` con tu API key, descubre los modelos y muestra el proveedor como **active**.

#### Paso 3: Iniciar Switch Flow

En la lista de proveedores, selecciona "Bifrost (Anthropic)" y presiona **Enter**.

#### Paso 4: Seleccionar CLI

Selecciona **claude-code**.

#### Paso 5: Mapear modelos

```
ANTHROPIC_DEFAULT_HAIKU_MODEL  → deepseek-v4-flash
ANTHROPIC_DEFAULT_SONNET_MODEL → deepseek-v4-pro
```

Usa "(Apply to all)" si quieres el mismo modelo para ambos.

#### Paso 6: Confirmar y aplicar

Revisa el dry-run. Presiona **Enter**.

#### Resultado

`~/.config/claude/settings.json` ahora contiene:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://ai.intranet.istmocenter.com",
    "ANTHROPIC_AUTH_TOKEN": "sk-ant-...",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "deepseek-v4-flash",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "deepseek-v4-pro"
  }
}
```

```bash
# Verificar
aimux list
# → claude-code → Bifrost (Anthropic) (ACTIVE)
```

---

### Ejemplo 2: OpenCode + proveedor OpenAI

Configurar **OpenCode** para usar un proveedor OpenAI-compatible.

#### Paso 1: Agregar proveedor

```
Name:          Mi OpenAI
Base URL:      https://api.openai.com/v1
API Key:       sk-...
Auth Token:    (vacío — usa API Key)
Discovery URL: (vacío)
API Type:      OpenAI
```

#### Paso 2: Switch Flow

Selecciona **opencode** como CLI. Como OpenCode soporta multi-proveedor, aimux te lleva a la vista **Manage Bindings**.

Presiona `a` para agregar binding y selecciona "Mi OpenAI".

#### Paso 3: Seleccionar modelos

Se muestran todos los modelos de OpenAI pre-seleccionados. Puedes desmarcar los que no quieras.

```
☑ gpt-4o
☑ gpt-4o-mini
☑ gpt-4-turbo
☐ gpt-3.5-turbo    ← desmarcado
```

#### Paso 4: Revisar configuración avanzada

```
  • gpt-4o | ctx: 128000 | max: 16384 | cost: $2.50/$10.00
  • gpt-4o-mini | ctx: 128000 | max: 16384 | cost: $0.15/$0.60
```

#### Paso 5: Aplicar

`~/.config/opencode/config.json` ahora tiene tu proveedor configurado.

---

### Ejemplo 3: GitHub Copilot + proveedor local

Configurar **GitHub Copilot** para usar un servidor local (ej. Ollama, llama.cpp).

#### Paso 1: Agregar proveedor

```
Name:          Local LLM
Base URL:      http://localhost:8080/v1
API Key:       (vacío o dummy — es local)
Auth Token:    (vacío)
Discovery URL: (vacío)
API Type:      OpenAI
```

#### Paso 2: Switch Flow

Selecciona **github-copilot** como CLI. Como Copilot soporta multi-proveedor, aimux muestra **Manage Bindings**.

Presiona `a` → selecciona "Local LLM".

#### Paso 3: Seleccionar modelo único

```
Select Model
  llama-3.1-8b
  mistral-7b
  deepseek-coder-6.7b
```

Elige el modelo que Copilot usará.

#### Paso 4: Aplicar

aimux escribe en tu shell profile (`~/.zshrc` o equivalente):

```bash
# >>> aimux copilot provider
# Managed by aimux — DO NOT EDIT BETWEEN MARKERS
export COPILOT_PROVIDER_BASE_URL="http://localhost:8080/v1"
export COPILOT_PROVIDER_TYPE="openai"
export COPILOT_MODEL="llama-3.1-8b"
# <<< aimux copilot provider
```

**Importante**: Reinicia tu terminal o ejecuta `source ~/.zshrc` para que Copilot tome los cambios.

---

### Ejemplo 4: pi + Gemini via Google AI

Configurar **pi** para usar **Gemini** de Google.

#### Paso 1: Agregar proveedor

```
Name:          Google Gemini
Base URL:      https://generativelanguage.googleapis.com/v1beta
API Key:       <tu-gemini-api-key>
Auth Token:    (vacío)
Discovery URL: (vacío)
API Type:      Google AI (Gemini)
```

aimux llama a `GET /v1beta/models` y descubre modelos como `gemini-2.5-pro`, `gemini-2.5-flash`, etc.

#### Paso 2: Switch Flow

Selecciona **pi-ai** como CLI.

#### Paso 3: Multi-proveedor

Si pi ya tiene otro proveedor vinculado (ej. Bifrost para Anthropic), aimux muestra la vista **Manage Bindings** con ambos. Presiona `a` para agregar "Google Gemini".

#### Paso 4: Seleccionar modelos

```
☑ gemini-2.5-pro
☑ gemini-2.5-flash
☑ gemini-1.5-pro
☐ gemini-1.5-flash    ← desmarcado
```

#### Paso 5: Aplicar

pi ahora tiene dos proveedores: Bifrost (Anthropic) y Google Gemini.

---

## Glosario

| Término | Definición |
|---------|-----------|
| **TUI** | Terminal User Interface — interfaz de usuario en terminal |
| **CLI** | Command Line Interface — interfaz de línea de comandos |
| **Proveedor** | Un servicio de API AI (OpenAI, Anthropic, Google, o compatible) |
| **Mutador** | Código que escribe la configuración en el formato específico de cada CLI |
| **Multiplex** | Vinculación activa entre un proveedor y un CLI |
| **Binding** | Sinónimo de multiplex: un proveedor vinculado a un CLI |
| **Switch Flow** | Flujo de 5 pasos para vincular proveedores a CLIs |
| **Dry-run** | Simulación de los cambios sin aplicarlos |
| **Discovery URL** | URL separada para descubrir modelos, diferente de la URL base |
| **Shell Profile** | Archivo de configuración del shell (`~/.zshrc`, `~/.bashrc`, etc.) |

---

## Notas Técnicas

- **Base de datos**: SQLite en `~/.config/aimux/matrix.db` con permisos `0600`
- **Logs**: `~/.config/aimux/aimux.log` (fechas, timestamps, errores)
- **Backups**: `~/.config/aimux/backups/` (organizados por hash de ruta)
- **Escritura atómica**: Las mutaciones usan temp-file + rename + fsync para seguridad ante cortes
- **Flock**: Bloqueo de archivo en lecturas de configuración para prevenir corrupción concurrente
- **Tolerancia a commas finales**: Las configuraciones JSON editadas a mano con commas extra se parsean correctamente
- **Sufijo `[1m]`**: Modelos con 1M+ de contexto reciben el sufijo automáticamente para Claude Code

---

*[imagen_tema] — Reemplazar con captura de pantalla real*
*Generado para aimux v0.2.0*
