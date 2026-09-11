---
name: worker-service-standard
description: "Trigger: worker service, windows service, Quartz, job programado, ETL, proceso batch, migrar de .NET Framework. Estandares de workers de HG."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.1"
---

## Cuándo usarla

Al crear un worker service nuevo, al revisar uno existente, o al migrar uno de .NET Framework
a .NET 10. También al decidir entre EF Core y Dapper dentro de un proceso por lotes.

Para un CRUD de API usá `backend-crud-standard`. Para cambios de esquema, `db-change-standard`.

## El esqueleto

Cuatro proyectos. **Es el objetivo, no un censo.**

```
Hg.<Contexto>.Domain     entidades, DTOs, interfaces, helpers
Hg.<Contexto>.Data       repositorios, DbContext, AddDataLayer()
Hg.<Contexto>.Business   lógica de negocio, AddBusinessLayer()
Hg.<Contexto>.Worker     host, Jobs programados, Program.cs, Configs/
```

El host se llama **`Worker`**, no `Service`, **en un worker nuevo**. Esa nomenclatura es de
workers y no se traslada. Para una API, `api-crud-standard` fija **API / CORE / SERVICE / DATA**
—donde `SERVICE` **es** la capa de negocio— y su propio alcance declara que aplica exclusivamente
a `portaltools-api`, `etruckssecurity-api` y `portaltools-webapp` (`spec.md:359`). Llevar `Domain`
y `Business` a esos repositorios contradice la convención vigente ahí.

El repositorio se llama `{funcionalidad}-worker` o `{funcionalidad}-ws` — `ws` por *worker
service*. **No `-service` ni `-winservice`**: son el nombre que se arrastra de la época de .NET
Framework y hoy no distinguen nada, porque los llevan tanto servicios modernos como legacy. Los
repositorios existentes no se renombran de paso.

Para una API nueva fuera del alcance de la spec no hay una regla escrita, pero tampoco es un hueco:
hay tres vocabularios en uso —API / CORE / SERVICE / DATA, DOMAIN / DATA / BUSINESS / API y
Domain / Data / Service—, y `backend-crud-standard` los enumera con sus repositorios. Se elige uno
de esos tres y se declara; no se inventa un cuarto.

`net10.0` en todos los `.csproj` de un proyecto nuevo. No se elige una versión más vieja por
alinearse con proyectos legacy — pero tampoco se sube un worker existente como limpieza
incidental: eso es una migración y tiene su propio orden más abajo.

Lo que un agente encuentra en repositorios existentes es otra cosa. Conviven esta forma de cuatro
capas, una variante con los proyectos bajo `src/` y `Core`/`Infrastructure` en lugar de
`Domain`/`Data` —`zametlordenescompra-winservice` es un ejemplo—, y servicios de un solo proyecto
sin separación de capas. Un worker existente que no coincida NO es un defecto a corregir de paso.

## Registro de dependencias

Un método de extensión por capa, encadenados en `Program.cs`. No decenas de `AddScoped` sueltos.

```csharp
builder.Services
    .AddDataLayer(builder.Configuration)
    .AddBusinessLayer()
    .AddPollyConfig(builder.Configuration)
    .AddQuartzScheduling(builder.Configuration)
    .AddScoped<INotificaEmail, NotificaEmail>();
```

El default de lifetime es `AddScoped`. Un `AddSingleton` SHOULD llevar un comentario que explique
por qué ese tipo es seguro compartido: sin estado, sin conexión abierta como campo, sin
credenciales locales. Es SHOULD y no MUST por decisión explícita — una revisión adversaria previa
(`service-architecture`, Rev. 3) lo bajó al comprobar que los propios proyectos de referencia no
lo cumplían, y una regla que el código modelo incumple entrena a ignorarla. Si no se puede
escribir esa frase, el registro va `AddScoped`.

## Scheduler

Quartz.NET, y nada más. `service-architecture` lo fija como MUST para todo worker nuevo que
necesite jobs programados; esta skill no abre excepciones a eso. Si alguna vez conviene otro
scheduler, se cambia primero la spec y después esta skill, en ese orden.

