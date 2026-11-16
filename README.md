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
- Badge `· steam` si ya tienes ese logro en Steam (solo referencia).
- Auto-desbloqueo genérico: mientras FeatUI está abierto, consulta Steam y
  refleja los logros que se desbloqueen durante la partida.
- Tecla `S` para sincronizar manualmente con Steam (pide confirmación) y
  recuperar lo desbloqueado mientras la app estaba cerrada.
- Sonido configurable al desbloquear: tema del sistema, archivo propio o chime
  incluido como último recurso.
- Reinicio de partida con confirmación y backup automático.
- Caché de esquemas: arranca rápido y sigue funcionando sin conexión.

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
  "auto_track": true,
  "poll_interval": 30,
  "sound": true,
  "sound_file": "",
  "recent_app_ids": []
}
```

| Campo | Descripción |
| --- | --- |
| `steam_api_key` | Clave de la API de Steam. Se obtiene gratis en <https://steamcommunity.com/dev/apikey>. |
| `steam_id_64` | Opcional. Se autodetecta desde `loginusers.vdf` si está vacío. Necesario para el badge y el auto-track. |
| `locale` | Idioma de los textos que devuelve Steam (`spanish`, `english`, ...). |
| `max_visible_count` | Máximo de logros visibles a la vez. |
| `notify` | Enviar notificación de escritorio al marcar o detectar un logro. |
| `auto_track` | Reflejar en local los logros que Steam desbloquee mientras FeatUI está abierto. |
| `poll_interval` | Segundos entre consultas a Steam. Se acota a 10–300; por defecto 30. |
| `sound` | Reproducir un aviso sonoro cuando se desbloquea un logro. |
| `sound_file` | Archivo de audio propio. Vacío = sonido del tema del sistema. |
| `recent_app_ids` | Últimos juegos abiertos. Los gestiona la app. |

> Las notificaciones de escritorio usan `notify-send` (Linux) por ahora. En
> macOS y Windows el aviso sonoro sigue funcionando, pero la notificación de
> escritorio está pendiente de backend nativo.

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

Todo respeta `XDG_CONFIG_HOME`, `XDG_DATA_HOME` y `XDG_CACHE_HOME` en Linux. En
macOS se usa `~/Library/Application Support` y `~/Library/Caches`, y en Windows
`%AppData%`, `%LocalAppData%` y la caché del usuario.

### Auto-desbloqueo

El auto-track es **genérico para cualquier juego**: no lee archivos de guardado
ni tiene lógica por juego. Consulta `GetPlayerAchievements` y compara el estado
con el sondeo anterior:

- Al abrir un juego se toma una **línea base**: no marca nada de golpe, aunque
  Steam tenga logros desbloqueados.
- Solo se marcan transiciones `pendiente → desbloqueado` observadas mientras
  FeatUI está abierto, usando el `unlocktime` real de Steam.
- Un desmarque manual **no se revierte** en el siguiente sondeo.
- Si un logro se desbloqueó con la app cerrada, usa `S` para sincronizar
  manualmente. La sincronización siempre pide confirmación.
- Si el perfil no es público o falta `steam_id_64`, el auto-track se desactiva
  con un aviso claro; el resto de la app sigue funcionando.
- Si Steam falla repetidamente se aplica un backoff progresivo (hasta 5 min).

### Sonido

El orden de resolución es:

1. `sound_file`, si está definido y existe (admite `~`).
2. El sonido del tema del sistema (Linux: `complete.oga`; macOS: `Glass.aiff`;
   Windows: `Windows Notify System Generic.wav`).
3. Un chime WAV generado por FeatUI en su caché, sin assets externos.

Para reproducir se detecta automáticamente `pw-play`, `paplay`,
`canberra-gtk-play`, `afplay`, `ffplay`, `mpv` o PowerShell `SoundPlayer`, según
el sistema. La reproducción tiene un timeout de 3 s y nunca bloquea la TUI.

## Controles

| Tecla | Acción |
| --- | --- |
| `↑` / `↓`, `k` / `j` | Mover el cursor |
| `espacio` | Marcar / desmarcar el logro seleccionado |
| `/` | Buscar por nombre o descripción |
| `f` | Alternar filtro: todos / pendientes / completados |
| `g` / `G` | Ir al principio / final |
| `S` | Sincronizar con Steam los logros ya desbloqueados (pide confirmación) |
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
- Backends de notificación nativos para macOS (`osascript`) y Windows.
- Exportar/importar progreso.

## English summary

FeatUI is a terminal achievement tracker for Steam. It only reads public
achievement metadata and player state from the Steam Web API and keeps your
progress in local JSON files. It does not unlock achievements on Steam and does
not interact with Valve systems. While open, it can mirror new Steam unlocks
generically for any game. Requires Go 1.27+.

## Licencia

MIT. Consulta [LICENSE](LICENSE).
