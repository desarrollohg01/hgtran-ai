---
name: db-change-standard
description: "Trigger: script SQL, migracion, columna de auditoria, SQL Server, esquema, datos de prueba. Estandares de base de datos de HG."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.0"
---

## Cuándo usarla

Al escribir o revisar cualquier cambio de esquema o script SQL sobre las bases de HG.

## Antes de escribir una sola línea de SQL

REQUIERE verificar el esquema real con las herramientas MCP: describir la tabla, confirmar nombres
y tipos de columna, contar filas.

**Nunca asumir que las columnas coinciden con lo que dice el código.** Una consulta escrita contra
13 columnas inexistentes pasó revisión y falló al ejecutarse: el catálogo real tenía otros nombres.

## Scripts

| Regla | Detalle |
|---|---|
| REQUIERE que sean IDEMPOTENTES | Volver a correrlos no debe fallar ni duplicar. `IF NOT EXISTS` en todo |
| Numerados y en `database/Scripts/` | `003_Corrige_Auditoria_ProveedorUsuario.sql` |
| `SET QUOTED_IDENTIFIER ON` al inicio | Sin esto, `sqlcmd` falla con Msg 1934 en índices filtrados y columnas calculadas |
| Preservan lo que ya existe | Una migración que cambia el tipo de una columna debe conservar el dato viejo, y decir qué pasa con las filas que no se pueden convertir |

## Auditoría

Columnas mínimas: `Activo`, `FechaCreacion`, `CreadoPor`, `FechaModificacion`, `ModificadoPor`.

`CreadoPor` / `ModificadoPor` guardan el **identificador** del operador, no su nombre: un nombre
deja de apuntar a nadie si la persona se renombra. El tipo y la forma dependen de **dónde vive la
identidad**, y eso se resuelve ANTES de escribir el `CREATE TABLE`.

- **Identidad local** — existe una tabla de usuarios en la misma base: `uniqueidentifier` con FK
  real a esa tabla, y el nombre en texto en la vista o el DTO, no como columna de la tabla base. Es
  la forma que fija `api-crud-standard`, cuyo alcance declarado son `portaltools-api`,
  `etruckssecurity-api` y `portaltools-webapp` (`spec.md:359`).
- **Identidad externa** — la gobierna otro sistema (HG.AccessExternal / Entra) y esta base no es
  dueña de esa tabla: se guarda el identificador que emite el token —el claim `sub`, un
  `uniqueidentifier`— **sin FK**, con índice y con un default explícito para las filas anteriores
  que no lo tienen. Es lo que hace `003_Corrige_Auditoria_ProveedorUsuario.sql`. Si el sistema no
  maneja un GUID de usuario, la columna es el UPN en texto; `zametlordenescompra-winservice` lo deja
  escrito en su `IAuditable`: "NO un Guid FK a la tabla de usuarios, porque Compras/OC no es dueño
  de esa tabla".
- **Ojo con lo que parece local y no lo es.** En `portaltools-api` la navegación de auditoría apunta
  a `VwUsuario`, mapeada con `ToView(...)`: el `[ForeignKey]` de EF declara una navegación, no una
  restricción en la base, y SQL Server no admite una FK contra una vista. Pedir "FK a la tabla de
  usuario" sin distinguir el caso deja al autor con una regla que no puede cumplir.
- Un **actor no humano** (ETL, job) no tiene sesión ni token. Con columna de texto va una constante
  explícita — `cat.Proveedor` usa `ETL_ProveedorSync`. Con identidad local y columna
  `uniqueidentifier`, el proceso REQUIERE su propia fila en la tabla de usuarios. Las columnas de
  texto que ya existen se quedan y se citan como excepción.
- El **nombre en texto** (`CreadoPorNombre`) sigue el mismo eje. Con identidad local NO va en la
  tabla base: la vista lo resuelve con un `JOIN` y duplicarlo en cada tabla se desactualiza. Con
  identidad externa que no expone consulta por lotes SÍ va, y es justo lo que evita el N+1: sin el
  nombre guardado, pintar el listado obliga a una llamada por fila a HG.AccessExternal. El token
  trae `sub` y `unique_name` juntos, así que guardar ambos no cuesta nada al escribir.
  `003_Corrige_Auditoria_ProveedorUsuario.sql` agrega `CreadoPorNombre` y `ModificadoPorNombre`
  sobre una tabla creada dos scripts antes, por esa razón exacta. El GUID responde QUIÉN fue
  —referencia inmutable—; el nombre, CÓMO SE LLAMABA al momento de la acción.
- Índice por `CreadoPor`: responde "qué hizo esta persona", la consulta típica de auditoría.

## Fechas

Se guardan en **UTC**. `datetime` no lleva zona, así que la aplicación tiene que marcarlas al leer
(ver `backend-crud-standard`). Si no, el navegador las corre el tamaño del huso y nadie entiende
por qué un alta de ayer aparece hoy.

## Bajas

La baja es **LÓGICA**: `Activo = 0`, el registro se conserva para auditoría. RECHAZA `DELETE` sobre
tablas de negocio.

Si la fila representa un acceso, la baja debe además revocarlo en el sistema que lo otorga.

## Tablas temporales y collation

Usar `COLLATE DATABASE_DEFAULT` en las columnas de texto de tablas temporales. `tempdb` hereda la
collation del SERVIDOR, no la de la base: sin esto aparece el error 468 al unir contra tablas
reales, y sólo en el ambiente donde ambas difieren.

## Cargas masivas

Bulk a una tabla de paso + un solo `MERGE`, no un `MERGE` por fila. Con `WHEN NOT MATCHED BY
SOURCE`, cuidado: si el origen viene incompleto, borra el catálogo entero.

Una sola fuente de verdad para el mapeo de columnas (nombre, tipo SQL, tipo CLR, lectura) que
genere el DDL, el mapeo del bulk y el MERGE. Tres listas separadas se desincronizan.

## Datos de prueba

- Prefijo reconocible (`ZZ...`) para poder encontrarlos y borrarlos.
- REQUIERE limpiarlos al terminar, y VERIFICARLO con un `SELECT` posterior.
- Ojo con las claves foráneas al borrar: puede haber sesiones u otras tablas apuntando.
- Si una prueba modifica un dato real, restaurarlo. Si no se puede, decirlo explícitamente.

## Permisos y menús

Los permisos de un menú se conceden al rol que corresponde a QUIÉN debe usarlo. Verificar que ese
rol no sea el mismo que se asigna a los usuarios finales del sistema: si coinciden, todos heredan
la administración. Comprobado con una petición a mano, no leyendo la tabla.