El host va como servicio de Windows con `AddWindowsService()` **bajo la guarda de
`WindowsServiceHelpers.IsWindowsService()`**, para que el mismo binario corra como servicio o como
consola. Sin la guarda, depurar obliga a compilar distinto. No schedulers propios, no `Timer` a
mano: reimplementar cron a mano trae el bug de horario de verano, y ya mordió en este dominio.

Cada Job MUST documentar qué lo dispara, con qué frecuencia, y qué pasa si una ejecución se
solapa con la anterior. Un Job sin política de solapamiento declarada es un Job que algún día
va a correr dos veces sobre los mismos datos.

`[DisallowConcurrentExecution]` protege **dentro de un scheduler**. Si el servicio puede correr en
más de un host —failover, un despliegue solapado, una segunda instalación— eso no garantiza nada:
ahí hace falta un store persistente en modo cluster.

### Persistencia del disparo

Quartz usa por defecto un store en memoria: un disparo programado y no ejecutado **se pierde al
reiniciar el servicio**. Si perder un disparo importa, el job store MUST ser persistente
(`JobStoreTX` sobre SQL Server).

Y con eso viene una decisión que MUST tomarse explícita: **la política de misfire**. El default de
Quartz es `SmartPolicy`, que para un `CronTrigger` resuelve en disparar UNA sola vez al volver y
seguir con la agenda; reencolar cada disparo perdido hay que pedirlo a propósito con
`IgnoreMisfires`. Nada de eso salva del problema real: tras una caída larga, todos los jobs
distintos que quedaron atrasados despiertan juntos y golpean la misma base al mismo tiempo. Para un
ETL eso es el daño que `[DisallowConcurrentExecution]` venía a evitar, autoinfligido — y esa
directiva no protege contra jobs distintos. La política se elige y se escribe:
`zamletl-winservice` fija `WithMisfireHandlingInstructionFireAndProceed()` con el motivo al lado.

Persistir el disparo recupera **la ejecución, no los datos**: un job que calcula su ventana con la
hora actual no puede reprocesar el día que se perdió. Para eso la ventana MUST venir de estado
persistido —una marca de agua que avanza recién cuando la escritura confirma—, no del reloj.

Las tablas del job store son un cambio de esquema: van por `db-change-standard` como cualquier
otra, con su retención acotada y declarada.
    
**Apagado ordenado.** Todos los métodos de negocio y de datos MUST aceptar y propagar el
`CancellationToken` que Quartz entrega al job, hasta la consulta (en Dapper, por
`CommandDefinition`). Un job que ignora el token convierte el apagado en un *hard kill* del
gestor de servicios y deja filas en `IN_PROGRESS` para siempre. [REVIEW]

## Librería de correo

`hgt_development/smtpemailclient-library`. La elección no es libre: submódulo git, salvo que
exista una razón que lo impida y quede escrita.

La consumen de más de una forma: submódulo git, copia vendorizada dentro del repositorio, un DLL
por `HintPath`, y un `PackageReference` que no resuelve. Buscar solo submódulos no las encuentra
todas. Un proyecto nuevo MUST montarla en `libs/smtpemailclient-library`. Los existentes NO se
renombran de paso: mover la ruta rompe lo que ya compila, y merece su propia tarea.

Vendorizar es legítimo cuando el submódulo no sirve, y hoy hay **una** razón real y verificada: no
existe un NuGet consumible, porque el feed privado que debería hospedarlo devuelve 401. Es la única
que aparece escrita en la copia que sí se justificó, la de `zametlordenescompra-winservice`.

**La diferencia de framework NO es razón para vendorizar.** El submódulo declara `net8.0` y se
consume por `ProjectReference` desde un worker `net10.0` sin problema: `zamletl-winservice` lo hace
y lo deja escrito en el `Directory.Build.props` de su `libs/`, con la fecha en que se verificó que
compila. Aceptarla como razón la vuelve siempre verdadera, y una excepción siempre verdadera anula
la regla del submódulo.

Cuando se vendoriza, la razón MUST quedar escrita en el `.csproj` de la copia, con la historia que
lo decidió. **Aplica a las copias nuevas.** Varias de las que ya existen no la tienen: son la misma
deuda que el resto de lo existente, se documentan cuando se toca ese repositorio, y no se arreglan
de paso.

