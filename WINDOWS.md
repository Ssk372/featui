# FeatUI en Windows 10/11

Esta guía es para usar el `.exe` ya compilado: **no necesitas instalar Go** ni
tener cuenta de GitHub.

> **Aviso:** FeatUI solo se ha probado en Linux. La versión de Windows se
> compila y debería funcionar, pero puede tener bugs o comportamientos
> desconocidos. Si algo falla, abre un issue indicando tu versión de Windows y
> lo que hiciste.

## 1. Descargar

1. Abre <https://github.com/Ssk372/featui/releases>.
2. En la última release, descarga **`featui_windows_amd64.zip`** (Windows
   normal de 64 bits) o **`featui_windows_arm64.zip`** si tu equipo es ARM.
3. Descomprime el zip. Dentro verás `featui.exe`, `README.md`, esta guía y
   `config.example.json`.

Puedes comprobar la descarga con el archivo `SHA256SUMS` de la release:

```powershell
Get-FileHash .\featui_windows_amd64.zip -Algorithm SHA256
```

## 2. Primera ejecución

1. Abre **Windows Terminal** (recomendado) o PowerShell en la carpeta
   descomprimida.
2. Ejecuta:

   ```powershell
   .\featui.exe
   ```

3. Si aparece **«Windows protegió tu PC»** (SmartScreen), pulsa «Más
   información» → «Ejecutar de todas formas». El ejecutable no está firmado
   digitalmente porque el proyecto no tiene certificado de firma.

> Si haces doble clic y la ventana se cierra al instante, ejecútalo desde la
> terminal para poder leer el error.

## 3. Tu API key de Steam

Cada persona debe usar **su propia** clave. No compartas la de otra persona:
es secreta y tiene límite de peticiones.

1. Entra en <https://steamcommunity.com/dev/apikey> con tu cuenta de Steam.
2. Copia la clave.
3. Edita `%AppData%\featui\config.json` (FeatUI lo crea en el primer arranque):

   ```powershell
   notepad "$env:AppData\featui\config.json"
   ```

4. Sustituye `TU_STEAM_API_KEY_AQUI` por tu clave:

   ```json
   {
     "steam_api_key": "AQUI_TU_CLAVE",
     ...
   }
   ```

   También puedes usar una variable de entorno (solo afecta a terminales
   nuevas):

   ```powershell
   setx FEATUI_STEAM_API_KEY "AQUI_TU_CLAVE"
   ```

5. Tu **perfil de Steam debe ser público** (al menos los detalles de juego)
   para que funcionen el badge `· steam` y el auto-desbloqueo.

## 4. SteamID64

FeatUI intenta detectarlo solo leyendo:

```
%ProgramFiles(x86)%\Steam\config\loginusers.vdf
%ProgramFiles%\Steam\config\loginusers.vdf
```

Si no lo encuentra, aparecerá el aviso «falta steam_id_64». Puedes ponerlo a
mano en `%AppData%\featui\config.json`:

```json
"steam_id_64": "76561198000000000"
```

Lo encuentras en la URL de tu perfil de Steam (`steamcommunity.com/profiles/…`)
o en `loginusers.vdf`.

## 5. Usar FeatUI

- Escribe el **AppID** del juego o pega su URL de la tienda y pulsa `enter`.
  Por ahora no hay búsqueda por nombre.
- `espacio` marca o desmarca un logro.
- `S` sincroniza con Steam lo que ya tengas desbloqueado (pide confirmación).
- `?` muestra todos los atajos; `q` sale.

Mientras la pantalla de logros esté abierta, FeatUI consulta Steam cada 30
segundos y refleja los logros nuevos.

## 6. Rutas y desinstalación

| Contenido | Ruta |
| --- | --- |
| Configuración y API key | `%AppData%\featui\config.json` |
| Progreso | `%LocalAppData%\featui\progress\<appid>.json` |
| Backup | `%LocalAppData%\featui\progress\<appid>.json.bak` |
| Caché de esquemas e iconos | `%LocalAppData%\featui\` |

Para desinstalar: borra `featui.exe` y las carpetas `%AppData%\featui` y
`%LocalAppData%\featui`.

## 7. Problemas comunes

| Problema | Solución |
| --- | --- |
| SmartScreen bloquea el `.exe` | «Más información» → «Ejecutar de todas formas». |
| La ventana se abre y se cierra | Ejecuta `.\featui.exe` desde Windows Terminal para ver el error. |
| «Falta la API key» | Revisa `%AppData%\featui\config.json` o `setx FEATUI_STEAM_API_KEY`. |
| «falta steam_id_64» | Añade `steam_id_64` a mano; comprueba que Steam esté instalado en la ruta habitual. |
| No aparecen logros | Verifica el AppID y que el juego tenga logros. Prueba `r` para recargar. |
| El auto-track no marca nada | El perfil debe ser público; `S` solo recupera lo desbloqueado con la app cerrada. |
| No hay notificación de escritorio | En Windows todavía no hay toast nativo; el **sonido sí funciona**. |
| No suena nada | Revisa `sound` y `sound_file` en el config; el aviso de la interfaz indica el fallback usado. |
| Antivirus sospecha | Es un binario de Go sin firma; puedes compilarlo tú mismo desde el código. |

## Limitaciones conocidas en Windows

- Sin notificaciones toast nativas (solo sonido).
- Sin búsqueda de juegos por nombre (AppID o URL).
- Binario sin firma digital.
- Probado solo en Linux: los bugs de Windows se irán corrigiendo con los
  reportes.
