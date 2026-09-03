---
name: backend-crud-standard
description: "Trigger: CRUD .NET, controller, EF Core, repositorio, auditoria, paginacion, API externa. Estandares de backend de HG."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.0"
---

## Cuándo usarla

Al escribir o revisar un CRUD en .NET: controladores, repositorios EF Core, servicios de negocio
o integraciones con servicios externos.

**El vocabulario de capas depende del repositorio.** Hoy conviven tres:

- **API / CORE / SERVICE / DATA**, donde `SERVICE` es la capa de negocio. Es la que fija
  `api-crud-standard`, y la usan `portaltools-api`, `etruckssecurity-api`, `edi-api` y
  `operationsboard-api`.
- **DOMAIN / DATA / BUSINESS / API** — `hgproveedorextranet-api`.
- **Domain / Data / Service** — `Hg.GpsApi` (donde la capa de negocio se llama `Services`),
  `datahubviaje-api` y el backend de `comedor`.

Verificá cuál usa la solución antes de crear proyectos. Para una API nueva fuera del alcance de
`api-crud-standard` no hay una regla escrita, pero tampoco es un hueco: la primera es la más
repetida —`edi-api` y `operationsboard-api` la adoptaron sin estar en ese alcance— y la tercera es
la de `Hg.GpsApi`, el proyecto que `service-architecture` toma como referencia del orden de
configuración de `Program.cs`. Se elige una de las tres y se declara; no se inventa una cuarta.

Cada regla de aquí nació de un defecto real que llegó a un ambiente. No son preferencias.

## Contrato de respuesta

| Regla | Detalle |
|---|---|
| REQUIERE `Response<T>` | `Items` para colecciones, `Item` para uno, `AddError` para fallos de negocio |
| Un fallo de NEGOCIO va como **HTTP 200** con `Success = false` | Los 4xx quedan para transporte y autorización |
| RECHAZA filtrar DTOs de servicios externos | BUSINESS traduce el sobre ajeno al propio |

El punto 2 sorprende a quien llega nuevo: consumir la API asumiendo "200 = salió bien" muestra
éxito sobre un rechazo. El veredicto está en `Success`, el motivo en `ErrorList`.

## Auditoría

| Regla | Por qué |
|---|---|
| REQUIERE guardar el **identificador** del operador, no su nombre | Un nombre deja de apuntar a nadie si la persona se renombra |
| El nombre en texto va donde NO genere un N+1 | Con tabla de usuarios local, en la vista o el DTO. Con identidad externa sin consulta por lotes, en la tabla base: ahí es lo único que evita una llamada por renglón |
| El autor sale del TOKEN, nunca del cuerpo de la petición | Si viene del cliente, cualquiera firma una baja con el nombre de otro |
| RECHAZA escribir auditoría incompleta | Sin identidad completa se responde 401. Un registro a medias parece confiable y no lo es |

El identificador vive en el claim `sub`; el nombre en `unique_name`. Los dos llegan gratis en el
mismo token al momento de escribir.

**Las cuatro reglas de arriba son para escrituras hechas por una persona.** Un ETL o un job no
tiene sesión ni token, y exigirle uno lo lleva a responder 401 por algo que no puede tener. Un
actor no humano REQUIERE una constante propia y explícita — `cat.Proveedor` usa
`ETL_ProveedorSync`. Antes de aplicar estas reglas a una tabla, verificá quién escribe en ella.

El tipo y el nombre de las columnas los fija `db-change-standard`, y dependen de dónde viva la
identidad. Con una tabla de usuarios en la propia base: `uniqueidentifier` con FK y la navegación
marcada `[JsonIgnore]`, con el nombre en texto en la vista o el DTO. Con identidad externa
(HG.AccessExternal / Entra): el `sub` del token sin FK —o el UPN en texto si ese sistema no maneja
un GUID de usuario—, porque no hay tabla local a la cual apuntar. Antes de copiar la forma con FK,
verificá que el destino sea una tabla y no una vista: `VwUsuario` está mapeada con `ToView(...)` y
SQL Server no admite una FK contra una vista.

## Fechas

**El tipo `datetime` de SQL Server no guarda zona.** EF Core lo materializa con
`DateTimeKind.Unspecified`, System.Text.Json lo serializa SIN la `Z`, y el navegador lo lee como
hora local: todas las fechas se corren el tamaño del huso.

- REQUIERE un `ValueConverter` que marque `DateTimeKind.Utc` al LEER.
- Un filtro por día se traduce a la ventana UTC `[inicio, fin)` de la zona del negocio
  (`America/Mexico_City`), NO se compara con `.Date`.
- Síntoma delator: la misma propiedad sale con `Z` al crear el registro (viene de `DateTime.UtcNow`)
  y sin `Z` al leerlo del listado.

## Paginación y orden