Si se usa submódulo, el `README` MUST decir que el clon necesita
`git submodule update --init --recursive`. Sin eso el proyecto no compila y quien llega nuevo no
sabe por qué.

## EF Core o Dapper

| Caso | Elección |
|---|---|
| Reportes, ETL, KPIs, lecturas masivas de solo lectura | **Dapper** |
| `upsert` o inserción condicional en lote | **Dapper** |
| CRUD con navegación entre entidades, tracking y migraciones | **EF Core** |

El híbrido está permitido: EF aporta el modelo y las entidades, Dapper aporta el rendimiento en
lote. Pero con una condición que no es opcional.

**Toda consulta de EF cuyo resultado no se va a persistir por EF MUST llevar `AsNoTracking()`.**

Sin eso, cada lectura construye un grafo de cambios que nadie va a consultar, y el costo crece
con el volumen del proceso. Un worker auditado tenía `AsNoTracking` en cero con `SaveChanges` en
uno: estaba pagando el seguimiento completo para no guardar nunca nada por EF.

Y si el conteo de `SaveChanges` se queda cerca de cero, la pregunta correcta no es cómo
optimizar el `DbContext` sino por qué sigue ahí.
    
**Transacción compartida en el híbrido.** Si EF y Dapper operan bajo la misma transacción de
negocio sobre las mismas tablas, Dapper MUST recibir por parámetro la misma instancia de
`DbConnection` y `DbTransaction` que usa el `DbContext` (`GetDbConnection()` +
`UseTransaction`). Dos transacciones independientes sobre las mismas tablas producen bloqueos
entre sí y commits parciales —una confirma, la otra revierte— que ningún reintento posterior
puede reconciliar. [REVIEW]

## Documentación de clases

Toda clase pública lleva `<summary>` con la **responsabilidad**, no con el nombre reformulado.
Un `/// Obtiene el usuario por id` sobre `GetUserById` no agrega nada y ensucia la lectura.

Lo que sí vale la pena documentar es el porqué:

```csharp
// El cliente SMTP es stateless: no guarda conexión abierta ni credenciales
// locales. Solo expone modelos y SendEmailAsync, así que compartir una
// instancia es seguro.
.AddSingleton<ISmtpClientService, SmtpClientService>()
```

MUST documentarse todo Job programado y toda
consulta cruda —Dapper o `SqlConnection`— explicando por qué no se usó el camino por defecto.

## Reintentos

Polly se configura en `AddPollyConfig`, con los parámetros leídos desde `appsettings`. **En un
worker nuevo el default son 3 reintentos**; los existentes tienen sus propios valores y no se
tocan de paso. Más de 3 se admite cuando el caso lo justifica, pero sigue siendo configuración:
nunca un número incrustado en el código.

Un reintento sin límite de tiempo no es una política: toda operación reintentada MUST tener
timeout, y **la duración del peor caso MUST ser menor que el intervalo del propio job**. Si no lo
es, la corrida siguiente arranca encima de la anterior y lo único que separa eso de una escritura
duplicada es la política de solapamiento.

Lo que MUST estar escrito es **qué operaciones son idempotentes**. Cuántas veces se reintenta y
qué se reintenta ya se leen de la configuración; la idempotencia no se lee de ningún lado.

Un ETL que reintenta un `upsert` no idempotente duplica datos, y lo hace en silencio. Antes de
envolver una operación en una política de reintento, verificá que correrla dos veces deje el
mismo resultado que correrla una.

## Migrar desde .NET Framework

El orden importa porque cada paso deja el servicio en un estado verificable. Mezclarlos hace
imposible saber si rompió el framework o el rediseño.

1. Inventariar los `.csproj` y leer el framework **del `.csproj`, nunca del `README`**.
2. Separar en las cuatro capas, todavía sobre el framework viejo.
3. Sustituir el host por el genérico, con `AddWindowsService()` bajo su guarda.
4. Reemplazar el scheduler propio por Quartz.NET.
5. Mover el envío de correo a la librería compartida: submódulo, o copia vendorizada si el
   submódulo no sirve y la razón queda escrita en el `.csproj`.
