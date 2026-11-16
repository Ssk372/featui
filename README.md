# FeatUI

Rastreador de logros de Steam para la terminal. Vuelve a «cazar» los logros de
tus juegos con una lista local, ligera y sin tocar Steam.

> **FeatUI no desbloquea logros en Steam ni interactúa con los sistemas de
> Valve.** Solo lee metadatos públicos de la API de Steam (nombres,
> descripciones, iconos) y guarda tu progreso en local. Es una herramienta para
> revivir la experiencia de conseguirlos, no para modificarlos.

## Características

- Multi-juego: escribe un AppID o pega una URL de Steam.
- Lista de juegos recientes para volver donde lo dejaste.
- Marca y desmarca logros con `espacio`; se guarda `unlocked_at` y el origen.
- Búsqueda por nombre o descripción, insensible a mayúsculas y acentos.
- Filtros: todos, pendientes, completados.
- Contador y barra de progreso reales.
- Badge `· steam` si ya tienes ese logro en Steam (solo referencia, nunca
  modifica tu progreso local).
- Reinicio de partida con confirmación y backup automático.
- Caché de esquemas: arranca rápido y sigue funcionando sin conexión.
- Watcher opcional que solo recarga el progreso cuando el juego guarda
  (de momento, Borderlands 2).

## Instalación

Requiere Go 1.27 o superior.

```bash
git clone https://github.com/Ssk372/featui.git
cd featui
go build -o featui ./cmd/featui
./featui
```

También puedes instalarlo en el `PATH`:

```bash
go install github.com/Ssk372/featui/cmd/featui@latest
```

## Configuración

FeatUI crea su configuración en `~/.config/featui/config.json` la primera vez
que se ejecuta:

```json
{
  "steam_api_key": "TU_STEAM_API_KEY_AQUI",
  "steam_id_64": "",
  "locale": "spanish",
  "max_visible_count": 15,
  "notify": true,
  "watch_profile": false,
  "recent_app_ids": []
}
```

| Campo | Descripción |
| --- | --- |
| `steam_api_key` | Clave de la API de Steam. Se obtiene gratis en <https://steamcommunity.com/dev/apikey>. |
| `steam_id_64` | Opcional. Se autodetecta desde `loginusers.vdf` si está vacío. Solo se usa para el badge informativo. |
| `locale` | Idioma de los textos que devuelve Steam (`spanish`, `english`, ...). |
| `max_visible_count` | Máximo de logros visibles a la vez. |
| `notify` | Enviar notificación de escritorio con `notify-send` al marcar un logro. |
| `watch_profile` | Activar el watcher de guardado (solo juegos con adaptador). |
| `recent_app_ids` | Últimos juegos abiertos. Los gestiona la app. |

Puedes evitar guardar la clave en disco con la variable de entorno:

```bash
export FEATUI_STEAM_API_KEY="tu_clave"
```

La variable tiene prioridad sobre el archivo y **nunca se escribe en él**.

### Dónde se guardan los datos

| Ruta | Contenido |
| --- | --- |
| `~/.config/featui/config.json` | Configuración y API key. Permisos `0600`. |
| `~/.local/share/featui/progress/<appid>.json` | Progreso local por juego. |
| `~/.local/share/featui/progress/<appid>.json.bak` | Versión anterior del progreso. |
| `~/.cache/featui/schema_<appid>_<locale>.json` | Caché del esquema de logros (24 h). |
| `~/.cache/featui/icons/` | Iconos descargados para las notificaciones. |

Todo respeta `XDG_CONFIG_HOME`, `XDG_DATA_HOME` y `XDG_CACHE_HOME`.

## Controles

| Tecla | Acción |
| --- | --- |
| `↑` / `↓`, `k` / `j` | Mover el cursor |
| `espacio` | Marcar / desmarcar el logro seleccionado |
| `/` | Buscar por nombre o descripción |
| `f` | Alternar filtro: todos / pendientes / completados |
| `g` / `G` | Ir al principio / final |
| `R` | Reiniciar la partida (pide confirmación) |
| `r` | Recargar el esquema desde Steam |
| `esc` | Volver a la pantalla de AppID |
| `?` | Mostrar / ocultar la ayuda |
| `q` / `ctrl+c` | Salir |

En la pantalla de AppID: `enter` abre el juego escrito (o el reciente
seleccionado), `↑` / `↓` recorren los recientes y `esc` sale.

## Desarrollo

```bash
gofmt -l .
go vet ./...
go test ./...
```

La lógica de fusión entre el esquema de Steam y el progreso local está en
`pkg/model/merge.go` y tiene tests de regresión: Steam nunca puede desbloquear
ni bloquear un logro por su cuenta.

## Hoja de ruta

- Selector de biblioteca con `GetOwnedGames` (SteamID64 + búsqueda).
- Adaptadores de watcher para más juegos.
- Exportar/importar progreso.

## English summary

FeatUI is a terminal achievement tracker for Steam. It only reads public
achievement metadata from the Steam Web API and keeps your progress in local
JSON files. It does not unlock achievements on Steam and does not interact with
Valve systems. Requires Go 1.27+.

## Licencia

MIT. Consulta [LICENSE](LICENSE).
