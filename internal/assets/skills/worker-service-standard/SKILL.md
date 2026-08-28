---
name: worker-service-standard
description: "Trigger: worker service, windows service, Quartz, job programado, ETL, proceso batch, migrar de .NET Framework. Estandares de workers de HG."
license: Apache-2.0
metadata:
  author: hgtransportaciones
  version: "1.0"
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
Hg.<Contexto>.Worker     host, Jobs de Quartz, Program.cs, Configs/
```

El host se llama **`Worker`**, no `Service`, **en un worker nuevo**. Esa nomenclatura es de
workers y no se traslada. Para una API, `api-crud-standard` fija **API / CORE / SERVICE / DATA**
—donde `SERVICE` **es** la capa de negocio— y su propio alcance declara que aplica exclusivamente
a `portaltools-api`, `etruckssecurity-api` y `portaltools-webapp` (`spec.md:359`). Llevar `Domain`
y `Business` a esos repositorios contradice la convención vigente ahí.

Para una API nueva el vocabulario todavía no está definido: la spec gobierna esos repositorios y
fuera de ellos no hay respuesta acordada. Es decisión pendiente del equipo, no un hueco a llenar
por cuenta propia.

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

Quartz.NET, y el host como servicio de Windows con `AddWindowsService()`. No schedulers propios,
no `Timer` a mano.

Cada Job MUST documentar qué lo dispara, con qué frecuencia, y qué pasa si una ejecución se
solapa con la anterior. Un Job sin política de solapamiento declarada es un Job que algún día
va a correr dos veces sobre los mismos datos.

## Librería de correo

`hgt_development/smtpemailclient-library`. La elección no es libre: submódulo git, salvo que
exista una razón que lo impida y quede escrita.

La consumen de más de una forma: submódulo git, copia vendorizada dentro del repositorio, un DLL
por `HintPath`, y un `PackageReference` que no resuelve. Buscar solo submódulos no las encuentra
todas. Un proyecto nuevo MUST montarla en `libs/smtpemailclient-library`. Los existentes NO se
renombran de paso: mover la ruta rompe lo que ya compila, y merece su propia tarea.

Vendorizar es legítimo cuando el submódulo no sirve, y hoy hay dos razones reales y verificadas:
no existe un NuGet consumible porque el feed privado devuelve 401, y el submódulo está en
`net8.0` mientras esta skill exige `net10.0`. Cuando se vendoriza, la razón MUST quedar escrita
en el `.csproj` de la copia, con la historia que lo decidió. Una copia sin esa justificación sí
es un defecto.

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

MUST documentarse todo Job de Quartz y toda
consulta cruda —Dapper o `SqlConnection`— explicando por qué no se usó el camino por defecto.

## Reintentos

Polly se configura en `AddPollyConfig`. Lo que MUST estar escrito, y hoy no lo está en ningún
worker revisado: **qué se reintenta, cuántas veces, y qué operaciones son idempotentes**.

Un ETL que reintenta un `upsert` no idempotente duplica datos, y lo hace en silencio. Antes de
envolver una operación en una política de reintento, verificá que correrla dos veces deje el
mismo resultado que correrla una.

## Migrar desde .NET Framework

El orden importa porque cada paso deja el servicio en un estado verificable. Mezclarlos hace
imposible saber si rompió el framework o el rediseño.

1. Inventariar los `.csproj` y leer el framework **del `.csproj`, nunca del `README`**.
2. Separar en las cuatro capas, todavía sobre el framework viejo.
3. Sustituir el host por el genérico con `AddWindowsService()`.
4. Reemplazar el scheduler propio por Quartz.NET.
5. Mover el envío de correo al submódulo compartido.
6. **Recién ahora** subir a `net10.0`.
7. Agregar health check y pipeline.

El framework va en el paso 6, no en el 1. Migrar la versión primero convierte cualquier falla
posterior en una discusión sobre quién la causó.

## Lo mínimo antes de dar por terminado

- Proyecto de pruebas. Un worker corre solo, de noche, sin nadie mirando: es justo el que más
  necesita una red.
- Health check.
- Pipeline en Bitbucket.
- El clon limpio compila, con los submódulos inicializados.