6. **Recién ahora** subir a `net10.0`.
7. Agregar el aviso de ciclo de vida y el pipeline.

El framework va en el paso 6, no en el 1. Migrar la versión primero convierte cualquier falla
posterior en una discusión sobre quién la causó.
    
## Versionado, Empaquetado y Despliegue

*El estándar no ata el worker a un SO específico, pero exige un contrato estricto de entrega y artefactos predecibles.*

* **[ARNÉS]** Todo worker MUST declarar `<Version>` en un solo lugar —`Directory.Build.props` (preferido) o el `.csproj`. Formato `MAYOR.MENOR.PARCHE` sin prefijo. Esta es la única fuente de verdad.
* **[MANUAL / CI]** Entregables concretos: El proceso de compilación MUST producir en la carpeta `releases/` un archivo `{Assembly}_v{Version}.zip`. Dentro del paquete, MUST incluirse `VERSION.txt` (versión, commit, fecha), `CAMBIOS.txt`, y excluir lo listado en `delivery-exclude.txt`.
* **[MANUAL]** Trazabilidad de cambios: `CAMBIOS.txt` es el artefacto autogenerado del release; `CHANGELOG.md` (cuando exista en el repo) es el curado a mano. Si ambos existen, el autogenerado MUST citar el rango de tags del que salió para que la discrepancia sea trazable.
* **[REVIEW]** Aprovisionamiento: El entorno destino MUST estar pre-aprovisionado (runtimes, compresor, acceso a red interna) antes del primer despliegue, verificado con checklist. Ningún script de despliegue descarga dependencias de internet en tiempo de ejecución. Alternativa aceptada: artefacto autocontenido (`--self-contained`; `PublishSingleFile` solo si el arranque en frío por descompresión lo tolera).
* **[REVIEW]** Archivos de configuración: El empaquetado final (`publish/`) MUST NOT contener secretos ni credenciales reales en sus `appsettings.json`. Viajan con placeholders.
* **[MANUAL]** Desarrollo local: Los secretos MUST gestionarse vía variables de entorno o `dotnet user-secrets`, nunca en archivos versionados.
* **[REVIEW]** Scripts de ciclo de vida (ej. `install/uninstall`): MUST ejecutarse bajo una cuenta de servicio gestionada (gMSA en Windows/AD; equivalente gestionado en otro SO) con privilegios SQL mínimos (NUNCA `LocalSystem` por defecto). MUST configurar recuperación automática ante fallos y manejar desinstalaciones limpias.
* **[MANUAL]** Ritual de release: Bump de `<Version>` → Commit → Tag `vX.Y.Z` → Build. El tag MUST existir *antes* del build que genera el `CAMBIOS.txt`; sin tag previo, el rango cae a todo el historial en silencio.

## Reglas ejecutables del arnés

*El estándar exige validación continua, pero las reglas deben ser útiles, no ruido.*

* **[ARNÉS]** Toda regla nueva del arnés motivada por un defecto MUST validarse contra un caso de prueba (o el código de origen, en migraciones) que garantice que la regla dispara (FAIL). Si no detecta el fallo que la motivó, no discrimina.
* **[ARNÉS]** El auditor estático MUST reportar falla si no pudo parsear/leer la totalidad del código. Un escáner que devuelve verde porque falló al leer un archivo (ej. literales `"""`) invalida la auditoría.
* **[REVIEW]** Un detector con falsos positivos repetitivos SHOULD degradarse a *Warning* o retirarse a una rama de calibración (con deuda registrada y dueño) hasta que deje de romper builds legítimas.

## Prevención de éxito silencioso

*Ninguna operación que falle debe pasar desapercibida ni dejar un estado inconsistente.*

