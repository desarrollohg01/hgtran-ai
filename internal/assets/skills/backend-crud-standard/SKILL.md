---
name: backend-crud-standard
description: "Trigger: CRUD .NET, controller, EF Core, repositorio, auditoria, paginacion, API externa. Estandares de backend de HG."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.0"
---

## Cuándo usarla

Al escribir o revisar un CRUD en .NET con capas DOMAIN / DATA / BUSINESS / API: controladores,
repositorios EF Core, servicios de negocio o integraciones con servicios externos.

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
| Guarda además el nombre como fotografía, en columna aparte | Evita una llamada por renglón al servicio de identidad sólo para poder listar |
| El autor sale del TOKEN, nunca del cuerpo de la petición | Si viene del cliente, cualquiera firma una baja con el nombre de otro |
| RECHAZA escribir auditoría incompleta | Sin identidad completa se responde 401. Un registro a medias parece confiable y no lo es |

El identificador vive en el claim `sub`; el nombre en `unique_name`. Los dos llegan gratis en el
mismo token al momento de escribir.

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
| Devuelve el TOTAL, no sólo la página | Es lo que permite al cliente saber si quedan más |
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

Ocultar la opción en la interfaz es comodidad visual, **no** la garantía. La prueba de que está
bien: construir la petición a mano con el token de un usuario sin permiso y recibir **403**.

Verifica también que el rol del recurso no sea el MISMO que se asigna a los usuarios finales: si
coinciden, todos heredan la administración. Pasó, y se demostró con un `PUT` de un proveedor sobre
un usuario de otro que devolvió 200.

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

- [ ] La API arranca y responde 401 sin token, 200 con token válido
- [ ] Probado el ciclo completo contra los servicios reales, no sólo las pruebas
- [ ] Datos de prueba creados y ELIMINADOS, verificado con un SELECT posterior
- [ ] Ningún secreto nuevo en `appsettings.json` versionado