| Regla | Por qué |
|---|---|
| El total se cuenta DESPUÉS de aplicar filtros | Es el universo de la búsqueda, no el de la tabla |
| REQUIERE desempate por la clave primaria | Sin orden total, la paginación repite u omite renglones entre páginas |
| Página 0 o negativa se trata como la primera | Devolver vacío ahí parece "no hay datos" |
| Devuelve el TOTAL en `TotalRecords`, no sólo la página | Es lo que permite al cliente saber si quedan más |
| Columna de orden y de filtro contra una lista BLANCA | Lo demás se ignora; nunca concatenar en SQL |

## Integración con servicios externos

- **Averigua si el `PUT` reemplaza o mezcla.** El de HG.AccessExternal REEMPLAZA los campos que
  acepta: mandar uno vacío lo BORRA. Nunca enviar una edición parcial.
- REQUIERE compensación: si la operación local falla después de crear algo en el servicio externo,
  deshabilítalo. Si no, queda un acceso huérfano.
- Una baja lógica DEBE revocar el acceso en el servicio externo, no sólo marcar la fila. Sin eso el
  renglón se ve como "Baja" y la persona sigue entrando.
- El servicio externo que no responde NO debe tumbar la operación entera: devuelve lo local y una
  bandera que diga qué falta.

## Autorización

RECHAZA un controlador que sólo declare `[Authorize]` cuando el recurso es de un rol específico.
`[Authorize]` responde "¿el token es válido?", no "¿este usuario puede hacer esto?". Si los usuarios
finales también tienen tokens válidos, ya está abierto.

Ocultar la opción en la interfaz es comodidad visual, **no** la garantía. La prueba de que está
bien: construir la petición a mano con el token de un usuario sin permiso y recibir **403**.

Verifica también que el rol del recurso no sea el MISMO que se asigna a los usuarios finales: si
coinciden, todos heredan la administración. Pasó, y se demostró con un `PUT` de un proveedor sobre
un usuario de otro que devolvió 200.

### Primero: ¿el token trae el rol?

No lo supongas, **decodifícalo**. Si no lo trae, la decisión obliga a consultar al emisor, y eso
cambia el diseño entero — deja de ser una comprobación local y pasa a ser una llamada de red dentro
del camino de autorización. De ahí salen tres exigencias que hay que resolver a propósito:

| Exigencia | Por qué |
|---|---|
| **Falla CERRADO**: si el emisor no responde, NIEGA | Asumir permiso convierte una caída del servicio de seguridad en una puerta abierta |
| **Cachea sólo veredictos concluyentes** | Sin caché hay una llamada externa por petición. Pero un "no pude preguntar" jamás se guarda: se reintenta |
| **Exige el rol Y que esté activo** | Un rol revocado no habilita nada |

### El manejador va Scoped, no Singleton

Si consume un cliente que lleva el token del usuario, ese cliente vive **por petición**. Un
manejador de vida larga captura el de la PRIMERA petición y resuelve a todos los demás con esa
identidad. El caché puede seguir compartido si el componente de caché sí es de vida larga.

Esto NO lo detecta ninguna prueba unitaria: lo detecta el contenedor al arrancar. Ver
`real-system-verification`.

### Probar el 403 no alcanza

Hay que probar también el **200 con el rol correcto**. Un manejador que niega todo pasa igual la
prueba del 403, y no es una política: es una pared. Matriz mínima, contra el servicio real:

| Caso | Esperado |
|---|---|
| Rol que el usuario NO tiene | 403 en **todos** los verbos, no sólo en el GET |
| Rol que el usuario SÍ tiene | 200 |
| Sin token / token fabricado | 401 |

### Protección apagada a propósito: que GRITE

Si la política queda desactivada por configuración —porque el rol todavía no existe, por ejemplo—
el arranque **DEBE** emitir una advertencia que lo diga. Un `= 0` silencioso es una vulnerabilidad
que nadie va a notar hasta que alguien la use. Y documenta en la configuración misma por qué está
en ese valor y qué hace falta para encenderla.

## Pruebas

- Los dobles reproducen SUPOSICIONES, no la realidad. Sirven para reglas de negocio; para contratos
  con servicios externos, no.
- Antes de cerrar: ejecutar contra el servicio real con un token real. Tres defectos críticos
  —claim inexistente, validador que rechazaba todo, token nunca enviado— pasaron 64 pruebas verdes
  porque todas usaban un cliente falso.
- Al corregir un defecto que es un PATRÓN (no un caso), audita TODAS sus apariciones:
  `rg "<llamada sospechosa>"` sobre el módulo y clasifica cada uso. Un mismo defecto reapareció
  tres veces por no hacerlo.

## Antes de dar por cerrado

- [ ] La API ARRANCA (el contenedor valida el registro; las pruebas no lo hacen)
- [ ] Responde 401 sin token, 200 con token válido
- [ ] Si hay política de rol: 403 con un rol que no la cumple **y** 200 con uno que sí
- [ ] Probado el ciclo completo contra los servicios reales, no sólo las pruebas
- [ ] Datos de prueba creados y ELIMINADOS, verificado con un SELECT posterior
- [ ] Ningún secreto nuevo en `appsettings.json` versionado
