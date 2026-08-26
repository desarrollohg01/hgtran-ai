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

- `CreadoPor` / `ModificadoPor` son **`uniqueidentifier`**, no texto: guardan el identificador del
  operador. Un nombre deja de apuntar a nadie si la persona se renombra.
- Junto a ellas, `CreadoPorNombre` / `ModificadoPorNombre` guardan el login como fotografía del
  momento. Es lo único que se muestra; el identificador es la referencia.
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
