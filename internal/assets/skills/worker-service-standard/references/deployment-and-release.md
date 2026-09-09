# Despliegue y release de un worker service

Detalle de las reglas de despliegue del estándar. Cada punto se validó en un despliegue real, y
donde hubo un error que lo motivó está escrito: la razón es lo que evita que alguien lo "simplifique"
de vuelta al problema.

## Versionado

`<Version>` se declara en **un solo lugar**: `Directory.Build.props` si el proyecto lo tiene, y si no
el `.csproj` del Worker. Formato `MAYOR.MENOR.PARCHE`, sin prefijo `v`.

Ese valor es la fuente para nombrar artefactos. Escribir el número a mano en el nombre del zip
produce paquetes cuya versión no coincide con el binario que traen dentro, y eso no se detecta hasta
que alguien audita un servidor.

## Scripts en la raíz

Tres scripts, en la raíz del repositorio, no en una subcarpeta:

**`build.bat`** — cinco pasos:
1. `publish` para `win-x64`.
2. Copia `install.bat` y `uninstall.bat` dentro de `publish/`, porque el operador solo recibe el zip.
3. Genera `VERSION.txt` con versión, commit y fecha.
4. Genera `CAMBIOS.txt` con secciones Novedades / Correcciones / Otros, derivadas de `git log` entre
   el último tag y `HEAD`. Sin tags, toma el historial completo.
5. Comprime.

**`install.bat`** — chequeo de administrador, crea la carpeta de logs, `sc create` con arranque
automático, `sc failure` con reintentos, `start`, y `query` para confirmar.

**`uninstall.bat`** — `stop` y `delete`, con reintento.

## Ritual de release

Bump de `<Version>` → commit → `git tag vX.Y.Z` → `build.bat`.

El orden importa: **el tag es lo que acota el changelog de la versión siguiente**. Taguear después de
compilar deja el `CAMBIOS.txt` describiendo un rango que ya incluye el propio release.

## Compresor: 7-Zip

Se busca en `PATH` y en `%ProgramFiles%\7-Zip`. Si falta, se instala por `winget`. Como último
recurso, PowerShell con tres reintentos.

**Por qué no `Compress-Archive` como camino principal:** aborta el ZIP completo cuando un archivo
está retenido por otro proceso. 7-Zip solo emite un warning y termina el paquete. En un servidor con
el servicio corriendo, el archivo retenido es lo normal, no la excepción.

## Exclusiones en archivo, nunca en la línea de comandos

Lo que no viaja al servidor se declara en `delivery-exclude.txt` en la raíz, y se pasa como `-x@`.

**Nunca pasar `!` en la línea de comandos del `.bat`:** con `delayed expansion` activo, `cmd` consume
el signo y 7-Zip recibe un `-x` pelado, que le rompe el parseo. El síntoma es un error de sintaxis de
7-Zip que no menciona la exclusión, así que cuesta relacionarlo con la causa.

## Etiquetas fuera de bloques (aprendizaje de cmd)

Las etiquetas `goto` / `:label` **no viven dentro de un bloque `if (...)`**: `cmd` corta el archivo
ahí con "No se esperaba...". Toda lógica con reintentos va en una subrutina invocada con `call :sub`,
declarada al final del archivo.

## Aviso de arranque con agenda

El correo y el log de arranque incluyen qué jobs están habilitados, su expresión cron, y el próximo
disparo en la zona configurada. El próximo disparo se calcula con `CronExpression` de Quartz, **nunca
a mano**: una cuenta manual acierta hasta el primer cambio de horario de verano.

Si no hay ningún job habilitado, el aviso lo dice en voz alta. Un servicio que arranca sin agenda y
no lo menciona parece sano y no hace nada.

El resumen **nunca lanza**. Un cron inválido se marca en el propio aviso; tumbar el aviso por un cron
mal escrito deja al operador sin la única señal de que el servicio arrancó.

## Verificación del paquete

Después de compilar:

```bat
7z l releases\{zip} | findstr /i "Development"
```

Debe listar `appsettings.json` y **ningún** `appsettings.Development.json`. Un paquete que se lleva la
configuración de desarrollo al servidor apunta el servicio a la base equivocada.
