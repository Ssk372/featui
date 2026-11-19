# FeatUI

Rastreador de logros de Steam para la terminal. Vuelve a «cazar» los logros de
tus juegos con una lista local, ligera y sin tocar Steam.

> **FeatUI no desbloquea logros en Steam ni interactúa con los sistemas de
> Valve.** Solo lee metadatos públicos de la API de Steam (nombres,
> descripciones, iconos) y guarda tu progreso en local. Es una herramienta para
> revivir la experiencia de conseguirlos, no para modificarlos.

> **Estado de las pruebas:** FeatUI solo se ha probado en **Linux**. Los
> binarios de **Windows 10/11 y macOS** se compilan y publican, y el código
> incluye soporte para ambos sistemas, pero **no se han probado en una máquina
> real**: pueden aparecer bugs o problemas desconocidos. Si encuentras alguno,
> abre un issue con tu sistema y lo que hiciste.

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

### Binarios listos para usar (recomendado)

Descarga el `.zip` de la última release desde
<https://github.com/Ssk372/featui/releases>. No necesitas Go ni cuenta de
GitHub.

| Sistema | Archivo |
| --- | --- |
| Windows 10/11 (64 bits) | `featui_windows_amd64.zip` |
| Windows en ARM | `featui_windows_arm64.zip` |
| Linux x86-64 | `featui_linux_amd64.zip` |
| Linux ARM64 | `featui_linux_arm64.zip` |
| macOS Intel | `featui_darwin_amd64.zip` |
| macOS Apple Silicon | `featui_darwin_arm64.zip` |

Cada zip incluye el ejecutable, `config.example.json`, este README,
`WINDOWS.md` y la licencia. La release incluye además `SHA256SUMS` con los
hashes de todos los archivos.

**Windows:** sigue la guía paso a paso en [WINDOWS.md](WINDOWS.md).

**Linux:**

```bash
unzip featui_linux_amd64.zip
cd featui_linux_amd64
chmod +x featui
./featui
```

**macOS:**

```bash
unzip featui_darwin_arm64.zip   # o darwin_amd64 en Intel
cd featui_darwin_arm64
chmod +x featui
xattr -d com.apple.quarantine featui   # si Gatekeeper lo bloquea
./featui
```

El binario no está firmado, así que macOS puede pedir confirmación la primera
vez. Si Gatekeeper sigue bloqueándolo: Ajustes → Privacidad y seguridad →
«Abrir igualmente».

### Desde el código fuente

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

FeatUI crea su configuración la primera vez que se ejecuta, en el directorio de
configuración del sistema:

- Linux: `~/.config/featui/config.json`
- Windows: `%AppData%\featui\config.json`
- macOS: `~/Library/Application Support/featui/config.json`

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
# Linux / macOS
export FEATUI_STEAM_API_KEY="tu_clave"
```

```powershell
# Windows (afecta a las terminales nuevas)
setx FEATUI_STEAM_API_KEY "tu_clave"
```

La variable tiene prioridad sobre el archivo y **nunca se escribe en él**.

### Dónde se guardan los datos

| Contenido | Linux | Windows |
| --- | --- | --- |
| Configuración | `~/.config/featui/config.json` | `%AppData%\featui\config.json` |
| Progreso | `~/.local/share/featui/progress/` | `%LocalAppData%\featui\progress\` |
| Backup del progreso | `<appid>.json.bak` junto al anterior | igual |
| Caché de esquemas e iconos | `~/.cache/featui/` | `%LocalAppData%\featui\` |

En macOS: configuración y progreso en
`~/Library/Application Support/featui/`, caché en `~/Library/Caches/featui/`.
Todo respeta `XDG_CONFIG_HOME`, `XDG_DATA_HOME` y `XDG_CACHE_HOME` en Linux.

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

## Windows

Guía completa en [WINDOWS.md](WINDOWS.md). Resumen:

1. Descarga `featui_windows_amd64.zip` y descomprímelo.
2. Abre **Windows Terminal** en esa carpeta y ejecuta `.\featui.exe`.
3. Si SmartScreen lo bloquea: «Más información» → «Ejecutar de todas formas».
4. Consigue tu propia API key en <https://steamcommunity.com/dev/apikey> y
   pégala en `%AppData%\featui\config.json`.
5. El `steam_id_64` se detecta solo desde Steam; si no, ponlo a mano.
6. Tu perfil de Steam debe ser público para el badge y el auto-track.

## macOS

1. Descarga el zip que corresponda a tu Mac (`arm64` para Apple Silicon).
2. Descomprímelo y ejecuta `./featui` desde Terminal.
3. Si Gatekeeper lo bloquea, usa `xattr -d com.apple.quarantine featui` o
   Ajustes → Privacidad y seguridad → «Abrir igualmente».
4. La configuración queda en `~/Library/Application Support/featui/config.json`.

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

### Publicar una release

El workflow `.github/workflows/release.yml` compila los seis binarios, los
empaqueta y crea el release al empujar un tag:

```bash
git tag -a v0.2.0 -m "FeatUI v0.2.0"
git push origin v0.2.0
```

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

Prebuilt binaries are available for Windows, Linux and macOS. **Only Linux has
been tested so far**; the Windows and macOS builds are provided as-is and may
still have unknown bugs.

## Licencia

MIT. Consulta [LICENSE](LICENSE).