* **[REVIEW]** El registro de auditoría/bitácora se rige por dos momentos estrictos:
* **Constancia de Inicio:** Va ANTES de comenzar el trabajo (ej. ciclo ETL). Si el proceso muere, debe quedar evidencia de la ejecución iniciada (`IN_PROGRESS`).
* **Constancia de Entrega:** Va DESPUÉS de la confirmación del sistema destino (ej. envío de correo, inserción en ERP). Registrar antes asume un éxito que puede no ocurrir.
* **[REVIEW]** Los booleanos de los sinks y los *rowcounts* se honran: un sink que devuelve `false` o un `UPDATE` que afecta 0 filas (Sin Efecto) MUST NOT avanzar la marca de agua ni contar como éxito.
* **[REVIEW]** Todo bloque de escritura MUST ser idempotente. Donde no sea viable (ej. un `INSERT` ciego sin clave única conducido por una marca externa), la ventana residual MUST quedar documentada con su dueño, su mitigación (aislamiento por destino, reintento en la escritura de la marca) y su condición de reapertura. Una ventana aceptada sin dueño es un defecto; una ventana aceptada con dueño es arquitectura.
* **[REVIEW]** Unicidad en destino: toda tabla destino de un proceso batch/ETL SHOULD tener una restricción de unicidad sobre su clave natural. La unicidad convierte una duplicación silenciosa en una excepción visible que la política de reintentos puede manejar —es el mecanismo que hace seguro reintentar—. Donde el esquema no se puede tocar, la excepción queda documentada con su dueño, igual que la ventana residual.

## Migraciones (Paridad y Alcance)

*La migración de sistemas legacy requiere fronteras claras.*

* **[MANUAL]** Paso 0: Antes de migrar, MUST inventariarse el 100% de los proyectos del repositorio origen y declarar por escrito el alcance (qué entra y qué muere explícitamente). El alcance implícito es inválido.
* **[MANUAL]** Todo *change* de migración cierra con paridad verificada contra el comportamiento observable real, no asumiendo lo que dice el código viejo.
* **[REVIEW]** Si el proceso origen está roto o es errático, el criterio de "comportamiento correcto" MUST tener un dueño de negocio/operaciones asignado que valide la nueva lógica.

## Patrones ETL Incremental

*Reglas para dominios de extracción masiva dentro de workers.*

* **[REVIEW]** La marca de agua avanza por la unidad lógica del lote (entidad raíz o solicitud), nunca por renglón de detalle. El corte debe caer siempre en el límite de la entidad para evitar pérdida silenciosa de datos.
* **[REVIEW]** La clave de la marca de agua MUST identificar unívocamente al origen/feed, no solo el nombre de la tabla local. Un worker con múltiples feeds a la misma tabla pisará sus marcas si no segrega por clave.
* **[REVIEW]** La selección de candidatos por ciclo MUST cubrir el universo o avanzar sobre él (cursor, tandas con exclusión de lo ya revisado). Un `TOP` fijo ordenado siempre igual, sobre un universo que crece, deja una cola que nunca es revisada.
* **[REVIEW]** La subconsulta de corte MUST llevar `DISTINCT` antes del `TOP`: el identificador por sí solo no es único cuando la cabecera se une por áreas o cruces múltiples.
* **[REVIEW]** El piso de fecha temporal MUST basarse en el año/mes de la propia marca de agua, NUNCA del reloj del sistema (`DateTime.Now`). Atarlo al año en curso pierde solicitudes rezagadas cada 1 de enero.
* **[ARNÉS]** Sin `NOLOCK` en lecturas que deciden cierres irreversibles. Para ser auditable, la consulta irreversible MUST llevar un marcador explícito que nombre la decisión (ej. `/* SIN NOLOCK: Cierre de orden irreversible */`). Si solo dice 'sin nolock', el test pasa pero la documentación se pudre.
* **[REVIEW]** Guardas contra el conjunto vacío: Todo ciclo de sincronización MUST abortar o saltar la ejecución si la lectura de origen devuelve cero filas, protegiendo al destino de truncados accidentales.
* **[REVIEW]** Operaciones con `IN @Ids` sobre conjuntos grandes MUST segmentarse en lotes (ej. tandas de 2,000) dentro del repositorio para evitar el límite de 2,100 parámetros de SQL Server.

## Estrategia de Pruebas

*La calidad no se negocia, pero la profundidad de la prueba se adapta al caso.*

* **[ARNÉS]** Unitarias: MUST existir para lógica de negocio, parsing y reglas del arnés. Para guardas críticas y flujos irreversibles, la prueba MUST verificarse por mutación al crearse (el test falla sin la guarda).
* **[ARNÉS]** Integración: MUST existir contra un motor real (LocalDB/Docker) siempre que haya SQL crudo, lógica de cursores o dependencias directas del motor.
* **[MANUAL]** Humo contra Producción: Estrictamente de lectura (dependencias responden, base alcanzable, marca legible) — NUNCA dispara escrituras.
* **[MANUAL]** Humo con Escrituras: SOLO contra ambientes segregados (ej. `smoke-test`), con las escrituras identificables por origen y documentadas antes de correr.

## Cross-link: Email Branding

* **[REVIEW]** Todo worker que envíe correos transaccionales SHOULD acoplarse al estándar `openspec/specs/email-branding` (esqueleto de 5 secciones, paleta/logos estándar, inyección `isHtml: true` respetando el contrato SMTP). *Nota: Ascender a MUST cuando el estándar de branding sea ratificado a nivel organización.*

### Anexo Técnico: Trampas de Empaquetado y Windows CMD

* **Compresores y Archivos Retenidos:** Usar 7-Zip en lugar de PowerShell `Compress-Archive`. Si un log está retenido por otro proceso, `Compress-Archive` aborta el trabajo entero; 7-Zip emite un *warning* pero empaqueta el resto. *(Evidencia: ZIP no generado en 3 intentos previos)*.
* **Exclusiones seguras:** Pasar exclusiones a 7-Zip mediante archivo (ej. `-x@delivery-exclude.txt`). Si se pasa el caracter `!` en la línea de comandos de un `.bat` con *delayed expansion* activo, CMD lo consume y 7-Zip recibe un parámetro `-x` pelado que aborta la compresión silenciosamente.
* **Rompimiento de Parseo por Etiquetas:** Las etiquetas de salto (`goto` / `:label`) NO deben vivir dentro de bloques `if` en CMD. Rompen el parseo prematuramente lanzando el error exacto: *"No se esperaba..."*. La lógica con reintentos debe ir en una subrutina invocada con `call`.


## Lo mínimo antes de dar por terminado

- **Que la muerte del servicio se note.** Son tres cosas distintas y hacen falta las tres:
  - Un arranque fallido MUST terminar con **código de salida distinto de cero**. Con código 0 el
    gestor de servicios de Windows entiende "arrancó bien y paró limpio": no reinicia, no marca
    error, no alerta. El servicio queda caído y el sistema cree que todo salió bien.
  - Un **vigilante fuera del proceso** que alerte por ausencia de señal — un latido y una marca de
    última corrida exitosa, y la alarma se dispara cuando envejecen. El aviso por correo NO cubre
    esto: si el proceso murió, no queda nadie para mandarlo, y `email-branding` lo dice
    explícitamente al declarar fuera de alcance el servicio que no vuelve a arrancar. El latido
    MUST escribirse en una tabla consultable desde fuera —la misma base de control donde ya vive
    la marca de última corrida—, con el nombre del proceso y `endTime` en UTC; la consulta exacta
    del vigilante MUST estar fijada por una prueba, para que un renombre no deje la alarma muda.
    Un endpoint HTTP de health es alternativa aceptable solo donde ya hay un orquestador que lo
    consulte: en un servicio que corre solo de noche, la fila en base es la que sobrevive a la
    muerte del proceso. [ARNÉS la consulta fijada por test / MANUAL el vigilante y la alarma]
  - El **aviso de ciclo de vida** al arrancar y al detenerse, obligatorio por `email-branding`.
    Sirve para enterarse de lo que sí pasó, no de lo que dejó de pasar.
- **Pipeline en Bitbucket**, y que falle si el clon limpio no compila con los submódulos
  inicializados.
- **Ningún secreto versionado.** `git ls-files` no debe devolver ningún archivo con una credencial.
    - Proyecto de pruebas: ver la sección "Estrategia de Pruebas" — unitarias obligatorias,
      integración contra motor real cuando hay SQL crudo, humo antes de liberar.
